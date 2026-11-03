package service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"peak/apps/recognition-service/internal/provider"
)

// newWhiteImage 返回白底画布，模拟试卷纸面。
func newWhiteImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	return img
}

// drawRectOutline 画一个黑色矩形边框（模拟几何图形线条）。
func drawRectOutline(img *image.RGBA, r image.Rectangle, thick int) {
	dark := color.RGBA{R: 20, G: 20, B: 20, A: 255}
	for t := 0; t < thick; t++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.Set(x, r.Min.Y+t, dark)
			img.Set(x, r.Max.Y-1-t, dark)
		}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			img.Set(r.Min.X+t, y, dark)
			img.Set(r.Max.X-1-t, y, dark)
		}
	}
}

func encodeTestJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCropAndScaleRegionDownscales(t *testing.T) {
	// 白底 + 黑色矩形边框，模拟"只有图形"的页面。
	img := newWhiteImage(3000, 2000)
	drawRectOutline(img, image.Rect(600, 200, 2100, 1000), 6)
	bbox := &provider.BoundingBox{X: 0.2, Y: 0.1, Width: 0.5, Height: 0.4}
	out, err := cropAndScaleRegion(encodeTestJPEG(t, img), bbox, 960)
	if err != nil {
		t.Fatalf("crop: %v", err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode out: %v", err)
	}
	w, h := decoded.Bounds().Dx(), decoded.Bounds().Dy()
	if w > 960 || h > 960 {
		t.Fatalf("not downscaled: %dx%d", w, h)
	}
	// 图形 1500×800，按连通域并集微调后等比缩放，约 960×512。
	if w < 900 || w > 960 || h < 470 || h > 550 {
		t.Fatalf("unexpected size: %dx%d", w, h)
	}
}

func TestCropAndScaleRegionKeepsSmall(t *testing.T) {
	img := newWhiteImage(200, 120)
	drawRectOutline(img, image.Rect(20, 20, 180, 100), 3)
	bbox := &provider.BoundingBox{X: 0, Y: 0, Width: 1, Height: 1}
	out, err := cropAndScaleRegion(encodeTestJPEG(t, img), bbox, 960)
	if err != nil {
		t.Fatalf("crop: %v", err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	w, h := decoded.Bounds().Dx(), decoded.Bounds().Dy()
	// 区域小于 maxDim 时不放大；按墨迹并集收紧后约 170×86。
	if w > 200 || h > 120 {
		t.Fatalf("should not upscale: %dx%d", w, h)
	}
	if w < 150 || h < 70 {
		t.Fatalf("figure cropped too tight: %dx%d", w, h)
	}
}

func TestCropAndScaleRegionInvalid(t *testing.T) {
	src := encodeTestJPEG(t, newWhiteImage(100, 100))
	if _, err := cropAndScaleRegion(src, nil, 960); err == nil {
		t.Fatal("expected error for nil bbox")
	}
	if _, err := cropAndScaleRegion([]byte("not-an-image"), &provider.BoundingBox{X: 0, Y: 0, Width: 1, Height: 1}, 960); err == nil {
		t.Fatal("expected error for bad image")
	}
}

func TestPlanGeometryRegionCompletesCutFigure(t *testing.T) {
	// 模型框只框住图形左半部分：应沿连通域把被切断的右边补回来，
	// 同时不能把旁边独立的小块并进来。
	img := newWhiteImage(400, 300)
	drawRectOutline(img, image.Rect(100, 80, 250, 200), 4)
	drawRectOutline(img, image.Rect(10, 10, 50, 30), 4)

	bbox := &provider.BoundingBox{X: 0.25, Y: 0.26, Width: 0.13, Height: 0.40}
	rect, note := planGeometryRegion(img, bbox)
	if note != "expand" {
		t.Fatalf("expected expand, got %q (rect=%v)", note, rect)
	}
	if rect.Max.X < 250 || rect.Max.Y < 200 {
		t.Fatalf("cut figure not completed: %v", rect)
	}
	if rect.Min.X < 60 {
		t.Fatalf("unrelated blob should be excluded: %v", rect)
	}
}

func TestPlanGeometryRegionBlankBBoxFallsBackFull(t *testing.T) {
	img := newWhiteImage(200, 160)
	drawRectOutline(img, image.Rect(20, 20, 80, 60), 3)
	// 框落在空白区域：模型框不可信，回退整图。
	bbox := &provider.BoundingBox{X: 0.6, Y: 0.6, Width: 0.2, Height: 0.2}
	rect, note := planGeometryRegion(img, bbox)
	if note != "full" {
		t.Fatalf("expected full fallback, got %q (rect=%v)", note, rect)
	}
	if rect != img.Bounds() {
		t.Fatalf("expected full bounds %v, got %v", img.Bounds(), rect)
	}
}
