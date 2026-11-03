package service

import (
	"context"
	"os"
	"testing"
	"time"

	"peak/apps/recognition-service/internal/provider"
)

// TestManualFunctionGraphExtend 手工验证函数图象题：三条直线是否输出 extend:true
// （不延长就会被画成线段，与原图"贯穿坐标系"不一致）。仅 GEOM_MANUAL=1 时执行。
func TestManualFunctionGraphExtend(t *testing.T) {
	if os.Getenv("GEOM_MANUAL") == "" {
		t.Skip("manual only")
	}
	raw, err := os.ReadFile("/home/git/test-data/math/微信图片_20261031170523_45_176.jpg")
	if err != nil {
		t.Fatal(err)
	}
	img, _ := normalizeImageOrientation(raw)
	// 该图侧躺：模拟入口方向探测结果 tilt=90 → 顺时针 270°。
	if upright, rerr := rotateImageData(img, 270); rerr == nil {
		img = upright
	}
	stem := "22. 如图，在平面直角坐标系xOy中，一次函数y=nx+7的图象l1分别与x，y轴交于A，B两点，" +
		"与正比例函数y=mx的图象l2交于点C(2,6)，若一次函数y=kx+k的图象l3与直线l1、l2不能围成三角形，则k的值为____"

	prov := provider.NewZhipuProvider(provider.ZhipuConfig{APIKey: os.Getenv("ZHIPU_API_KEY")})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	spec, err := prov.ExtractGeometrySpec(ctx, img, stem, "")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	t.Logf("spec = %s", spec)
}
