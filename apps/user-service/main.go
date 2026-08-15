// user-service 是用户服务（预留），负责认证、用户、班级/年级信息。
// 第一迭代仅提供最小骨架与健康检查，完整逻辑后续迭代实现。
package main

import (
	"os"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"peak/libs/config"
	httpx "peak/libs/http"
	"peak/libs/logger"
)

func main() {
	cfgPath := os.Getenv("USER_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		panic(err)
	}

	appLog := logger.New(cfg.Bool("log.development", true))
	server := httpx.NewServer(appLog, cfg.Bool("log.development", true))

	engine := server.Engine()
	engine.GET("/healthz", func(c *gin.Context) {
		httpx.OK(c, gin.H{"status": "ok"})
	})
	// 预留用户接口。
	engine.GET("/api/users/me", func(c *gin.Context) {
		httpx.OK(c, gin.H{"id": 1, "name": "mock-user"})
	})

	addr := ":" + cfg.String("server.port", "8083")
	if err := server.Run(addr); err != nil {
		appLog.Error("server exit", zap.String("error", err.Error()))
	}
}
