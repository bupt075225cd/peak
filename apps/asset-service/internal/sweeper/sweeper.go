// Package sweeper 到期注销清扫：执行密码学删除。
package sweeper

import (
	"context"
	"time"

	"go.uber.org/zap"

	"peak/apps/asset-service/internal/service"
	"peak/libs/logger"
)

// Sweeper 定时清扫已过冷静期的注销申请。
type Sweeper struct {
	svc      *service.Service
	log      *logger.Logger
	interval time.Duration
}

// New 创建清扫器。
func New(svc *service.Service, log *logger.Logger, interval time.Duration) *Sweeper {
	if interval <= 0 {
		interval = time.Hour
	}
	return &Sweeper{svc: svc, log: log, interval: interval}
}

// Start 启动后台清扫（启动即扫一次，随后按间隔执行）。
func (s *Sweeper) Start(ctx context.Context) {
	go func() {
		s.runOnce(ctx)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runOnce(ctx)
			}
		}
	}()
}

func (s *Sweeper) runOnce(ctx context.Context) {
	purged, err := s.svc.PurgeDue(ctx)
	if err != nil {
		s.log.Warn("ledger deletion sweep failed", zap.String("error", err.Error()))
		return
	}
	if purged > 0 {
		// 仅记录数量，不记录任何主体标识。
		s.log.Info("ledger deletion purged", zap.Int("count", purged))
	}
}
