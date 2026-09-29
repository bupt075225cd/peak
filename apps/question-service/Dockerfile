# syntax=docker/dockerfile:1
# 构建阶段：Go workspace 多模块，需在仓库根目录构建
FROM golang:1.24-alpine AS builder

ENV GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct \
    GOSUMDB=off \
    GOTOOLCHAIN=local \
    CGO_ENABLED=0

WORKDIR /build

COPY go.work go.work.sum ./
COPY libs/ ./libs/
COPY apps/ ./apps/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd apps/question-service && go build -ldflags="-s -w" -o /out/question-service .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 app

USER app

COPY --from=builder /out/question-service /app/question-service
COPY apps/question-service/config.yaml /app/config.yaml

WORKDIR /app

EXPOSE 8081

ENTRYPOINT ["/app/question-service"]
