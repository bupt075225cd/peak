package export

import (
	"bytes"
	"image/png"
	"testing"
)

const simpleSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="50" viewBox="0 0 100 50">` +
	`<rect x="10" y="10" width="80" height="30" fill="#336699"/></svg>`

func TestRasterizeSVGProducesPNG(t *testing.T) {
	asset, err := rasterizeSVG([]byte(simpleSVG), 0)
	if err != nil {
		t.Fatalf("rasterizeSVG: %v", err)
	}
	if asset.Format != "png" {
		t.Fatalf("format = %q, want png", asset.Format)
	}
	// 原始 SVG 仅 100x50，应放大到目标宽度以保证嵌入 PDF 后清晰。
	if asset.Width != svgRenderWidth || asset.Height != svgRenderWidth/2 {
		t.Fatalf("size = %dx%d, want %dx%d", asset.Width, asset.Height, svgRenderWidth, svgRenderWidth/2)
	}
	if _, err := png.Decode(bytes.NewReader(asset.Data)); err != nil {
		t.Fatalf("rasterized bytes are not a valid png: %v", err)
	}
}

func TestSvgTargetSize(t *testing.T) {
	// 小于目标宽度：放大并保持比例。
	if w, h := svgTargetSize(100, 50, 0); w != svgRenderWidth || h != svgRenderWidth/2 {
		t.Fatalf("size = %dx%d, want %dx%d", w, h, svgRenderWidth, svgRenderWidth/2)
	}

	// 已大于目标宽度：不做额外放大。
	if w, h := svgTargetSize(4000, 2000, 0); w != 4000 || h != 2000 {
		t.Fatalf("size = %dx%d, want 4000x2000", w, h)
	}

	// 最大宽度约束优先。
	if w, h := svgTargetSize(100, 50, 50); w != 50 || h != 25 {
		t.Fatalf("size = %dx%d, want 50x25", w, h)
	}

	// 极高图形按高度上限收缩，比例保持（100:1000 → 300:3000）。
	if w, h := svgTargetSize(100, 1000, 0); w != 300 || h != svgRenderHeightLimit {
		t.Fatalf("size = %dx%d, want 300x%d", w, h, svgRenderHeightLimit)
	}
}

func TestRasterizeSVGScalesToMaxWidth(t *testing.T) {
	asset, err := rasterizeSVG([]byte(simpleSVG), 50)
	if err != nil {
		t.Fatalf("rasterizeSVG: %v", err)
	}
	if asset.Width != 50 || asset.Height != 25 {
		t.Fatalf("size = %dx%d, want 50x25", asset.Width, asset.Height)
	}
}

func TestRasterizeSVGRejectsInvalidInput(t *testing.T) {
	if _, err := rasterizeSVG([]byte("definitely not svg"), 0); err == nil {
		t.Fatal("expected error for non-svg input")
	}
}

func TestRasterizeSVGRejectsEmptyViewBox(t *testing.T) {
	empty := `<svg xmlns="http://www.w3.org/2000/svg"></svg>`
	if _, err := rasterizeSVG([]byte(empty), 0); err == nil {
		t.Fatal("expected error for svg without size")
	}
}
