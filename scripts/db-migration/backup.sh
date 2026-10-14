#!/usr/bin/env bash
# 从自建 MySQL 容器一致性导出数据库并压缩备份。
#
# 用法（在部署主机执行）：
#   SRC_PASSWORD=xxx ./backup.sh
#
# 环境变量：
#   MYSQL_DATABASE  库名（默认 peak）
#   SRC_CONTAINER   源 MySQL 容器名（默认 peak-mysql）
#   SRC_USER        源库用户（默认 root）
#   SRC_PASSWORD    源库密码（必填）
#   BACKUP_DIR      备份输出目录（默认 ./db-backups）
#
# 输出：<BACKUP_DIR>/<库名>_<时间戳>.sql.gz
set -euo pipefail

MYSQL_DATABASE="${MYSQL_DATABASE:-peak}"
SRC_CONTAINER="${SRC_CONTAINER:-peak-mysql}"
SRC_USER="${SRC_USER:-root}"
SRC_PASSWORD="${SRC_PASSWORD:?需要 SRC_PASSWORD（源库密码）}"
BACKUP_DIR="${BACKUP_DIR:-$(pwd)/db-backups}"

mkdir -p "$BACKUP_DIR"
STAMP="$(date +%Y%m%d_%H%M%S)"
OUT="$BACKUP_DIR/${MYSQL_DATABASE}_${STAMP}.sql.gz"

echo "==> 检查源容器 $SRC_CONTAINER ..."
docker inspect "$SRC_CONTAINER" > /dev/null 2>&1 || {
  echo "错误：容器 $SRC_CONTAINER 不存在或未运行" >&2
  exit 1
}

echo "==> mysqldump 一致性导出 $MYSQL_DATABASE ..."
# --single-transaction：InnoDB 一致性快照，不锁表
# --set-gtid-purged=OFF：避免导入 RDS 时 GTID 冲突
if ! docker exec -e MYSQL_PWD="$SRC_PASSWORD" "$SRC_CONTAINER" \
    sh -c "exec mysqldump --user=\"$SRC_USER\" --single-transaction --routines --triggers --set-gtid-purged=OFF --default-character-set=utf8mb4 \"$MYSQL_DATABASE\"" \
    | gzip > "$OUT"; then
  rm -f "$OUT"
  echo "错误：导出失败，已删除残留文件" >&2
  exit 1
fi

# 备份完整性粗检：文件末尾应有 Dump completed 标记
if ! gunzip -c "$OUT" | tail -n 1 | grep -q "Dump completed"; then
  echo "错误：备份文件不完整（缺少 Dump completed 标记），请重试" >&2
  exit 1
fi

SIZE="$(du -h "$OUT" | cut -f1)"
echo "==> 备份完成：$OUT ($SIZE)"
echo "==> 建议记录校验和用于存档："
echo "    sha256sum $OUT"
