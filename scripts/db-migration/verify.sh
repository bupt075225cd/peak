#!/usr/bin/env bash
# 迁移数据校验：源（自建容器）与目标（RDS）逐表比对行数与 CHECKSUM。
#
# 用法（在部署主机执行，切流前与回滚判断前各跑一次）：
#   SRC_PASSWORD=<源库密码> DST_HOST=<rds地址> DST_USER=<用户> DST_PASSWORD=<密码> \
#     ./verify.sh
#
# 环境变量：
#   MYSQL_DATABASE  库名（默认 peak）
#   SRC_CONTAINER   源 MySQL 容器名（默认 peak-mysql）
#   SRC_USER        源库用户（默认 root）
#   SRC_PASSWORD    源库密码（必填）
#   DST_HOST        目标库地址（必填）
#   DST_PORT        目标库端口（默认 3306）
#   DST_USER        目标库用户（必填）
#   DST_PASSWORD    目标库密码（必填）
#   MYSQL_IMAGE     用于客户端容器的 MySQL 镜像（默认 mysql:8.0）
#
# 退出码：0=全部一致；1=存在差异（禁止切流，先排查）
set -euo pipefail

MYSQL_DATABASE="${MYSQL_DATABASE:-peak}"
SRC_CONTAINER="${SRC_CONTAINER:-peak-mysql}"
SRC_USER="${SRC_USER:-root}"
SRC_PASSWORD="${SRC_PASSWORD:?需要 SRC_PASSWORD（源库密码）}"
DST_HOST="${DST_HOST:?需要 DST_HOST（目标库地址）}"
DST_PORT="${DST_PORT:-3306}"
DST_USER="${DST_USER:?需要 DST_USER（目标库用户）}"
DST_PASSWORD="${DST_PASSWORD:?需要 DST_PASSWORD（目标库密码）}"
MYSQL_IMAGE="${MYSQL_IMAGE:-mysql:8.0}"

src_sql() {
  docker exec -e MYSQL_PWD="$SRC_PASSWORD" "$SRC_CONTAINER" \
    sh -c "exec mysql --user=\"$SRC_USER\" --default-character-set=utf8mb4 -N -e \"$1\" \"$MYSQL_DATABASE\""
}

dst_sql() {
  docker run --rm -i -e MYSQL_PWD="$DST_PASSWORD" "$MYSQL_IMAGE" \
    mysql -h "$DST_HOST" -P "$DST_PORT" -u "$DST_USER" \
    --default-character-set=utf8mb4 -N -e "$1" "$MYSQL_DATABASE"
}

echo "==> 读取源/目标表清单 ..."
SRC_TABLES="$(src_sql "SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_TYPE='BASE TABLE' ORDER BY TABLE_NAME;")"
DST_TABLES="$(dst_sql "SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_TYPE='BASE TABLE' ORDER BY TABLE_NAME;")"

if [[ -z "$SRC_TABLES" ]]; then
  echo "错误：源库 $MYSQL_DATABASE 中没有表，请确认容器与库名" >&2
  exit 1
fi

FAIL=0
MISSING_IN_DST=0
# 以源库表清单为基准逐表比对
while IFS= read -r t; do
  [[ -z "$t" ]] && continue
  if ! grep -qxF "$t" <<< "$DST_TABLES"; then
    echo "✗ 表 $t 在目标库缺失"
    MISSING_IN_DST=$((MISSING_IN_DST + 1))
    FAIL=1
    continue
  fi
  SRC_COUNT="$(src_sql "SELECT COUNT(*) FROM \`$t\`;")"
  DST_COUNT="$(dst_sql "SELECT COUNT(*) FROM \`$t\`;")"
  SRC_CK="$(src_sql "CHECKSUM TABLE \`$t\`;")"
  DST_CK="$(dst_sql "CHECKSUM TABLE \`$t\`;")"
  if [[ "$SRC_COUNT" == "$DST_COUNT" && "$SRC_CK" == "$DST_CK" ]]; then
    echo "✓ $t  rows=$SRC_COUNT  checksum 一致"
  else
    echo "✗ $t  rows: src=$SRC_COUNT dst=$DST_COUNT  checksum: src=$SRC_CK dst=$DST_CK"
    FAIL=1
  fi
done <<< "$SRC_TABLES"

# 目标库多出的表（通常为 AutoMigrate 预建的空表，仅提示）
while IFS= read -r t; do
  [[ -z "$t" ]] && continue
  if ! grep -qxF "$t" <<< "$SRC_TABLES"; then
    echo "ℹ 目标库多出表：$t（如为 GORM AutoMigrate 预建空表可忽略）"
  fi
done <<< "$DST_TABLES"

if [[ "$MISSING_IN_DST" -gt 0 ]]; then
  echo "错误：目标库缺失 $MISSING_IN_DST 张表，多半是导入不完整" >&2
fi

if [[ "$FAIL" -ne 0 ]]; then
  echo "==> 校验失败：存在差异，禁止切流，请重新导入后复核" >&2
  exit 1
fi
TABLE_N="$(grep -c . <<< "$SRC_TABLES")"
echo "==> 校验通过：源/目标 $TABLE_N 张表行数与 CHECKSUM 全部一致"
