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

## 一键全跑：静态检查 + 编译 + 测试（提交前执行）
ci: check build test
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

.PHONY: check build test ci watch install-tools
