# 多租户实现与交付说明

本实现依据 `multi-tenancy-prd.md`、`multi-tenancy-design.md` 和工作区 `prototypes/`，分支为 `feat/organization-management`。

## 已确认的产品与技术决策

- 组织角色采用原型的 **Owner / Admin / Member**。平台角色独立，组织 Admin 不因此获得平台管理权限。
- 界面仅切换“个人 / 具体组织”；个人组织只作为代码层的数据归属抽象，不展示内部标识或组织角色。无团队时隐藏切换器，个人设置中保留创建组织入口。管理员栏由平台账号角色决定，始终独立于当前组织；全平台看板与日志使用 `/platform/dashboard/:section`、`/platform/usage-logs/:section`，与个人/组织查询及缓存分开。
- 成员上限跟随订阅的额度重置周期；没有订阅时按 UTC 自然月。一个周期开始后固定组织周期，续购不会提前清零成员消费。
- 团队有非零余额、有效订阅、待支付订单或未结算任务时禁止删除。先停用，再由平台完成资金和任务处置。个人组织不可删除或转让。
- **完整功能一次启用**，采用停机备份、迁移、验证和恢复工具交付。文档中的五阶段保留为功能分解，不维护五套运行开关，也不允许旧、新版本同时写库。
- **组织钱包采用数据库事务**：持久化 `OrganizationCharge` 保存请求预留、原始资金来源和结算状态；组织、订阅、成员上限与 Key 限额在同一事务中处理。此方案替代设计稿中的组织钱包 Redis Lua 预留方案。
- Token 缓存继续携带组织 ID、状态、档位和设置。热缓存解析租户无需额外查组织表；计费事务需要查库并锁定组织行。Redis 中的 Key 余额不是组织请求的权威限额，实际限额在计费事务中校验。元数据变更仍通过缓存 fence 防止旧权限重新写入缓存。
- 支付联调在独立本地测试库中完成，验收未调用真实支付或真实模型上游。

## 功能落点

| 范围 | 实现 |
| --- | --- |
| 组织与迁移 | 用户注册、存量用户自动创建唯一个人组织；钱包使用 bigint；回填 Key、订单、充值、订阅、任务、日志、用量归属；重复启动不重复复制余额 |
| 组织上下文 | 顶栏切换和搜索，组织设置中创建组织；按登录用户保存选中组织；切换清理缓存和表单；延迟响应不能污染新组织页面；提交中的请求固定原组织 |
| 权限 | Casbin domain、平台策略迁移、组织能力矩阵；普通接口拒绝无效或非成员组织；平台跨组织入口独立 |
| 成员 | 邮箱邀请、哈希 Token、过期、重发、撤销、身份匹配、席位校验、角色调整、停用/移除；移除后 Key 作为组织资产保留 |
| Key | 组织与创建者双重归属；Owner/Admin 全组织、Member 仅自己；一次性 Secret、掩码列表、批量操作与创建者列 |
| 用量 | 组织概览、模型/成员/Key 聚合、日志筛选、任务与制品隔离；普通成员只能读自己的明细；支持独立日志库与 ClickHouse |
| 采购 | Audience、成员席位、组织档位、订阅额度、钱包溢出；余额购买及现有五类在线支付接入；订单持久化组织和套餐快照；幂等回调、档位到期恢复、订单历史 |
| 计费 | 订阅优先、钱包兜底、成员周期上限、Key 硬限额；数据库条件检查和行锁；预扣、增量预留、结算、退款与异步任务调整保留原组织和原资金来源 |
| 预算告警 | 按周期和收件对象去重；Owner/Admin、告警邮箱、Webhook；持久化队列、租约与重试 |
| 设置 | 名称、Logo、通知、默认成员上限、模型收窄；不允许组织覆盖价格、倍率、渠道或支付密钥 |
| 生命周期 | 停用/恢复、目标成员确认所有权转让、删除影响预览、软删除与后台清理 |
| 审计 | 成功管理写入、失败/拒绝请求、采购与支付事件；记录组织、操作者、对象、结果；不保存请求密钥或正文 |
| 国际化 | 英语、简体中文、繁体中文、法语、日语、俄语、越南语 |

## 计费与故障恢复边界

组织行锁串行化同一组织的资金和上限检查；SQLite 由写事务串行化，MySQL/PostgreSQL 使用 `lockForUpdate`。请求凭据使用唯一请求 ID，重复预扣只预留目标总量，重复结算和退款不重复记账。Key 预扣和组织预扣在同一事务内，任何上限或余额检查失败会一并回滚。结算超过预估沿用现有最终费用补扣语义，允许欠费但不允许整数溢出。跨订阅重置周期退款不会向新周期凭空增加额度。

组织计费不依赖 Redis 余额。Redis 故障时从数据库恢复 Token 身份读取；个人钱包兼容字段仍随组织余额同步，缓存失效失败不会回滚已验证的资金事务。权限和组织状态修改采取失败关闭：无法安全失效缓存时，操作不会提交。

持久化凭据可用于重试和核对，但无法凭空判断进程崩溃时上游是否已执行。异常中断后，先核对 `reserved` 凭据、上游结果与任务状态，再按原请求 ID 完成结算或退款；不要仅按创建时间自动退款。离线恢复到上线前快照会丢失快照后的写入，因此开放流量后如需回滚，应先停流量并导出、对账和处理新增订单、支付回调及消费。

## 停机升级

工具说明与完整配置见 [运维工具](../tools/multi-tenancy/README.md)。以下命令从仓库根目录执行。

1. 保存旧版二进制/镜像、环境配置、上传文件、签名和加密密钥。停止所有 API 节点、定时任务、支付回调接收与队列写入。待进行中的请求和异步任务处理完毕或登记待核对项。
2. 将主库、独立日志库信息填入私有配置，创建并校验快照：

   ```sh
   python3 tools/multi-tenancy/snapshot.py backup --offline --config /secure/source.json --snapshot /secure/pre-tenancy
   python3 tools/multi-tenancy/snapshot.py verify --snapshot /secure/pre-tenancy
   go build -o /secure/multi-tenancy-migrate ./tools/multi-tenancy-migrate
   ```

3. 保持与配置对应的 `SQL_DSN`、`LOG_SQL_DSN`、`SQLITE_PATH` 环境变量，执行两次迁移，再执行只读核对：

   ```sh
   /secure/multi-tenancy-migrate -offline -action migrate -snapshot /secure/pre-tenancy
   /secure/multi-tenancy-migrate -offline -action migrate -snapshot /secure/pre-tenancy
   /secure/multi-tenancy-migrate -offline -action verify
   ```

4. 使用新的专属 Redis 实例或空逻辑库，避免旧缓存中的用户余额、Key 身份和策略污染；不要清空共享 Redis。保持签名密钥和文件存储配置不变。
5. 启动新版本，先限制外部流量，验证个人余额、Key 归属、日志、一个团队的邀请与消费。全部节点切换同一版本后开放流量并恢复回调。

`migrate` 必须收到有效快照目录，校验全部文件 SHA-256 后才开始；快照需由操作者确认属于当前 DSN，工具不推断生产库身份。ClickHouse 排序键变更会复制日志表并在停机窗口重命名，保留 `logs_tenancy_legacy` 供核对；需预留日志表复制空间。这不是在线无锁迁移。

## 回滚

1. 停止新版本全部写入，额外保存当前状态供对账。
2. 创建新的空主库与空日志库（SQLite 使用新路径）。准备对应目标配置，恢复上线前快照：

   ```sh
   python3 tools/multi-tenancy/snapshot.py restore --offline --config /secure/rollback-target.json --snapshot /secure/pre-tenancy
   ```

3. 核对旧用户余额、Key、订阅和日志，将旧版本的 DSN 指向恢复后的库，使用新的空 Redis 逻辑库，并保留原密钥和文件存储。
4. 启动旧版本两次，确认数据库和权限初始化均正常，再开放流量。保留升级后的数据库及备份用于核对；**不要让旧版本直接使用已升级数据库**。

恢复工具在写任何目标前检查所有库为空，不覆盖正在使用的数据。恢复失败时保留目标诊断现场，重新创建一组空目标再重试。

## 验证记录（2026-09-03）

实际数据库：SQLite 3.50.4（Go pure-Go driver）、SQLite 3.43.2（Python 备份客户端）、MySQL 5.7.44 / 9.7.1、PostgreSQL 9.6.24 / 17.11、ClickHouse 25.8.33.6、Redis 7.2.16。最低版本分支覆盖 MySQL 5.7 与 PostgreSQL 9.6；本实现未新增高于项目原最低小版本的专属 SQL 特性。

升级基线为实际发布的 [v1.0.0-rc.30](https://github.com/QuantumNous/new-api/releases/tag/v1.0.0-rc.30)，提交 `27ff6a87`，2026-08-31 发布。使用该版本模型与迁移创建代表性旧库，而非手写旧表结构。夹具覆盖大额个人余额、Key、充值订单、套餐/订阅订单、任务、Midjourney、用量和日志。可复用夹具见 `tools/multi-tenancy/released-fixture.go.txt`，必须复制到该发布版本工作树后构建。

| 验证 | 结果 |
| --- | --- |
| SQLite、MySQL 5.7、PostgreSQL 9.6、SQLite 主库 + ClickHouse 日志库的新建/发布版升级 | 连续两次初始化通过，余额、资源归属、日志、唯一索引核对通过 |
| 独立 SQL 日志库 | MySQL / PostgreSQL 主库与日志库分开配置通过 |
| 原生备份 → 迁移两次 → 空库恢复 → 旧版初始化两次 | 四种数据库路径通过，旧版余额、Key 和日志原值保留 |
| 模型行为矩阵 | SQLite、MySQL 5.7/9.7、PostgreSQL 9.6/17 通过；覆盖隔离、并发上限、付款幂等、生命周期及结算退款 |
| 缓存 | 真实 Redis 热缓存读取在 DB 不可用时仍能解析身份；停用立即拒绝；故障回退通过 |
| 后端、前端及构建 | `go test ./...`、relaykit 独立构建通过；前端全量 411 项 + 新增成员交互 4 项、订阅余额回归 1 项通过（63 个文件），最终受影响 10 项再次通过；类型检查、涉及文件 lint、格式检查、生产构建通过 |
| 本地接口与浏览器 | 邀请身份匹配、成员加入、角色隔离、Key 创建/掩码、越权拒绝、成员触顶拒绝、零元套餐购买、模拟上游实际消费与钱包对账通过 |

复验命令（外部 DSN 必须指向可销毁的独立测试库，测试会重建表）：

```sh
go test ./...
go test ./model -run TestOrganization -count=1
TENANCY_TEST_MYSQL_DSN="$TENANCY_MYSQL_DSN" go test ./model -run TestOrganization -count=1
TENANCY_TEST_POSTGRES_DSN="$TENANCY_POSTGRES_DSN" go test ./model -run TestOrganization -count=1
TENANCY_TEST_REDIS_ADDR=127.0.0.1:16379 go test ./model -run TestOrganizationCachedToken -count=1
TENANCY_VERIFY_STARTUP=1 go run ./tools/multi-tenancy-verify
(cd relaykit && GOWORK=off go build ./...)
(cd web && bun run build:check)
(cd web && NODE_OPTIONS=--no-experimental-webstorage bun run test --maxWorkers=2)
```

Node 25 的实验性 Web Storage 与项目测试环境冲突，测试禁用该特性；两个 worker 避免本机高并发构建争抢资源导致既有测试超时。生产构建不需要该测试参数。

支付接入通过本地订单/回调回归验证，没有使用支付商真实账户或完成真实扣款。部署时仍需使用运营方自己的渠道配置完成支付商沙箱或小额验收。

静态文案检查覆盖涉及页面的 468 个键，七种语言均无缺失。复验请使用工具说明中的独立测试库流程。
