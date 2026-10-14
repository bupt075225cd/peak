#!/usr/bin/env bash
# 将备份文件导入目标 MySQL（云 RDS 或外部实例）。
#
# 用法（在部署主机执行）：
#   DST_HOST=<rds地址> DST_USER=<用户> DST_PASSWORD=<密码> \
#     ./restore.sh [备份文件.sql.gz]
#   不指定备份文件时，自动选用 BACKUP_DIR 下最新的 .sql.gz
#
# 环境变量：
#   MYSQL_DATABASE  库名（默认 peak）
#   DST_HOST        目标库地址（必填）
#   DST_PORT        目标库端口（默认 3306）
#   DST_USER        目标库用户（必填）
#   DST_PASSWORD    目标库密码（必填）
#   BACKUP_DIR      备份目录（默认 ./db-backups，未指定文件时取最新）
#   MYSQL_IMAGE     用于客户端容器的 MySQL 镜像（默认 mysql:8.0）
set -euo pipefail

MYSQL_DATABASE="${MYSQL_DATABASE:-peak}"
DST_HOST="${DST_HOST:?需要 DST_HOST（目标库地址）}"
DST_PORT="${DST_PORT:-3306}"
DST_USER="${DST_USER:?需要 DST_USER（目标库用户）}"
DST_PASSWORD="${DST_PASSWORD:?需要 DST_PASSWORD（目标库密码）}"
BACKUP_DIR="${BACKUP_DIR:-$(pwd)/db-backups}"
MYSQL_IMAGE="${MYSQL_IMAGE:-mysql:8.0}"

# 目标库客户端：借 mysql 镜像跑一次性容器（主机无需安装 mysql 客户端）
mysql_client() {
  docker run --rm -i -e MYSQL_PWD="$DST_PASSWORD" "$MYSQL_IMAGE" \
    mysql -h "$DST_HOST" -P "$DST_PORT" -u "$DST_USER" \
    --default-character-set=utf8mb4 "$@"
}

# ── 选定备份文件 ─────────────────────────────────────────
BACKUP_FILE="${1:-}"
if [[ -z "$BACKUP_FILE" ]]; then
  BACKUP_FILE="$(ls -1t "$BACKUP_DIR"/*.sql.gz 2>/dev/null | head -n 1 || true)"
fi
if [[ -z "$BACKUP_FILE" || ! -f "$BACKUP_FILE" ]]; then
  echo "错误：未找到备份文件（$BACKUP_DIR/*.sql.gz），请先执行 backup.sh 或显式指定" >&2
  exit 1
fi
echo "==> 使用备份文件：$BACKUP_FILE"

# ── 预检：连通性、账号权限、库与字符集 ───────────────────
echo "==> 预检目标库连通性 ..."
mysql_client -e "SELECT VERSION() AS version;" > /dev/null || {
  echo "错误：无法连接 $DST_HOST:$DST_PORT，请检查网络白名单与账号" >&2
  exit 1
}
echo "    连接成功"

echo "==> 预检目标库与字符集 ..."
# 建库（若账号无建库权限且库已存在，忽略报错）
if ! mysql_client -e "CREATE DATABASE IF NOT EXISTS \`$MYSQL_DATABASE\` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" 2>/dev/null; then
  echo "    提示：无建库权限（RDS 常见），假定目标库已在控制台预先创建"
fi
DB_EXISTS="$(mysql_client -N -e "SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME='$MYSQL_DATABASE';")"
if [[ -z "$DB_EXISTS" ]]; then
  echo "错误：目标库 $MYSQL_DATABASE 不存在，请在 RDS 控制台创建（utf8mb4）后重试" >&2
  exit 1
fi
CHARSET="$(mysql_client -N -e "SELECT DEFAULT_CHARACTER_SET_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME='$MYSQL_DATABASE';")"
echo "    库存在，默认字符集：$CHARSET（预期 utf8mb4）"

# ── 导入 ────────────────────────────────────────────────
echo "==> 导入数据（数据量越大耗时越长，请勿中断）..."
gunzip -c "$BACKUP_FILE" | mysql_client "$MYSQL_DATABASE"

echo "==> 导入完成，抽查表数量："
mysql_client -N -e "SELECT COUNT(*) AS tables_in_db FROM information_schema.TABLES WHERE TABLE_SCHEMA='$MYSQL_DATABASE';"
echo "==> restore.sh 执行成功，下一步请运行 verify.sh 比对数据"
