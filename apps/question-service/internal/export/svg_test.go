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
	if asset.Width != 100 || asset.Height != 50 {
		t.Fatalf("size = %dx%d, want 100x50", asset.Width, asset.Height)
	}
	if _, err := png.Decode(bytes.NewReader(asset.Data)); err != nil {
		t.Fatalf("rasterized bytes are not a valid png: %v", err)
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
