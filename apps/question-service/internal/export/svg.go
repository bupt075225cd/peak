package export

import (
	"bytes"
	"fmt"
	"image"
	"image/png"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// rasterizeSVG 把 SVG 字节光栅化为 PNG 位图。
//
// 几何重绘产物是 SVG，而 PDF/docx 都只接受位图，因此这里用纯 Go 的
// oksvg + rasterx 做光栅化，避免引入外部渲染进程。
// 解析采用 IgnoreErrorMode：项目自绘的 SVG 若含少量不支持的元素，
// 跳过即可，不应导致整次导出失败。
func rasterizeSVG(data []byte, maxWidth int) (*ImageAsset, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data), oksvg.IgnoreErrorMode)
	if err != nil {
		return nil, fmt.Errorf("parse svg: %w", err)
	}

	vw, vh := int(icon.ViewBox.W), int(icon.ViewBox.H)
	if vw <= 0 || vh <= 0 {
		return nil, fmt.Errorf("svg has invalid viewbox %dx%d", vw, vh)
	}

	w, h := scaleSize(vw, vh, maxWidth)
	icon.SetTarget(0, 0, float64(w), float64(h))

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	icon.Draw(rasterx.NewDasher(w, h, scanner), 1.0)

	var buf bytes.Buffer
	if err := png.Encode(&buf, rgba); err != nil {
		return nil, fmt.Errorf("encode rasterized svg: %w", err)
	}
	return &ImageAsset{Data: buf.Bytes(), Format: "png", Width: w, Height: h}, nil
}
