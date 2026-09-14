package service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"peak/apps/recognition-service/internal/provider"
)

func makeJPEGBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8((x * 7) % 256), G: uint8((y * 3) % 256), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCropAndScaleRegionDownscales(t *testing.T) {
	src := makeJPEGBytes(t, 3000, 2000)
	bbox := &provider.BoundingBox{X: 0.2, Y: 0.1, Width: 0.5, Height: 0.4}
	out, err := cropAndScaleRegion(src, bbox, 960)
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
	// 区域像素 1500×800，等比缩放后约为 960×512。
	if w < 920 || w > 960 || h < 490 || h > 530 {
		t.Fatalf("unexpected size: %dx%d", w, h)
	}
}

func TestCropAndScaleRegionKeepsSmall(t *testing.T) {
	src := makeJPEGBytes(t, 200, 120)
	bbox := &provider.BoundingBox{X: 0, Y: 0, Width: 1, Height: 1}
	out, err := cropAndScaleRegion(src, bbox, 960)
	if err != nil {
		t.Fatalf("crop: %v", err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := decoded.Bounds().Dx(); got != 200 {
		t.Fatalf("expected keep size 200, got %d", got)
	}
}

func TestCropAndScaleRegionInvalid(t *testing.T) {
	src := makeJPEGBytes(t, 100, 100)
	if _, err := cropAndScaleRegion(src, nil, 960); err == nil {
		t.Fatal("expected error for nil bbox")
	}
	if _, err := cropAndScaleRegion([]byte("not-an-image"), &provider.BoundingBox{X: 0, Y: 0, Width: 1, Height: 1}, 960); err == nil {
		t.Fatal("expected error for bad image")
	}
}
