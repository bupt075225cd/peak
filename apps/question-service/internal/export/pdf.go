package export

import (
	"bytes"
	"fmt"

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
)

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
		png, pxW, pxH, err := renderItemImage(rc, fonts, i+1, items[i])
		if err != nil {
			return nil, fmt.Errorf("render item %d: %w", i+1, err)
		}
		if pxW <= 0 || pxH <= 0 || len(png) == 0 {
			continue
		}

		name := fmt.Sprintf("item%d", i+1)
		pdf.RegisterImageOptionsReader(name, opts, bytes.NewReader(png))

		// 按内容宽度等比缩放；单题过高时改为按内容高度缩放，避免跨页断裂。
		dispW := contentWidth
		dispH := dispW * float64(pxH) / float64(pxW)
		if dispH > contentHeight {
			dispH = contentHeight
			dispW = dispH * float64(pxW) / float64(pxH)
		}

		// fpdf 的 GetY/SetY 与 ImageOptions 的 y 均为绝对坐标（已含上边距），
		// 因此统一按绝对坐标判断与绘制，避免边距被重复计入。
		y := pdf.GetY()
		if y+dispH > bottom {
			pdf.AddPage()
			y = top
		}
		pdf.ImageOptions(name, pdfMarginMM, y, dispW, dispH, false, opts, 0, "")
		pdf.SetY(y + dispH + pdfItemGapMM)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("write pdf output: %w", err)
	}
	return buf.Bytes(), nil
}
