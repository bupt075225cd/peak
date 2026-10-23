#!/usr/bin/env bash
# 将备份文件导入目标 PostgreSQL（云数据库或外部实例）。
#
# 用法（在部署主机执行）：
#   DST_HOST=<实例地址> DST_USER=<用户> DST_PASSWORD=<密码> \
#     ./restore.sh [备份文件.dump]
#   不指定备份文件时，自动选用 BACKUP_DIR 下最新的 .dump
#
# 环境变量：
#   PG_DATABASE     库名（默认 peak）
#   DST_HOST        目标库地址（必填）
#   DST_PORT        目标库端口（默认 5432）
#   DST_USER        目标库用户（必填）
#   DST_PASSWORD    目标库密码（必填）
#   BACKUP_DIR      备份目录（默认 ./db-backups，未指定文件时取最新）
#   POSTGRES_IMAGE  用于客户端容器的 PostgreSQL 镜像（默认 postgres:16-alpine）
#
# 注意：导入使用 --clean --if-exists，会先删除目标库中同名对象再重建，
#       请务必确认 DST_HOST/PG_DATABASE 指向正确的实例与库。
set -euo pipefail

PG_DATABASE="${PG_DATABASE:-peak}"
DST_HOST="${DST_HOST:?需要 DST_HOST（目标库地址）}"
DST_PORT="${DST_PORT:-5432}"
DST_USER="${DST_USER:?需要 DST_USER（目标库用户）}"
DST_PASSWORD="${DST_PASSWORD:?需要 DST_PASSWORD（目标库密码）}"
BACKUP_DIR="${BACKUP_DIR:-$(pwd)/db-backups}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:16-alpine}"

# 目标库客户端：借 postgres 镜像跑一次性容器（主机无需安装 psql/pg_restore）。
pg_client() {
  docker run --rm -i -e PGPASSWORD="$DST_PASSWORD" "$POSTGRES_IMAGE" \
    psql -h "$DST_HOST" -p "$DST_PORT" -U "$DST_USER" -d "$PG_DATABASE" "$@"
}

# 维护库连接：用于建库与实例级检查。
admin_client() {
  docker run --rm -i -e PGPASSWORD="$DST_PASSWORD" "$POSTGRES_IMAGE" \
    psql -h "$DST_HOST" -p "$DST_PORT" -U "$DST_USER" -d postgres "$@"
}

# ── 选定备份文件 ─────────────────────────────────────────
BACKUP_FILE="${1:-}"
if [[ -z "$BACKUP_FILE" ]]; then
  BACKUP_FILE="$(ls -1t "$BACKUP_DIR"/*.dump 2>/dev/null | head -n 1 || true)"
fi
if [[ -z "$BACKUP_FILE" || ! -f "$BACKUP_FILE" ]]; then
  echo "错误：未找到备份文件（$BACKUP_DIR/*.dump），请先执行 backup.sh 或显式指定" >&2
  exit 1
fi
echo "==> 使用备份文件：$BACKUP_FILE"

# ── 预检：连通性、库与字符集 ─────────────────────────────
echo "==> 预检目标库连通性 ..."
admin_client -tAc "SELECT version();" > /dev/null || {
  echo "错误：无法连接 $DST_HOST:$DST_PORT，请检查网络白名单与账号" >&2
  exit 1
}
echo "    连接成功"

echo "==> 预检目标库与字符集 ..."
DB_EXISTS="$(admin_client -tAc "SELECT 1 FROM pg_database WHERE datname='$PG_DATABASE';")"
if [[ -z "$DB_EXISTS" ]]; then
  echo "    目标库不存在，尝试创建 ..."
  if ! admin_client -c "CREATE DATABASE \"$PG_DATABASE\" ENCODING 'UTF8';" > /dev/null 2>&1; then
    echo "错误：无法创建库 $PG_DATABASE（云数据库账号通常无建库权限），请在控制台创建后重试" >&2
    exit 1
  fi
fi
ENCODING="$(pg_client -tAc "SELECT pg_encoding_to_char(encoding) FROM pg_database WHERE datname=current_database();")"
echo "    库存在，字符编码：$ENCODING（预期 UTF8）"

# ── 导入 ────────────────────────────────────────────────
echo "==> 导入数据（--clean 覆盖同名对象；数据量越大耗时越长，请勿中断）..."
if ! docker run --rm -i -e PGPASSWORD="$DST_PASSWORD" "$POSTGRES_IMAGE" \
    pg_restore -h "$DST_HOST" -p "$DST_PORT" -U "$DST_USER" -d "$PG_DATABASE" \
    --clean --if-exists --no-owner --no-privileges < "$BACKUP_FILE"; then
  echo "错误：导入失败，请按报错处理后重跑（目标库将以最后一次成功导入为准）" >&2
  exit 1
fi

echo "==> 导入完成，抽查表数量："
pg_client -tAc "SELECT count(*) FROM pg_tables WHERE schemaname='public';"
echo "==> restore.sh 执行成功，下一步请运行 verify.sh 比对数据"
