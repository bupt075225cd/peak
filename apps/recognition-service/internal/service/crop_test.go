package service

import (
	"bytes"
	"image"
	"testing"

	"peak/apps/recognition-service/internal/provider"
)

func TestCropGeometryImage(t *testing.T) {
	original := makeTestJPEG(t, 200, 100)

	// 左上角 1/4 区域：x=0, y=0, w=0.5, h=0.5，外扩 10% 后被 clamp 到 0~1，
	// 实际裁剪区域约为 0.7 * 原图大小 -> 140x70。
	cropped, err := cropGeometryImage(original, &provider.BoundingBox{X: 0, Y: 0, Width: 0.5, Height: 0.5})
	if err != nil {
		t.Fatalf("crop: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(cropped))
	if err != nil {
		t.Fatalf("decode cropped: %v", err)
	}
	if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w != 140 || h != 70 {
		t.Fatalf("unexpected crop size: %dx%d (expected 140x70 after 10%% padding)", w, h)
	}
}

func TestCropGeometryImageNilBBox(t *testing.T) {
	if _, err := cropGeometryImage(makeTestJPEG(t, 10, 10), nil); err == nil {
		t.Fatal("expected error for nil bbox")
	}
}

func TestCropGeometryImageInvalidData(t *testing.T) {
	if _, err := cropGeometryImage([]byte("not-an-image"), &provider.BoundingBox{X: 0, Y: 0, Width: 1, Height: 1}); err == nil {
		t.Fatal("expected error for non-image bytes")
	}
}

func TestCropGeometryImageFullBBox(t *testing.T) {
	original := makeTestJPEG(t, 80, 60)
	// 全图 bbox，外扩 10% 后仍被 clamp 到整图。
	cropped, err := cropGeometryImage(original, &provider.BoundingBox{X: 0, Y: 0, Width: 1, Height: 1})
	if err != nil {
		t.Fatalf("crop full: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(cropped))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w != 80 || h != 60 {
		t.Fatalf("unexpected size: %dx%d (expected 80x60)", w, h)
	}
}

func TestExpandBBox(t *testing.T) {
	tests := []struct {
		name string
		in   *provider.BoundingBox
		want *provider.BoundingBox
	}{
		{"nil", nil, nil},
		{"center expand", &provider.BoundingBox{X: 0.4, Y: 0.4, Width: 0.2, Height: 0.2},
			&provider.BoundingBox{X: 0.3, Y: 0.3, Width: 0.4, Height: 0.4}},
		{"left edge clamp", &provider.BoundingBox{X: 0, Y: 0, Width: 0.5, Height: 0.5},
			&provider.BoundingBox{X: 0, Y: 0, Width: 0.7, Height: 0.7}},
		{"right edge clamp", &provider.BoundingBox{X: 0.6, Y: 0.6, Width: 0.3, Height: 0.3},
			&provider.BoundingBox{X: 0.5, Y: 0.5, Width: 0.5, Height: 0.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandBBox(tt.in, 0.1)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil")
			}
			if !approxEqualFloat(got.X, tt.want.X) || !approxEqualFloat(got.Y, tt.want.Y) ||
				!approxEqualFloat(got.Width, tt.want.Width) || !approxEqualFloat(got.Height, tt.want.Height) {
				t.Fatalf("got %+v, want %+v", *got, *tt.want)
			}
		})
	}
}

// approxEqualFloat 浮点近似比较，容忍 1e-9 误差。
func approxEqualFloat(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}
