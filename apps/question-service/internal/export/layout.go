package export

import "math"

// 配图显示尺寸策略：PDF 与 Word 共用，保证同一张图在两种文档中大小一致。
//
// 以 A4 正文区域（四周 15mm 边距，约 180mm × 267mm）为基准，把每张配图映射到合理
// 的显示尺寸：
//   - 宽高比越大（越扁）显示越宽，越小（越竖）显示越窄；
//   - 竖高图按显示高度上限压缩宽度，避免独占版面、也避免 PDF 中整题被缩小；
//   - 低分辨率位图限制放大倍数，避免拉伸模糊。
//
// 尺寸以毫米为单位计算，PDF / Word 各自换算为自己需要的单位，从而在物理尺寸上一致。
const (
	// layoutContentWidthMM A4 正文宽度（210 - 2×15）。
	layoutContentWidthMM = 180.0
	// layoutContentHeightMM A4 正文高度（297 - 2×15）。
	layoutContentHeightMM = 267.0

	// 配图显示宽度占正文宽度的比例区间。
	//
	// 校准依据是"图形内标注字号与正文字号相当"：几何图 canvas 通常约 100 单位、
	// 标注 font-size 约 9，正文宽 180mm 时显示宽度 ≈ 0.30×正文宽（约 54mm）可使
	// 标注约 13pt，接近正文字号；再大就会明显压过正文、整张图显得偏大。
	imageMinWidthRatio = 0.22
	imageMaxWidthRatio = 0.45

	// imageAspectMin/imageAspectMax 宽高比过渡区间：区间内线性映射到宽度比例上下限。
	// 小于 min 视为极竖图（取下限），大于 max 视为极扁图（取上限）。
	imageAspectMin = 0.5
	imageAspectMax = 2.0

	// imageMaxHeightRatio 配图显示高度占正文高度的比例上限（约束竖高图）。
	imageMaxHeightRatio = 0.42

	// imageMaxUpscale 位图放大倍数上限：低分辨率小图不被拉伸过大，避免模糊。
	imageMaxUpscale = 2.0

	// imageDefaultWidthRatio 尺寸缺失时的默认显示宽度比例（接近正方形图）。
	imageDefaultWidthRatio = 0.30

	// mmPerInch 每英寸毫米数，用于把原始像素换算为毫米。
	mmPerInch = 25.4
	// imageSourceDPI 位图原始像素假定的分辨率（与 docx 的 EMU 换算一致）。
	imageSourceDPI = 96.0
)

// imageDisplaySizeMM 返回配图在文档中的显示尺寸（毫米），保持原始宽高比。
func imageDisplaySizeMM(asset ImageAsset) (wMM, hMM float64) {
	if asset.Width <= 0 || asset.Height <= 0 {
		wMM = imageDefaultWidthRatio * layoutContentWidthMM
		return wMM, wMM / 2
	}

	wMM = imageDisplayWidthRatio(asset.Width, asset.Height) * layoutContentWidthMM
	hMM = wMM * float64(asset.Height) / float64(asset.Width)

	// 位图限制放大倍数：矢量图（由 SVG 光栅化而来）不受此限。
	if !asset.Vector && asset.NaturalWidth > 0 {
		maxMM := float64(asset.NaturalWidth) / imageSourceDPI * mmPerInch * imageMaxUpscale
		if wMM > maxMM {
			wMM = maxMM
			hMM = wMM * float64(asset.Height) / float64(asset.Width)
		}
	}
	return wMM, hMM
}

// imageDisplayWidthRatio 按宽高比计算配图显示宽度占正文宽度的比例。
//
// 先按宽高比在 [imageMinWidthRatio, imageMaxWidthRatio] 内线性取值，再受显示高度
// 上限约束（此时允许低于下限，优先保证不出现竖高巨图）。
func imageDisplayWidthRatio(w, h int) float64 {
	if w <= 0 || h <= 0 {
		return imageDefaultWidthRatio
	}

	aspect := float64(w) / float64(h)
	t := (aspect - imageAspectMin) / (imageAspectMax - imageAspectMin)
	t = math.Max(0, math.Min(1, t))
	ratio := imageMinWidthRatio + (imageMaxWidthRatio-imageMinWidthRatio)*t

	// 显示高度（占正文宽）= ratio/aspect，其占正文高的比例为 ratio/(aspect*contentAspect)，
	// 其中 contentAspect = 正文高/正文宽，故高度上限要求 ratio <= maxHeight*contentAspect*aspect。
	contentAspect := layoutContentHeightMM / layoutContentWidthMM
	if maxByHeight := imageMaxHeightRatio * contentAspect * aspect; ratio > maxByHeight {
		ratio = maxByHeight
	}
	return ratio
}
