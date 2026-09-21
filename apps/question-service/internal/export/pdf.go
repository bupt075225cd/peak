package export

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"

	"github.com/go-pdf/fpdf"
)

const (
	// pdfPageWidthMM A4 页宽（毫米）。
	pdfPageWidthMM = 210.0
	// pdfPageHeightMM A4 页高（毫米）。
	pdfPageHeightMM = 297.0
	// pdfMarginMM 页面四周留白（毫米）。
	pdfMarginMM = 15.0
	// pdfItemGapMM 相邻题目位图之间的间距（毫米）。
	pdfItemGapMM = 6.0
	// pdfRenderWidth 题目位图渲染宽度（像素）。
	//
	// 位图最终按 A4 正文宽度（约 180mm）展示，该宽度决定打印清晰度：
	// 1600px 对应约 209 DPI，高于 150 DPI 的常规打印要求。
	pdfRenderWidth = 1600
	// pdfPixelsPerMM 题目位图像素与毫米的换算：位图整幅按 A4 正文宽度展示。
	pdfPixelsPerMM = float64(pdfRenderWidth) / (pdfPageWidthMM - 2*pdfMarginMM)
)

// pdfStripHeightPx 单页可容纳的位图高度（像素）。
//
// 超出该高度的题目位图会按此高度纵向切片分页，保证以正文宽度展示时不跨页，
// 也无需把整题缩小（缩小会连带把题干文字压小）。
var pdfStripHeightPx = int(math.Floor((pdfPageHeightMM - 2*pdfMarginMM) * pdfPixelsPerMM))

// buildPDF 生成截图式 PDF：每道题先渲染为位图，再按 A4 等比排布。
//
// 采用位图而非矢量排版，可绕开 PDF 中文字体嵌入问题；版式由渲染器完全控制，
// 因此打印结果与屏幕展示一致。
func buildPDF(title string, items []renderItem, fonts *FontProvider) ([]byte, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items to export")
	}
	if fonts == nil {
		return nil, fmt.Errorf("font provider is required for pdf export")
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(title, true)
	pdf.SetMargins(pdfMarginMM, pdfMarginMM, pdfMarginMM)
	pdf.SetAutoPageBreak(false, pdfMarginMM)
	pdf.AddPage()

	contentWidth := pdfPageWidthMM - 2*pdfMarginMM
	contentHeight := pdfPageHeightMM - 2*pdfMarginMM
	opts := fpdf.ImageOptions{ImageType: "PNG"}

	// 正文可用区域的上下边界（绝对坐标）。
	top := pdfMarginMM
	bottom := pdfMarginMM + contentHeight

	rc := defaultRenderConfig()

	for i := range items {
		layout, err := renderItemLayout(rc, fonts, i+1, items[i])
		if err != nil {
			return nil, fmt.Errorf("render item %d: %w", i+1, err)
		}
		if layout.width <= 0 || layout.height <= 0 || len(layout.data) == 0 {
			continue
		}

		parts, err := splitItemImage(layout.data, layout.width, layout.height, layout.breaks)
		if err != nil {
			return nil, fmt.Errorf("split item %d: %w", i+1, err)
		}

		for j, part := range parts {
			name := fmt.Sprintf("item%d_%d", i+1, j)
			pdf.RegisterImageOptionsReader(name, opts, bytes.NewReader(part.data))

			// 按内容宽度等比缩放；切片高度已保证单页可容纳，正常不会触发收缩。
			dispW := contentWidth
			dispH := dispW * float64(part.pxH) / float64(part.pxW)
			if dispH > contentHeight {
				dispH = contentHeight
				dispW = dispH * float64(part.pxW) / float64(part.pxH)
			}

			// fpdf 的 GetY/SetY 与 ImageOptions 的 y 均为绝对坐标（已含上边距），
			// 因此统一按绝对坐标判断与绘制，避免边距被重复计入。
			y := pdf.GetY()
			if y+dispH > bottom {
				pdf.AddPage()
				y = top
			}

			// 宽度不足正文宽度时水平居中，避免缩小的内容挤在左侧。
			x := pdfMarginMM
			if dispW < contentWidth {
				x = pdfMarginMM + (contentWidth-dispW)/2
			}
			pdf.ImageOptions(name, x, y, dispW, dispH, false, opts, 0, "")
			pdf.SetY(y + dispH + pdfItemGapMM)
		}
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("write pdf output: %w", err)
	}
	return buf.Bytes(), nil
}

// pdfImagePart 单题位图的一个分页片段。
type pdfImagePart struct {
	data []byte
	pxW  int
	pxH  int
}

// splitItemImage 把单题位图按页面可用高度纵向切片；未超高时原样返回单片段。
//
// 单题内容超过一页时，若仍整题缩放会把题干文字一起压小，因此改为按页高度切片，
// 逐片以正文宽度绘制到连续页面上。切片只在 breaks 给出的安全位置进行（文字行之间、
// 配图边界处），因此不会把配图从中间截断。
func splitItemImage(data []byte, pxW, pxH int, breaks []int) ([]pdfImagePart, error) {
	if pxH <= pdfStripHeightPx {
		return []pdfImagePart{{data: data, pxW: pxW, pxH: pxH}}, nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode item image: %w", err)
	}

	parts := make([]pdfImagePart, 0, 2)
	top := 0
	for _, bottom := range splitCuts(pxH, breaks) {
		strip := image.NewRGBA(image.Rect(0, 0, pxW, bottom-top))
		draw.Draw(strip, strip.Bounds(), img, image.Pt(0, top), draw.Src)

		var buf bytes.Buffer
		if err := png.Encode(&buf, strip); err != nil {
			return nil, fmt.Errorf("encode item strip: %w", err)
		}
		parts = append(parts, pdfImagePart{data: buf.Bytes(), pxW: pxW, pxH: bottom - top})
		top = bottom
	}
	return parts, nil
}

// splitCuts 计算分页切点（自顶向下的累计像素位置）。
//
// 每页在可容纳的高度内尽量切在最后一个安全断点上；只有在整页范围内都没有安全断点
// （例如单个元素自身就超过一页）时才退化为按页高硬切，保证内容总能排出。配图高度
// 远小于一页，实际不会走到该分支。
func splitCuts(pxH int, breaks []int) []int {
	var cuts []int
	for top := 0; top < pxH; {
		limit := top + pdfStripHeightPx
		if limit >= pxH {
			cuts = append(cuts, pxH)
			break
		}
		cut := 0
		for _, b := range breaks {
			if b > top && b <= limit && b > cut {
				cut = b
			}
		}
		if cut == 0 {
			cut = limit
		}
		cuts = append(cuts, cut)
		top = cut
	}
	return cuts
}
