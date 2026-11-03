package service

import (
	"context"
	"os"
	"testing"
	"time"

	"peak/libs/logger"

	"peak/apps/recognition-service/internal/provider"
)

// TestManualOrientationRepeat 同一张侧躺图连续做"方向探测 + 正立确认"，
// 检查最终旋转角是否稳定（该图正确结果是顺时针 270°）。仅 GEOM_MANUAL=1 时执行。
func TestManualOrientationRepeat(t *testing.T) {
	if os.Getenv("GEOM_MANUAL") == "" {
		t.Skip("manual only")
	}
	raw, err := os.ReadFile("/home/git/test-data/math/微信图片_20261031170521_43_176.jpg")
	if err != nil {
		t.Fatal(err)
	}
	prov := provider.NewZhipuProvider(provider.ZhipuConfig{APIKey: os.Getenv("ZHIPU_API_KEY")})
	svc := &Service{prov: prov, log: logger.NewNop()}

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()
	for i := 1; i <= 3; i++ {
		start := time.Now()
		_, deg := svc.uprightImage(ctx, 1, raw)
		t.Logf("round %d: rotation=%d (want 270) cost=%dms", i, deg, time.Since(start).Milliseconds())
	}
}
