#!/usr/bin/env bash
# 从 PostgreSQL 容器一致性导出数据库（自定义格式，供 pg_restore 恢复）。
#
# 用法（在部署主机执行）：
#   SRC_PASSWORD=xxx ./backup.sh
#
# 环境变量：
#   PG_DATABASE     库名（默认 peak）
#   SRC_CONTAINER   源 PostgreSQL 容器名（默认 peak-postgres）
#   SRC_USER        源库用户（默认 peak）
#   SRC_PASSWORD    源库密码（必填）
#   BACKUP_DIR      备份输出目录（默认 ./db-backups）
#
# 输出：<BACKUP_DIR>/<库名>_<时间戳>.dump
set -euo pipefail

PG_DATABASE="${PG_DATABASE:-peak}"
SRC_CONTAINER="${SRC_CONTAINER:-peak-postgres}"
SRC_USER="${SRC_USER:-peak}"
SRC_PASSWORD="${SRC_PASSWORD:?需要 SRC_PASSWORD（源库密码）}"
BACKUP_DIR="${BACKUP_DIR:-$(pwd)/db-backups}"

mkdir -p "$BACKUP_DIR"
STAMP="$(date +%Y%m%d_%H%M%S)"
OUT="$BACKUP_DIR/${PG_DATABASE}_${STAMP}.dump"

echo "==> 检查源容器 $SRC_CONTAINER ..."
docker inspect "$SRC_CONTAINER" > /dev/null 2>&1 || {
  echo "错误：容器 $SRC_CONTAINER 不存在或未运行" >&2
  exit 1
}

echo "==> pg_dump 一致性导出 $PG_DATABASE ..."
# -Fc：自定义格式（压缩、支持并行/选择性恢复）；MVCC 快照保证一致性，不锁表。
if ! docker exec -e PGPASSWORD="$SRC_PASSWORD" "$SRC_CONTAINER" \
    pg_dump --username="$SRC_USER" --dbname="$PG_DATABASE" --format=custom \
    > "$OUT"; then
  rm -f "$OUT"
  echo "错误：导出失败，已删除残留文件" >&2
  exit 1
fi

# 备份完整性粗检：dump 目录可正常解析即视为完整。
if ! docker exec -i "$SRC_CONTAINER" pg_restore --list < "$OUT" > /dev/null 2>&1; then
  rm -f "$OUT"
  echo "错误：备份文件不完整（pg_restore 无法解析），请重试" >&2
  exit 1
fi

SIZE="$(du -h "$OUT" | cut -f1)"
echo "==> 备份完成：$OUT ($SIZE)"
echo "==> 建议记录校验和用于存档："
echo "    sha256sum $OUT"
