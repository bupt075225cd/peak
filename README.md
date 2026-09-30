# Peak · 中学生错题本

Peak 是一款面向中学生的错题本软件，核心解决「收集错题 → 分类管理 → 组卷练习 → 记录练习数据」的学习闭环。

## 架构概览

前后端分离 + 微服务（单仓库多服务 monorepo），后端 Go，前端 Vue3 + TypeScript。

```
                    ┌─────────────┐
                    │   web (Vue3) │
                    └──────┬──────┘
                           │ HTTP
                    ┌──────▼──────┐
                    │   gateway   │  统一入口/鉴权/跨域/链路透传
                    └──────┬──────┘
              ┌────────────┼────────────┐
              ▼            ▼            ▼
      ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
      │question-svc  │ │recognition-svc│ │ user-svc(预留)│
      └──────┬───────┘ └──────┬───────┘ └──────────────┘
             │                │
             │                │  几何重绘（VLM 坐标直出 + Go 渲染）
             │                ▼
             │        ┌──────────────────┐
             │        │ recognition-svc  │
             │        │ internal/geom    │
             │        └──────────────────┘
             ▼                ▼
        MySQL (GORM)    文件存储(本地→S3兼容对象存储) + 第三方AI(阿里云)
```

## 目录结构

```
peak/
├── go.work                  # Go workspace 根
├── Makefile                 # 本地开发统一入口（check/build/test/ci/watch）
├── .husky/                  # git pre-commit hook（husky 管理）
├── .github/workflows/       # CI 流水线（编译/静态检查/测试）
├── apps/                    # 微服务
│   ├── gateway/             # API 网关 (:8080)
│   ├── question-service/    # 题目/错题服务 (:8081)
│   ├── recognition-service/ # 识别服务 (:8082，含内置几何渲染 internal/geom)
│   └── user-service/        # 用户服务（预留 :8083）
├── libs/                    # 公共库
│   ├── config/              # 配置加载（YAML + 环境变量）
│   ├── logger/              # zap 日志封装
│   ├── errors/              # 统一错误码
│   ├── http/                # HTTP 封装、中间件
│   ├── storage/             # FileStorage 接口 + LocalStorage + S3Storage(对象存储)
│   ├── domain/              # 领域模型 + GORM 迁移 + 数据库方言
│   └── observability/       # Prometheus 指标 + OTel 追踪
├── web/                     # Vue3 前端
└── deploy/                  # 部署配置（prometheus 等）
```

## 开发 Quick Start

### 0. 环境要求

- **Go 1.24+**（workspace 模式，`go.work` 已就绪）
- **Node.js 20+**（前端 Vue3 + Vite）
- **Docker**（可选，用于启动 MySQL 基础设施）

### 1. 克隆并安装依赖

```bash
git clone <repo-url> && cd peak

# 安装前端依赖（会自动启用 git pre-commit hook，见下文）
npm install
cd web && npm install
```

> `npm install` 会通过 husky 的 `prepare` 脚本自动启用提交前静态检查，无需手动配置。

### 2. 一键安装辅助工具（可选）

```bash
make install-tools   # 安装 watchexec（文件监听）+ air（Go 热重载）
```

### 3. 日常开发工作流

项目根目录提供了 `Makefile` 统一入口，一条命令完成常用动作：

| 命令 | 作用 | 场景 |
|---|---|---|
| `make check` | 静态检查：Go vet + 前端 vue-tsc 类型检查 | 改完代码秒级反馈 |
| `make lint` | golangci-lint 逐模块检查（未安装会自动安装，与远端 CI 同版本） | 提交前代码质量检查 |
| `make build` | 编译：Go 全部模块 + 前端构建 | 确认可编译 |
| `make test` | 单元测试：前后端（含覆盖率） | 确认逻辑正确 |
| `make ci` | 一键全跑 = check + lint + build + test + 覆盖率门禁 | **提交前执行**，等价 CI |
| `make watch` | 监听 `.go/.ts/.vue`，保存即自动 check | 开发中持续反馈 |
| `make install-tools` | 安装辅助工具 | 首次环境准备 |
| `make run-sqlite` | 编译并以 SQLite 空库一键启动全部服务（gateway/question/recognition） | 本地免 MySQL 调测，重启即清空数据 |
| `make stop-sqlite` | 停止 `run-sqlite` 启动的服务并清空数据目录与日志 | 结束调测 |

```bash
# 开发中（可选，开着自动检查）
make watch

# 改完一批代码
make check

# 提交前全量验证（和 CI 结果一致）
make ci
```

### 4. 提交与推送的本地门禁（git hook）

已通过 **husky** 配置两个 hook，随代码提交，全团队一致：

| Hook | 触发时机 | 执行内容 | 失败后果 |
|---|---|---|---|
| `pre-commit` | `git commit` 前 | `make check`（Go vet + 前端类型检查，轻量） | 阻断提交 |
| `pre-push` | `git push` 前 | `make ci`（check + lint + build + test + 覆盖率门禁，等价远端 CI） | **阻断推送** |

- 提交时仅做轻量静态检查；**推送前**才跑完整 `make ci`（含覆盖率门禁），确保与远端 GitHub Actions 结果一致，避免推送后才发现失败
- 覆盖率门禁：Go 总体 70% + 逐包阈值、前端 80%（实现见 `scripts/coverage-gate.sh`）
- 紧急跳过（不推荐）：`git commit --no-verify` / `git push --no-verify`
- hook 脚本位于 `.husky/pre-commit`、`.husky/pre-push`

---

## 快速开始

### 1. 启动基础设施

```bash
docker compose up -d mysql
```

### 2. 启动后端服务

```bash
# gateway
cd apps/gateway && go run .

# question-service
cd apps/question-service && go run .

# recognition-service（默认 mock provider，无需密钥即可跑通）
cd apps/recognition-service && go run .
```

> 依赖下载如遇网络问题，可设置代理：
> `export GOPROXY=https://goproxy.cn,direct GOSUMDB=off GOTOOLCHAIN=local`

### 3. 启动前端

```bash
cd web && npm install && npm run dev
```

访问 http://localhost:5173

## 数据库

默认 MySQL（可替换为 PostgreSQL / SQLite，通过 `config.yaml` 的 `database.dialect` 切换）：

```yaml
database:
  dialect: "mysql"   # mysql / postgres / sqlite
  dsn: "root:peak123456@tcp(127.0.0.1:3306)/peak?charset=utf8mb4&parseTime=True&loc=Local"
```

核心表：`users`、`questions`、`mistakes`、`images`、`categories`、`question_categories`、`recognition_tasks`。

### 本地 SQLite 调测（免 MySQL）

不想起 MySQL 时，可用 `make run-sqlite` 一键以 SQLite 空库启动全部服务，数据落在 `/tmp/peak-run/data/*.db`，与仓库隔离，且每次重启都重建为空库（自动清空上次测试数据）：

```bash
make run-sqlite                              # recognition 用 aliyun provider（需配 DASHSCOPE_API_KEY）
make run-sqlite RECOGNITION_PROVIDER=mock    # 用 mock 识别，无需 API Key（推荐日常调测）
make stop-sqlite                             # 停止服务并清空 /tmp/peak-run/data 与日志
```

服务端口：gateway `:8080`、question `:8081`、recognition `:8082`；日志见 `/tmp/peak-run/{gateway,question,recognition}.log`。

## AI 识别服务（provider 可配置切换）

识别服务通过能力级接口隔离厂商，通过 `recognition.provider` 配置切换：

- `mock`：无密钥本地跑通（默认）
- `aliyun`：阿里云（通义千问-VL 多模态 + 通义万相手写擦除）
- `zhipu`：智谱开放平台（GLM 系列多模态，OpenAI 兼容接口；手写擦除暂不支持，原样返回图片）

```yaml
recognition:
  provider: "aliyun"     # mock / aliyun / zhipu
  aliyun:
    access_key_id: ""
    access_secret: ""
    dash_key: ""
    dash_model: "qwen3.8-flash"
  zhipu:
    api_key: ""                      # 或环境变量 ZHIPU_API_KEY / GLM_API_KEY
    model: "glm-5.3-flash"           # 视觉多模态模型
    endpoint: ""                     # 留空使用 https://open.bigmodel.cn/api/paas/v4/chat/completions
    max_tokens: 8192                 # GLM 默认值偏小会截断长题干转录
    reasoning_effort: "low"          # 思考强度 low/high/max；off 表示不下发该参数
```

> 说明：`glm-5.3-flash` 始终开启思考且无法关闭，默认思考强度下单次识别耗时会远超
> HTTP 超时，因此 `reasoning_effort` 默认取 `low`（实测 low 约 15s，默认强度 >180s）。

各厂商的 VLM 能力（整题解析 / OCR / 几何识别 / 文档解析 / 几何描述提取）共用同一套
OpenAI 兼容多模态对话客户端（`provider.ChatClient`）与能力集合（`provider.vlmCapabilities`），
新增厂商只需提供客户端配置并实现厂商特有能力（如阿里云的手写擦除）。

能力接口：`OCRProvider` / `FormulaProvider` / `ErasureProvider` / `GeometryProvider` / `DocumentProvider`。

### 几何重绘（AI 重绘）

识别服务支持对错题图片中的几何图形做 **AI 重绘**：VLM 直接给出各点坐标与图元结构
（坐标直出），由服务内置的 Go 渲染器（`apps/recognition-service/internal/geom`）
完成清洗、结构校验、文字避让与 SVG 生成，全程无数值求解、无跨语言调用。

```
几何子图 + 题干文本 ──► VLM 坐标直出 spec JSON ──► geom.Normalize/Validate
                                       ▲                    │
                                       └─ 结构问题清单回喂修正 ─┘（最多 max_attempts 轮）
                                                            ▼
                                     geom.RenderSVG（含文字避让）存 geometry/task_<id>.svg
```

- **渲染为纯 Go 实现**（`internal/geom`：手写 SVG 字符串、零第三方依赖、零 CGO），
  覆盖线段/直线/射线（`extend`）、多边形（可填充）、圆、圆弧、直角标记、角弧标记、
  等长标记（ticks）、平行标记（parallels）、点与字母标签、自由文字标注
- **文字避让**：点标签与文字标注自动避开线段、圆、弧与彼此，做到不压线、不重叠、不出画布；
  文字宽度用启发式字符类估算（CJK≈1em、ASCII≈0.55em），无需内嵌字体
- **结构校验替代残差判据**：校验 JSON 合法性、点引用完整性、坐标有限性、图元字段完备性，
  问题清单回喂 VLM 修正后重试（`geometry.max_attempts`，默认 3 轮）
- **题干独立输入**：角度/长度等已知量从题干文本读取，不采信示意图目测值
- **精度说明**：坐标直出方案不再做数值求解，图形度量以"比例协调、不违背题意"为准，
  不再保证数学精确（这是换取部署简单与链路缩短的明确取舍）
- **多子图**：一张原图含多个子图（图1/图2/图3）时，每个子图各自渲染一张独立 SVG
- **服务端 SVG 安全清洗**：渲染产物（含模型兜底 `svg` 字段）经白名单清洗后才落库，
  剥离 script/foreignObject/事件属性/外部引用，避免存储型 XSS
- 前端在识别结果中展示重绘 SVG（矢量缩放，走既有 files 端点）；
  `redraw_report.consistent` 表示全部子图是否通过结构校验

配置（`apps/recognition-service/config.yaml`）：

```yaml
geometry:
  enabled: true      # 启用内置几何重绘（默认 false）
  max_attempts: 3    # 结构校验失败回喂修正最大轮数
```

### 文档识别（word/pdf）

识别服务支持上传 `.docx` / `.pdf` 文档，自动提取文本与内嵌图片，并按题号拆分多道题：

- 文档解析为纯 Go 实现（无 cgo、无外部命令依赖），位于 `apps/recognition-service/internal/docparse`：
  - `.docx`：解 zip 读取 `word/document.xml` 段落文本 + `word/media/` 内嵌图片
  - `.pdf`：基于 `ledongthuc/pdf` 逐页提取文本
- 文档中的图片项交由 provider 的 OCR 能力处理（aliyun 下走通义千问-VL）
- 识别结果以 `questions` 数组返回多道题，前端可逐题确认后保存
- 上传入口：`POST /api/recognition/tasks` 的 `document` 字段（图片走 `image` 字段）
- 暂不支持旧版 `.doc`（需先转为 `.docx`）

## 文件存储（本地 → S3 兼容对象存储平滑迁移）

业务层依赖 `storage.FileStorage` 接口，通过配置切换实现：

- `LocalStorage`：本地磁盘（第一迭代默认）
- `S3Storage`：基于 AWS S3 SDK 的对象存储，通过 `Endpoint` 适配多种 S3 兼容存储：
  - **阿里云 OSS**：`Endpoint` 指向 OSS 的 S3 兼容端点
  - **AWS S3 / Ceph RGW / MinIO**：`Endpoint` 指向对应服务（MinIO 需 `PathStyle=true`）

> `S3Storage` 已通过 AWS SDK Go v2 统一实现，后续迁移到阿里云 OSS、AWS S3、MinIO 等只需修改 `Endpoint` 与 `PathStyle` 配置，无需改动代码。

### 双桶隔离与跨桶提交拷贝

recognition-service 与 question-service 各自使用一个专属桶：

- **recognition 桶**（`S3_RECOGNITION_BUCKET`，默认 `peak-recognition`）：存放 `transient/` 临时区（识别原图、重绘 SVG），由对象存储生命周期规则定期清理；
- **question 桶**（`S3_QUESTION_BUCKET`，默认 `peak-question`）：存放 `committed/` 正式区（已提交错题的配图），导出/文件访问只读本桶。

提交错题时（`transient/` → `committed/`），由 `storage.Copier` 完成跨桶拷贝：
S3 后端走对象存储**服务端 CopyObject**（数据不经过应用进程，需同一 Endpoint 下
凭证可读源桶）；本地磁盘后端回退为跨目录复制（源目录 `STORAGE_SOURCE_ROOT`）。

## 错题导出（PDF / Word）

错题列表页支持勾选导出，未勾选时导出当前筛选（学科 + 关键词）结果。

- **接口**：`POST /api/mistakes/export`，请求体 `{"ids":[1,2,3],"format":"pdf"}`（或 `docx`）；按 `X-User-Id` 过滤归属，直接返回文件流（`Content-Disposition` 带 UTF-8 文件名）。
- **PDF**：每题先渲染为位图再按 A4 排布（截图式排版，打印效果与页面一致），页面超出自动分页，单题过高时等比缩放避免跨页断裂。
- **Word**：手写最小 OOXML 生成 `.docx`，标题、元信息与题干为可编辑文本，配图以图片形式内嵌。
- **配图**：经 recognition-service 的 `GET /api/recognition/files/*key` 拉取；几何重绘产出的 SVG 在本地光栅化为 PNG；单张配图失败只跳过该图，不影响整次导出。
- **中文字体**：随二进制 `go:embed` 打包（Noto Sans SC 子集，覆盖 GB2312 与常用符号），容器内无需安装系统字体。

配置（`apps/question-service/config.yaml`）：

```yaml
recognition:
  base_url: "http://localhost:8082"   # 图片服务地址（可用 RECOGNITION_SERVICE_URL 覆盖）
export:
  max_items: 200                      # 单次导出题数上限
  max_image_width: 1200               # 配图最大像素宽度，0 表示不限制
  fetch_timeout: 10s                  # 单张配图拉取超时
  font_path: ""                       # 外部字体路径，留空使用内置字体
```

## 可观测性

三支柱（Metrics / Logs / Traces）统一 Trace ID 贯通：**指标报警 → 点击 trace_id 跳 Tempo 链路 → 下钻 Loki 日志**。

### 统一 Trace ID（前后端打通）

- 前端 axios 拦截器为每个 API 请求注入 W3C `traceparent` 头；网关反代透传（并兼容旧的 `X-Trace-Id`）
- 后端各服务经 OTel 中间件创建入口 span，日志（zap JSON）与指标维度自动携带同一 `trace_id`
- 异步识别任务经 `context.WithoutCancel` 继承触发请求的 trace，任务链路可在 Tempo 中串联

### 指标（Prometheus，各服务 `/metrics`）

| 指标 | 说明 |
| --- | --- |
| `http_requests_total` / `http_request_duration_seconds` | QPS 与延迟分位（P50/P95/P99） |
| `go_gc_duration_seconds` / `go_goroutines` / `go_memstats_*` | Go 运行时：GC 频率/耗时、goroutine、堆内存 |
| `db_connections_*` / `db_wait_count_total` | DB 连接池与等待（池饱和告警） |
| `recognition_task_total` / `recognition_task_duration_seconds` | 识别任务成败率与端到端耗时 |
| `ai_call_total` / `ai_call_duration_seconds` | 第三方 AI（VLM）各能力调用的失败率与耗时 |
| `mistake_ops_total` | 错题创建/导出业务量 |
| `frontend_report_total` | 前端事件（PV/错误/Web Vitals） |

### 链路（OpenTelemetry → Tempo）

- HTTP 入口 span（含路由模板、状态码）、GORM SQL span（`db.statement` 参数化 SQL）、AI provider client span（`ai.<operation>`）
- 配置 `tracing.endpoint`（如 `tempo:4317`）启用；留空自动 no-op，本地开发零开销

### 日志（zap JSON → Alloy → Loki）

- 容器 stdout JSON 日志由 Alloy 采集入 Loki（`container`/`job` 标签），支持按 `trace_id` 检索
- 慢查询（`database.slow_query_threshold`，默认 200ms）以 Warn 级落日志

### 前端监控（自建轻量 SDK，`web/src/monitor`）

- **性能**：LCP / FID / CLS / TTFB（官方 `web-vitals`）
- **错误**：全局 JS 错误、Promise 未处理拒绝、静态资源加载失败（捕获阶段监听）
- **行为**：PV/UV（session 去重）、路由切换耗时、页面停留时长
- **上报**：批量（20 条或 5s）+ 采样（错误 100%、行为 10%）+ `sendBeacon`/`fetch keepalive`，页面关闭兜底 flush；端点 `POST /api/monitor/report`（网关公开端点）

### 存储与可视化（Grafana 全家桶，生产编排内置）

| 组件 | 选型理由 |
| --- | --- |
| Prometheus | 事实标准，`libs/observability` 原生对接，零迁移成本（备选 VictoriaMetrics 写入压缩更优但需换生态） |
| Loki + Alloy | 只索引标签、单机成本低、与 Tempo trace-to-logs 原生互通（备选 ELK 全文检索强但资源占用高 3-5 倍） |
| Tempo | OTLP 原生接收、按 TraceID 检索与本项目查询模式匹配（备选 Jaeger UI 成熟但存储依赖 ES/Cassandra） |
| Grafana + Alertmanager | 统一入口 + 告警分组通知 |

- **预置看板**（Grafana `:3000`，provisioning 自动导入）：全局总览、Go 运行时、数据库连接池、识别业务、前端监控
- **告警规则**（`deploy/prometheus/alerts.yml`）：服务宕机、5xx 率、P99 延迟、识别失败率、AI 错误率、DB 池等待、前端错误突增；通知渠道在 `deploy/alertmanager/alertmanager.yml` 配置 webhook

## 生产部署

业务镜像由 CI 自动构建并推送到**阿里云 ACR**（`docker-push` job，见下文 CI 章节），
生产编排 `docker-compose.prod.yml` 直接引用镜像仓库中的镜像，部署机**无需源码和构建环境**。

```bash
# 方式一：一键远程部署（在能 SSH 到目标主机的机器上执行）
cp deploy/.env.production.example deploy/.env.production   # 填入真实配置
./deploy/deploy-to-host.sh root@<host>        # 生产模式
./deploy/deploy-to-host.sh -d root@<host>     # 开发调试模式（叠加 dev.yml，
                                              # 开启超级验证码 000000 与 debug_code）

# 方式二：手动部署（在部署机上，仓库中的 docker-compose.prod.yml + deploy/prometheus.yml）
docker compose --env-file .env.production -f docker-compose.prod.yml pull
docker compose --env-file .env.production -f docker-compose.prod.yml up -d
```

- 镜像版本由 `.env.production` 的 `IMAGE_TAG` 控制（`latest` 或 commit sha，回滚即改此值重新 pull + up）
- 私有镜像仓库需先在部署机 `docker login`（见 `deploy/deploy.env.example`）
- 环境变量模板与逐项说明见 `deploy/.env.production.example`

### 识别服务存储后端（local / s3）

recognition-service（错题原图、重绘 SVG 等文件的存储位置）支持两种存储后端，
由 `STORAGE_TYPE` 环境变量切换，默认 `local`：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `STORAGE_TYPE` | `local` | 存储类型：`local`（本地磁盘）/ `s3`（S3 兼容对象存储） |
| `STORAGE_ROOT` | `./data` | local 后端根目录（STORAGE_TYPE=local 回退时使用） |
| `S3_ENDPOINT` | —（s3 时必填） | 第三方 S3 兼容服务地址，需含协议前缀（阿里云 OSS / AWS S3 / Ceph 等） |
| `S3_REGION` | `us-east-1` | 区域（OSS 用实际区域，如 cn-hangzhou） |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | —（s3 时必填） | 访问凭证；为空时回退 SDK 默认链（实例角色等） |
| `S3_BUCKET` | `peak-recognition` | 桶名（需已在第三方服务上创建；生产编排取 `S3_RECOGNITION_BUCKET`） |
| `S3_USE_SSL` | `false` | 是否 HTTPS（endpoint 已含协议前缀时可省略） |
| `S3_PATH_STYLE` | `false` | AWS S3/OSS 用虚拟主机风格；Ceph/MinIO 等自建服务置 `true` |

双桶相关（question-service 侧）：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `S3_QUESTION_BUCKET` | `peak-question` | question-service 专属桶（`committed/` 正式区） |
| `S3_RECOGNITION_BUCKET` | `peak-recognition` | recognition-service 专属桶（`transient/` 临时区），提交拷贝的源 |
| `S3_SOURCE_BUCKET` | `peak-recognition` | question-service 配置的源桶名，须与 `S3_RECOGNITION_BUCKET` 一致 |
| `STORAGE_SOURCE_ROOT` | `./data` | local 后端回退复制的源目录（recognition-service 的数据目录） |

约定：

- **本地调试**：不注入任何变量，默认 local（`./data`），零配置可跑。
- **Docker/K8s 部署**：使用**外部第三方 S3 兼容对象存储**（不在编排内自建 MinIO），
  通过 `.env` 或部署环境注入 `S3_ENDPOINT`/`S3_RECOGNITION_BUCKET`/`S3_QUESTION_BUCKET`/
  `S3_ACCESS_KEY`/`S3_SECRET_KEY`；生产编排缺少必填变量会在启动前直接报错，避免误用本地盘。
- **跨桶权限**：question-service 需能对 recognition 桶执行读取（CopyObject 源），
  两桶须在同一 Endpoint 下（同一账号或已授权跨账号读取）。

详细部署流程（配置注入、敏感信息管理、健康检查、回滚、前端部署）见 [`deploy/README.md`](deploy/README.md)。


## 测试

推荐使用 `make` 命令统一执行前后端测试：

```bash
make test        # 前后端单元测试（含覆盖率）
make ci          # 静态检查 + lint + 编译 + 测试 + 覆盖率门禁（等价 CI）
```

也可分别执行：

```bash
# 后端（Go workspace）
go test -race -coverprofile=coverage.out -covermode=atomic peak/...

# 前端（Vitest，含覆盖率）
cd web && npm run test:cov
```

测试覆盖：

- **后端**：错误码、存储接口、题目/错题服务（SQLite 集成）、识别任务状态机（MockProvider）、文档解析（docx/pdf 提取与多题拆分）
- **前端**：API 封装层、路由、导航组件、错题录入/列表交互（Vitest + @vue/test-utils）

## 持续集成（CI）

`.github/workflows/ci.yml` 定义了五类 job，`push` 到 `main` 或提交 PR 时自动触发：

1. **Build**：Go 全模块编译
2. **Static Analysis**：`go vet` + `golangci-lint`（v1.64，逐模块）
3. **Unit Test**：Go 测试 + 覆盖率门禁（总体 ≥70%、逐包阈值）
4. **Web Unit Test**：前端类型检查 + Vitest 测试 + 覆盖率门禁（总体 ≥80%）
5. **Docker Image Build & Push**：构建 5 个业务镜像（gateway / question / recognition / user / web）；
   `push` 到 `main` 时推送到阿里云 ACR（tag 同时打 commit sha 与 `latest`），PR 事件只构建验证不推送。
   推送需要仓库 Secrets：`ALIYUN_ACR_USERNAME` / `ALIYUN_ACR_PASSWORD`

本地执行 `make ci` 即可得到与 CI 一致的结果。
