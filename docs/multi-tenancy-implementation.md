# new-api 组织功能交付说明

产品范围见 [产品需求](multi-tenancy-prd.md)，权限、计费和数据模型见 [设计](multi-tenancy-design.md)。本文件只记录部署、恢复边界与可复验命令。

## 功能落点

| 范围 | 主要入口 |
| --- | --- |
| 资源隔离 | `model/resource_scope.go`、`model/scoped_*.go` |
| 组织上下文 | `middleware/org_context.go`、`middleware/auth.go` |
| 角色和平台权限 | `service/authz/organization.go`、`router/organization.go` |
| 成员与生命周期 | `model/organization_members.go`、`model/organization_lifecycle.go` |
| 组织资金与支付 | `model/organization_billing.go`、`model/organization_subscription.go`、`service/organization_funding.go` |
| 异步任务结算 | `model/organization_task_billing.go` |
| 预算通知 | `model/organization_notification.go`、`service/organization_notification.go` |
| 控制台 | `web/src/features/organizations/`、`web/src/stores/organization-store.ts` |

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

3. 正式发布版升级使用应用正常的数据库初始化。个人资源不需要组织回填。本分支未发布的开发库不作为公共升级基线，结构清理在独立备份后进行。

4. 使用新的专属 Redis 实例或空逻辑库，避免旧缓存中的用户余额、Key 身份和策略污染；不要清空共享 Redis。保持签名密钥和文件存储配置不变。
5. 启动新版本，先限制外部流量，验证个人余额、Key 归属、日志、一个团队的邀请与消费。全部节点切换同一版本后开放流量并恢复回调。

已有日志与资源保留原始用户归属，不重建已有 ClickHouse 日志排序键。

## 验证流程

以实际发布版 [v1.0.0-rc.36](https://github.com/QuantumNous/new-api/releases/tag/v1.0.0-rc.36) 创建代表性旧库，夹具构建方式见 [工具说明](../tools/multi-tenancy/README.md)。外部测试 DSN 必须指向可销毁的独立测试库，模型测试会重建表。

```sh
go test ./model ./controller ./middleware ./service ./service/authz ./router -count=1
TENANCY_TEST_MYSQL_DSN="$MYSQL_TEST_DSN" go test ./model -run 'TestOrganization|TestAccount' -count=1
TENANCY_TEST_POSTGRES_DSN="$POSTGRES_TEST_DSN" go test ./model -run 'TestOrganization|TestAccount' -count=1
TENANCY_TEST_REDIS_ADDR=127.0.0.1:16379 go test ./model -run TestOrganizationCachedToken -count=1
TENANCY_VERIFY_STARTUP=1 go run ./tools/multi-tenancy-verify
(cd web && bun run typecheck)
(cd web && NODE_OPTIONS=--no-experimental-webstorage bun run test --maxWorkers=2)
(cd web && bun run build)
```

每种数据库分别验证空库及正式版夹具升级。旧夹具运行两次保存基线，新版启动验证器内部运行两次，核对钱包、Key、日志、归属、唯一索引及原有约束。MySQL/PostgreSQL 同时配置独立日志库。涉及 ClickHouse 或缓存路径的变化另行运行相应集成测试。

Node 的原生 Web Storage 与 jsdom 测试环境可能冲突，测试命令关闭该功能。支付测试使用本地订单和模拟回调，不代表支付商真实扣款验收。

历史验证过程保留在 Git 历史；下方仅记录当前删减的验证结果，避免把旧提交的通过结果当作当前结果。

## 当前删减验证（2026-09-10）

SQLite **3.50.4**、MySQL **5.7.44**、PostgreSQL **9.6.24** 的组织与个人作用域行为测试通过。三种引擎的全新库及最新正式发布版 **v1.0.0-rc.36** 夹具升级均连续启动两次通过；旧钱包、Key、日志数值、资源归属、原有索引和 PostgreSQL 约束保留。MySQL/PostgreSQL 覆盖独立日志库，SQLite 使用共享日志库。

验证使用本机新建的临时容器 `new-api-trim-mysql` 和 `new-api-trim-postgres`，没有修改运行中的应用数据库。以下密码只用于本次可销毁测试实例：

```sh
TENANCY_TEST_MYSQL_DSN='root:trim-test-only@tcp(127.0.0.1:23317)/behavior?charset=utf8mb4&parseTime=true' go test ./model -run 'TestOrganization|TestAccount' -count=1
TENANCY_TEST_POSTGRES_DSN='host=127.0.0.1 port=25437 user=postgres password=trim-test-only dbname=behavior sslmode=disable' go test ./model -run 'TestOrganization|TestAccount' -count=1
go build -o /tmp/new-api-trim-check/startup ./tools/multi-tenancy-verify
# released-fixture36 由 git archive v1.0.0-rc.36 导出的正式版代码和发布夹具构建。
python3 /tmp/new-api-trim-check/verify.py
python3 /tmp/new-api-trim-check/verify36.py
```

临时脚本与日志保存在 `/tmp/new-api-trim-check/`。`verify.py` 覆盖新建库及 rc.35 基线；`verify36.py` 补充 rc.36 升级。每个升级库先运行旧版夹具两次，再运行内部包含两次初始化的当前验证器，比较升级前后的全部原有索引及约束。可长期复用的夹具与操作说明见 `tools/multi-tenancy/README.md`。

后端 `model/controller/middleware/service/service/authz/router` 测试通过。前端相关七个测试文件 **42 项通过**，类型检查、改动文件 lint/格式检查及生产构建通过。当前机器没有 Bun，使用已安装的同名工具：

```sh
cd web
./node_modules/.bin/tsgo -b
./node_modules/.bin/rsbuild build
NODE_OPTIONS=--no-experimental-webstorage ./node_modules/.bin/vitest run src/features/organizations src/features/dashboard/hooks/__tests__/model-analytics.test.tsx src/features/dashboard/components/overview/__tests__/organization-credit.test.tsx src/features/usage-logs/__tests__/platform-scope.test.ts --maxWorkers=2
```

本次删除 `Organization.Kind/Version` 的模型字段和 API 字段，不增加针对未发布开发库的自动删列迁移。曾运行中间版本的开发库可能仍保留无默认值的 NOT NULL 旧列，继续使用前需备份并清理旧结构；不要将该路径当作正式版升级路径。
