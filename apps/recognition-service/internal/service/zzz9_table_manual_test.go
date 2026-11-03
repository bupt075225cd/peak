package service

import (
	"context"
	"os"
	"testing"
	"time"

	"peak/apps/recognition-service/internal/provider"
)

// TestManualTableParse 手工验证表格题目：真实图片走一遍方向转正 + ParseQuestion，
// 检查题干里是否留下 {{table1}} 占位、tables 的行列是否与图一致。仅 GEOM_MANUAL=1 时执行。
func TestManualTableParse(t *testing.T) {
	if os.Getenv("GEOM_MANUAL") == "" {
		t.Skip("manual only")
	}
	raw, err := os.ReadFile("/home/git/test-data/math/微信图片_20261031170520_42_176.jpg")
	if err != nil {
		t.Fatal(err)
	}
	img, _ := normalizeImageOrientation(raw)
	// 该图侧躺：模拟入口方向探测结果 tilt=90 → 顺时针转 270°。
	if upright, rerr := rotateImageData(img, 270); rerr == nil {
		img = upright
	}
	prov := provider.NewZhipuProvider(provider.ZhipuConfig{APIKey: os.Getenv("ZHIPU_API_KEY")})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	res, err := prov.ParseQuestion(ctx, img)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	t.Logf("stem_text = %q", res.Text)
	for i, tbl := range res.Tables {
		t.Logf("table %d id=%s headers=%v rows=%v", i+1, tbl.ID, tbl.Headers, tbl.Rows)
	}
}
