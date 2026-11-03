package service

import (
	"context"
	"os"
	"testing"
	"time"

	"peak/apps/recognition-service/internal/geom"
	"peak/apps/recognition-service/internal/provider"
)

func TestManualGridRepeat(t *testing.T) {
	if os.Getenv("GEOM_MANUAL") == "" {
		t.Skip("manual only")
	}
	raw, err := os.ReadFile("/home/git/test-data/math/微信图片_20261031164900_36_176.jpg")
	if err != nil {
		t.Fatal(err)
	}
	img, _ := normalizeImageOrientation(raw)
	prov := provider.NewZhipuProvider(provider.ZhipuConfig{APIKey: os.Getenv("ZHIPU_API_KEY")})
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Second)
	defer cancel()

	geo, err := prov.RecognizeGeometry(ctx, img)
	if err != nil {
		t.Fatalf("recognize: %v", err)
	}
	if geo.BoundingBox == nil {
		t.Fatal("no bbox")
	}
	bbox := geo.BoundingBox
	t.Logf("bbox=%+v tilt=%d", bbox, geo.Rotation.Degrees())

	stemText := "20. 在如图所示的小正方形网格中，A,B,C,D均为小正方形的顶点，线段AB和CD相交于点O，则∠AOC的度数为____"
	maxDim := geometryRegionMaxDim
	if isGridFigure(stemText, geo.Description) {
		maxDim = geometryGridRegionMaxDim
	}
	t.Logf("regionMaxDim=%d grid=%v", maxDim, isGridFigure(stemText, geo.Description))
	extractImage := img
	if small, _, cerr := cropAndScaleRegionPlanned(img, bbox, maxDim); cerr == nil {
		extractImage = small
		_ = os.WriteFile("/tmp/g36d_crop.jpg", small, 0o644)
	}
	if fix := geo.Rotation.FixDegrees(); fix != 0 {
		if rot, rerr := rotateImageData(extractImage, fix); rerr == nil {
			extractImage = rot
		}
	}
	stem := stemText

	for round := 1; round <= 2; round++ {
		spec, err := prov.ExtractGeometrySpec(ctx, extractImage, stem, "")
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		panels, perr := geom.ParsePanels(spec)
		if perr != nil {
			t.Fatalf("parse: %v", perr)
		}
		geom.CheckGridCoverage(panels)
		g := panels[0].Spec.Grid
		t.Logf("round %d: grid=%+v issues=%v", round, g, panels[0].Issues)
		_ = os.WriteFile("/tmp/g36c_spec_"+string(rune('0'+round))+".json", []byte(spec), 0o644)
	}
}
