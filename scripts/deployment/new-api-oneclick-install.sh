#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PACKAGE_DIR="${PACKAGE_DIR:-$SCRIPT_DIR}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/git-project/new-api}"
ASSUME_YES=0
FORCE_REPLACE=0
RESTORE_POSTGRES=1
RESTORE_REDIS=1
FORCE_BUILD=0
SKIP_HEALTHCHECK=0

usage() {
  cat <<USAGE
new-api Docker 一键迁移安装脚本

用法：
  sudo ./install.sh [选项]

选项：
  --dir <路径>              安装目录，默认 ~/git-project/new-api
  --yes                     非交互执行
  --force                   如果安装目录非空，先备份后替换
  --build                   即使包内带 Docker 镜像，也在目标机重新构建 new-api 镜像
  --no-restore-postgres     不恢复 PostgreSQL dump
  --no-restore-redis        不恢复 Redis dump
  --skip-healthcheck        跳过 /api/status 健康检查
  -h, --help                显示帮助

包内约定文件：
  source.tar.gz             项目源码和 docker-compose.yml
  runtime-files.tar.gz      .runtime/prod/.env、data、logs、backups 等运行时文件
  dumps/postgres.sql.gz     PostgreSQL 逻辑备份
  dumps/redis-dump.rdb.gz   Redis RDB 备份（可选）
  docker-image.tar.gz       new-api 应用镜像（可选）
USAGE
}

log() { printf '\033[1;34m[INFO]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[WARN]\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31m[ERROR]\033[0m %s\n' "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "缺少命令：$1"
}

confirm_or_die() {
  if [[ "$ASSUME_YES" == "1" ]]; then
    return 0
  fi
  printf "%s [y/N] " "$1"
  read -r answer
  case "$answer" in
    y|Y|yes|YES) return 0 ;;
    *) die "已取消" ;;
  esac
}

compose() {
  (cd "$INSTALL_DIR" && docker compose --env-file .runtime/prod/.env -f docker-compose.yml "$@")
}

compose_service_container() {
  (cd "$INSTALL_DIR" && docker compose --env-file .runtime/prod/.env -f docker-compose.yml ps -q "$1")
}

wait_for_postgres() {
  log "等待 PostgreSQL 就绪..."
  local cid
  cid="$(compose_service_container postgres)"
  [[ -n "$cid" ]] || die "未找到 postgres 容器"
  for _ in $(seq 1 60); do
    if docker exec "$cid" pg_isready -U root -d new-api >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  die "PostgreSQL 未在预期时间内就绪"
}

wait_for_http() {
  [[ "$SKIP_HEALTHCHECK" == "1" ]] && return 0
  log "等待 new-api 健康检查通过..."
  for _ in $(seq 1 60); do
    if command -v curl >/dev/null 2>&1; then
      if curl -fsS http://127.0.0.1:3001/api/status | grep -q '"success"[[:space:]]*:[[:space:]]*true'; then
        return 0
      fi
    elif command -v wget >/dev/null 2>&1; then
      if wget -q -O - http://127.0.0.1:3001/api/status | grep -q '"success"[[:space:]]*:[[:space:]]*true'; then
        return 0
      fi
    else
      warn "未找到 curl/wget，跳过 HTTP 健康检查"
      return 0
    fi
    sleep 3
  done
  warn "健康检查未通过，请查看：cd $INSTALL_DIR && docker compose --env-file .runtime/prod/.env logs --tail=200 new-api"
}

restore_postgres() {
  local dump="$PACKAGE_DIR/dumps/postgres.sql.gz"
  [[ "$RESTORE_POSTGRES" == "1" ]] || return 0
  [[ -f "$dump" ]] || { warn "未找到 $dump，跳过 PostgreSQL 恢复"; return 0; }

  wait_for_postgres
  log "恢复 PostgreSQL 数据库 new-api..."
  compose exec -T postgres psql -U root -d postgres -v ON_ERROR_STOP=1 <<'SQL'
SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'new-api' AND pid <> pg_backend_pid();
DROP DATABASE IF EXISTS "new-api";
CREATE DATABASE "new-api";
SQL
  gzip -dc "$dump" | compose exec -T postgres psql -U root -d new-api -v ON_ERROR_STOP=1
}

restore_redis() {
  local dump="$PACKAGE_DIR/dumps/redis-dump.rdb.gz"
  [[ "$RESTORE_REDIS" == "1" ]] || return 0
  [[ -f "$dump" ]] || { warn "未找到 $dump，跳过 Redis 恢复"; return 0; }

  log "恢复 Redis RDB 数据..."
  compose stop redis >/dev/null 2>&1 || true
  mkdir -p "$INSTALL_DIR/.runtime/prod/redis"
  rm -rf "$INSTALL_DIR/.runtime/prod/redis/appendonlydir" "$INSTALL_DIR/.runtime/prod/redis/dump.rdb"
  gzip -dc "$dump" > "$INSTALL_DIR/.runtime/prod/redis/dump.rdb"
  compose up -d redis
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --dir) INSTALL_DIR="${2:?缺少 --dir 参数}"; shift 2 ;;
      --yes|-y) ASSUME_YES=1; shift ;;
      --force) FORCE_REPLACE=1; shift ;;
      --build) FORCE_BUILD=1; shift ;;
      --no-restore-postgres) RESTORE_POSTGRES=0; shift ;;
      --no-restore-redis) RESTORE_REDIS=0; shift ;;
      --skip-healthcheck) SKIP_HEALTHCHECK=1; shift ;;
      -h|--help) usage; exit 0 ;;
      *) die "未知参数：$1" ;;
    esac
  done
}

main() {
  parse_args "$@"
  need_cmd docker
  need_cmd tar
  need_cmd gzip
  docker info >/dev/null 2>&1 || die "Docker daemon 不可用，请先安装并启动 Docker"
  docker compose version >/dev/null 2>&1 || die "缺少 Docker Compose v2 插件"

  [[ -f "$PACKAGE_DIR/source.tar.gz" ]] || die "包内缺少 source.tar.gz：$PACKAGE_DIR"
  [[ -f "$PACKAGE_DIR/runtime-files.tar.gz" ]] || die "包内缺少 runtime-files.tar.gz：$PACKAGE_DIR"

  if [[ -d "$INSTALL_DIR" && -n "$(find "$INSTALL_DIR" -mindepth 1 -maxdepth 1 2>/dev/null | head -n 1)" ]]; then
    if [[ "$FORCE_REPLACE" != "1" ]]; then
      die "安装目录非空：$INSTALL_DIR。请换目录，或加 --force 自动备份后替换。"
    fi
    local backup_dir="${INSTALL_DIR}.backup-$(date +%Y%m%d-%H%M%S)"
    confirm_or_die "将把现有 $INSTALL_DIR 移动到 $backup_dir，然后安装新包。继续吗？"
    log "备份现有安装目录到 $backup_dir"
    mv "$INSTALL_DIR" "$backup_dir"
  fi

  log "释放项目到 $INSTALL_DIR"
  mkdir -p "$INSTALL_DIR"
  tar -xzf "$PACKAGE_DIR/source.tar.gz" -C "$INSTALL_DIR"
  tar -xzf "$PACKAGE_DIR/runtime-files.tar.gz" -C "$INSTALL_DIR"
  mkdir -p "$INSTALL_DIR/.runtime/prod/data" "$INSTALL_DIR/.runtime/prod/logs" "$INSTALL_DIR/.runtime/prod/postgres" "$INSTALL_DIR/.runtime/prod/redis"

  [[ -f "$INSTALL_DIR/.runtime/prod/.env" ]] || die "缺少 $INSTALL_DIR/.runtime/prod/.env"
  if grep -Eq '^(HTTP_PROXY|HTTPS_PROXY)=https?://127\.0\.0\.1:' "$INSTALL_DIR/.runtime/prod/.env"; then
    warn ".runtime/prod/.env 中包含 127.0.0.1 代理；如果新服务器没有同样代理，请安装后编辑该文件并重启。"
  fi

  if [[ -f "$PACKAGE_DIR/docker-image.tar.gz" ]]; then
    log "加载包内 Docker 镜像..."
    gzip -dc "$PACKAGE_DIR/docker-image.tar.gz" | docker load
  fi

  log "启动 PostgreSQL 和 Redis..."
  compose up -d postgres redis
  restore_redis
  restore_postgres

  if [[ "$FORCE_BUILD" == "1" || ! -f "$PACKAGE_DIR/docker-image.tar.gz" ]]; then
    log "构建并启动 new-api 应用..."
    compose up -d --build new-api
  else
    log "使用包内镜像启动 new-api 应用..."
    compose up -d new-api
  fi

  wait_for_http
  log "安装完成。访问地址：http://<服务器IP>:3001"
  log "运行目录：$INSTALL_DIR"
  log "查看日志：cd $INSTALL_DIR && docker compose --env-file .runtime/prod/.env logs -f --tail=100 new-api"
}

main "$@"
