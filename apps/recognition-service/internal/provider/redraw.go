package provider

import "context"

// GeometrySpecExtractor 几何描述提取能力：把几何子图 + 题干文本翻译为
// 坐标直出的几何描述 JSON（schema 见 internal/geom 的 Spec 与 panels 包装）。
// correction 非空时表示上一轮输出存在结构问题，需据此修正后重新输出完整 JSON。
type GeometrySpecExtractor interface {
	ExtractGeometrySpec(ctx context.Context, geoImage []byte, stemText, correction string) (string, error)
}
