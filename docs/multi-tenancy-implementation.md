# 多租户实现与交付说明

本实现依据 `multi-tenancy-prd.md`、`multi-tenancy-design.md` 和工作区 `prototypes/`，分支为 `feat/organization-management`。

## 已确认的产品与技术决策

- 组织角色采用原型的 **Owner / Admin / Member**。平台角色独立，组织 Admin 不因此获得平台管理权限。
- 界面仅切换“个人 / 具体组织”；个人资源直接使用用户 ID 和零组织 ID，不创建个人组织或成员关系。个人账户不再请求组织 context 或模拟 org.* 权限；/api/account/context 已移除，/api/account/summary 仅返回个人资金、订阅与用量。只有选中团队时才加载 /api/org/context 和成员权限。无团队时隐藏切换器，个人设置中保留创建组织入口。管理员栏由平台账号角色决定，始终独立于当前组织；全平台看板与日志使用 `/platform/dashboard/:section`、`/platform/usage-logs/:section`，与个人/组织查询及缓存分开。
- 成员上限跟随订阅的额度重置周期；没有订阅时按 UTC 自然月。一个周期开始后固定组织周期，续购不会提前清零成员消费。
- 团队有非零余额、有效订阅、待支付订单或未结算任务时禁止删除。先停用，再由平台完成资金和任务处置。个人账户不适用组织的删除或转让操作。
- **完整功能一次启用**，采用停机备份、迁移、验证和恢复工具交付。文档中的五阶段保留为功能分解，不维护五套运行开关，也不允许旧、新版本同时写库。
- **组织钱包采用数据库事务**：持久化 `OrganizationCharge` 保存请求预留、原始资金来源和结算状态；组织、订阅、成员上限与 Key 限额在同一事务中处理。此方案替代设计稿中的组织钱包 Redis Lua 预留方案。
- Token 缓存继续携带组织 ID、状态、档位和设置。热缓存解析租户无需额外查组织表；计费事务需要查库并锁定组织行。Redis 中的 Key 余额不是组织请求的权威限额，实际限额在计费事务中校验。元数据变更仍通过缓存 fence 防止旧权限重新写入缓存。
- 支付联调在独立本地测试库中完成，验收未调用真实支付或真实模型上游。

## 功能落点

| 范围 | 实现 |
| --- | --- |
| 组织与迁移 | 仅真实团队创建组织；个人钱包直接使用 User.Quota；个人资源保留 org_id 为零（兼容旧库 NULL），不回填个人组织；用户与团队资源按统一作用域查询 |
| 组织上下文 | 顶栏切换和搜索，组织设置中创建组织；按登录用户保存选中组织；切换清理缓存和表单；延迟响应不能污染新组织页面；提交中的请求固定原组织 |
| 权限 | Casbin domain、平台策略迁移、组织能力矩阵；普通接口拒绝无效或非成员组织；平台跨组织入口独立 |
| 成员 | 用户名站内邀请、通知中心接受/拒绝、过期、重发、撤销、账号匹配、席位校验、角色调整、停用/移除；停用/移除后禁用其组织 Key，历史用量与费用保留 |
| Key | 组织与创建者双重归属；所有角色仅管理自己的 Key；一次性 Secret、掩码列表、批量操作；移除创建者列和按创建者筛选 |
| 用量 | 组织概览、模型/成员用量与费用聚合、日志筛选、任务与制品隔离；普通成员只能读自己的明细；支持独立日志库与 ClickHouse |
| 采购 | Audience、成员席位、组织档位、订阅额度、钱包溢出；余额购买及现有五类在线支付接入；订单持久化组织和套餐快照；幂等回调、档位到期恢复、订单历史 |
| 计费 | 订阅优先、钱包兜底、成员周期上限、Key 硬限额；数据库条件检查和行锁；预扣、增量预留、结算、退款与异步任务调整保留原组织和原资金来源 |
| 预算告警 | 按周期和收件对象去重；Owner/Admin、告警邮箱、Webhook；持久化队列、租约与重试 |
| 设置 | 名称、Logo、通知、默认成员上限、模型收窄；不允许组织覆盖价格、倍率、渠道或支付密钥 |
| 生命周期 | 停用/恢复、目标成员确认所有权转让、删除影响预览、软删除与后台清理 |
| 审计 | 成功管理写入、失败/拒绝请求、采购与支付事件；记录组织、操作者、对象、结果；不保存请求密钥或正文 |
| 国际化 | 英语、简体中文、繁体中文、法语、日语、俄语、越南语 |

## 计费与故障恢复边界

组织行锁串行化同一组织的资金和上限检查；SQLite 由写事务串行化，MySQL/PostgreSQL 使用 `lockForUpdate`。请求凭据使用唯一请求 ID，重复预扣只预留目标总量，重复结算和退款不重复记账。Key 预扣和组织预扣在同一事务内，任何上限或余额检查失败会一并回滚。结算超过预估沿用现有最终费用补扣语义，允许欠费但不允许整数溢出。跨订阅重置周期退款不会向新周期凭空增加额度。

组织计费不依赖 Redis 余额。Redis 故障时从数据库恢复 Token 身份读取；个人请求复用用户钱包及个人订阅路径，不再维护组织钱包投影。组织状态、分组和设置不复制到 Token；组织鉴权通过数据库读取当前成员与组织状态，单次请求复用该结果，不建立跨请求组织缓存。组织配置修改无需批量更新 Token。成员撤销仍禁用其 Key，并要求 Token 缓存失效成功。

持久化凭据可用于重试和核对，但无法凭空判断进程崩溃时上游是否已执行。异常中断后，先核对 `reserved` 凭据、上游结果与任务状态，再按原请求 ID 完成结算或退款；不要仅按创建时间自动退款。离线恢复到上线前快照会丢失快照后的写入，因此开放流量后如需回滚，应先停流量并导出、对账和处理新增订单、支付回调及消费。

## 停机升级

工具说明与完整配置见 [运维工具](../tools/multi-tenancy/README.md)。以下命令从仓库根目录执行。

1. 保存旧版二进制/镜像、环境配置、上传文件、签名和加密密钥。停止所有 API 节点、定时任务、支付回调接收与队列写入。待进行中的请求和异步任务处理完毕或登记待核对项。
2. 将主库、独立日志库信息填入私有配置，创建并校验快照：

   ```sh
   python3 tools/multi-tenancy/snapshot.py backup --offline --config /secure/source.json --snapshot /secure/pre-tenancy
   python3 tools/multi-tenancy/snapshot.py verify --snapshot /secure/pre-tenancy
   ```

3. 正式发布版升级使用应用正常的数据库初始化。个人资源不需要组织回填。仅曾运行个人组织实现的私有开发库需要仓库外的一次性数据转换；该转换不随应用启动执行，也不作为公共迁移工具分发。

4. 使用新的专属 Redis 实例或空逻辑库，避免旧缓存中的用户余额、Key 身份和策略污染；不要清空共享 Redis。保持签名密钥和文件存储配置不变。
5. 启动新版本，先限制外部流量，验证个人余额、Key 归属、日志、一个团队的邀请与消费。全部节点切换同一版本后开放流量并恢复回调。

不再为了个人组织重建 ClickHouse 日志排序键。已有日志与资源保留原始用户归属。

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


## 用户名站内邀请（2026-09-04）

管理员输入已注册且启用账号的用户名发出站内邀请（去除首尾空格后精确匹配）。邀请保存 `invitee_id` 和用户名快照；接受、拒绝和重发均绑定原账号 ID，账号改名或原用户名被其他账号使用不会转移邀请。不要求绑定邮箱，也不发送邮件或生成邀请链接。

右上角通知中心显示待处理邀请数量、组织、邀请人、角色与到期时间。登录时加载，前台每 30 秒刷新，重新聚焦窗口时刷新；离线用户下次登录仍可处理。通知按账号查询，不受当前组织选择影响。用户主动接受后才创建成员关系并切换组织；拒绝不切换组织。重复接受/拒绝幂等；过期、撤销和停用组织的邀请不能加入。默认有效期七天，管理员可撤销或重发；重发延长有效期，不需要复制链接。

接口：`GET /api/organizations/invites` 仅列出当前账号的有效邀请；`POST /api/organizations/invites/:invite_id/accept` 和 `/decline` 仅允许该邀请的目标账号操作。创建与重发仅返回站内邀请信息。管理员列表支持 `declined` 状态，拒绝写入组织审计。

邀请记录仅包含账号、组织、角色、状态、有效期与审计所需信息；接收人/状态/有效期使用复合索引。开发期间的邀请链接页面、令牌字段和生成代码、邮箱兼容分支及相关翻译均已清理，不作为发布功能保留。开发数据库在备份后一次性移除废弃字段及未绑定接收账号的无效记录，已有站内邀请保留。

验证使用独立测试库：SQLite **3.50.4**、MySQL **5.7.44**、PostgreSQL **9.6.24**。覆盖无邮箱加入、账号隔离、错误/停用账号拒绝、精确用户名、改名后身份不转移、重复邀请、拒绝、撤销、过期、重发、组织停用及接受幂等。三种数据库验证一次性开发库清理后现有站内邀请和索引保留；并验证新建及从实际发布版 **v1.0.0-rc.30 / 27ff6a87** 创建的代表性数据库升级、连续两次完整初始化，核对原余额、资源归属、日志及唯一性。本次邀请表不属于独立日志库。

验证命令（外部 DSN 必须指向可销毁的独立库）：

```sh
go test ./model -run TestOrganizationInvit -count=1
TENANCY_TEST_MYSQL_DSN="$INVITE_MYSQL_DSN" go test ./model -run TestOrganizationInvit -count=1
TENANCY_TEST_POSTGRES_DSN="$INVITE_POSTGRES_DSN" go test ./model -run TestOrganizationInvit -count=1
go build -o /tmp/new-api-cleanup-startup ./tools/multi-tenancy-verify
# 每种数据库分别设置 SQL_DSN 或 SQLITE_PATH。
# fresh 直接运行；upgrade 先运行由发布版本构建的 released-fixture。
TENANCY_VERIFY_STARTUP=1 /tmp/new-api-cleanup-startup
```

前端 19 项组织相关测试、类型检查、涉及文件 lint 和生产构建通过。本地独立 SQLite 应用完成真实 HTTP 联调：初始化管理员、注册无邮箱账号、按用户名邀请、通知列表账号隔离、错误账号拒绝、拒绝后重新邀请、接受并查询成员关系、重复接受幂等；创建响应不含 token 或链接。

## 个人密钥与组织付费（2026-09-04）

所有角色只能查看和管理自己在当前组织内创建的 API Key，组织钱包仍统一结算。Owner/Admin 保留成员用量、模型、费用和预算管理。创建提示明确“为自己创建，使用组织额度”。

停用/移除成员会在同一事务中禁用其组织 Key，并失效缓存；其他组织和个人 Key 不受影响。恢复成员后需本人显式启用旧 Key 或创建新 Key。已预扣请求继续结算/退款，历史记录保留。Token 缓存使用 `token:org-v1:` 隔离正式版不含组织字段的缓存；不在启动时修复未发布中间版本的成员 Key。

升级前备份数据库，停止旧实例并统一切换版本。若回滚到正式发布版，恢复升级前数据库备份及匹配的旧镜像，并使用独立空 Redis 库；不要与新版本混跑。

本次验证：SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 9.6.24、Redis 7.2.16。三种数据库均通过全部 `TestOrganization` 行为测试（该次历史验证包含成员撤销、恢复、历史结算和现已移除的启动清理），并分别完成全新库和实际发布版 `v1.0.0-rc.30 / 27ff6a87` 夹具升级的两次启动校验。余额、资源归属、历史日志和唯一索引均保留。命令如下；所有 DSN 必须指向可删除的独立测试库：

```sh
go test ./model ./controller ./service/authz -run 'TestOrganization|TestGetAllTokens|TestGetToken|TestSearchTokens|TestUpdateToken' -count=1
TENANCY_TEST_MYSQL_DSN="$MYSQL_TEST_DSN" go test ./model -run TestOrganization -count=1
TENANCY_TEST_POSTGRES_DSN="$POSTGRES_TEST_DSN" go test ./model -run TestOrganization -count=1
TENANCY_TEST_REDIS_ADDR=127.0.0.1:16379 go test ./model -run TestOrganizationCachedToken -count=1
go build -o /tmp/new-api-private-keys-startup ./tools/multi-tenancy-verify
# 每种数据库各建 fresh 和 upgrade 独立库；upgrade 库先运行已发布版本的 release-fixture。
TENANCY_VERIFY_STARTUP=1 SQL_DSN="$TEST_DSN" SQLITE_PATH="$TEST_SQLITE_PATH" /tmp/new-api-private-keys-startup
cd web
bun run typecheck
NODE_OPTIONS=--no-experimental-webstorage bun run test src/features/keys src/features/organizations src/features/usage-logs/__tests__/platform-scope.test.ts
bun run build
```

真实 Redis 测试验证热缓存撤销；前端 51 项测试、类型检查、涉及文件 lint 和生产构建通过。本地独立应用的真实 HTTP 联调覆盖 Owner/Admin/Member 各自密钥列表、伪造成员筛选、详情/修改/删除/混合批量越权拒绝、平台密钥列表关闭、用量查询保留、停用后 relay 认证返回 401、恢复成员后需创建者显式启用。

清理邀请链接的复验：三种数据库在独立库中建立清理前开发表，连续两次清理并初始化；现有站内邀请的 ID、有效期、组织/接收人索引保留，废弃字段消失，清理后创建、接受、拒绝均通过。一次性开发表夹具不进入发布代码。前端仍为 19 项组织测试通过，七种语言合计清除 12 个废弃文案键，生成路由中已无邀请链接页面。

## 未发布中间版本兼容清理（2026-09-09）

本分支仅在个人开发环境运行过，发布升级只以正式版为基线：

- 用户缓存结构与主分支一致，`userCacheSchemaVersion` 恢复为 `2`。
- Token 缓存保留首次组织字段隔离命名空间 `token:org-v1:`，移除中间版本的编号递增。
- 移除启动时为旧组织资产策略禁用非活跃成员 Key 的修复及专用测试。正常成员停用/移除事务仍负责撤销 Key。
- 组织任务和 Midjourney 结算必须有原始请求 ID 和账单记录；缺失时返回错误，不生成 `migrated-task` / `migrated-mj` ID，也不补造账单。新增测试断言失败时钱包、Key、任务额度及账单数量均不变。
- 保留正式版升级需要的组织字段、ClickHouse 字段、SQLite 套餐字段和 Casbin domain 迁移。

验证使用 `gcys@10.0.29.49` 上独立临时容器和数据库，没有更新运行中的开发应用。引擎版本为 SQLite **3.50.4**、MySQL **8.0.46**、PostgreSQL **15.19**。三引擎的组织行为测试、全新数据库两次启动、正式发布版 **v1.0.0-rc.35** 夹具升级后两次启动均通过。MySQL/PostgreSQL 同时覆盖独立日志库；SQLite 使用主日志共享库。升级校验覆盖个人余额、Key 余额、日志额度、资源归属及组织唯一索引。

实际执行命令（工作目录 `/tmp/new-api-compat-check`，DSN 均指向临时库）：

```sh
# 本地编译当前代码和正式版夹具
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/new-api-compat-check/model.test ./model
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/new-api-compat-check/startup ./tools/multi-tenancy-verify
# released-fixture 使用 git archive v1.0.0-rc.35 导出的代码及 tools/multi-tenancy/released-fixture.go.txt 构建

# 远端行为矩阵
./model.test -test.run 'TestOrganization|TestUserAuth|TestPendingUserAuth|TestCommittedUserAuth|TestTokenCache|TestTryReserveQuota' -test.count=1
TENANCY_TEST_MYSQL_DSN="$MYSQL_TEST_DSN" ./model.test -test.run TestOrganization -test.count=1
TENANCY_TEST_POSTGRES_DSN="$POSTGRES_TEST_DSN" ./model.test -test.run TestOrganization -test.count=1

# 每种数据库分别配置 SQL_DSN、LOG_SQL_DSN 或 SQLITE_PATH
# fresh 直接执行 startup；upgrade 先连续执行两次 released-fixture
./released-fixture
./released-fixture
TENANCY_VERIFY_STARTUP=1 ./startup # 内部连续执行两次初始化及断言

# 本地相关包回归
go test ./model ./service/authz -run 'TestOrganization|TestUserAuth|TestPendingUserAuth|TestCommittedUserAuth|TestTokenCache|TestTryReserveQuota' -count=1
go test ./controller ./service -run 'TestOrganization|TestMidjourneyImage' -count=1
```

另以同一正式版夹具新建基线库，比较升级后所有原有索引定义及 PostgreSQL 约束，均保留：SQLite 主库 153 条，MySQL 主库/日志库 213/29 条，PostgreSQL 主库/日志库 217/25 条。临时验证脚本及日志保存在远端 `/tmp/new-api-compat-check`，日志另复制到本地 `/tmp/new-api-compat-check/results`。测试容器验证后移除。

## PR 审查修复验证（2026-09-10）

本次修复四个问题：平台封禁不能由组织 Owner 解除；平台组织日志按 Admin/Root 分别脱敏；个人日志继续隐藏渠道名称并返回分页展示 ID；Personal 模式的模型分析正常加载，已选择但尚未取得 context 的团队仍等待验证。

平台停用接口继续接受 `status=2`，但保存独立的 `OrganizationSuspended=4`；Owner 自行停用仍保存 `OrganizationDisabled=2`。Owner 生命周期操作只允许 Active/Disabled，平台封禁组织不进入个人组织选择列表，平台恢复接口可以解除封禁。没有新增字段、索引或 schema 迁移。此分支尚未发布；已有开发库若保存了旧的 `status=2` 平台封禁，需通过平台停用接口重新执行封禁，不能仅靠旧状态区分平台封禁与 Owner 自行停用。

先增加回归测试，确认修复前四个场景失败，再修复并验证。测试在本机可销毁数据库运行，没有更新 `10.0.29.49` 的部署。

数据库实际版本：SQLite **3.50.4**（Go pure-Go driver）、MySQL **5.7.44**、PostgreSQL **9.6.24**。MySQL 临时库使用 `utf8mb4_unicode_ci`，连接使用 `charset=utf8mb4`；初次使用容器默认 Latin-1 时中文日志测试失败，修正测试库字符集后全量组织行为测试通过。

仓库根目录执行命令（端口对应本次临时容器，数据库均为可删除测试库）：

```sh
go test ./model ./controller ./middleware ./service ./service/authz ./router -count=1
TENANCY_TEST_MYSQL_DSN='root@tcp(127.0.0.1:59504)/review_test?charset=utf8mb4&parseTime=true' go test ./model -run TestOrganization -count=1
TENANCY_TEST_POSTGRES_DSN='host=127.0.0.1 port=59503 user=postgres dbname=review_test sslmode=disable' go test ./model -run TestOrganization -count=1
ORGANIZATION_API_TEST_MYSQL_DSN='root@tcp(127.0.0.1:59504)/review_api?charset=utf8mb4&parseTime=true' go test ./controller -run 'TestOrganizationPublicAPIBoundary|TestOrganizationLogVisibility' -count=1
ORGANIZATION_API_TEST_POSTGRES_DSN='host=127.0.0.1 port=59503 user=postgres dbname=review_api sslmode=disable' go test ./controller -run 'TestOrganizationPublicAPIBoundary|TestOrganizationLogVisibility' -count=1
```

以上均通过。新增平台封禁测试覆盖 Owner 恢复/再次停用/删除均被拒绝、成员访问被拒绝、Key 状态更新、平台恢复，以及 Owner 自行停用后仍可自行恢复。日志响应测试覆盖 Personal、团队、平台 Admin 和 Root。此轮未重跑发布版升级和独立日志库矩阵，历史验证记录见前文。

本机没有 Bun，前端使用已安装的 `web/node_modules/.bin` 执行同一套工具：`tsgo -b`、两份改动文件的 `oxlint -c .oxlintrc.json` 和 `oxfmt` 均通过；`NODE_OPTIONS=--no-experimental-webstorage ./node_modules/.bin/vitest run src/features/dashboard/hooks/__tests__/model-analytics.test.tsx src/features/organizations` 共 **30 项通过**。Node 参数用于避免本机原生 Web Storage 与 jsdom 冲突。

## Midjourney 公开图片链接（2026-09-10）

个人和组织统一保留 main 的公开图片链接语义：`/mj/image/:id` 无需登录、组织头或签名，按原有 `MjId` 查询并代理图片；任务不存在仍返回 400。生成链接恢复原格式，未完成任务保留 `?rand=`。移除本分支新增的 MJ 图片签名服务及其测试，不处理原有上游 ID 潜在重复问题。任务列表、详情、操作和计费继续使用组织作用域，原有图片代理 SSRF 校验保留。

替代回归测试直接验证两种归属的无凭证图片访问和链接格式。SQLite **3.50.4**、MySQL **5.7.44**、PostgreSQL **9.6.24** 均通过；使用本机独立临时数据库，没有更新远端部署。没有 schema 变更。

```sh
go test ./relay ./controller ./service ./router -count=1
MJ_IMAGE_TEST_MYSQL_DSN='root@tcp(127.0.0.1:62153)/mj_test?charset=utf8mb4&parseTime=true' go test ./relay -run TestMidjourneyImagesKeepPublicLinks -count=1
MJ_IMAGE_TEST_POSTGRES_DSN='host=127.0.0.1 port=62154 user=postgres dbname=mj_test sslmode=disable' go test ./relay -run TestMidjourneyImagesKeepPublicLinks -count=1
```

## 个人与组织共享资源重构（2026-09-10）

将共享查询的 `OrganizationResourceScope` / `OrganizationTokenScope` 改为 `ResourceScope` / `TokenScope`，共享 API 使用 `Scoped*` 命名。`resource_scope.go` 定义个人或单一组织的读范围；`scoped_resources.go`、`scoped_tokens.go` 承载共享资源操作；`organization_tokens.go` 仅保留成员 Key 撤销。已购套餐读取改名 `GetPurchasedSubscriptionPlan`，移回通用订阅模块。

Controller 的共享用量入口改名 `GetScopedLogs` / `GetScopedLogStats`，个人作用域直接返回，不经过组织权限计算。日志响应仍明确区分个人 `FormatUserLogs` 与组织 `FormatOrganizationLogs`。删除两个已被作用域入口替代、没有路由调用的旧日志处理器，以及成员 Key 撤销的单次调用转发 helper。HTTP 路由、字段、查询条件、事务和计费行为保持不变，没有 schema 变更。

复用现有个人生命周期、组织隔离、权限边界和日志脱敏回归测试；本机 SQLite **3.50.4**、MySQL **5.7.44**、PostgreSQL **9.6.24** 均通过。MySQL 测试库使用 `utf8mb4_unicode_ci`。以下外部 DSN 均为本次创建并删除的独立临时库，没有更新远端部署：

```sh
go test ./model ./controller ./relay ./middleware ./service ./service/authz ./router -count=1
TENANCY_TEST_MYSQL_DSN='root@tcp(127.0.0.1:62988)/scope_test?charset=utf8mb4&parseTime=true' go test ./model -run 'TestOrganization|TestAccount' -count=1
TENANCY_TEST_POSTGRES_DSN='host=127.0.0.1 port=62987 user=postgres dbname=scope_test sslmode=disable' go test ./model -run 'TestOrganization|TestAccount' -count=1
ORGANIZATION_API_TEST_MYSQL_DSN='root@tcp(127.0.0.1:62988)/scope_api?charset=utf8mb4&parseTime=true' go test ./controller -run 'TestOrganizationPublicAPIBoundary|TestOrganizationLogVisibility' -count=1
ORGANIZATION_API_TEST_POSTGRES_DSN='host=127.0.0.1 port=62987 user=postgres dbname=scope_api sslmode=disable' go test ./controller -run 'TestOrganizationPublicAPIBoundary|TestOrganizationLogVisibility' -count=1
```


## 组织鉴权移除 Token 状态副本（2026-09-10）

Token 保留 `OrgId`，移除 `OrgStatus`、`OrgGroup`、`OrgSettings` 模型字段和 Redis 写入，以及所有组织配置变更后的批量 Token 同步。个人鉴权继续使用用户缓存；组织鉴权从数据库读取有效成员和组织，普通 relay、只读 Token 接口与 Playground 均检查组织状态。分组与模型限制使用当前组织数据，模型限制仍与 Key 限制取交集。查询失败时拒绝请求。

此方案每个组织请求增加成员和组织各一次查询；仅在请求内复用，不引入有失效窗口的组织缓存。正式版 Token 表没有这三个副本列；新建及正式版升级不再创建它们。未发布开发库已有列可暂留，运行时不再读写，不执行启动删列。旧 Redis hash 中的额外字段不再参与鉴权。

验证数据库：SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 9.6.24。三种数据库的 `TestOrganization` 行为测试通过；鉴权、只读接口、Playground 相关上下文、视频路由测试通过。热 Token 缓存下组织停用/恢复、成员撤销、分组与模型限制变化均按当前数据库状态处理。每种数据库均完成新建库和既有正式版夹具升级的两次启动，检查原余额、Token、日志、索引和约束；MySQL/PostgreSQL 包含独立日志库。验证脚本与结果位于 `/tmp/new-api-orgauth-check/`，未修改生产数据库。

```sh
go test ./middleware ./model ./router ./controller -run 'Organization|TokenAuth|SetupContextForToken|Video|Jimeng|Playground|SubscriptionOrder' -count=1 -timeout=120s
# 分别设置指向独立测试库的 TENANCY_TEST_MYSQL_DSN / TENANCY_TEST_POSTGRES_DSN
go test ./model -run 'TestOrganization' -count=1
go build -o /tmp/new-api-orgauth-check/startup ./tools/multi-tenancy-verify
python3 /tmp/new-api-orgauth-check/verify.py
```
