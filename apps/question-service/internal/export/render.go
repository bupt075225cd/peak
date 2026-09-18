package export

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/fogleman/gg"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
)

// renderConfig 题目位图渲染参数（单位：像素）。
type renderConfig struct {
	width       int     // 画布宽度
	padding     int     // 内边距
	gap         int     // 段落间距
	metaSize    float64 // 元信息字号
	stemSize    float64 // 题干字号
	lineSpacing float64 // 行高倍数
}

// defaultRenderConfig 返回默认渲染参数。
func defaultRenderConfig() renderConfig {
	return renderConfig{
		width:       1000,
		padding:     36,
		gap:         20,
		metaSize:    20,
		stemSize:    22,
		lineSpacing: 1.6,
	}
}

// 渲染配色。
var (
	colorInk     = color.RGBA{R: 0x11, G: 0x18, B: 0x27, A: 0xff} // 题干
	colorMeta    = color.RGBA{R: 0x1f, G: 0x29, B: 0x37, A: 0xff} // 元信息
	colorWarning = color.RGBA{R: 0xb4, G: 0x53, B: 0x09, A: 0xff} // 配图失败提示
	colorPaper   = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

// renderItemImage 把一道错题渲染为 PNG 位图，返回字节与像素尺寸。
//
// 高度先按文本行数与配图尺寸算好，再创建画布绘制，避免二次裁剪。
func renderItemImage(rc renderConfig, fonts *FontProvider, index int, it renderItem) (data []byte, width, height int, err error) {
	metaFace, err := fonts.Face(rc.metaSize)
	if err != nil {
		return nil, 0, 0, err
	}
	stemFace, err := fonts.Face(rc.stemSize)
	if err != nil {
		return nil, 0, 0, err
	}

	contentWidth := float64(rc.width - 2*rc.padding)
	if contentWidth <= 0 {
		return nil, 0, 0, fmt.Errorf("render width %d is too small", rc.width)
	}

	metaLines := wrapText(metaFace, itemMetaLine(index, it.item), contentWidth)
	stemText := strings.TrimSpace(it.item.StemText)
	stemLines := wrapText(stemFace, stemText, contentWidth)

	metaLineH := lineHeight(metaFace, rc.lineSpacing)
	stemLineH := lineHeight(stemFace, rc.lineSpacing)

	placed, err := decodeImages(it.images, contentWidth)
	if err != nil {
		return nil, 0, 0, err
	}

	hasStem := stemText != ""
	hasNote := it.imageFailed

	totalHeight := float64(rc.padding) + float64(len(metaLines))*metaLineH
	if hasStem {
		totalHeight += float64(rc.gap) + float64(len(stemLines))*stemLineH
	}
	if hasNote {
		totalHeight += float64(rc.gap) + metaLineH
	}
	for _, p := range placed {
		totalHeight += float64(rc.gap) + p.h
	}
	totalHeight += float64(rc.padding)

	canvasH := int(math.Ceil(totalHeight))
	if canvasH < 1 {
		canvasH = 1
	}

	dc := gg.NewContext(rc.width, canvasH)
	dc.SetColor(colorPaper)
	dc.Clear()

	x := float64(rc.padding)
	y := float64(rc.padding)

	dc.SetColor(colorMeta)
	y = drawTextLines(dc, metaFace, metaLines, x, y, metaLineH)

	if hasStem {
		y += float64(rc.gap)
		dc.SetColor(colorInk)
		y = drawTextLines(dc, stemFace, stemLines, x, y, stemLineH)
	}

	if hasNote {
		y += float64(rc.gap)
		dc.SetColor(colorWarning)
		y = drawTextLines(dc, metaFace, []string{"（配图加载失败）"}, x, y, metaLineH)
	}

	for _, p := range placed {
		y += float64(rc.gap)
		dw, dh := int(math.Round(p.w)), int(math.Round(p.h))
		if dw < 1 || dh < 1 {
			continue
		}
		scaled := image.NewRGBA(image.Rect(0, 0, dw, dh))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), p.src, p.src.Bounds(), draw.Over, nil)
		dc.DrawImage(scaled, rc.padding, int(math.Round(y)))
		y += p.h
	}

	var buf bytes.Buffer
	if err := dc.EncodePNG(&buf); err != nil {
		return nil, 0, 0, fmt.Errorf("encode item image: %w", err)
	}
	return buf.Bytes(), rc.width, canvasH, nil
}

// placedImage 已解码并计算好显示尺寸的配图。
type placedImage struct {
	src  image.Image
	w, h float64
}

// decodeImages 解码配图并按内容宽度等比计算显示尺寸，解码失败的图片直接跳过。
func decodeImages(assets []ImageAsset, contentWidth float64) ([]placedImage, error) {
	out := make([]placedImage, 0, len(assets))
	for _, asset := range assets {
		if asset.Width <= 0 || asset.Height <= 0 {
			continue
		}
		src, _, err := image.Decode(bytes.NewReader(asset.Data))
		if err != nil {
			continue
		}
		w := contentWidth
		h := w * float64(asset.Height) / float64(asset.Width)
		out = append(out, placedImage{src: src, w: w, h: h})
	}
	return out, nil
}

// drawTextLines 逐行绘制文本，返回绘制后下一段的 y 起点。
func drawTextLines(dc *gg.Context, face font.Face, lines []string, x, y, lineH float64) float64 {
	dc.SetFontFace(face)
	ascent := float64(face.Metrics().Ascent) / 64.0
	for i, line := range lines {
		if line == "" {
			continue
		}
		dc.DrawString(line, x, y+float64(i)*lineH+ascent)
	}
	return y + float64(len(lines))*lineH
}

// lineHeight 返回按行距倍数放大的行高。
func lineHeight(face font.Face, spacing float64) float64 {
	if spacing < 1 {
		spacing = 1
	}
	return float64(face.Metrics().Height) / 64.0 * spacing
}

// wrapText 按可用宽度对文本换行，支持显式换行符。
//
// 中文之间没有空格，必须逐字符测量宽度；同时优先在空格处断行，
// 避免把英文单词从中间截断。
func wrapText(face font.Face, text string, maxWidth float64) []string {
	if maxWidth <= 0 {
		return []string{text}
	}

	out := make([]string, 0, 8)
	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimRight(para, "\r")
		if para == "" {
			out = append(out, "")
			continue
		}

		line := make([]rune, 0, len(para))
		var width float64
		for _, r := range para {
			rw := runeWidth(face, r)
			if width+rw > maxWidth && len(line) > 0 {
				cut := len(line)
				for i := len(line) - 1; i >= 0; i-- {
					if line[i] == ' ' {
						cut = i
						break
					}
				}
				out = append(out, string(line[:cut]))

				rest := make([]rune, len(line)-cut)
				copy(rest, line[cut:])
				line = rest
				for len(line) > 0 && line[0] == ' ' {
					line = line[1:]
				}

				width = 0
				for _, lr := range line {
					width += runeWidth(face, lr)
				}
			}
			line = append(line, r)
			width += rw
		}
		if len(line) > 0 {
			out = append(out, string(line))
		}
	}

	if len(out) == 0 {
		out = append(out, "")
	}
	return out
}

// runeWidth 返回单个字符的显示宽度（像素）。
func runeWidth(face font.Face, r rune) float64 {
	return float64(font.MeasureString(face, string(r))) / 64.0
}
