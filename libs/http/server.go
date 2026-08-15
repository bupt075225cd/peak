package http

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"peak/libs/logger"
)

// Server 封装 Gin 服务与优雅启停。
type Server struct {
	engine *gin.Engine
	log    *logger.Logger
}

// NewServer 创建服务实例，注入通用中间件。
func NewServer(log *logger.Logger, dev bool) *Server {
	if dev {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	engine.Use(Recover(log), TraceID(), AccessLog(log))
	return &Server{engine: engine, log: log}
}

// Engine 返回底层 Gin engine，供各服务注册路由。
func (s *Server) Engine() *gin.Engine {
	return s.engine
}

// Run 启动服务，监听系统信号实现优雅关闭。
func (s *Server) Run(addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s.engine,
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("server started", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-quit:
		s.log.Info("shutting down", zap.String("signal", sig.String()))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}
