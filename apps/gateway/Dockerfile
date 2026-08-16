# 构建阶段：Go workspace 多模块，需在仓库根目录构建
FROM golang:1.24-alpine AS builder

# 国内镜像加速
ENV GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=off \
    GOTOOLCHAIN=local \
    CGO_ENABLED=0

WORKDIR /build

# 先拷贝 go.work 与模块定义，充分利用依赖缓存
COPY go.work go.work.sum ./
COPY libs/ ./libs/
COPY apps/ ./apps/

# 构建 gateway（从 workspace 根目录）
RUN cd apps/gateway && go build -ldflags="-s -w" -o /out/gateway .

# 运行阶段：精简镜像
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 app

USER app

COPY --from=builder /out/gateway /app/gateway
COPY apps/gateway/config.yaml /app/config.yaml

WORKDIR /app

EXPOSE 8080

ENTRYPOINT ["/app/gateway"]
