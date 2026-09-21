package export

import (
	"math"
	"testing"
)

func nearlyEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestImageDisplayWidthRatioByAspect(t *testing.T) {
	cases := []struct {
		name string
		w, h int
		want float64
	}{
		{"square", 100, 100, 0.2966666666666667},
		{"wide", 200, 100, imageMaxWidthRatio},
		{"extremely wide", 500, 50, imageMaxWidthRatio},
		{"tall", 60, 100, 0.2353333333333333},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := imageDisplayWidthRatio(tc.w, tc.h)
			if !nearlyEqual(got, tc.want) {
				t.Fatalf("ratio = %.6f, want %.6f", got, tc.want)
			}
		})
	}
}

func TestImageDisplayWidthRatioRespectsHeightLimit(t *testing.T) {
	// 极竖图受显示高度上限约束，允许低于宽度下限。
	const w, h = 20, 100
	got := imageDisplayWidthRatio(w, h)

	contentAspect := layoutContentHeightMM / layoutContentWidthMM
	heightRatio := got / (float64(w) / float64(h)) / contentAspect
	if !nearlyEqual(heightRatio, imageMaxHeightRatio) {
		t.Fatalf("height ratio = %.6f, want %.6f", heightRatio, imageMaxHeightRatio)
	}
	if got >= imageMinWidthRatio {
		t.Fatalf("ratio = %.6f, want below min %.6f for extremely tall image", got, imageMinWidthRatio)
	}
}

func TestImageDisplayWidthRatioFallsBackOnInvalidSize(t *testing.T) {
	for _, tc := range [][2]int{{0, 0}, {100, 0}, {-1, 50}} {
		if got := imageDisplayWidthRatio(tc[0], tc[1]); !nearlyEqual(got, imageDefaultWidthRatio) {
			t.Fatalf("ratio(%d,%d) = %.6f, want %.6f", tc[0], tc[1], got, imageDefaultWidthRatio)
		}
	}
}

func TestImageDisplaySizeMMKeepsAspectAndBand(t *testing.T) {
	cases := []struct {
		name string
		w, h int
	}{
		{"square", 100, 100},
		{"wide", 200, 100},
		{"tall", 60, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wMM, hMM := imageDisplaySizeMM(ImageAsset{Width: tc.w, Height: tc.h})
			if !nearlyEqual(wMM*float64(tc.h), hMM*float64(tc.w)) {
				t.Fatalf("aspect changed: %gx%g for %dx%d", wMM, hMM, tc.w, tc.h)
			}
			if wMM < imageMinWidthRatio*layoutContentWidthMM || wMM > imageMaxWidthRatio*layoutContentWidthMM {
				t.Fatalf("width %.2fmm out of band", wMM)
			}
			if hMM > imageMaxHeightRatio*layoutContentHeightMM {
				t.Fatalf("height %.2fmm exceeds limit", hMM)
			}
		})
	}
}

func TestImageDisplaySizeMMLimitsBitmapUpscale(t *testing.T) {
	asset := ImageAsset{Width: 100, Height: 100, NaturalWidth: 100, NaturalHeight: 100}
	wMM, hMM := imageDisplaySizeMM(asset)

	want := float64(100) / imageSourceDPI * mmPerInch * imageMaxUpscale
	if !nearlyEqual(wMM, want) || !nearlyEqual(hMM, want) {
		t.Fatalf("size = %.4f×%.4fmm, want %.4f×%.4f", wMM, hMM, want, want)
	}
}

func TestImageDisplaySizeMMIgnoresUpscaleForVector(t *testing.T) {
	asset := ImageAsset{Width: 100, Height: 100, NaturalWidth: 100, NaturalHeight: 100, Vector: true}
	wMM, _ := imageDisplaySizeMM(asset)

	want := imageDisplayWidthRatio(100, 100) * layoutContentWidthMM
	if !nearlyEqual(wMM, want) {
		t.Fatalf("vector width = %.4fmm, want %.4f", wMM, want)
	}
}

func TestImageDisplaySizeMMFallsBackOnInvalidSize(t *testing.T) {
	wMM, hMM := imageDisplaySizeMM(ImageAsset{})
	if !nearlyEqual(wMM, imageDefaultWidthRatio*layoutContentWidthMM) || !nearlyEqual(hMM, wMM/2) {
		t.Fatalf("fallback size = %g×%g", wMM, hMM)
	}
}
