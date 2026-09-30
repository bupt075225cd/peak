package provider

import "context"

// GeometrySpecExtractor 几何描述提取能力：把几何子图 + 题干文本翻译为
// 坐标直出的几何描述 JSON（schema 见 internal/geom 的 Spec 与 panels 包装）。
// correction 非空时表示上一轮输出存在结构问题，需据此修正后重新输出完整 JSON。
type GeometrySpecExtractor interface {
	ExtractGeometrySpec(ctx context.Context, geoImage []byte, stemText, correction string) (string, error)
}

// AngleMarkVerifier 原图角弧线标记核对能力（可选实现）。
// 角弧线要求"忠实原图"：VLM 提取 spec 时可能臆造 angle_marks，因此对声称
// 含角标记的子图，用同一次输入的原图让 VLM 逐子图二次核对；未确认的一律剔除。
// 返回 map：子图标题（如"图1"）→ 原图中是否确实画有角的弧线标记。
type AngleMarkVerifier interface {
	VerifyAngleMarks(ctx context.Context, geoImage []byte, titles []string) (map[string]bool, error)
}
