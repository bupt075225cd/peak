# 生产部署指南

本文档说明 Peak 各微服务在生产环境的部署方式。核心思路：**CI 构建镜像推送 ACR + 部署机拉取镜像 + 环境变量注入配置 + Docker Compose 编排**。

## 架构

```
                    ┌─────────────┐
                    │   客户端    │
                    └──────┬──────┘
                           │ :8080
                    ┌──────▼──────┐
                    │   gateway   │  (对外唯一入口)
                    └──────┬──────┘
              ┌────────────┼────────────┐
              ▼            ▼            ▼
       question-svc  recognition-svc  user-svc
              │            │
              ▼            ▼
           MySQL     本地存储/S3 + 阿里云AI
```

- `gateway` 是唯一对外的服务（暴露 8080），其余服务仅内网访问（`expose` 不映射端口）
- 各服务通过容器网络内服务名互相访问（如 `question-service:8081`）

## 前置要求

- Docker 20.10+ 与 Docker Compose v2
- 可访问阿里云 ACR（私有仓库需 `docker login`，凭证见 `deploy/deploy.env.example`）

## 1. 获取镜像

业务镜像由 CI（`docker-push` job）在每次 `push` 到 `main` 后自动构建并推送到阿里云 ACR，
镜像地址形如 `crpi-xxx.cn-chengdu.personal.cr.aliyuncs.com/peak2026/peak-<服务名>:<tag>`，
tag 同时打 commit sha（精确版本）与 `latest`（最新版）。**部署机不需要源码与构建环境。**

如需本地构建（如离线环境应急），各服务多阶段 Dockerfile 仍在仓库中：

```bash
# 从仓库根目录构建全部服务镜像
docker compose -f docker-compose.prod.yml build   # 编排需临时加回 build: 配置

# 或单独构建某个服务
docker build -f apps/gateway/Dockerfile -t peak-gateway .
docker build -f web/Dockerfile -t peak-web ./web
```

> 构建采用 Go workspace 上下文，`.dockerignore` 已排除测试/无关文件以加速构建。

## 2. 配置注入（环境变量）

各服务的 `config.yaml` 使用 `${VAR:-default}` 占位符，支持环境变量覆盖：

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 端口 | `SERVER_PORT` | 8080/8081/8082 | 各服务端口 |
| 日志模式 | `LOG_DEV` | true | 生产应设 `false` |
| 数据库 DSN | `DB_DSN` | 本地 MySQL | 生产指向 mysql 服务 |
| 数据库方言 | `DB_DIALECT` | mysql | mysql/postgres/sqlite |
| 识别 Provider | `RECOGNITION_PROVIDER` | mock | mock/aliyun |
| 阿里云密钥 | `ALIYUN_ACCESS_KEY_ID` 等 | 空 | 生产必填（aliyun 模式） |
| 追踪端点 | `TRACING_ENDPOINT` | 空 | 留空则不启用 OTel |

### 敏感配置管理

**切勿**将数据库密码、阿里云密钥写入 `config.yaml` 或提交到仓库。生产部署使用
`.env.production`（已 gitignore），模板见 [`deploy/.env.production.example`](.env.production.example)：

```bash
cp deploy/.env.production.example deploy/.env.production
vi deploy/.env.production   # 填入 JWT_SECRET、MySQL 密码、S3 凭证、识别密钥、IMAGE_TAG 等
```

Docker Compose 通过 `--env-file .env.production` 读取。

## 3. 启动

```bash
# 登录镜像仓库（私有仓库必须，一次即可）
docker login --username=<ACR用户名> <ACR实例地址>

# 拉取镜像并后台启动
docker compose --env-file .env.production -f docker-compose.prod.yml pull
docker compose --env-file .env.production -f docker-compose.prod.yml up -d
```

也可在能 SSH 到目标主机的机器上一键完成（上传文件 + 登录 + 拉取 + 启动）：

```bash
./deploy/deploy-to-host.sh root@<host>      # 生产模式
./deploy/deploy-to-host.sh -d root@<host>   # 开发调试模式：额外上传并叠加
                                            # docker-compose.dev.yml（LOG_DEV/AUTH_MASTER_CODE
                                            # 置 true，超级验证码 000000 可登录）
```

启动后服务分布：

| 服务 | 容器内端口 | 对外访问 |
|---|---|---|
| web（前端） | 80 | `http://<host>/` |
| gateway | 8080 | `http://<host>:8080`（或经 web 反代 `/api`） |
| question-service | 8081 | 仅内网 |
| recognition-service | 8082 | 仅内网 |
| mysql | 3306 | 仅内网 |
| prometheus | 9090 | `http://<host>:9090` |

> 前端 `web` 容器内 Nginx 已配置将 `/api` 反代到 `gateway:8080`，因此生产环境浏览器直接访问 `http://<host>/` 即可同时访问前端与后端 API。

## 4. 健康检查与监控

- **健康检查**：四个后端服务均实现 `/healthz`，prod 编排已配置容器 healthcheck（wget 探活，15s 间隔）
- **指标**：各服务暴露 `/metrics`，Prometheus 通过服务名抓取（见 `deploy/prometheus.yml`）
- **数据库**：mysql 服务配置了 healthcheck，`question-service`/`recognition-service` 依赖其 `service_healthy` 后才启动
- **日志**：`docker compose logs -f <service>`；生产日志同时由 Alloy 采集入 Loki

### 可观测栈（Grafana 全家桶）

| 服务 | 端口 | 说明 |
| --- | --- | --- |
| grafana | **仅 127.0.0.1:3001** | 统一看板入口（强制登录）：预置 5 块看板（全局/运行时/数据库/识别业务/前端），provisioning 自动导入 |
| prometheus | 内网 9090 | 指标抓取 + 告警规则求值（`deploy/prometheus/alerts.yml`） |
| alertmanager | 内网 9093 | 告警分组与通知（webhook 渠道见 `deploy/alertmanager/alertmanager.yml`） |
| loki | 内网 3100 | 集中日志存储（容器 stdout，JSON，含 trace_id） |
| alloy | 内网 | 日志采集代理（挂载 docker.sock，自动发现容器） |
| tempo | 内网 4317/3200 | OTLP 链路接收（业务 `TRACING_ENDPOINT=tempo:4317`）与查询 |

**Grafana 访问（安全双保险）**：端口只绑定主机回环地址 + 强制账号登录（匿名访问已关闭，密码经 `.env` 的 `GRAFANA_ADMIN_PASSWORD` 注入，无默认弱口令）。运维经 SSH 隧道访问：

```bash
ssh -L 3001:127.0.0.1:3001 root@<host>
# 浏览器打开 http://127.0.0.1:3001，用 GRAFANA_ADMIN_USER/PASSWORD 登录
```

> 注意：管理员密码只在数据卷首次初始化时生效；已初始化后如需改密，登入 Grafana 修改，或
> `docker compose down grafana && docker volume rm peak_grafana-data`（会丢失看板收藏等本地改动，provisioning 内容会自动重建）。

**三支柱互跳**：Grafana 内 Loki 的 `trace_id` 字段可点击跳转 Tempo 链路；Tempo 链路详情可跳回 Loki 按 trace_id 查日志。报警 → 链路 → 日志一条路径完成下钻。

**日常排查路径**：

1. Grafana「全局服务总览」看板发现 5xx 率或 P99 异常（或收到 Alertmanager 告警）
2. 从响应头 `X-Trace-Id`（或前端事件中的 trace_id）到 Tempo 查询链路，确认卡在 SQL（`gorm.*` span 的 `db.statement`）还是第三方 AI（`ai.*` span）
3. 到 Loki 用 `{container="peak-gateway"} | json | trace_id="<id>"` 拉出该请求全链路日志

告警通知渠道默认是 webhook 占位，接入钉钉/飞书时修改 `deploy/alertmanager/alertmanager.yml` 后 `up -d alertmanager` 生效。

## 5. 更新与回滚

版本由 `.env.production` 中的 `IMAGE_TAG` 控制（CI 为每次 main 推送都打 commit sha tag）：

```bash
# 更新到最新版
vi .env.production   # IMAGE_TAG=latest（或目标 commit sha）
docker compose --env-file .env.production -f docker-compose.prod.yml pull
docker compose --env-file .env.production -f docker-compose.prod.yml up -d

# 查看运行状态
docker compose --env-file .env.production -f docker-compose.prod.yml ps

# 回滚：把 IMAGE_TAG 改回旧 commit sha，重新 pull + up 即可
```

## 6. 前端部署

前端 `web/` 已容器化（`web/Dockerfile`），采用「Node 构建 + Nginx 托管」：

- Nginx 托管 `dist/` 静态资源，并开启 gzip 压缩与静态资源缓存
- 内置 SPA 路由回退（`try_files ... /index.html`），适配 vue-router history 模式
- 反代 `/api` 到 `gateway:8080`（见 `web/nginx.conf`）

```bash
# 作为 compose 的一部分整体部署（推荐）
docker compose --env-file .env.production -f docker-compose.prod.yml pull
docker compose --env-file .env.production -f docker-compose.prod.yml up -d
```

如需将前端与后端分离部署（前端走 CDN / 对象存储）：

```bash
cd web && npm install && npm run build
# 将 dist/ 上传到 OSS/S3 + CDN 加速
```

此时需保证 CDN 上 `/api` 路径能回源到 gateway，或在前端单独配置 API 域名。

## 7. 数据持久化

- **MySQL**：挂载卷 `mysql-data`（自动持久化到 Docker volume）
- **识别服务存储**：挂载卷 `recognition-data`（本地存储模式）

若切换到 S3 对象存储，将 `STORAGE_ROOT` 本地存储替换为 `S3Storage` 的 endpoint 配置即可（见主 README「文件存储」章节）。

## 8. 生产环境建议清单

- [ ] 修改 MySQL root 密码（`.env`）
- [ ] 配置阿里云密钥（若使用 `aliyun` provider）
- [ ] 设置 `LOG_DEV=false`
- [ ] 为 gateway 配置 TLS（通过前置 Nginx/负载均衡器）
- [ ] 接入 OpenTelemetry（`TRACING_ENDPOINT`）
- [ ] 配置 MySQL 备份策略
- [ ] 使用 `restart: unless-stopped` 保障服务自愈（已默认配置）
