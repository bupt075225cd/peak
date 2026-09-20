package export

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

const (
	// svgRenderWidth SVG 光栅化目标宽度（像素）。
	//
	// 几何重绘产出的 SVG 通常只有几百像素宽，若按原始尺寸光栅化再放大到 PDF 正文
	// 宽度会明显模糊；这里统一放大到与题目位图同量级的宽度。
	svgRenderWidth = 1600
	// svgRenderHeightLimit SVG 光栅化后的高度上限，避免极端长图占用过多内存。
	svgRenderHeightLimit = 3000
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

	w, h := svgTargetSize(vw, vh, maxWidth)
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

// svgTargetSize 计算 SVG 光栅化尺寸。
//
// 宽于目标宽度时保持原始尺寸（不做无谓放大），窄于目标宽度时放大到目标宽度，
// 以保证嵌入 PDF 后的清晰度；同时受 maxWidth 与高度上限约束。
func svgTargetSize(vw, vh, maxWidth int) (int, int) {
	w, h := vw, vh

	// 先放大到目标宽度，保证嵌入 PDF 后的清晰度。
	if w < svgRenderWidth {
		h = int(math.Round(float64(vh) * float64(svgRenderWidth) / float64(vw)))
		w = svgRenderWidth
	}

	// 再受最大宽度约束（可能来自配置）。
	if maxWidth > 0 && w > maxWidth {
		w, h = scaleSize(vw, vh, maxWidth)
	}

	if h > svgRenderHeightLimit {
		w = int(math.Round(float64(w) * float64(svgRenderHeightLimit) / float64(h)))
		h = svgRenderHeightLimit
	}

	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}
