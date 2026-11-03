package service

// 几何区域图片预处理：约束提取的 VLM 输入瘦身，并保证图形完整性。
// 仅对"请求发送给大模型的图片"做裁剪+下采样（不落库、不改产物），
// 可显著降低 vision token，从而降低单次调用延迟。

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"strings"

	"peak/apps/recognition-service/internal/provider"
)

// geometryRegionMaxDim 约束提取输入小图的最大边长（像素）。
const geometryRegionMaxDim = 960

// geometryGridRegionMaxDim 网格图（小正方形网格/方格纸）的输入最大边长。
// 网格线是很淡的印刷虚线，缩到 960 宽会被插值糊掉，模型数格子会在 3~6 列之间漂移；
// 提高分辨率后同一张图能稳定数对行列数（实测 3 列 2 行）。
// 只对网格图放宽，普通几何图仍用 960 以控制 vision token。
const geometryGridRegionMaxDim = 1600

// isGridFigure 判断题目/图形描述里是否出现网格（用于提高提取输入的分辨率）。
func isGridFigure(texts ...string) bool {
	for _, t := range texts {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if strings.Contains(t, "网格") || strings.Contains(t, "方格") || strings.Contains(t, "格点") {
			return true
		}
	}
	return false
}

const (
	// regionGridMaxDim 墨迹分析网格的最长边：越小越快，边界精度越低（320 对 2000+ 像素图约 1/7~1/17）。
	regionGridMaxDim = 320
	// regionInkLevel 灰度不大于该值视为墨迹（纸面、浅灰底纹、淡阴影不算）。
	regionInkLevel = 175
	// regionPadRatio bbox 四周先外扩的比例（模型框普遍偏紧）。
	regionPadRatio = 0.12
	// regionMarginRatio 连通域并集外再留的边距（占并集尺寸比例）。
	regionMarginRatio = 0.03
	// regionMinUnionRatio 并集小于原框该比例时认为模型框更可靠，退回原框。
	regionMinUnionRatio = 0.15
	// regionMaxImageRatio 并集超过整图该比例时回退整图，避免把整页噪声都并进来。
	regionMaxImageRatio = 0.92
)

// errRegionInvalid bbox 无效或图片无法解析。
var errRegionInvalid = errors.New("invalid geometry region")

// cropAndScaleRegion 按归一化 bbox 从原图中裁剪出几何区域，并等比缩放到
// 最长边不超过 maxDim（小于 maxDim 则保持原尺寸）。返回 JPEG 字节；
// 失败返回 error，由调用方决定回退使用整张原图。
func cropAndScaleRegion(src []byte, bbox *provider.BoundingBox, maxDim int) ([]byte, error) {
	out, _, err := cropAndScaleRegionPlanned(src, bbox, maxDim)
	return out, err
}

// cropAndScaleRegionPlanned 与 cropAndScaleRegion 相同，但额外返回区域规划结果，
// 便于调用方记录日志：
//   - ""       规划框与模型框基本一致；
//   - "expand" 用墨迹连通域补全了被模型框切断的图形；
//   - "base"   并集过小，退回模型框；
//   - "full"   框内没有墨迹或并集过大，回退整张原图。
func cropAndScaleRegionPlanned(src []byte, bbox *provider.BoundingBox, maxDim int) ([]byte, string, error) {
	if bbox == nil || bbox.Width <= 0 || bbox.Height <= 0 {
		return nil, "", errRegionInvalid
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, "", err
	}
	rect, note := planGeometryRegion(img, bbox)
	out, err := encodeRegion(img, rect, maxDim)
	if err != nil {
		return nil, note, err
	}
	return out, note, nil
}

// planGeometryRegion 规划最终裁剪区域（像素坐标）。
// 核心思路：模型给的 bbox 只当作"种子"，真正的裁剪范围取与种子相交的
// 墨迹连通域的并集——这样被 bbox 边界切断的图形（例如轴、边、端点）会被
// 完整补进来；框内没有任何墨迹时回退整图。
func planGeometryRegion(img image.Image, bbox *provider.BoundingBox) (image.Rectangle, string) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return b, "full"
	}

	base := paddedRect(b, bbox)
	if base.Empty() {
		return b, "full"
	}

	grid := buildInkGrid(img)
	union, touch, expanded := grid.componentUnion(base)
	if !touch {
		// 模型框里一点墨迹都没有：bbox 不可信，整图喂给模型更安全。
		return b, "full"
	}
	if !expanded {
		// 与框相交的墨迹都在框内：图形没有被切断，保持模型框（仅外扩 12%）。
		return base, ""
	}

	mx := int(math.Round(float64(union.Dx()) * regionMarginRatio))
	my := int(math.Round(float64(union.Dy()) * regionMarginRatio))
	union = image.Rect(union.Min.X-mx, union.Min.Y-my, union.Max.X+mx, union.Max.Y+my).Intersect(b)
	if union.Empty() {
		return b, "full"
	}

	baseArea := areaOf(base)
	unionArea := areaOf(union)
	if unionArea > int64(float64(w*h)*regionMaxImageRatio) {
		return b, "full"
	}
	if baseArea > 0 && unionArea < int64(float64(baseArea)*regionMinUnionRatio) {
		return base, "base"
	}
	return union, "expand"
}

// paddedRect 把归一化 bbox 换算成像素框，并四周外扩 regionPadRatio。
func paddedRect(b image.Rectangle, bbox *provider.BoundingBox) image.Rectangle {
	w, h := float64(b.Dx()), float64(b.Dy())
	x0 := bbox.X * w
	y0 := bbox.Y * h
	x1 := (bbox.X + bbox.Width) * w
	y1 := (bbox.Y + bbox.Height) * h
	padX := (x1 - x0) * regionPadRatio
	padY := (y1 - y0) * regionPadRatio
	r := image.Rect(
		int(math.Floor(x0-padX)), int(math.Floor(y0-padY)),
		int(math.Ceil(x1+padX)), int(math.Ceil(y1+padY)),
	)
	return r.Add(b.Min).Intersect(b)
}

func areaOf(r image.Rectangle) int64 {
	if r.Empty() {
		return 0
	}
	return int64(r.Dx()) * int64(r.Dy())
}

// inkGrid 原图的降采样"是否有墨迹"网格：格内只要有一个墨点就算墨迹，
// 细线（1~2px）不会因为降采样而丢失。
type inkGrid struct {
	minX, minY int
	step       int
	w, h       int
	dark       []bool
}

func buildInkGrid(img image.Image) *inkGrid {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	step := 1
	for (w+step-1)/step > regionGridMaxDim || (h+step-1)/step > regionGridMaxDim {
		step++
	}
	gw, gh := (w+step-1)/step, (h+step-1)/step
	g := &inkGrid{minX: b.Min.X, minY: b.Min.Y, step: step, w: gw, h: gh, dark: make([]bool, gw*gh)}

	at := grayReader(img)
	for y := 0; y < h; y++ {
		gy := y / step
		row := gy * gw
		for x := 0; x < w; x++ {
			if at(b.Min.X+x, b.Min.Y+y) <= regionInkLevel {
				g.dark[row+x/step] = true
			}
		}
	}
	return g
}

// componentUnion 分析 rect 内的墨迹连通域（8 连通）：
//   - touch：是否有连通域与 rect 相交（没有说明 bbox 完全落在空白处）；
//   - expanded：是否有连通域伸出 rect 边界（伸出说明图形被框切断，需要补全）。
//
// 只有伸出边界的连通域才参与并集，完全落在框内的墨迹（例如题干文字）
// 不会把裁剪范围撑大；返回值为补全后的像素坐标。
func (g *inkGrid) componentUnion(rect image.Rectangle) (union image.Rectangle, touch, expanded bool) {
	gx0 := clampInt((rect.Min.X-g.minX)/g.step, 0, g.w)
	gy0 := clampInt((rect.Min.Y-g.minY)/g.step, 0, g.h)
	gx1 := clampInt((rect.Max.X-g.minX+g.step-1)/g.step, 0, g.w)
	gy1 := clampInt((rect.Max.Y-g.minY+g.step-1)/g.step, 0, g.h)
	if gx1 <= gx0 || gy1 <= gy0 {
		return image.Rectangle{}, false, false
	}

	visited := make([]bool, g.w*g.h)
	stack := make([]int, 0, 1024)

	for gy := 0; gy < g.h; gy++ {
		for gx := 0; gx < g.w; gx++ {
			idx := gy*g.w + gx
			if !g.dark[idx] || visited[idx] {
				continue
			}
			visited[idx] = true
			stack = append(stack[:0], idx)
			minX, minY, maxX, maxY := gx, gy, gx, gy
			inRect := false
			for len(stack) > 0 {
				cur := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				cx, cy := cur%g.w, cur/g.w
				if cx < minX {
					minX = cx
				}
				if cy < minY {
					minY = cy
				}
				if cx > maxX {
					maxX = cx
				}
				if cy > maxY {
					maxY = cy
				}
				if cx >= gx0 && cx < gx1 && cy >= gy0 && cy < gy1 {
					inRect = true
				}
				for dy := -1; dy <= 1; dy++ {
					ny := cy + dy
					if ny < 0 || ny >= g.h {
						continue
					}
					for dx := -1; dx <= 1; dx++ {
						if dx == 0 && dy == 0 {
							continue
						}
						nx := cx + dx
						if nx < 0 || nx >= g.w {
							continue
						}
						ni := ny*g.w + nx
						if g.dark[ni] && !visited[ni] {
							visited[ni] = true
							stack = append(stack, ni)
						}
					}
				}
			}
			if !inRect {
				continue
			}
			touch = true
			if minX >= gx0 && minY >= gy0 && maxX < gx1 && maxY < gy1 {
				continue // 整个连通域都在框内：没有切断，不需要扩张
			}
			comp := image.Rect(minX, minY, maxX+1, maxY+1)
			if !expanded {
				union = comp
				expanded = true
			} else {
				union = union.Union(comp)
			}
		}
	}
	if !expanded {
		return image.Rectangle{}, touch, false
	}
	return image.Rect(
		g.minX+union.Min.X*g.step, g.minY+union.Min.Y*g.step,
		g.minX+union.Max.X*g.step, g.minY+union.Max.Y*g.step,
	), touch, true
}

// grayReader 返回按像素取灰度的函数；常见图像类型走快路径，其余回退 At()。
func grayReader(img image.Image) func(x, y int) uint8 {
	switch m := img.(type) {
	case *image.Gray:
		return func(x, y int) uint8 { return m.Pix[m.PixOffset(x, y)] }
	case *image.YCbCr:
		return func(x, y int) uint8 { return m.Y[m.YOffset(x, y)] }
	case *image.NRGBA:
		return func(x, y int) uint8 {
			i := m.PixOffset(x, y)
			r, g, b := int(m.Pix[i]), int(m.Pix[i+1]), int(m.Pix[i+2])
			return uint8((299*r + 587*g + 114*b) / 1000)
		}
	case *image.RGBA:
		return func(x, y int) uint8 {
			i := m.PixOffset(x, y)
			if m.Pix[i+3] < 128 {
				return 255
			}
			r, g, b := int(m.Pix[i]), int(m.Pix[i+1]), int(m.Pix[i+2])
			return uint8((299*r + 587*g + 114*b) / 1000)
		}
	default:
		return func(x, y int) uint8 {
			return color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y
		}
	}
}

// encodeRegion 把 img 的 rect 区域等比缩放到最长边 maxDim 并编码为 JPEG。
func encodeRegion(img image.Image, rect image.Rectangle, maxDim int) ([]byte, error) {
	rect = rect.Intersect(img.Bounds())
	if rect.Empty() {
		return nil, errRegionInvalid
	}
	cw, ch := rect.Dx(), rect.Dy()
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
		copyRegion(out, img, rect.Min.X, rect.Min.Y, cw, ch)
	} else {
		bilinearResample(out, img, rect.Min.X, rect.Min.Y, cw, ch)
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
