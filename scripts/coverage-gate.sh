#!/usr/bin/env bash
# 覆盖率门禁：本地 make ci 与 GitHub Actions CI 共用的单一实现。
# 用法：
#   scripts/coverage-gate.sh          # 在仓库根目录执行（含 go.work）
# 依赖：
#   - Go 覆盖率文件已生成于 /tmp/peak-coverage.out（go test -coverprofile）
#   - 前端覆盖率已生成于 web/coverage/coverage-summary.json（npm run test:cov）
set -euo pipefail

TOTAL_MIN=${TOTAL_MIN:-70}        # Go 总体覆盖率门槛（%）
WEB_TOTAL_MIN=${WEB_TOTAL_MIN:-80} # 前端总体语句覆盖率门槛（%）

# 每个 Go 包的最低覆盖率门槛（%）。key 为包路径，value 为阈值。
# 与 .github/workflows/ci.yml 保持一致。
declare -A PER_PKG_MIN=(
  ["peak/apps/gateway"]=60
  ["peak/apps/question-service/internal/handler"]=80
  ["peak/apps/question-service/internal/repository"]=85
  ["peak/apps/question-service/internal/service"]=95
  ["peak/apps/recognition-service/internal/handler"]=70
  ["peak/apps/recognition-service/internal/provider"]=85
  ["peak/apps/recognition-service/internal/service"]=80
  ["peak/libs/config"]=90
  ["peak/libs/domain"]=70
  ["peak/libs/errors"]=95
  ["peak/libs/http"]=60
  ["peak/libs/logger"]=90
  ["peak/libs/observability"]=60
  ["peak/libs/storage"]=45
)

echo "────────────────────────────"
echo "==> Go 覆盖率门禁"

# 用 go test -cover 的逐包结果（已是按包聚合的覆盖率）。
declare -A pkg_cov
while read -r status pkg rest; do
  case "$status" in
    ok|FAIL)
      pct=$(echo "$rest" | grep -oE 'coverage: [0-9.]+%' | grep -oE '[0-9.]+' | cut -d. -f1)
      [ -n "$pct" ] && pkg_cov["$pkg"]=$pct
      ;;
  esac
done < <(go test -cover peak/... 2>/dev/null)

pkg_failed=0
for pkg in "${!PER_PKG_MIN[@]}"; do
  threshold=${PER_PKG_MIN[$pkg]}
  got=${pkg_cov[$pkg]:-0}
  if [ "$got" -lt "$threshold" ]; then
    echo "✗ $pkg 覆盖率 ${got}% < 门槛 ${threshold}%"
    pkg_failed=1
  else
    echo "✓ $pkg 覆盖率 ${got}% (门槛 ${threshold}%)"
  fi
done

# 总体覆盖率（基于 coverage.out 的函数级聚合）。
if [ -f /tmp/peak-coverage.out ]; then
  total=$(go tool cover -func=/tmp/peak-coverage.out | tail -1 | awk '{print $3}' | tr -d '%' | cut -d. -f1)
  echo "────────────────────────────"
  echo "Go 总体覆盖率: ${total}% (门槛 ${TOTAL_MIN}%)"
  if [ "$total" -lt "$TOTAL_MIN" ]; then
    echo "✗ Go 总体覆盖率低于 ${TOTAL_MIN}%"
    pkg_failed=1
  fi
else
  echo "✗ 未找到 /tmp/peak-coverage.out，请先运行 go test -coverprofile"
  pkg_failed=1
fi

echo "────────────────────────────"
echo "==> 前端覆盖率门禁"

if [ -f web/coverage/coverage-summary.json ]; then
  web_total=$(node -e "
    const s = require('./web/coverage/coverage-summary.json');
    console.log(Math.floor(s.total.statements.pct));
  ")
  echo "前端总体语句覆盖率: ${web_total}% (门槛 ${WEB_TOTAL_MIN}%)"
  if [ "$web_total" -lt "$WEB_TOTAL_MIN" ]; then
    echo "✗ 前端覆盖率低于 ${WEB_TOTAL_MIN}%"
    pkg_failed=1
  fi
else
  echo "✗ 未找到 web/coverage/coverage-summary.json，请先运行 npm run test:cov"
  pkg_failed=1
fi

if [ "$pkg_failed" -ne 0 ]; then
  echo "────────────────────────────"
  echo "✗ 存在未达标的覆盖率门禁，流水线失败"
  exit 1
fi

echo "────────────────────────────"
echo "✓ 全部覆盖率门禁通过"
