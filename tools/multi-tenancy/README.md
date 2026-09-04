# 多租户停机备份、迁移与恢复

配合 [实现说明](../../docs/multi-tenancy-implementation.md) 使用。Python 工具只依赖标准库；MySQL 与 PostgreSQL 需要原生客户端。请选择与服务器兼容的客户端版本：本次 MySQL 5.7 与 PostgreSQL 9.6 的回滚使用服务器容器内同版本客户端验证。PostgreSQL 17 的 dump/restore 不能据此推断能恢复到 9.6。

配置文件应只对操作者可读（例如权限 `0600`），备份目录和文件由工具创建为 `0700` / `0600`。密码建议从环境变量读取。工具不会保存配置和密码到 manifest；备份内容本身包含业务数据和密钥，必须按数据库备份保管。

## 配置

配置只接受 `main` 与可选的 `log`。日志与主库共享时省略 `log`。可以为不同角色选择不同引擎，ClickHouse 仅用于日志。

SQLite：

```json
{
  "main": {"engine": "sqlite", "path": "/srv/new-api/data/new-api.db"}
}
```

MySQL 主库与独立日志库：

```json
{
  "main": {"engine": "mysql", "host": "127.0.0.1", "port": 3306, "user": "backup", "password_env": "TENANCY_BACKUP_PASSWORD", "database": "new_api", "bin_dir": "/opt/mysql/bin"},
  "log": {"engine": "mysql", "host": "127.0.0.1", "port": 3306, "user": "backup", "password_env": "TENANCY_BACKUP_PASSWORD", "database": "new_api_logs", "bin_dir": "/opt/mysql/bin"}
}
```

也支持 `socket` 替代 host/port。如果客户端在 Docker 容器中，使用 `client_container` 指定容器；此时 host/port 从容器内部解释，`bin_dir` 也是容器内路径。密码通过环境转发，不作为命令行密码参数传入。

PostgreSQL 主库与 ClickHouse 日志库：

```json
{
  "main": {"engine": "postgres", "host": "127.0.0.1", "port": 5432, "user": "backup", "password_env": "TENANCY_BACKUP_PASSWORD", "database": "new_api", "sslmode": "require", "bin_dir": "/opt/postgresql/bin"},
  "log": {"engine": "clickhouse", "url": "https://clickhouse.example.test:8443", "user": "backup", "password_env": "TENANCY_LOG_BACKUP_PASSWORD", "database": "new_api_logs"}
}
```

SQLite 使用 backup API；MySQL 包含触发器、存储程序和事件；PostgreSQL 使用 custom 格式（数据/结构，不复制角色与 ACL）；ClickHouse 保存每张表的建表语句和 Native 数据。恢复账号/权限、用户文件、Redis、对象存储和应用配置不在数据库快照内，应单独保留。标准 new-api 表结构已经验证；自定义 ClickHouse 视图、分布式表或复制集拓扑需另做运维备份。

## 操作

先停止全部写入。`--offline` 是操作者声明，工具不会自动停服务器或阻断支付回调。

```sh
python3 tools/multi-tenancy/snapshot.py backup --offline --config /secure/source.json --snapshot /secure/pre-tenancy
python3 tools/multi-tenancy/snapshot.py verify --snapshot /secure/pre-tenancy
go build -o /secure/multi-tenancy-migrate ./tools/multi-tenancy-migrate
/secure/multi-tenancy-migrate -offline -action migrate -snapshot /secure/pre-tenancy
/secure/multi-tenancy-migrate -offline -action migrate -snapshot /secure/pre-tenancy
/secure/multi-tenancy-migrate -offline -action verify
```

迁移工具从 `SQL_DSN`、`LOG_SQL_DSN`、`SQLITE_PATH` 读取应用连接配置，不读取 Python 配置。两者必须指向同一组库。迁移只运行数据库与权限初始化，不接收 HTTP、不启动计费任务。`verify` 不执行 AutoMigrate。

回滚时新建空数据库并修改目标配置的 database/path。SQL 数据库需事先创建并授予恢复账号权限；SQLite 文件可以不存在，但父目录必须存在。

```sh
python3 tools/multi-tenancy/snapshot.py restore --offline --config /secure/rollback-target.json --snapshot /secure/pre-tenancy
```

所有目标会在恢复开始前检查为空。恢复不会覆盖旧库或升级后的库；完成后核对数据，再将旧版本 DSN 切换至恢复目标。回滚必须使用原旧版本与原签名/加密密钥，并使用新的空 Redis 实例或专属逻辑库。快照恢复只恢复备份时点，不能保留之后产生的支付和消费；先处理增量账务，再决定是否开放旧版流量。

## 发布版升级夹具

`released-fixture.go.txt` 是使用真实旧模型生成代表性测试库的源码，禁止用于生产库。验证基线为 `v1.0.0-rc.30`，从仓库根目录执行：

```sh
git worktree add --detach /tmp/new-api-release-check v1.0.0-rc.30
mkdir -p /tmp/new-api-release-check/cmd/tenancy-fixture
cp tools/multi-tenancy/released-fixture.go.txt /tmp/new-api-release-check/cmd/tenancy-fixture/main.go
(cd /tmp/new-api-release-check && go build -o /tmp/new-api-release-fixture ./cmd/tenancy-fixture)
```

为每种引擎配置独立测试 DSN 后，旧版夹具启动两次并保存快照；当前版运行 `TENANCY_VERIFY_STARTUP=1 go run ./tools/multi-tenancy-verify`（内部两次）；另建空目标恢复快照，再运行旧版夹具两次。夹具会断言余额 `123456789012`、Key 余额 `12345`、日志额度 `321` 保持原值，并执行旧版 Casbin 初始化。权限配置和唯一索引由真实版本初始化创建。
