package provider

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// AliyunProvider 阿里云识别能力组合实现（方案 B：统一使用通义千问-VL）。
// 文字/公式/几何识别全部走 DashScope 千问-VL 多模态大模型，
// 手写擦除走通义万相图像编辑（wanx2.1-imageedit）异步任务。
type AliyunProvider struct {
	dash *DashScopeClient
	wanx *WanxClient
}

// AliyunConfig 阿里云 provider 配置。
type AliyunConfig struct {
	// 保留字段以兼容历史配置（方案 B 不再使用传统 OCR）。
	AccessKeyID  string
	AccessSecret string
	OCREndpoint  string
	// 千问-VL 配置。
	DashKey      string // DashScope API Key
	DashModel    string // 千问-VL 模型名，默认 qwen-vl-max
	DashEndpoint string // DashScope API endpoint（可注入，便于测试）
	// 万相图像编辑配置（手写擦除）。
	WanxModel    string // 万相图像编辑模型名，默认 wanx2.1-imageedit
	WanxEndpoint string // 万相接口基地址（可注入，便于测试）
}

// NewAliyunProvider 创建阿里云 provider。
func NewAliyunProvider(cfg AliyunConfig) *AliyunProvider {
	return &AliyunProvider{
		dash: NewDashScopeClient(DashScopeConfig{
			APIKey:   cfg.DashKey,
			Model:    cfg.DashModel,
			Endpoint: cfg.DashEndpoint,
		}),
		wanx: NewWanxClient(cfg.DashKey, cfg.WanxModel, cfg.WanxEndpoint),
	}
}

func (a *AliyunProvider) Name() string { return "aliyun" }

// RecognizeText 调用千问-VL 提取图片中的全部文字（含题目）。
func (a *AliyunProvider) RecognizeText(ctx context.Context, image []byte) (*TextResult, error) {
	prompt := "请识别图片中的全部文字内容，按阅读顺序原样输出，不要添加任何解释。"
	out, err := a.dash.chat(ctx, prompt, image)
	if err != nil {
		return nil, err
	}
	return &TextResult{Text: out, Confidence: 0.9}, nil
}

// RecognizeFormula 调用千问-VL 识别图片中的公式并输出 LaTeX。
func (a *AliyunProvider) RecognizeFormula(ctx context.Context, image []byte) (*FormulaResult, error) {
	prompt := "请识别图片中的数学公式，仅输出对应的 LaTeX 表达式，不要添加任何其他内容。若图片中没有公式，输出空。"
	out, err := a.dash.chat(ctx, prompt, image)
	if err != nil {
		return nil, err
	}
	return &FormulaResult{LaTeX: out, RawText: out}, nil
}

// RecognizeGeometry 调用千问-VL 输出结构化几何描述（JSON），并附带几何图形在原图中的位置框。
func (a *AliyunProvider) RecognizeGeometry(ctx context.Context, image []byte) (*GeometryResult, error) {
	prompt := `识别图片中的几何图形，仅输出如下 JSON（不要输出其他内容、不要用 Markdown 代码块）：
{"shape_type":"triangle|circle|quadrilateral|other","properties":{"边长":"","角度":"","位置关系":""},"description":"对图形的文字描述","bounding_box":{"x":0.1,"y":0.2,"width":0.5,"height":0.4}}

其中 bounding_box 是几何图形（仅图形本身，不含题目文字）在原图中的外接矩形，坐标采用归一化（0~1），x/y 为左上角，width/height 为宽高。
重要：若图中有多块几何图形（例如多个子图、图1/图2/图3），请输出包含「所有几何图形整体」的最大外接矩形，确保完整覆盖每一个图，不要只框出其中一部分。适度宽松即可，不要缩得太紧。若图片中没有几何图形，bounding_box 设为 null。`
	out, err := a.dash.chat(ctx, prompt, image)
	if err != nil {
		return nil, err
	}
	return parseGeometry(out), nil
}

// EraseHandwriting 调用通义万相图像编辑擦除手写笔迹，返回擦除后的图片字节。
// 万相为异步任务（提交→轮询→下载），此处设置整体 deadline 约 150s，避免无限等待。
func (a *AliyunProvider) EraseHandwriting(ctx context.Context, image []byte) (*ErasureResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()

	data, err := a.wanx.EraseHandwriting(ctx, image)
	if err != nil {
		return nil, err
	}
	return &ErasureResult{ImageData: data}, nil
}

// parseGeometry 解析千问-VL 返回的几何 JSON（容错处理非 JSON 输出）。
func parseGeometry(out string) *GeometryResult {
	var g GeometryResult
	if err := json.Unmarshal([]byte(out), &g); err != nil {
		// 若模型未按 JSON 输出，将原文放入 description 兜底。
		return &GeometryResult{
			ShapeType:   "unknown",
			Properties:  map[string]string{},
			Description: out,
		}
	}
	g.BoundingBox = normalizeBBox(g.BoundingBox)
	return &g
}

// normalizeBBox 规范化几何图形外接矩形：
//   - 越界/非法坐标（<0 或 >1、宽高非正、x+width>1 等）视为无效，返回 nil。
//   - 有效时 clamp 到 [0,1] 区间，保证服务端裁剪安全。
func normalizeBBox(b *BoundingBox) *BoundingBox {
	if b == nil {
		return nil
	}
	if b.Width <= 0 || b.Height <= 0 {
		return nil
	}
	clamp01 := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	x := clamp01(b.X)
	y := clamp01(b.Y)
	w := clamp01(b.Width)
	h := clamp01(b.Height)
	// 宽高越界修正：不能超出图片范围。
	if x+w > 1 {
		w = 1 - x
	}
	if y+h > 1 {
		h = 1 - y
	}
	if w <= 0 || h <= 0 {
		return nil
	}
	return &BoundingBox{X: x, Y: y, Width: w, Height: h}
}

// ClassifyQuestion 调用千问-VL 判断题目所属学科与题型。
func (a *AliyunProvider) ClassifyQuestion(ctx context.Context, image []byte) (*QuestionClassifyResult, error) {
	prompt := `请判断这道题目的学科和题型。
学科只能从以下选项中选择：数学、语文、英语、物理、化学。
题型只能从以下选项中选择：选择题、填空题、解答题。
仅输出如下 JSON（不要 Markdown 代码块、不要解释）：
{"subject":"数学","question_type":"解答题"}`
	out, err := a.dash.chat(ctx, prompt, image)
	if err != nil {
		return nil, err
	}
	return parseClassify(out), nil
}

// parseClassify 解析千问-VL 返回的学科/题型 JSON，容错非 JSON 输出。
func parseClassify(out string) *QuestionClassifyResult {
	var r QuestionClassifyResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &r); err != nil {
		return &QuestionClassifyResult{Subject: "", QuestionType: ""}
	}
	return &r
}
