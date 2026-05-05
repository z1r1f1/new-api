# new-api Docker 迁移安装包

本项目提供两个脚本，用于把本机 Docker 部署的 new-api（源码、`.runtime/prod/.env`、运行时目录、PostgreSQL 数据库、Redis 数据）打成一个 tar.gz，并在另一台服务器一键恢复运行。

## 生成安装包

在旧服务器/当前项目目录执行：

```bash
# 推荐：同时打包当前 new-api Docker 镜像，目标服务器无需重新构建应用镜像
scripts/deployment/create-transfer-package.sh --include-image
```

生成位置默认在：

```text
.runtime/prod/transfer-packages/new-api-transfer-YYYYmmdd-HHMMSS.tar.gz
```

如果目标服务器可以联网并允许重新构建镜像，也可以不加 `--include-image`。

## 上传并安装

```bash
scp .runtime/prod/transfer-packages/new-api-transfer-*.tar.gz root@新服务器:/root/
ssh root@新服务器
cd /root
tar -xzf new-api-transfer-*.tar.gz
cd new-api-transfer-*
sudo ./install.sh --yes
```

默认安装目录是 `~/git-project/new-api`。如需自定义：

```bash
sudo ./install.sh --yes --dir /data/new-api
```

如果目标目录已经有旧部署，需要自动备份后替换：

```bash
sudo ./install.sh --yes --force
```

## 包内包含什么

- `source.tar.gz`：项目源码与 Docker 部署文件，不包含 `.git`、`.runtime`、`node_modules`、构建产物。
- `runtime-files.tar.gz`：`.runtime/prod/.env`、`data`、`logs`、`backups`。
- `dumps/postgres.sql.gz`：通过 `pg_dump --no-owner --no-privileges` 导出的 PostgreSQL 数据。
- `dumps/redis-dump.rdb.gz`：通过 `redis-cli SAVE` 导出的 Redis RDB（可用 `--no-redis` 跳过）。
- `docker-image.tar.gz`：可选，使用 `--include-image` 时写入。
- `install.sh`：目标服务器一键安装/恢复脚本。

## 目标服务器要求

- 已安装 Docker。
- 已安装 Docker Compose v2：`docker compose version` 可用。
- 当前 `docker-compose.yml` 使用 host 网络，目标服务器的 `3001`、`5432`、`6379` 端口需未被占用。
- 包内包含 `.runtime/prod/.env` 和数据库，请按敏感数据保管，不要上传到公开位置或提交到 git。

## 常用选项

生成包：

```bash
scripts/deployment/create-transfer-package.sh --help
```

安装包：

```bash
./install.sh --help
```

常用跳过项：

```bash
# 不带日志，减小包体积
scripts/deployment/create-transfer-package.sh --include-image --no-logs

# 只迁移源码和运行时文件，不导出数据库/Redis
scripts/deployment/create-transfer-package.sh --no-postgres --no-redis

# 目标机重新构建应用镜像
sudo ./install.sh --yes --build
```

## 安装后检查

```bash
cd ~/git-project/new-api
docker compose --env-file .runtime/prod/.env ps
docker compose --env-file .runtime/prod/.env logs -f --tail=100 new-api
curl http://127.0.0.1:3001/api/status
```

安装脚本会自动恢复数据库、恢复 Redis、启动 new-api，并尝试访问 `/api/status` 做健康检查。

## 注意代理配置

如果旧服务器的 `.runtime/prod/.env` 中有类似：

```text
HTTP_PROXY=http://127.0.0.1:7890
HTTPS_PROXY=http://127.0.0.1:7890
```

迁移到新服务器后，请确认新服务器也有同样代理；否则编辑 `~/git-project/new-api/.runtime/prod/.env` 删除或替换代理后重启：

```bash
cd ~/git-project/new-api
docker compose --env-file .runtime/prod/.env up -d
```
