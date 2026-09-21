package export

import (
	"bytes"
	"image/png"
	"testing"
)

const simpleSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="50" viewBox="0 0 100 50">` +
	`<rect x="10" y="10" width="80" height="30" fill="#336699"/></svg>`

func TestRasterizeSVGProducesPNG(t *testing.T) {
	asset, err := rasterizeSVG([]byte(simpleSVG), 0, nil)
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
	// 自然尺寸记录 viewBox，并标记为矢量图（不受放大倍数限制）。
	if asset.NaturalWidth != 100 || asset.NaturalHeight != 50 || !asset.Vector {
		t.Fatalf("natural size = %dx%d vector=%v, want 100x50 vector",
			asset.NaturalWidth, asset.NaturalHeight, asset.Vector)
	}
	if _, err := png.Decode(bytes.NewReader(asset.Data)); err != nil {
		t.Fatalf("rasterized bytes are not a valid png: %v", err)
	}
}

// svgWithText 带文字标注的 SVG：oksvg 不渲染 <text>，用于验证补绘逻辑。
const svgWithText = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 50" width="100" height="50">` +
	`<rect x="0" y="0" width="100" height="50" fill="#ffffff"/>` +
	`<text x="10" y="25" font-size="10" font-weight="700" text-anchor="middle" dominant-baseline="middle" fill="#111827">A</text>` +
	`<text x="60" y="25" font-size="10" fill="#000000">B</text>` +
	`</svg>`

func TestScaleSVGStrokeWidth(t *testing.T) {
	in := []byte(`<line stroke-width="0.95"/><line stroke-width="2"/>`)
	out := scaleSVGStrokeWidth(in, 10)
	if !bytes.Contains(out, []byte(`stroke-width="9.5000"`)) {
		t.Fatalf("stroke width not scaled: %s", out)
	}
	if !bytes.Contains(out, []byte(`stroke-width="20.0000"`)) {
		t.Fatalf("stroke width not scaled: %s", out)
	}
}

func TestParseSVGTexts(t *testing.T) {
	texts := parseSVGTexts([]byte(svgWithText))
	if len(texts) != 2 {
		t.Fatalf("parsed %d texts, want 2", len(texts))
	}

	first := texts[0]
	if first.Content != "A" {
		t.Fatalf("content = %q, want A", first.Content)
	}
	if first.X != 10 || first.Y != 25 || first.FontSize != 10 {
		t.Fatalf("unexpected geometry: %+v", first)
	}
	if first.Anchor != "middle" || first.Baseline != "middle" {
		t.Fatalf("unexpected alignment: %+v", first)
	}
	if first.Color.R != 0x11 || first.Color.G != 0x18 || first.Color.B != 0x27 {
		t.Fatalf("unexpected color: %+v", first.Color)
	}

	if texts[1].Anchor != "start" || texts[1].Baseline != "alphabetic" {
		t.Fatalf("unexpected defaults: %+v", texts[1])
	}
}

func TestRasterizeSVGDrawsTextElements(t *testing.T) {
	asset, err := rasterizeSVG([]byte(svgWithText), 0, testFonts(t))
	if err != nil {
		t.Fatalf("rasterizeSVG: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(asset.Data))
	if err != nil {
		t.Fatalf("decode raster: %v", err)
	}

	// 文字位于用户坐标 (10, 25)，按缩放换算到像素后应有深色字形像素。
	scale := float64(asset.Width) / 100.0
	cx, cy := int(10*scale), int(25*scale)

	dark := 0
	for y := cy - 60; y <= cy+60; y++ {
		for x := cx - 60; x <= cx+60; x++ {
			if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
				continue
			}
			if r, _, _, _ := img.At(x, y).RGBA(); r < 0x8000 {
				dark++
			}
		}
	}
	if dark == 0 {
		t.Fatal("no glyph pixels found where svg text should be drawn")
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

	// 回归：上限大于原始宽度但小于目标宽度时，应按上限收缩，
	// 而不是因为原始尺寸够小就退回原始大小（曾因此导致几何图以原始小尺寸光栅化而模糊）。
	if w, h := svgTargetSize(100, 90, 1200); w != 1200 || h != 1080 {
		t.Fatalf("size = %dx%d, want 1200x1080", w, h)
	}

	// 极高图形按高度上限收缩，比例保持（100:1000 → 300:3000）。
	if w, h := svgTargetSize(100, 1000, 0); w != 300 || h != svgRenderHeightLimit {
		t.Fatalf("size = %dx%d, want 300x%d", w, h, svgRenderHeightLimit)
	}
}

func TestRasterizeSVGScalesToMaxWidth(t *testing.T) {
	asset, err := rasterizeSVG([]byte(simpleSVG), 50, nil)
	if err != nil {
		t.Fatalf("rasterizeSVG: %v", err)
	}
	if asset.Width != 50 || asset.Height != 25 {
		t.Fatalf("size = %dx%d, want 50x25", asset.Width, asset.Height)
	}
}

func TestRasterizeSVGRejectsInvalidInput(t *testing.T) {
	if _, err := rasterizeSVG([]byte("definitely not svg"), 0, nil); err == nil {
		t.Fatal("expected error for non-svg input")
	}
}

func TestRasterizeSVGRejectsEmptyViewBox(t *testing.T) {
	empty := `<svg xmlns="http://www.w3.org/2000/svg"></svg>`
	if _, err := rasterizeSVG([]byte(empty), 0, nil); err == nil {
		t.Fatal("expected error for svg without size")
	}
}
