package provider

import "context"

// RedrawConstraintResidual 单条约束的残差明细。
//
// Deprecated: 仅服务于已停用的 geometry-sidecar（Python 约束求解）残差报告，
// 新链路（内置 Go 渲染，坐标直出）不再产生残差。保留类型以维持 sidecar 源码可编译。
type RedrawConstraintResidual struct {
	Index    int     `json:"index"`
	Type     string  `json:"type"`
	Severity string  `json:"severity"` // hard / soft
	Residual float64 `json:"residual"`
}

// GeometryRedrawReport sidecar 求解报告（分层残差）。
//
// Deprecated: 仅服务于已停用的 geometry-sidecar，保留类型以维持其源码可编译。
type GeometryRedrawReport struct {
	MaxHard       float64                    `json:"max_hard"`
	MaxSoft       float64                    `json:"max_soft"`
	MaxAll        float64                    `json:"max_all"`
	PerConstraint []RedrawConstraintResidual `json:"per_constraint,omitempty"`
}

// RedrawSVGItem 一张重绘输出（一个子图/panel 对应一张独立 SVG）。
type RedrawSVGItem struct {
	Title string // 子图标题（如"图1"，可能为空）
	Data  []byte // SVG 字节
}

// GeometryRedrawResult 几何重绘结果。
// 一张原图可能含多个几何子图（图1/图2/图3），每个子图独立渲染为一张 SVG。
//
// Deprecated: 仅服务于已停用的 geometry-sidecar，保留类型以维持其源码可编译。
type GeometryRedrawResult struct {
	SVGs   []RedrawSVGItem       // 逐子图渲染出的独立 SVG 列表
	Report *GeometryRedrawReport // 求解残差报告（多子图时为各 panel 的最大值）
}

// GeometryRedrawProvider 几何重绘能力：约束求解（最小二乘）+ SVG 渲染。
// 原由 geometry-sidecar（Python FastAPI）承载，Go 侧仅做 HTTP 调用。
//
// Deprecated: sidecar 已停用（源码保留在 apps/geometry-sidecar 以便回滚），
// 新链路由 service 层直接调用 internal/geom 渲染，不再经过该接口。
type GeometryRedrawProvider interface {
	// RedrawGeometry 输入约束 spec JSON，返回逐子图渲染的独立 SVG 列表与残差报告。
	RedrawGeometry(ctx context.Context, spec string) (*GeometryRedrawResult, error)
}

// GeometrySpecExtractor 几何描述提取能力：把几何子图 + 题干文本翻译为
// 坐标直出的几何描述 JSON（schema 见 internal/geom 的 Spec 与 panels 包装）。
// correction 非空时表示上一轮输出存在结构问题，需据此修正后重新输出完整 JSON。
type GeometrySpecExtractor interface {
	ExtractGeometrySpec(ctx context.Context, geoImage []byte, stemText, correction string) (string, error)
}
