package service

import (
	"context"
	"os"
	"testing"
	"time"

	"peak/apps/recognition-service/internal/provider"
)

// TestMakeHiResCrop 生成 1600 宽的裁剪图，供 provider 包测试用。
func TestMakeHiResCrop(t *testing.T) {
	if os.Getenv("GEOM_MANUAL") == "" {
		t.Skip("manual only")
	}
	raw, err := os.ReadFile("/home/git/test-data/math/微信图片_20261031164900_36_176.jpg")
	if err != nil {
		t.Fatal(err)
	}
	img, _ := normalizeImageOrientation(raw)
	prov := provider.NewZhipuProvider(provider.ZhipuConfig{APIKey: os.Getenv("ZHIPU_API_KEY")})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	geo, gerr := prov.RecognizeGeometry(ctx, img)
	if gerr != nil || geo.BoundingBox == nil {
		t.Fatalf("recognize: %v", gerr)
	}
	hi, plan, cerr := cropAndScaleRegionPlanned(img, geo.BoundingBox, 1600)
	if cerr != nil {
		t.Fatalf("crop: %v", cerr)
	}
	t.Logf("plan=%s bytes=%d", plan, len(hi))
	if err := os.WriteFile("/tmp/g36_hi.jpg", hi, 0o644); err != nil {
		t.Fatal(err)
	}
}
