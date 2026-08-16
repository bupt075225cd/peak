package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"peak/apps/recognition-service/internal/docparse"
)

// SubQuestion 一道大题下的子问，如 (1)(2)(3)。
type SubQuestion struct {
	Label        string `json:"label"`         // 子问标签，如 "(1)"、"（2）"
	Text         string `json:"text"`          // 子问内容
	GeometryRefs []int  `json:"geometry_refs"` // 关联的图片序号（从 1 开始，指向文档内嵌图）
	GeometryDesc string `json:"geometry_desc"` // 该子问对应的几何图形描述
}

// StructuredItem 文档中识别出的一道完整题（含所有子问）。
type StructuredItem struct {
	StemText     string         `json:"stem_text"`     // 题干（含主问题，可能包含全部子问文本）
	SubQuestions []SubQuestion  `json:"sub_questions"` // 子问列表
	Answer       string         `json:"answer"`        // 答案（若模型识别到）
	Geometry     GeometryResult `json:"geometry"`      // 整体几何描述（无子问时使用）
	RawText      string         `json:"-"`             // 原始拼接文本（调试/兜底用，不序列化）
}

// StructuredResult 结构化文档识别结果。
type StructuredResult struct {
	Items     []StructuredItem `json:"items"`
	PageCount int              `json:"page_count"`
	// Images 文档中按出现顺序排列的图片字节（供上层存储，与 geometry_refs 序号对应，从 1 开始）。
	Images [][]byte `json:"-"`
}

// StructuredDocumentProvider 结构化文档识别能力：
// 解析文档 → 逐图 OCR → 合并后交给模型按“大题/子问”结构化拆题。
type StructuredDocumentProvider interface {
	ExtractStructured(ctx context.Context, data []byte, filename string) (*StructuredResult, error)
}

// structuredPrompt 要求模型将合并后的文档内容拆分为结构化题目 JSON。
// 关键约束：
//   - 一行内的 "18." 属于大题号，其后的 "(1)(2)(3)" 属于子问，必须归入同一道题；
//   - 顶部无题号的标题（如“相交线与平行线…”）不是题，应作为题干说明或忽略；
//   - 与子问对应的几何图形写入 geometry_refs 与 geometry_desc。
const structuredPrompt = `你是数学试卷结构化解析器。请将下面的文档内容拆分为多道完整的题目。

文档内容中会以 "[图N]" 标记内嵌图片的位置（N 为图片序号）。请据此判断每个子问关联哪张图。

要求：
1. 每一道“大题”是独立的一道题；大题内部可能包含若干子问，如 "(1)"、"(2)"、"（1）" 等，这些子问必须归入同一道大题，不要拆成多道题。
2. 文档开头的标题、副标题（如“相交线与平行线（角度计算与证明）”“七下第3周周中练习·18题”等）不是题目，不要单独输出为一道题；若它紧邻某道题，可作为该题的说明前缀并入题干。
3. 形如 "18."、"第18题" 的编号属于题号，题号本身与题干正文合并输出。
4. 每道题包含：题干 stem_text（完整正文，包含所有子问的原始文字）、子问列表 sub_questions（每个子问含 label、text、geometry_desc、geometry_refs）。
5. geometry_refs 是整数数组，表示该子问关联的图片序号（对应文档中的 [图N]，从 1 开始）。若子问无关联图片，则为空数组 []。例如子问提到“如图2”，且该图是文档中的第 2 张图，则 geometry_refs 为 [2]。
6. 仅输出如下 JSON 数组，不要输出任何解释或 Markdown 代码块：

[
  {
    "stem_text": "完整题干（含所有子问）",
    "sub_questions": [
      {"label": "(1)", "text": "子问1内容", "geometry_desc": "该子问几何图形描述，若无则为空", "geometry_refs": []}
    ]
  }
]`

// ExtractStructured 三步走实现：
//  1. docparse.Parse 抽取文本与内嵌图片；
//  2. 逐张内嵌图片调用千问-VL 做 OCR（提取文字 + 几何描述）；
//  3. 将文本项与图片 OCR 结果按顺序拼接成索引化文本，再调用一次千问-VL 做结构化拆题。
func (a *AliyunProvider) ExtractStructured(ctx context.Context, data []byte, filename string) (*StructuredResult, error) {
	res, err := docparse.Parse(data, filename)
	if err != nil {
		return nil, err
	}

	// 第一步：按顺序拼出索引化文本；对每张图片调用 OCR；同时收集图片字节。
	var sb strings.Builder
	var images [][]byte
	imgIndex := 0
	for _, it := range res.Items {
		switch it.Kind {
		case "text":
			sb.WriteString(strings.TrimSpace(it.Text))
			sb.WriteString("\n")
		case "image":
			imgIndex++
			images = append(images, it.Image)
			// 对图片 OCR，提取文字 + 几何描述。
			ocrText, geoText := a.ocrImageWithGeometry(ctx, it.Image)
			// 始终写入 [图N] 标记（即使 OCR 为空），便于模型定位图片位置。
			sb.WriteString(fmt.Sprintf("[图%d]\n", imgIndex))
			if ocrText != "" {
				sb.WriteString(ocrText + "\n")
			}
			if geoText != "" {
				sb.WriteString(fmt.Sprintf("几何描述：%s\n", geoText))
			}
		}
	}

	rawText := strings.TrimSpace(sb.String())
	if rawText == "" {
		return nil, fmt.Errorf("document contains no extractable content")
	}

	// 第三步：调用千问-VL 做结构化拆题。
	out, err := a.dash.chatTextOnly(ctx, structuredPrompt+"\n\n文档内容：\n"+rawText)
	if err != nil {
		return nil, fmt.Errorf("structured split: %w", err)
	}

	items, err := parseStructured(out)
	if err != nil {
		// 模型未按 JSON 输出时，退化：整体作为一道题返回，保证可用。
		return &StructuredResult{
			Items:     []StructuredItem{{StemText: rawText, RawText: rawText}},
			PageCount: res.PageCount,
			Images:    images,
		}, nil
	}

	for i := range items {
		items[i].RawText = items[i].StemText
	}
	return &StructuredResult{Items: items, PageCount: res.PageCount, Images: images}, nil
}

// ocrImageWithGeometry 对单张图片调用千问-VL，返回图片内文字与该图几何描述。
func (a *AliyunProvider) ocrImageWithGeometry(ctx context.Context, image []byte) (string, string) {
	prompt := `请识别图片中的全部文字，并判断是否包含几何图形。
若包含几何图形，简要描述图形结构（如“三角形ABC，AB//CD，∠EOF=100°”）。
按如下两行输出（无则留空）：
文字：<图片中的文字>
几何：<几何图形描述，无则留空>`
	out, err := a.dash.chat(ctx, prompt, image)
	if err != nil {
		return "", ""
	}
	text, geo := parseOcrGeo(out)
	return text, geo
}

// parseOcrGeo 解析图片 OCR 返回的“文字/几何”两行结构。
func parseOcrGeo(out string) (string, string) {
	var text, geo string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "文字："):
			text = strings.TrimSpace(strings.TrimPrefix(line, "文字："))
		case strings.HasPrefix(line, "文字:"):
			text = strings.TrimSpace(strings.TrimPrefix(line, "文字:"))
		case strings.HasPrefix(line, "几何："):
			geo = strings.TrimSpace(strings.TrimPrefix(line, "几何："))
		case strings.HasPrefix(line, "几何:"):
			geo = strings.TrimSpace(strings.TrimPrefix(line, "几何:"))
		}
	}
	return text, geo
}

// parseStructured 解析模型返回的结构化 JSON 数组，容错剥离 Markdown 代码块。
func parseStructured(out string) ([]StructuredItem, error) {
	s := strings.TrimSpace(out)
	// 剥离 ```json ... ``` 代码块。
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
		s = strings.TrimSuffix(s, "```")
	}
	s = strings.TrimSpace(s)

	var items []StructuredItem
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// chatTextOnly 纯文本调用千问-VL（无图片），复用 DashScope 客户端。
func (d *DashScopeClient) chatTextOnly(ctx context.Context, prompt string) (string, error) {
	// 复用现有 chat 能力：无图片时传 nil，由 chat 内部跳过 image part。
	return d.chat(ctx, prompt, nil)
}

// questionNoRe 题号前缀正则（provider 包内 mock 与兜底使用）。
var questionNoRe = regexp.MustCompile(`^[\s]*(\d{1,3})[.、)）]|^第[\s]*(\d{1,3})[\s]*题|^[（(][\s]*(\d{1,3})[\s]*[)）]`)
