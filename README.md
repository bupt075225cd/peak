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
│   ├── recognition-service/ # 识别服务 (:8082)
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

- **Go 1.21+**（workspace 模式，`go.work` 已就绪）
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
| `make build` | 编译：Go 全部模块 + 前端构建 | 确认可编译 |
| `make test` | 单元测试：前后端（含覆盖率） | 确认逻辑正确 |
| `make ci` | 一键全跑 = check + build + test | **提交前执行**，等价 CI |
| `make watch` | 监听 `.go/.ts/.vue`，保存即自动 check | 开发中持续反馈 |
| `make install-tools` | 安装辅助工具 | 首次环境准备 |

```bash
# 开发中（可选，开着自动检查）
make watch

# 改完一批代码
make check

# 提交前全量验证（和 CI 结果一致）
make ci
```

### 4. 提交与静态检查（git hook）

已通过 **husky** 配置 `pre-commit` hook，每次 `git commit` 自动执行 `make check`：

- 静态检查失败会**阻断提交**
- 紧急跳过（不推荐）：`git commit --no-verify`
- hook 脚本位于 `.husky/pre-commit`，随代码提交，全团队一致

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

## AI 识别服务（provider 可配置切换）

识别服务通过能力级接口隔离厂商，通过 `recognition.provider` 配置切换：

- `mock`：无密钥本地跑通（默认）
- `aliyun`：阿里云（通用 OCR + 公式识别 + 通义千问-VL）

```yaml
recognition:
  provider: "aliyun"
  aliyun:
    access_key_id: ""
    access_secret: ""
    dash_key: ""
    dash_model: "qwen-vl-max"
```

能力接口：`OCRProvider` / `FormulaProvider` / `ErasureProvider` / `GeometryProvider`。

## 文件存储（本地 → S3 兼容对象存储平滑迁移）

业务层依赖 `storage.FileStorage` 接口，通过配置切换实现：

- `LocalStorage`：本地磁盘（第一迭代默认）
- `S3Storage`：基于 AWS S3 SDK 的对象存储，通过 `Endpoint` 适配多种 S3 兼容存储：
  - **阿里云 OSS**：`Endpoint` 指向 OSS 的 S3 兼容端点
  - **AWS S3 / Ceph RGW / MinIO**：`Endpoint` 指向对应服务（MinIO 需 `PathStyle=true`）

> `S3Storage` 已通过 AWS SDK Go v2 统一实现，后续迁移到阿里云 OSS、AWS S3、MinIO 等只需修改 `Endpoint` 与 `PathStyle` 配置，无需改动代码。

## 可观测性

- **日志**：zap 结构化日志，统一 traceID
- **指标**：Prometheus，各服务暴露 `/metrics`
- **追踪**：OpenTelemetry，配置 `tracing.endpoint` 启用

## 生产部署

各服务提供多阶段 Dockerfile，通过 Docker Compose 编排部署，配置经环境变量注入。

```bash
docker compose -f docker-compose.prod.yml up -d --build
```

详细部署流程（配置注入、敏感信息管理、健康检查、回滚、前端部署）见 [`deploy/README.md`](deploy/README.md)。

## 测试

推荐使用 `make` 命令统一执行前后端测试：

```bash
make test        # 前后端单元测试（含覆盖率）
make ci          # 静态检查 + 编译 + 测试（等价 CI）
```

也可分别执行：

```bash
# 后端（Go workspace）
go test -race -coverprofile=coverage.out -covermode=atomic peak/...

# 前端（Vitest，含覆盖率）
cd web && npm run test:cov
```

测试覆盖：

- **后端**：错误码、存储接口、题目/错题服务（SQLite 集成）、识别任务状态机（MockProvider）
- **前端**：API 封装层、路由、导航组件、错题录入/列表交互（Vitest + @vue/test-utils）

## 持续集成（CI）

`.github/workflows/ci.yml` 定义了四类 job，`push` 到 `main` 或提交 PR 时自动触发：

1. **Build**：Go 全模块编译
2. **Static Analysis**：`go vet` + `golangci-lint`
3. **Unit Test**：Go 测试 + 覆盖率门禁（总体 ≥70%、逐包阈值）
4. **Web Unit Test**：前端类型检查 + Vitest 测试 + 覆盖率门禁（总体 ≥80%）

本地执行 `make ci` 即可得到与 CI 一致的结果。
