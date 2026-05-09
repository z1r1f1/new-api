#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="/home/ubuntu/git-project/new-api"
CONTAINER_NAME="new-api-postgres"
DB_USER="root"
DB_NAME="new-api"
BACKUP_DIR="$REPO_DIR/.runtime/prod/backups"
LOG_FILE="$BACKUP_DIR/backup-postgres.log"
LOCK_FILE="$BACKUP_DIR/backup-postgres.lock"
RETENTION_DAYS="${RETENTION_DAYS:-30}"

mkdir -p "$BACKUP_DIR"
cd "$REPO_DIR"

exec 9>"$LOCK_FILE"
if ! /usr/bin/flock -n 9; then
  echo "[$(/bin/date '+%F %T %z')] backup already running; skip" >> "$LOG_FILE"
  exit 0
fi

TS="$(/bin/date '+%Y%m%d-%H%M%S')"
OUT="$BACKUP_DIR/new-api-${TS}-postgres.sql.gz"
TMP="$OUT.tmp"

cleanup() {
  rm -f "$TMP"
}
trap cleanup EXIT

{
  echo "[$(/bin/date '+%F %T %z')] backup start: db=$DB_NAME container=$CONTAINER_NAME output=$OUT"

  /usr/bin/docker exec "$CONTAINER_NAME" pg_isready -U "$DB_USER" -d "$DB_NAME" >/dev/null
  /usr/bin/docker exec "$CONTAINER_NAME" pg_dump -U "$DB_USER" -d "$DB_NAME" | /usr/bin/gzip -9 > "$TMP"
  mv "$TMP" "$OUT"

  size="$(du -h "$OUT" | awk '{print $1}')"
  echo "[$(/bin/date '+%F %T %z')] backup success: $OUT size=$size"

  if [[ "$RETENTION_DAYS" =~ ^[0-9]+$ ]] && [ "$RETENTION_DAYS" -gt 0 ]; then
    /usr/bin/find "$BACKUP_DIR" -maxdepth 1 -type f -name 'new-api-*-postgres.sql.gz' -mtime +"$RETENTION_DAYS" -print -delete | sed 's/^/[retention deleted] /' || true
  fi
} >> "$LOG_FILE" 2>&1
