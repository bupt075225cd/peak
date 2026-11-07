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
// oksvg 有三处固有限制需要在这里补足：
//   - 描边尺寸（stroke-width / stroke-dasharray / stroke-dashoffset）不随
//     SetTarget 的缩放同步放大，直接放大画布会让线条细如发丝、虚线糊成实线；
//   - 不支持 <text> 元素，几何图的顶点标注会丢失；
//   - ParseSVGColor 不支持 rgba() 颜色，且该解析错误绕过 ErrorMode 直接导致
//     ReadIconStream 整体失败（存量几何产物即踩中此坑）。
func rasterizeSVG(data []byte, maxWidth int, fonts *FontProvider) (*ImageAsset, error) {
	// 兼容存量产物：把 fill/stroke 上的 rgba(r,g,b,a) 改写为
	// rgb(r,g,b) + fill-opacity/stroke-opacity 属性后再交给 oksvg。
	data = normalizeSVGRGBA(data)

	icon, err := oksvg.ReadIconStream(bytes.NewReader(data), oksvg.IgnoreErrorMode)
	if err != nil {
		return nil, fmt.Errorf("parse svg: %w", err)
	}

	vw, vh := int(icon.ViewBox.W), int(icon.ViewBox.H)
	if vw <= 0 || vh <= 0 {
		return nil, fmt.Errorf("svg has invalid viewbox %dx%d", vw, vh)
	}

	w, h := svgTargetSize(vw, vh, maxWidth)

	// 缩放不为 1 时预先换算描边尺寸类属性，再重新解析。
	if scale := float64(w) / float64(vw); scale != 1 {
		scaled := scaleSVGStrokeMetrics(data, scale)
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
	return &ImageAsset{
		Data: buf.Bytes(), Format: "png",
		Width: w, Height: h,
		NaturalWidth: vw, NaturalHeight: vh,
		Vector: true,
	}, nil
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

var (
	// svgStrokeWidthRe 匹配线宽属性。
	svgStrokeWidthRe = regexp.MustCompile(`stroke-width="([0-9.]+)"`)
	// svgDashArrayRe 匹配虚线节距属性（值可为空格/逗号分隔的多个数字）。
	svgDashArrayRe = regexp.MustCompile(`stroke-dasharray="([0-9.,\s]+)"`)
	// svgDashOffsetRe 匹配虚线相位偏移属性。
	svgDashOffsetRe = regexp.MustCompile(`stroke-dashoffset="([0-9.]+)"`)
)

// svgRGBARe 匹配 fill/stroke 属性上的 rgba(r,g,b,a) 颜色。
var svgRGBARe = regexp.MustCompile(`(fill|stroke)="rgba\(\s*([0-9]+)\s*,\s*([0-9]+)\s*,\s*([0-9]+)\s*,\s*([0-9.]+)\s*\)"`)

// normalizeSVGRGBA 把 fill/stroke 上的 rgba(r,g,b,a) 颜色改写为
// rgb(r,g,b) + fill-opacity/stroke-opacity 属性。
//
// oksvg 的 ParseSVGColor 只支持 rgb()/hsl()/#hex，rgba() 会返回解析错误；
// 该错误经 PushStyle 直接上抛、不受 IgnoreErrorMode 兜底，最终让
// ReadIconStream 整体失败、整张配图导出为占位或被 docx 静默丢弃。
// 浏览器对两种写法均支持，此改写不影响前端展示语义。
func normalizeSVGRGBA(data []byte) []byte {
	return svgRGBARe.ReplaceAllFunc(data, func(m []byte) []byte {
		sub := svgRGBARe.FindSubmatch(m)
		if len(sub) != 6 {
			return m
		}
		return []byte(fmt.Sprintf(`%s="rgb(%s,%s,%s)" %s-opacity="%s"`,
			sub[1], sub[2], sub[3], sub[4], sub[1], sub[5]))
	})
}

// scaleSVGStrokeMetrics 按比例换算 SVG 描边的尺寸类属性：
// stroke-width、stroke-dasharray（节距）与 stroke-dashoffset（相位）。
//
// oksvg/rasterx 对这三者都按属性原值在目标像素空间处理，不随 SetTarget
// 的缩放同步变化，因此放大画布前需要先换算到目标像素尺度。
// 若只补偿线宽而不补偿虚线节距，放大后 gap 会远小于线宽（例如 3/2.4 的
// 节距在 16 倍放大下变成 2.4px 的缝 vs 15px 的粗线），缝隙在文档缩小显示时
// 被抹平，虚线视觉上糊成实线。
func scaleSVGStrokeMetrics(data []byte, scale float64) []byte {
	data = svgStrokeWidthRe.ReplaceAllFunc(data, func(m []byte) []byte {
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

	data = svgDashOffsetRe.ReplaceAllFunc(data, func(m []byte) []byte {
		sub := svgDashOffsetRe.FindSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		v, err := strconv.ParseFloat(string(sub[1]), 64)
		if err != nil {
			return m
		}
		return []byte(fmt.Sprintf(`stroke-dashoffset="%.4f"`, v*scale))
	})

	return svgDashArrayRe.ReplaceAllFunc(data, func(m []byte) []byte {
		sub := svgDashArrayRe.FindSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		// 值可为空格或逗号分隔的多个数字（如 "3 2.4" / "3,2.4"），逐个换算，
		// 统一以空格连接（oksvg 解析时两者等价）。
		vals := strings.FieldsFunc(string(sub[1]), func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t'
		})
		out := make([]string, 0, len(vals))
		for _, s := range vals {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return m
			}
			out = append(out, strconv.FormatFloat(v*scale, 'f', -1, 64))
		}
		if len(out) == 0 {
			return m
		}
		return []byte(fmt.Sprintf(`stroke-dasharray="%s"`, strings.Join(out, " ")))
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
// 缺字形字符（下标/上标数字等）按合成回退绘制。
func drawSVGTexts(dst *image.RGBA, data []byte, scale float64, fonts *FontProvider) error {
	if fonts == nil {
		return nil
	}

	for _, t := range parseSVGTexts(data) {
		size := t.FontSize * scale
		face, err := fonts.Face(size)
		if err != nil {
			return fmt.Errorf("load svg text face: %w", err)
		}

		x := t.X * scale
		if w := runesWidth(face, []rune(t.Content)); w > 0 {
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

		for _, seg := range splitTextSegments(fonts, size, t.Content) {
			segFace := face
			segY := y
			if seg.face != nil {
				segFace = seg.face
				segY += seg.dyEm * size
			}
			drawer := &font.Drawer{
				Dst:  dst,
				Src:  image.NewUniform(t.Color),
				Face: segFace,
				Dot: fixed.Point26_6{
					X: fixed.Int26_6(math.Round(x * 64)),
					Y: fixed.Int26_6(math.Round(segY * 64)),
				},
			}
			drawer.DrawString(seg.text)
			x += runesWidth(face, []rune(seg.text)) * seg.scale
		}
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
