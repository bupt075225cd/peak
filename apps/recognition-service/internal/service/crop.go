// Package service 实现识别任务的业务编排与异步处理状态机。
package service

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"math"

	// 注册图片解码器（jpeg/png/gif），保证 image.Decode 能识别常见格式。
	_ "image/gif"
	_ "image/png"

	"peak/apps/recognition-service/internal/provider"
)

// cropGeometryImage 根据几何图形外接矩形（归一化坐标）从原图中裁剪出子图，
// 返回编码后的 JPEG 字节。bbox 为 nil 或无效时返回错误。
// 为避免模型给出的 bbox 偏紧、贴边截断，裁剪前对 bbox 各边做 10% 的外扩兜底。
func cropGeometryImage(original []byte, bbox *provider.BoundingBox) ([]byte, error) {
	if bbox == nil {
		return nil, fmt.Errorf("nil bounding box")
	}
	img, _, err := image.Decode(bytes.NewReader(original))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	bounds := img.Bounds()
	imgW := bounds.Dx()
	imgH := bounds.Dy()
	if imgW <= 0 || imgH <= 0 {
		return nil, fmt.Errorf("invalid image dimensions: %dx%d", imgW, imgH)
	}

	// bbox 外扩 10%（各方向），clamp 到 [0,1] 区间，避免贴边截断。
	expanded := expandBBox(bbox, 0.1)

	// 归一化坐标换算为像素，并做 clamp 兜底（防止浮点误差越界）。
	x0 := clampInt(int(float64(imgW)*expanded.X), 0, imgW-1)
	y0 := clampInt(int(float64(imgH)*expanded.Y), 0, imgH-1)
	x1 := clampInt(int(float64(imgW)*(expanded.X+expanded.Width)), x0+1, imgW)
	y1 := clampInt(int(float64(imgH)*(expanded.Y+expanded.Height)), y0+1, imgH)
	if x1 <= x0 || y1 <= y0 {
		return nil, fmt.Errorf("empty crop rect: (%d,%d)-(%d,%d)", x0, y0, x1, y1)
	}

	cropped := cropRect(img, image.Rect(x0, y0, x1, y1))

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, cropped, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}

// expandBBox 对归一化 bbox 各边按 fraction 比例外扩，结果仍 clamp 到 [0,1]。
// 例如 fraction=0.1 表示左右各扩 10% 的图宽、上下各扩 10% 的图高。
func expandBBox(b *provider.BoundingBox, fraction float64) *provider.BoundingBox {
	if b == nil {
		return nil
	}
	dx := fraction
	dy := fraction
	x := b.X - dx
	y := b.Y - dy
	w := b.Width + 2 * dx
	h := b.Height + 2 * dy
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x > 1 {
		x = 1
	}
	if y > 1 {
		y = 1
	}
	if x+w > 1 {
		w = 1 - x
	}
	if y+h > 1 {
		h = 1 - y
	}
	if w <= 0 || h <= 0 {
		return b
	}
	return &provider.BoundingBox{X: x, Y: y, Width: w, Height: h}
}

// clampInt 将 v 限制在 [lo, hi] 区间。
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// wanx 图像编辑约束：输入图宽高须落在 [minWanxSide, maxWanxSide]。
const (
	minWanxSide = 512
	maxWanxSide = 4096
)

// ensureWanSize 调整图片尺寸至 wanx 约束范围：
// 短边 < 512 时等比放大到 512；长边 > 4096 时等比缩小到 4096；
// 同时满足时原样返回。返回编码后的 JPEG 字节。
func ensureWanSize(original []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(original))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("invalid image dimensions: %dx%d", w, h)
	}

	// 已满足 wanx 约束（短边 >= 512 且 长边 <= 4096）则原样返回。
	minSide := min(w, h)
	maxSide := max(w, h)
	if minSide >= minWanxSide && maxSide <= maxWanxSide {
		return original, nil
	}

	// 等比缩放，使两边同时落入 [minWanxSide, maxWanxSide]。
	// 短边 < 512 时 scale 必须 >= 512/minSide；长边 > 4096 时 scale 必须 <= 4096/maxSide。
	scale := 1.0
	if float64(minSide) < minWanxSide {
		scale = math.Max(scale, minWanxSide/float64(minSide))
	}
	if float64(maxSide) > maxWanxSide {
		scale = math.Min(scale, maxWanxSide/float64(maxSide))
	}
	nw := int(math.Round(float64(w) * scale))
	nh := int(math.Round(float64(h) * scale))
	// 防浮点误差越界做兜底夹紧。
	nw = max(minWanxSide, min(maxWanxSide, nw))
	nh = max(minWanxSide, min(maxWanxSide, nh))

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	// 最近邻缩放：保留原线条清晰度（放大/缩小后由 AI 重绘，无需插值平滑）。
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			sx := x * w / nw
			sy := y * h / nh
			dst.Set(x, y, img.At(bounds.Min.X+sx, bounds.Min.Y+sy))
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}

// cropRect 从 src 中裁剪出矩形子图。若 src 本身是可裁剪的类型则直接 SubImage；
// 否则通过绘制复制到新图像，保证任意 image.Image 类型都可用。
func cropRect(src image.Image, rect image.Rectangle) image.Image {
	if sub, ok := src.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return sub.SubImage(rect)
	}
	dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			dst.Set(x-rect.Min.X, y-rect.Min.Y, src.At(x, y))
		}
	}
	return dst
}
