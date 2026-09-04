# 组织管理版本的 Docker Compose 部署

使用 `docker-compose.organization.yml` 从当前源码构建应用镜像，配套 PostgreSQL 15 与 Redis 7.2。该配置不使用官方 `latest` 应用镜像，因为其中不包含本分支的组织管理功能。

数据库和 Redis 只在 Compose 网络内访问，应用默认发布到 `3000` 端口。应用文件和日志保存在部署目录的 `data/`、`logs/`，数据库与 Redis 使用独立命名卷。

## 初始化配置

进入部署目录，用 Python 3 创建私有 `.env`。密码采用十六进制，避免连接 URI 中的特殊字符转义问题；脚本拒绝覆盖已有密钥。

```sh
python3 - <<'PY'
import os
import secrets

os.umask(0o077)
with open('.env', 'x') as output:
    output.write('APP_PORT=3000\nIMAGE_TAG=local\n')
    for name in ('POSTGRES_PASSWORD', 'REDIS_PASSWORD', 'SESSION_SECRET', 'CRYPTO_SECRET'):
        output.write(f'{name}={secrets.token_hex(32)}\n')
PY
```

`.env` 已被项目 Git 规则忽略，不应提交或共享。更新应用时保留现有 `.env` 和数据卷。

## 构建与启动

执行账号需要能够访问 Docker Engine。

如果 Linux 主机能下载依赖，但默认构建网络无法访问 DNS，可在 `.env` 中设置 `BUILD_NETWORK=host`，让镜像构建使用主机网络；运行中的服务仍使用独立的 Compose 网络。

如果无法访问默认 Go 模块下载源，可在 `.env` 中配置可访问的模块代理，例如 `GOPROXY=https://goproxy.cn,direct`。依赖仍通过 `go.sum` 和 Go 校验数据库验证。

构建也会读取当前环境或 `.env` 中的 `HTTP_PROXY`、`HTTPS_PROXY`、`NO_PROXY`。代理仅用于构建，不会配置到运行中的应用容器；使用主机回环地址上的代理时，需同时设置 `BUILD_NETWORK=host`。

```sh
docker compose -f docker-compose.organization.yml config --quiet
docker compose -f docker-compose.organization.yml up -d --build --wait --wait-timeout 300
docker compose -f docker-compose.organization.yml ps
```

浏览器访问服务器的 `3000` 端口，完成管理员初始化。可在 `.env` 中设置 `APP_PORT` 和 `BIND_ADDRESS` 改变监听地址。

该配置默认直接提供 HTTP。如果放在 HTTPS 反向代理后，将 `SESSION_COOKIE_SECURE` 设置为 `true`，同时设置 `SESSION_COOKIE_TRUSTED_URL` 为实际 HTTPS Origin、`TRUSTED_PROXIES` 为代理地址。

## 查看状态与停止

```sh
docker compose -f docker-compose.organization.yml logs --tail 100 new-api
docker compose -f docker-compose.organization.yml stop
docker compose -f docker-compose.organization.yml up -d --wait
```

停止服务会保留持久化数据。需要升级已有单用户计费数据库时，先执行 [停机迁移与回滚流程](multi-tenancy-implementation.md)，不要把旧库直接挂入新部署来试运行。
