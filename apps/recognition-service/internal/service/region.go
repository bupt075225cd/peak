package service

// 几何区域图片预处理：约束提取的 VLM 输入瘦身。
// 仅对"请求发送给大模型的图片"做裁剪+下采样（不落库、不改产物），
// 可显著降低 vision token，从而降低单次调用延迟。

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math"

	"peak/apps/recognition-service/internal/provider"
)

// geometryRegionMaxDim 约束提取输入小图的最大边长（像素）。
const geometryRegionMaxDim = 960

// errRegionInvalid bbox 无效或图片无法解析。
var errRegionInvalid = errors.New("invalid geometry region")

// cropAndScaleRegion 按归一化 bbox 从原图中裁剪出几何区域，并等比缩放到
// 最长边不超过 maxDim（小于 maxDim 则保持原尺寸）。返回 JPEG 字节；
// 失败返回 error，由调用方决定回退使用整张原图。
func cropAndScaleRegion(src []byte, bbox *provider.BoundingBox, maxDim int) ([]byte, error) {
	if bbox == nil || bbox.Width <= 0 || bbox.Height <= 0 {
		return nil, errRegionInvalid
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, errRegionInvalid
	}

	// 归一化 bbox → 像素矩形（clamp 到图内）。
	x0 := clampInt(int(math.Round(bbox.X*float64(w))), 0, w)
	y0 := clampInt(int(math.Round(bbox.Y*float64(h))), 0, h)
	x1 := clampInt(int(math.Round((bbox.X+bbox.Width)*float64(w))), 0, w)
	y1 := clampInt(int(math.Round((bbox.Y+bbox.Height)*float64(h))), 0, h)
	if x1 <= x0 || y1 <= y0 {
		return nil, errRegionInvalid
	}
	cw, ch := x1-x0, y1-y0

	// 目标尺寸：等比缩放，最长边 = maxDim；若区域已小于 maxDim 则保持原尺寸。
	tw, th := cw, ch
	m := cw
	if m < ch {
		m = ch
	}
	if m > maxDim {
		ratio := float64(maxDim) / float64(m)
		tw = maxInt(1, int(math.Round(float64(cw)*ratio)))
		th = maxInt(1, int(math.Round(float64(ch)*ratio)))
	}

	out := image.NewRGBA(image.Rect(0, 0, tw, th))
	if tw == cw && th == ch {
		copyRegion(out, img, x0, y0, cw, ch)
	} else {
		bilinearResample(out, img, x0, y0, cw, ch)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// copyRegion 把 src 上以 (x0,y0) 为左上角的 cw×ch 区域拷贝到 dst。
func copyRegion(dst *image.RGBA, src image.Image, x0, y0, cw, ch int) {
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			dst.Set(x, y, src.At(x0+x, y0+y))
		}
	}
}

// bilinearResample 把 src 上 (x0,y0,cw,ch) 区域双线性重采样到 dst 尺寸。
func bilinearResample(dst *image.RGBA, src image.Image, x0, y0, cw, ch int) {
	tw, th := dst.Bounds().Dx(), dst.Bounds().Dy()
	for dy := 0; dy < th; dy++ {
		sy := float64(y0) + (float64(dy)+0.5)*float64(ch)/float64(th) - 0.5
		for dx := 0; dx < tw; dx++ {
			sx := float64(x0) + (float64(dx)+0.5)*float64(cw)/float64(tw) - 0.5
			dst.Set(dx, dy, bilinearAt(src, sx, sy))
		}
	}
}

// bilinearAt 在 src 上取 (fx,fy) 处的双线性插值颜色。
func bilinearAt(src image.Image, fx, fy float64) color.Color {
	x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
	tx, ty := fx-float64(x0), fy-float64(y0)
	b := src.Bounds()

	sample := func(x, y int) [4]float64 {
		if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
			return [4]float64{}
		}
		r, g, bl, a := src.At(x, y).RGBA()
		return [4]float64{float64(r), float64(g), float64(bl), float64(a)}
	}
	c00, c10 := sample(x0, y0), sample(x0+1, y0)
	c01, c11 := sample(x0, y0+1), sample(x0+1, y0+1)
	var out [4]float64
	for i := 0; i < 4; i++ {
		top := c00[i]*(1-tx) + c10[i]*tx
		bot := c01[i]*(1-tx) + c11[i]*tx
		out[i] = top*(1-ty) + bot*ty
	}
	return color.RGBA64{
		R: uint16(out[0]), G: uint16(out[1]), B: uint16(out[2]), A: uint16(out[3]),
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
