#!/usr/bin/env bash
# 迁移数据校验：源（自建容器）与目标（云数据库）逐表比对行数与内容指纹。
#
# 用法（在部署主机执行，切流前与回滚判断前各跑一次）：
#   SRC_PASSWORD=<源库密码> DST_HOST=<实例地址> DST_USER=<用户> DST_PASSWORD=<密码> \
#     ./verify.sh
#
# 环境变量：
#   PG_DATABASE     库名（默认 peak）
#   SRC_CONTAINER   源 PostgreSQL 容器名（默认 peak-postgres）
#   SRC_USER        源库用户（默认 peak）
#   SRC_PASSWORD    源库密码（必填）
#   DST_HOST        目标库地址（必填）
#   DST_PORT        目标库端口（默认 5432）
#   DST_USER        目标库用户（必填）
#   DST_PASSWORD    目标库密码（必填）
#   POSTGRES_IMAGE  用于客户端容器的 PostgreSQL 镜像（默认 postgres:16-alpine）
#
# 说明：内容指纹用 sum(hashtext(整行::text))，能发现"行数相同但内容不同"；
#       该计算会全表扫描，大表建议放到业务低峰执行。
#
# 退出码：0=全部一致；1=存在差异（禁止切流，先排查）
set -euo pipefail

PG_DATABASE="${PG_DATABASE:-peak}"
SRC_CONTAINER="${SRC_CONTAINER:-peak-postgres}"
SRC_USER="${SRC_USER:-peak}"
SRC_PASSWORD="${SRC_PASSWORD:?需要 SRC_PASSWORD（源库密码）}"
DST_HOST="${DST_HOST:?需要 DST_HOST（目标库地址）}"
DST_PORT="${DST_PORT:-5432}"
DST_USER="${DST_USER:?需要 DST_USER（目标库用户）}"
DST_PASSWORD="${DST_PASSWORD:?需要 DST_PASSWORD（目标库密码）}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:16-alpine}"

src_sql() {
  docker exec -e PGPASSWORD="$SRC_PASSWORD" "$SRC_CONTAINER" \
    psql -U "$SRC_USER" -d "$PG_DATABASE" -tAc "$1"
}

dst_sql() {
  docker run --rm -i -e PGPASSWORD="$DST_PASSWORD" "$POSTGRES_IMAGE" \
    psql -h "$DST_HOST" -p "$DST_PORT" -U "$DST_USER" -d "$PG_DATABASE" -tAc "$1"
}

# 表指纹：行数 + 整行文本哈希合计。
fingerprint_sql() {
  local t="$1"
  echo "SELECT count(*) || '|' || coalesce(sum(hashtext(x::text))::text, '0') FROM (SELECT * FROM \"$t\") x;"
}

echo "==> 读取源/目标表清单 ..."
LIST_SQL="SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename NOT LIKE 'pg_%' ORDER BY tablename;"
SRC_TABLES="$(src_sql "$LIST_SQL")"
DST_TABLES="$(dst_sql "$LIST_SQL")"

if [[ -z "$SRC_TABLES" ]]; then
  echo "错误：源库 $PG_DATABASE 中没有表，请确认容器与库名" >&2
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
  SRC_FP="$(src_sql "$(fingerprint_sql "$t")")"
  DST_FP="$(dst_sql "$(fingerprint_sql "$t")")"
  if [[ "$SRC_FP" == "$DST_FP" ]]; then
    echo "✓ $t  rows=${SRC_FP%%|*}  指纹一致"
  else
    echo "✗ $t  rows/指纹: src=$SRC_FP dst=$DST_FP"
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
echo "==> 校验通过：源/目标 $TABLE_N 张表行数与内容指纹全部一致"
