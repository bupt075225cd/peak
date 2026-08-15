# 生产部署指南

本文档说明 Peak 各微服务在生产环境的部署方式。核心思路：**容器化 + 环境变量注入配置 + Docker Compose 编排**。

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
- 可访问镜像源（构建 Go 依赖时需网络）

## 1. 构建镜像

各服务已提供多阶段 Dockerfile，从仓库根目录构建：

- 后端：`apps/{gateway,question-service,recognition-service}/Dockerfile`（Go workspace 上下文）
- 前端：`web/Dockerfile`（Node 构建 + Nginx 托管）

```bash
# 构建全部服务镜像
docker compose -f docker-compose.prod.yml build

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

**切勿**将数据库密码、阿里云密钥写入 `config.yaml` 或提交到仓库。推荐用 `.env` 文件（已 gitignore）：

```bash
# .env（不提交）
MYSQL_ROOT_PASSWORD=your-strong-password
MYSQL_DATABASE=peak
ALIYUN_ACCESS_KEY_ID=xxx
ALIYUN_ACCESS_SECRET=xxx
ALIYUN_DASH_KEY=xxx
RECOGNITION_PROVIDER=aliyun
```

Docker Compose 会自动读取同目录的 `.env` 文件。

## 3. 启动

```bash
# 前台启动（调试）
docker compose -f docker-compose.prod.yml up

# 后台启动 + 构建
docker compose -f docker-compose.prod.yml up -d --build
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

- **Prometheus**：各服务暴露 `/metrics`，Prometheus 通过服务名抓取（见 `deploy/prometheus.yml`）
- **数据库**：mysql 服务配置了 healthcheck，`question-service`/`recognition-service` 依赖其 `service_healthy` 后才启动
- **日志**：`docker compose logs -f <service>` 查看

## 5. 更新与回滚

```bash
# 拉取新代码后重建并滚动更新
git pull
docker compose -f docker-compose.prod.yml up -d --build

# 查看运行状态
docker compose -f docker-compose.prod.yml ps

# 回滚：切换到旧 commit 后重新构建
```

## 6. 前端部署

前端 `web/` 已容器化（`web/Dockerfile`），采用「Node 构建 + Nginx 托管」：

- Nginx 托管 `dist/` 静态资源，并开启 gzip 压缩与静态资源缓存
- 内置 SPA 路由回退（`try_files ... /index.html`），适配 vue-router history 模式
- 反代 `/api` 到 `gateway:8080`（见 `web/nginx.conf`）

```bash
# 作为 compose 的一部分整体部署（推荐）
docker compose -f docker-compose.prod.yml up -d --build

# 或单独构建/运行前端镜像
docker build -f web/Dockerfile -t peak-web ./web
docker run -d -p 80:80 --network peak_default peak-web
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
