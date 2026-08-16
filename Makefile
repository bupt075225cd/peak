# Peak 本地开发统一入口
# 用法：
#   make check        静态检查（Go vet + 前端类型检查）
#   make build        编译（Go 全部模块 + 前端构建）
#   make test         单元测试（前后端）
#   make ci           一键全跑（等价 CI，提交前跑这个）
#   make watch        文件监听，保存即自动 check（开发中）
#   make install-tools 一键安装辅助工具（watchexec / air）

SHELL := /bin/bash
.SILENT:

# Go 网络/工具链配置，与 CI 及 README 保持一致
export GOPROXY := https://goproxy.cn,direct
export GOSUMDB := off
export GOTOOLCHAIN := local

# 前端目录
WEB := web

# 本地工具安装目录（加入 PATH 后无需 root）
GOBIN ?= $(shell go env GOPATH)/bin

# 本地 SQLite 调测配置
PEAK_RUN ?= /tmp/peak-run
PEAK_BIN := $(PEAK_RUN)/bin
PEAK_DATA := $(PEAK_RUN)/data
QUESTION_DIR := $(CURDIR)/apps/question-service
RECOGNITION_DIR := $(CURDIR)/apps/recognition-service
GATEWAY_DIR := $(CURDIR)/apps/gateway
# recognition-service 默认用 aliyun provider，可覆盖：make run-sqlite RECOGNITION_PROVIDER=mock
RECOGNITION_PROVIDER ?= aliyun

## 静态检查：Go vet + 前端 vue-tsc 类型检查
check:
	@echo "==> Go vet"
	go vet peak/...
	@echo "==> 前端类型检查 (vue-tsc)"
	cd $(WEB) && npm run typecheck
	@echo "✓ 静态检查通过"

## 编译：Go 全部模块 + 前端构建
build:
	@echo "==> Go build (workspace)"
	go build peak/...
	@echo "==> 前端构建 (vue-tsc + vite)"
	cd $(WEB) && npm run build
	@echo "✓ 编译通过"

## 单元测试：Go（含覆盖率）+ 前端（含覆盖率）
test:
	@echo "==> Go 测试"
	go test -race -coverprofile=/tmp/peak-coverage.out -covermode=atomic peak/...
	@echo "==> 前端测试"
	cd $(WEB) && npm run test:cov
	@echo "✓ 测试通过"

## 覆盖率门禁：与远端 CI 一致（Go 70% + 逐包阈值、前端 80%）
coverage-gate:
	bash scripts/coverage-gate.sh

## 提交前门禁：静态检查 + 测试 + 覆盖率门禁（等价 CI，不含 build）
precommit: check test coverage-gate
	@echo "✓ 提交前检查通过"

## 一键全跑：静态检查 + 编译 + 测试 + 覆盖率门禁（推送前执行，等价远端 CI）
ci: check build test coverage-gate
	@echo "✓ 全部通过，可以提交"

## 一键安装辅助工具：watchexec（文件监听）+ air（Go 热重载）
install-tools:
	@echo "==> 安装 watchexec（文件监听）"
	@if command -v watchexec >/dev/null 2>&1; then \
		echo "  watchexec 已安装: $(command -v watchexec)"; \
	elif command -v brew >/dev/null 2>&1; then \
		echo "  通过 Homebrew 安装..."; brew install watchexec; \
	elif command -v cargo >/dev/null 2>&1; then \
		echo "  通过 cargo 安装..."; cargo install watchexec-cli; \
	else \
		echo "  ✗ 未找到 brew/cargo，请手动安装 watchexec"; \
		echo "    详见 https://github.com/watchexec/watchexec"; \
	fi
	@echo "==> 安装 air（Go 热重载，可选）"
	@if command -v air >/dev/null 2>&1; then \
		echo "  air 已安装: $(command -v air)"; \
	elif command -v go >/dev/null 2>&1; then \
		echo "  通过 go install 安装到 $(GOBIN)..."; \
		go install github.com/air-verse/air@latest; \
		echo "  请确保 $(GOBIN) 在 PATH 中"; \
	else \
		echo "  ✗ 未找到 go，跳过 air 安装"; \
	fi
	@echo "✓ 工具安装完成"

## 文件监听：保存即自动静态检查（开发中）
watch:
	@if ! command -v watchexec >/dev/null 2>&1; then \
		echo "✗ 未检测到 watchexec，请先执行: make install-tools"; \
		exit 1; \
	fi
	@command -v air >/dev/null 2>&1 && echo "检测到 air，可用于 Go 热重载" || true
	watchexec --restart --exts go,ts,vue -- "make check"

# 停止记录的调测进程（按 .pid 文件，不误杀编译进程）
define stop-sqlite-proc
	@for f in question recognition gateway; do \
		pidf=$(PEAK_RUN)/$$f.pid; \
		if [ -f "$$pidf" ]; then \
			pid=$$(cat "$$pidf" 2>/dev/null); \
			if [ -n "$$pid" ] && kill -0 "$$pid" 2>/dev/null; then kill "$$pid" 2>/dev/null && echo "  停止 $$f (PID $$pid)"; fi; \
			rm -f "$$pidf"; \
		fi; \
	done
endef

## 本地 SQLite 调测：编译并以 sqlite 空库启动全部服务（gateway/question/recognition）
## 用法：
##   make run-sqlite              默认启动（recognition 用 aliyun provider）
##   make run-sqlite RECOGNITION_PROVIDER=mock   用 mock 识别，无需 API Key
##   make stop-sqlite             停止全部服务并清空数据
run-sqlite:
	@mkdir -p $(PEAK_BIN) $(PEAK_DATA)
	$(stop-sqlite-proc)
	@sleep 1
	@echo "==> 编译服务"
	go build -o $(PEAK_BIN)/gateway ./apps/gateway
	go build -o $(PEAK_BIN)/question-service ./apps/question-service
	go build -o $(PEAK_BIN)/recognition-service ./apps/recognition-service
	@echo "==> 启动服务（sqlite 空库）"
	cd $(QUESTION_DIR) && exec env DB_DIALECT=sqlite DB_DSN=$(PEAK_DATA)/question.db nohup $(PEAK_BIN)/question-service > $(PEAK_RUN)/question.log 2>&1 & echo $$! > $(PEAK_RUN)/question.pid
	cd $(RECOGNITION_DIR) && exec env DB_DIALECT=sqlite DB_DSN=$(PEAK_DATA)/recognition.db RECOGNITION_PROVIDER=$(RECOGNITION_PROVIDER) nohup $(PEAK_BIN)/recognition-service > $(PEAK_RUN)/recognition.log 2>&1 & echo $$! > $(PEAK_RUN)/recognition.pid
	cd $(GATEWAY_DIR) && exec nohup $(PEAK_BIN)/gateway > $(PEAK_RUN)/gateway.log 2>&1 & echo $$! > $(PEAK_RUN)/gateway.pid
	@sleep 2
	@echo "✓ 已启动: gateway=:8080  question=:8081  recognition=:8082"
	@echo "  日志: $(PEAK_RUN)/{gateway,question,recognition}.log"
	@echo "  数据库(空库): $(PEAK_DATA)/{question,recognition}.db"

## 停止本地 SQLite 调测服务并清空数据目录
stop-sqlite:
	@echo "==> 停止服务"
	$(stop-sqlite-proc)
	@sleep 1
	@rm -rf $(PEAK_DATA)
	@rm -f $(PEAK_RUN)/question.log $(PEAK_RUN)/recognition.log $(PEAK_RUN)/gateway.log
	@echo "✓ 已停止并清空 $(PEAK_RUN)/data 与日志"

.PHONY: check build test coverage-gate precommit ci watch install-tools run-sqlite stop-sqlite
