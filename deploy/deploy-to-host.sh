#!/usr/bin/env bash
# 远程一键部署：把编排文件与生产 .env 推送到目标主机并拉起服务。
# 在「能 SSH 到目标主机」的机器上执行（本仓库沙箱环境与内网不通时，用你的办公机/跳板机）。
#
# 用法：
#   ./deploy-to-host.sh [-d] [--obs] <user>@<host> [端口]
#   -d     开发调试模式：额外上传并叠加 docker-compose.dev.yml
#          （开启 LOG_DEV/AUTH_MASTER_CODE，超级验证码 000000 可登录）
#   --obs  同时部署可选观测栈（Prometheus/Alertmanager/Loki/Alloy/Tempo/Grafana，
#          Compose profiles: observability）。默认只部署 6 个业务容器。
# 前置：
#   1) 已能 ssh <user>@<host>（密钥或交互输密码均可）
#   2) deploy/.env.production 已按模板填好真实值
#      （--obs 时 GRAFANA_ADMIN_PASSWORD 必须为强密码）
#   3) deploy/deploy.env（可选）配置 ACR 登录：ACR_USER / ACR_PASS
set -euo pipefail

DEV=0
OBS=0
# 解析选项（-d 与 --obs 可任意组合）
while [ "${1:-}" = "-d" ] || [ "${1:-}" = "--obs" ]; do
  if [ "$1" = "-d" ]; then DEV=1; else OBS=1; fi
  shift
done

TARGET=${1:?用法: ./deploy-to-host.sh [-d] [--obs] <user>@<host> [端口]}
PORT=${2:-22}
REMOTE_DIR=/opt/peak

cd "$(dirname "$0")/.."
[ -f deploy/.env.production ] || { echo "✗ 缺少 deploy/.env.production，请按 .env.production.example 填写"; exit 1; }
if [ "$DEV" = "1" ] && [ ! -f docker-compose.dev.yml ]; then
  echo "✗ 开发模式需要 docker-compose.dev.yml（仓库根目录）"; exit 1
fi
if [ "$OBS" = "1" ] && ! grep -q '^GRAFANA_ADMIN_PASSWORD=..*' deploy/.env.production; then
  echo "✗ --obs 需要 deploy/.env.production 中设置 GRAFANA_ADMIN_PASSWORD（强密码）"; exit 1
fi

# compose -f 参数（顺序重要：后面的文件覆盖前面的）
COMPOSE_FILES="-f docker-compose.prod.yml"
if [ "$DEV" = "1" ]; then
  COMPOSE_FILES="-f docker-compose.prod.yml -f docker-compose.dev.yml"
fi
# 观测栈走 Compose profiles：默认不部署；--obs 时激活 observability profile
OBS_ARGS=""
if [ "$OBS" = "1" ]; then
  OBS_ARGS="--profile observability"
fi

echo "==> 上传部署文件到 $TARGET:$REMOTE_DIR（模式：$([ "$DEV" = "1" ] && echo 开发调试 || echo 生产)$([ "$OBS" = "1" ] && echo " + 观测栈" || echo "")）"
ssh -p "$PORT" "$TARGET" "mkdir -p $REMOTE_DIR/deploy"
scp -P "$PORT" docker-compose.prod.yml "$TARGET:$REMOTE_DIR/"
if [ "$DEV" = "1" ]; then scp -P "$PORT" docker-compose.dev.yml "$TARGET:$REMOTE_DIR/"; fi
scp -P "$PORT" deploy/.env.production "$TARGET:$REMOTE_DIR/.env.production"

# 可观测栈配置：仅 --obs 时上传（默认轻量部署，远端不留观测配置）
# prometheus 告警规则 / loki / tempo / alloy / alertmanager / grafana provisioning
if [ "$OBS" = "1" ]; then
  scp -P "$PORT" deploy/prometheus.yml "$TARGET:$REMOTE_DIR/deploy/"
  ssh -p "$PORT" "$TARGET" "mkdir -p $REMOTE_DIR/deploy/prometheus $REMOTE_DIR/deploy/loki \
    $REMOTE_DIR/deploy/tempo $REMOTE_DIR/deploy/alloy $REMOTE_DIR/deploy/alertmanager \
    $REMOTE_DIR/deploy/grafana/provisioning/datasources $REMOTE_DIR/deploy/grafana/provisioning/dashboards \
    $REMOTE_DIR/deploy/grafana/dashboards"
  scp -P "$PORT" deploy/prometheus/alerts.yml "$TARGET:$REMOTE_DIR/deploy/prometheus/"
  scp -P "$PORT" deploy/loki/loki-config.yml "$TARGET:$REMOTE_DIR/deploy/loki/"
  scp -P "$PORT" deploy/tempo/tempo-config.yml "$TARGET:$REMOTE_DIR/deploy/tempo/"
  scp -P "$PORT" deploy/alloy/config.alloy "$TARGET:$REMOTE_DIR/deploy/alloy/"
  scp -P "$PORT" deploy/alertmanager/alertmanager.yml "$TARGET:$REMOTE_DIR/deploy/alertmanager/"
  # grafana provisioning + 看板整体传输（tar 管道，避免 scp -r 目录嵌套与 "/." 兼容问题）
  tar -C deploy -cf - grafana | ssh -p "$PORT" "$TARGET" "tar -C $REMOTE_DIR/deploy -xf -"
fi

# 可选：在目标主机登录阿里云 ACR（私有仓库需要）
if [ -f deploy/deploy.env ]; then
  echo "==> 登录阿里云 ACR"
  . ./deploy/deploy.env
  ssh -p "$PORT" "$TARGET" "docker login --username='$ACR_USER' --password-stdin \
    crpi-vb458bo8ja2welze.cn-chengdu.personal.cr.aliyuncs.com" <<< "$ACR_PASS"
fi

echo "==> 拉取镜像并启动"
ssh -t -p "$PORT" "$TARGET" "cd $REMOTE_DIR && \
  docker compose --env-file .env.production $OBS_ARGS $COMPOSE_FILES pull && \
  docker compose --env-file .env.production $OBS_ARGS $COMPOSE_FILES up -d && \
  docker compose --env-file .env.production $OBS_ARGS $COMPOSE_FILES ps"

if [ "$DEV" = "1" ]; then
  echo "✓ 开发环境部署完成，超级验证码 000000 可登录，访问 http://<主机IP>:80"
else
  echo "✓ 部署完成，访问 http://<主机IP>:80"
fi
if [ "$OBS" = "1" ]; then
  echo "✓ 观测栈已启动：Grafana http://<主机IP>:3001（Prometheus 内网 9090 / Tempo OTLP 4317）"
else
  echo "ℹ 观测栈未部署（默认轻量模式）；如需开启：./deploy-to-host.sh --obs <user>@<host>"
fi
