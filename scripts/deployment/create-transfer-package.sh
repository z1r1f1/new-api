#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNTIME_DIR="$PROJECT_ROOT/.runtime/prod"
OUTPUT_DIR="$RUNTIME_DIR/transfer-packages"
PACKAGE_NAME="new-api-transfer-$(date +%Y%m%d-%H%M%S)"
POSTGRES_CONTAINER="new-api-postgres"
POSTGRES_USER="root"
POSTGRES_DB="new-api"
REDIS_CONTAINER="new-api-redis"
IMAGE_NAME="new-api-prod-local:latest"
INCLUDE_IMAGE=0
INCLUDE_POSTGRES=1
INCLUDE_REDIS=1
INCLUDE_LOGS=1
INCLUDE_BACKUPS=1
INSTALL_DIR='$HOME/git-project/new-api'
WORK_DIR=""

usage() {
  cat <<USAGE
生成 new-api Docker 一键迁移安装包。

用法：
  scripts/deployment/create-transfer-package.sh [选项]

常用：
  scripts/deployment/create-transfer-package.sh --include-image
  scp .runtime/prod/transfer-packages/new-api-transfer-*.tar.gz root@新服务器:/root/
  ssh root@新服务器 'tar -xzf new-api-transfer-*.tar.gz && cd new-api-transfer-* && ./install.sh --yes'

选项：
  --output-dir <目录>          输出目录，默认 .runtime/prod/transfer-packages
  --name <名称>                包名，默认 new-api-transfer-时间戳
  --install-dir <目录>         写入安装脚本提示的默认安装目录，默认 ~/git-project/new-api
  --include-image              打包当前应用镜像，目标机无需重新构建 new-api 镜像
  --image <镜像名>             应用镜像名，默认 new-api-prod-local:latest
  --postgres-container <名称>  PostgreSQL 容器名，默认 new-api-postgres
  --postgres-db <名称>         PostgreSQL 数据库名，默认 new-api
  --postgres-user <用户>       PostgreSQL 用户，默认 root
  --redis-container <名称>     Redis 容器名，默认 new-api-redis
  --no-postgres                不导出 PostgreSQL
  --no-redis                   不导出 Redis
  --no-logs                    不打包 .runtime/prod/logs
  --no-backups                 不打包 .runtime/prod/backups
  -h, --help                   显示帮助
USAGE
}

log() { printf '\033[1;34m[INFO]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[WARN]\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31m[ERROR]\033[0m %s\n' "$*" >&2; exit 1; }
need_cmd() { command -v "$1" >/dev/null 2>&1 || die "缺少命令：$1"; }

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --output-dir) OUTPUT_DIR="${2:?缺少 --output-dir 参数}"; shift 2 ;;
      --name) PACKAGE_NAME="${2:?缺少 --name 参数}"; shift 2 ;;
      --install-dir) INSTALL_DIR="${2:?缺少 --install-dir 参数}"; shift 2 ;;
      --include-image) INCLUDE_IMAGE=1; shift ;;
      --image) IMAGE_NAME="${2:?缺少 --image 参数}"; shift 2 ;;
      --postgres-container) POSTGRES_CONTAINER="${2:?缺少 --postgres-container 参数}"; shift 2 ;;
      --postgres-db) POSTGRES_DB="${2:?缺少 --postgres-db 参数}"; shift 2 ;;
      --postgres-user) POSTGRES_USER="${2:?缺少 --postgres-user 参数}"; shift 2 ;;
      --redis-container) REDIS_CONTAINER="${2:?缺少 --redis-container 参数}"; shift 2 ;;
      --no-postgres) INCLUDE_POSTGRES=0; shift ;;
      --no-redis) INCLUDE_REDIS=0; shift ;;
      --no-logs) INCLUDE_LOGS=0; shift ;;
      --no-backups) INCLUDE_BACKUPS=0; shift ;;
      -h|--help) usage; exit 0 ;;
      *) die "未知参数：$1" ;;
    esac
  done
}

require_running_container() {
  local name="$1"
  docker inspect -f '{{.State.Running}}' "$name" 2>/dev/null | grep -qx true || die "容器未运行：$name"
}

create_source_archive() {
  local pkg_dir="$1"
  log "打包项目源码..."
  tar -czf "$pkg_dir/source.tar.gz" \
    -C "$PROJECT_ROOT" \
    --exclude='./.git' \
    --exclude='./.runtime' \
    --exclude='./.omx' \
    --exclude='./.gitnexus' \
    --exclude='./web/node_modules' \
    --exclude='./web/default/node_modules' \
    --exclude='./web/classic/node_modules' \
    --exclude='./web/dist' \
    --exclude='./web/default/dist' \
    --exclude='./web/classic/dist' \
    --exclude='./tmp' \
    .
}

create_runtime_archive() {
  local pkg_dir="$1"
  local paths=(".runtime/prod/.env" ".runtime/prod/data")
  [[ "$INCLUDE_LOGS" == "1" && -d "$RUNTIME_DIR/logs" ]] && paths+=(".runtime/prod/logs")
  [[ "$INCLUDE_BACKUPS" == "1" && -d "$RUNTIME_DIR/backups" ]] && paths+=(".runtime/prod/backups")

  [[ -f "$RUNTIME_DIR/.env" ]] || die "缺少 $RUNTIME_DIR/.env"
  log "打包运行时文件：${paths[*]}"
  tar -czf "$pkg_dir/runtime-files.tar.gz" -C "$PROJECT_ROOT" "${paths[@]}"
}

export_postgres() {
  local pkg_dir="$1"
  [[ "$INCLUDE_POSTGRES" == "1" ]] || return 0
  need_cmd docker
  require_running_container "$POSTGRES_CONTAINER"
  mkdir -p "$pkg_dir/dumps"
  log "导出 PostgreSQL：container=$POSTGRES_CONTAINER db=$POSTGRES_DB"
  docker exec "$POSTGRES_CONTAINER" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --no-privileges | gzip -c > "$pkg_dir/dumps/postgres.sql.gz"
}

export_redis() {
  local pkg_dir="$1"
  [[ "$INCLUDE_REDIS" == "1" ]] || return 0
  need_cmd docker
  require_running_container "$REDIS_CONTAINER"
  mkdir -p "$pkg_dir/dumps"
  log "导出 Redis RDB：container=$REDIS_CONTAINER"
  docker exec "$REDIS_CONTAINER" redis-cli SAVE >/dev/null
  docker cp "$REDIS_CONTAINER:/data/dump.rdb" "$pkg_dir/dumps/redis-dump.rdb"
  gzip -f "$pkg_dir/dumps/redis-dump.rdb"
}

export_image() {
  local pkg_dir="$1"
  [[ "$INCLUDE_IMAGE" == "1" ]] || return 0
  need_cmd docker
  docker image inspect "$IMAGE_NAME" >/dev/null 2>&1 || die "镜像不存在：$IMAGE_NAME"
  log "打包 Docker 镜像：$IMAGE_NAME"
  docker save "$IMAGE_NAME" | gzip -c > "$pkg_dir/docker-image.tar.gz"
}

write_manifest() {
  local pkg_dir="$1"
  local git_commit="unknown"
  if command -v git >/dev/null 2>&1 && git -C "$PROJECT_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git_commit="$(git -C "$PROJECT_ROOT" rev-parse --short HEAD 2>/dev/null || true)"
  fi
  cat > "$pkg_dir/MANIFEST.txt" <<MANIFEST
new-api transfer package
created_at=$(date -Iseconds)
project_root=$PROJECT_ROOT
git_commit=$git_commit
postgres_included=$INCLUDE_POSTGRES
redis_included=$INCLUDE_REDIS
logs_included=$INCLUDE_LOGS
backups_included=$INCLUDE_BACKUPS
image_included=$INCLUDE_IMAGE
image_name=$IMAGE_NAME
install_dir_default=$INSTALL_DIR
MANIFEST
}

write_readme() {
  local pkg_dir="$1"
  cat > "$pkg_dir/README.txt" <<README
new-api Docker 一键迁移包

1. 把整个 tar.gz 上传到新服务器：
   scp ${PACKAGE_NAME}.tar.gz root@新服务器:/root/

2. 在新服务器解压并安装：
   tar -xzf ${PACKAGE_NAME}.tar.gz
   cd ${PACKAGE_NAME}
   sudo ./install.sh --yes

3. 如目标目录已有旧部署，使用：
   sudo ./install.sh --yes --force

4. 默认安装到 ~/git-project/new-api，访问：
   http://<服务器IP>:3001

5. 常用维护命令：
   cd ~/git-project/new-api
   docker compose --env-file .runtime/prod/.env ps
   docker compose --env-file .runtime/prod/.env logs -f --tail=100 new-api

注意：
- 新服务器需已安装 Docker 和 Docker Compose v2。
- 当前 docker-compose.yml 使用 host 网络，请确保 3001、5432、6379 端口未被占用。
- 包内包含 .runtime/prod/.env 和数据库备份，请按敏感数据保管。
README
}

main() {
  parse_args "$@"
  need_cmd tar
  need_cmd gzip
  need_cmd docker
  need_cmd python3
  [[ -d "$PROJECT_ROOT" ]] || die "项目目录不存在：$PROJECT_ROOT"
  [[ -d "$RUNTIME_DIR" ]] || die "运行时目录不存在：$RUNTIME_DIR"

  mkdir -p "$OUTPUT_DIR"
  WORK_DIR="$(mktemp -d)"
  trap '[[ -n "${WORK_DIR:-}" && -d "$WORK_DIR" ]] && rm -rf "$WORK_DIR"' EXIT
  local pkg_dir="$WORK_DIR/$PACKAGE_NAME"
  mkdir -p "$pkg_dir"

  cp "$PROJECT_ROOT/scripts/deployment/new-api-oneclick-install.sh" "$pkg_dir/install.sh"
  python3 - "$pkg_dir/install.sh" "$INSTALL_DIR" <<'PYEDIT'
from pathlib import Path
import sys
path = Path(sys.argv[1])
install_dir = sys.argv[2]
text = path.read_text()
text = text.replace('INSTALL_DIR="${INSTALL_DIR:-$HOME/git-project/new-api}"', f'INSTALL_DIR="${{INSTALL_DIR:-{install_dir}}}"')
path.write_text(text)
PYEDIT
  chmod +x "$pkg_dir/install.sh"
  create_source_archive "$pkg_dir"
  create_runtime_archive "$pkg_dir"
  export_postgres "$pkg_dir"
  export_redis "$pkg_dir"
  export_image "$pkg_dir"
  write_manifest "$pkg_dir"
  write_readme "$pkg_dir"

  local out_tar="$OUTPUT_DIR/${PACKAGE_NAME}.tar.gz"
  log "生成最终安装包：$out_tar"
  tar -czf "$out_tar" -C "$WORK_DIR" "$PACKAGE_NAME"
  log "完成：$out_tar"
  log "下一步：scp '$out_tar' root@新服务器:/root/ && ssh root@新服务器 'tar -xzf /root/${PACKAGE_NAME}.tar.gz && cd /root/${PACKAGE_NAME} && sudo ./install.sh --yes'"
}

main "$@"
