package export

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
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
// 几何重绘产物是 SVG，而 PDF/docx 都只接受位图，因此用纯 Go 的 oksvg + rasterx
// 做光栅化，避免引入外部渲染进程。
//
// oksvg 有两处固有限制需要在这里补足：
//   - stroke-width 不随 SetTarget 的缩放同步放大，直接放大画布会让线条细如发丝；
//   - 不支持 <text> 元素，几何图的顶点标注会丢失。
func rasterizeSVG(data []byte, maxWidth int, fonts *FontProvider) (*ImageAsset, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data), oksvg.IgnoreErrorMode)
	if err != nil {
		return nil, fmt.Errorf("parse svg: %w", err)
	}

	vw, vh := int(icon.ViewBox.W), int(icon.ViewBox.H)
	if vw <= 0 || vh <= 0 {
		return nil, fmt.Errorf("svg has invalid viewbox %dx%d", vw, vh)
	}

	w, h := svgTargetSize(vw, vh, maxWidth)

	// 缩放不为 1 时预先换算描边宽度，再重新解析。
	if scale := float64(w) / float64(vw); scale != 1 {
		scaled := scaleSVGStrokeWidth(data, scale)
		icon, err = oksvg.ReadIconStream(bytes.NewReader(scaled), oksvg.IgnoreErrorMode)
		if err != nil {
			return nil, fmt.Errorf("parse scaled svg: %w", err)
		}
	}

	icon.SetTarget(0, 0, float64(w), float64(h))

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	icon.Draw(rasterx.NewDasher(w, h, scanner), 1.0)

	// 补绘 oksvg 忽略的文字标注。
	if err := drawSVGTexts(rgba, data, float64(w)/float64(vw), fonts); err != nil {
		return nil, err
	}

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

	// 再受最大宽度约束（可能来自配置）。注意这里要基于已放大的尺寸收缩，
	// 否则原始尺寸小于上限时会被误判为"无需缩放"而退回原始大小。
	if maxWidth > 0 && w > maxWidth {
		w, h = scaleSize(w, h, maxWidth)
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

var svgStrokeWidthRe = regexp.MustCompile(`stroke-width="([0-9.]+)"`)

// scaleSVGStrokeWidth 按比例换算 SVG 中的 stroke-width。
//
// oksvg/rasterx 描边时按属性原值处理，不会随 SetTarget 的缩放变化，
// 因此放大画布前需要先把线宽换算到目标像素尺度。
func scaleSVGStrokeWidth(data []byte, scale float64) []byte {
	return svgStrokeWidthRe.ReplaceAllFunc(data, func(m []byte) []byte {
		sub := svgStrokeWidthRe.FindSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		v, err := strconv.ParseFloat(string(sub[1]), 64)
		if err != nil {
			return m
		}
		return []byte(fmt.Sprintf(`stroke-width="%.4f"`, v*scale))
	})
}

// svgText SVG 中的文字元素（oksvg 不渲染，需要自行补绘）。
type svgText struct {
	X, Y     float64
	FontSize float64
	Anchor   string     // start / middle / end
	Baseline string     // alphabetic / middle / hanging
	Color    color.RGBA // 填充色
	Content  string
}

// parseSVGTexts 解析 SVG 中的所有 <text> 元素。
//
// 解析失败或结构异常时返回已解析到的部分，不影响图形本身的渲染。
func parseSVGTexts(data []byte) []svgText {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var out []svgText

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "text" {
			continue
		}

		t := svgText{Anchor: "start", Baseline: "alphabetic", Color: color.RGBA{A: 0xff}}
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "x":
				t.X, _ = strconv.ParseFloat(attr.Value, 64)
			case "y":
				t.Y, _ = strconv.ParseFloat(attr.Value, 64)
			case "font-size":
				t.FontSize, _ = strconv.ParseFloat(attr.Value, 64)
			case "text-anchor":
				t.Anchor = attr.Value
			case "dominant-baseline":
				t.Baseline = attr.Value
			case "fill":
				t.Color = parseHexColor(attr.Value)
			}
		}

		t.Content = strings.TrimSpace(readElementText(dec))
		if t.Content != "" && t.FontSize > 0 {
			out = append(out, t)
		}
	}
	return out
}

// readElementText 读取当前元素内的纯文本内容（忽略嵌套子元素）。
func readElementText(dec *xml.Decoder) string {
	var sb strings.Builder
	depth := 0

	for {
		tok, err := dec.Token()
		if err != nil {
			return sb.String()
		}
		switch v := tok.(type) {
		case xml.CharData:
			if depth == 0 {
				sb.Write(v)
			}
		case xml.StartElement:
			depth++
		case xml.EndElement:
			if depth == 0 {
				return sb.String()
			}
			depth--
		}
	}
}

// drawSVGTexts 把 SVG 中的文字绘制到位图上。
//
// 坐标系与 SVG 用户单位一致，因此按 scale 换算到像素后再定位，
// 并按 text-anchor / dominant-baseline 做对齐。
func drawSVGTexts(dst *image.RGBA, data []byte, scale float64, fonts *FontProvider) error {
	if fonts == nil {
		return nil
	}

	for _, t := range parseSVGTexts(data) {
		face, err := fonts.Face(t.FontSize * scale)
		if err != nil {
			return fmt.Errorf("load svg text face: %w", err)
		}

		x := t.X * scale
		if w := float64(font.MeasureString(face, t.Content)) / 64.0; w > 0 {
			switch t.Anchor {
			case "middle":
				x -= w / 2
			case "end":
				x -= w
			}
		}

		metrics := face.Metrics()
		ascent := float64(metrics.Ascent) / 64.0
		descent := float64(metrics.Descent) / 64.0

		y := t.Y * scale
		switch t.Baseline {
		case "middle":
			y += (ascent - descent) / 2
		case "hanging":
			y += ascent
		}

		drawer := &font.Drawer{
			Dst:  dst,
			Src:  image.NewUniform(t.Color),
			Face: face,
			Dot: fixed.Point26_6{
				X: fixed.Int26_6(math.Round(x * 64)),
				Y: fixed.Int26_6(math.Round(y * 64)),
			},
		}
		drawer.DrawString(t.Content)
	}
	return nil
}

// parseHexColor 解析 #rrggbb 形式的颜色，非法值回退为黑色。
func parseHexColor(s string) color.RGBA {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return color.RGBA{A: 0xff}
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{A: 0xff}
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}
