package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"peak/apps/recognition-service/internal/docparse"
)

// vlmCapabilities 基于任意 OpenAI 兼容多模态对话客户端（ChatClient）实现的
// 通用 VLM 能力集合：整题解析、OCR、几何识别、文档解析、结构化拆题、几何描述提取。
// 由 aliyun（通义千问-VL）与 zhipu（智谱 GLM）等 provider 内嵌复用，
// 各厂商只需提供 ChatClient 配置与自身特有的能力（如阿里云的手写擦除）。
type vlmCapabilities struct {
	dash *ChatClient
	// geometryEffort 几何描述提取专用的思考强度覆盖（如 high）；空表示沿用客户端默认。
	// 几何描述提取是整条链路里对推理要求最高的一步（看图直出坐标），
	// 低强度下模型经常漏画直线、点跑到线外，需要更高的思考强度兜底。
	geometryEffort string
}

// specChat 几何描述提取用的对话客户端：配置了独立思考强度时返回覆盖副本。
func (v *vlmCapabilities) specChat() *ChatClient {
	if v.geometryEffort == "" {
		return v.dash
	}
	return v.dash.withEffort(v.geometryEffort)
}

// RecognizeText 调用多模态大模型提取图片中的全部文字（文档/子图 OCR 场景使用）。
func (v *vlmCapabilities) RecognizeText(ctx context.Context, image []byte) (*TextResult, error) {
	prompt := "请识别图片中的全部文字内容，按阅读顺序原样输出，不要添加任何解释。"
	out, err := v.dash.chat(ctx, prompt, image)
	if err != nil {
		return nil, err
	}
	return &TextResult{Text: out, Confidence: 0.9}, nil
}

// ParseQuestion 一次 VLM 调用完成整题解析：题干文本（OCR）+ 学科 + 题型 + 题干表格。
func (v *vlmCapabilities) ParseQuestion(ctx context.Context, image []byte) (*QuestionParseResult, error) {
	prompt := `识别图片中的这道题目，仅输出如下 JSON（不要 Markdown 代码块、不要解释）：
{"text":"题干原文（按阅读顺序原样输出，不要修改、不要省略）","subject":"数学|语文|英语|物理|化学","question_type":"选择题|填空题|解答题","tables":[{"id":"table1","headers":["列1","列2"],"rows":[["值1","值2"]]}]}

要求：
- text 必须包含题目的完整题干文字（含数字与单位，若图中仅有图形则输出空字符串）。
- text 保留原图的换行版式：原图中哪里换行，text 就在哪里换行（JSON 字符串中用 \n 表示）；不要把多行合并为一行，也不要增加原图没有的换行。
- 题干中有表格（分档计费表、数据表、对照表、统计表等）时，必须把表格从 text 中摘出来：
  在表格原来的位置只写占位符 {{table1}}（第二张表用 {{table2}}，依次递增），表格内容按行列填进 tables。
  headers 是表头行（原图没有表头时给空数组 []），rows 是数据行；每个单元格一个字符串，
  不要把两列塞进同一个单元格，也不要漏行漏列或改动单元格里的数字、不等号、单位。
- 图中没有表格时，tables 输出 []，且 text 中不出现 {{table 占位符。
- subject 只能从 数学/语文/英语/物理/化学 中选择；question_type 只能从 选择题/填空题/解答题 中选择。不确定时用最接近的值。`
	out, err := v.dash.chat(ctx, prompt, image)
	if err != nil {
		return nil, err
	}
	return parseQuestion(out), nil
}

// parseQuestion 解析整题解析 JSON；容错非 JSON 输出时把原文整体当作题干文本。
func parseQuestion(out string) *QuestionParseResult {
	obj, err := extractJSON(out)
	if err != nil {
		// 模型未按 JSON 输出：整段文本作为题干，学科/题型留空交给规则判断兜底。
		return &QuestionParseResult{Text: strings.TrimSpace(out)}
	}
	var r struct {
		Text         string  `json:"text"`
		Subject      string  `json:"subject"`
		QuestionType string  `json:"question_type"`
		Tables       []Table `json:"tables"`
	}
	if err := json.Unmarshal([]byte(obj), &r); err != nil {
		return &QuestionParseResult{Text: strings.TrimSpace(out)}
	}
	res := &QuestionParseResult{
		Subject:      strings.TrimSpace(r.Subject),
		QuestionType: strings.TrimSpace(r.QuestionType),
	}
	res.Text, res.Tables = normalizeTables(normalizeTranscript(r.Text), r.Tables)
	if res.Text == "" {
		res.Text = normalizeTranscript(out)
	}
	return res
}

// geometryNoneKeywords 几何描述中出现这些表述时，视为模型自述"没有几何图形"。
// 模型偶尔一边描述"无几何图形/只有残缺碎片"一边仍返回 bounding_box（自相矛盾），
// 此时以文字描述为准丢弃 bbox，避免对纯文字题触发重绘。
var geometryNoneKeywords = []string{
	"无几何图形", "没有几何图形", "无完整几何图形", "没有完整几何",
	"无几何图", "没有几何图", "不是几何图", "无示意图", "没有示意图",
	"纯文字", "只有文字", "仅有文字", "只有题目文字", "残缺",
}

// descriptionSaysNoGeometry 判断几何描述是否自述"没有几何图形"。
func descriptionSaysNoGeometry(desc string) bool {
	for _, kw := range geometryNoneKeywords {
		if strings.Contains(desc, kw) {
			return true
		}
	}
	return false
}

// RecognizeGeometry 调用多模态大模型输出结构化几何描述（JSON），并附带几何图形在原图中的位置框。
func (v *vlmCapabilities) RecognizeGeometry(ctx context.Context, image []byte) (*GeometryResult, error) {
	prompt := `识别图片中的几何图形，仅输出如下 JSON（不要输出其他内容、不要用 Markdown 代码块）：
{"shape_type":"triangle|circle|quadrilateral|axes|function_graph|other","properties":{"边长":"","角度":"","位置关系":""},"description":"对图形的文字描述","rotation":0,"bounding_box":{"x":0.1,"y":0.2,"width":0.5,"height":0.4},"line_labels":["l1","l2"]}

其中 line_labels **必须输出**，它是**原图上印刷出来的直线名称**（图上直接写着 l₁、l₂、l₃ 这类标注），
按图上实际印的写，如 ["l1","l2"]；图上没有印任何直线名称就写 []（空数组也要写出来）。
注意：题干文字里出现"直线 l₁""图象 l₂"只是代号，不算原图印刷——只认图形区域里真实印着的标注。
其中 rotation 表示图片内容相对正立状态顺时针倾斜了多少度，只能是数字 0/90/180/270。
判断方法：题干文字需要逆时针转多少度才能读正，就填多少度。
例如手机竖拍的图里文字侧躺、需要逆时针转 90 度才读正，填 90；需要逆时针转 270 度才读正，填 270；图片本来就是正的填 0。
其中 bounding_box 是几何图形（仅图形本身，不含题目文字）在原图中的外接矩形，坐标采用归一化（0~1），x/y 为左上角，width/height 为宽高。
重要：若图中有多块几何图形（例如多个子图、图1/图2/图3），请输出包含「所有几何图形整体」的最大外接矩形，确保完整覆盖每一个图，不要只框出其中一部分。适度宽松即可，不要缩得太紧。
判定标准：只有与题目对应、完整可辨认的几何示意图（由点、线段、角、圆、弧、多边形等构成）才算有几何图形。
平面直角坐标系、坐标轴、函数图像（抛物线/双曲线/直线型函数）同样属于几何图形：
只要图中有带箭头的 x 轴、y 轴或函数曲线，即使线条很细、看起来像表格或网格，也必须返回 bounding_box，shape_type 取 axes 或 function_graph，严禁判为 null。
以下情况一律视为没有几何图形，bounding_box 必须设为 null：
- 图片只有题目文字、公式、代数式或表格；
- 只有残缺不全的局部线条或图形碎片（例如顶部残留一小段弧线、半条边）；
- 图形属于旁边/上一题，与本题无关（以题干文字为准）。
宁可返回 null，也不要把文字、公式或残缺碎片误判为几何图形。若判定没有几何图形，description 写明"无几何图形"。`
	out, err := v.dash.chat(ctx, prompt, image)
	if err != nil {
		return nil, err
	}
	return parseGeometry(out), nil
}

// DetectOrientation 判断图片内容的倾斜角（0/90/180/270），供调用方把整图转正。
// 方向定义与 RecognizeGeometry 的 rotation 字段保持一致：填"文字需要逆时针转多少度才读正"。
func (v *vlmCapabilities) DetectOrientation(ctx context.Context, image []byte) (Rotation, error) {
	prompt := `判断图片内容相对正立状态顺时针倾斜的角度，只输出 JSON：{"rotation":0}（只能是 0/90/180/270，不要解释、不要 Markdown）。
- 文字水平、从左到右读得通 → 0；文字水平但上下颠倒 → 180；
- 文字竖直、字的顶部朝左（需逆时针转 90°）→ 90；字的顶部朝右 → 270。
- 90/270 易混，按"字的顶部朝哪边"定，不要凭手感。
- 文字很少时，按字母标记、坐标轴箭头的朝向判断。`
	out, err := v.dash.chat(ctx, prompt, image)
	if err != nil {
		return 0, err
	}
	return parseOrientation(out), nil
}

// IsUpright 判断图片里的文字是否已经正立、可以直接阅读（不需要旋转）。
func (v *vlmCapabilities) IsUpright(ctx context.Context, image []byte) (bool, error) {
	prompt := `只判断这张图片里的文字现在是否正立、可以直接正常阅读（不需要歪头、不需要旋转）。
仅输出如下 JSON（不要解释、不要 Markdown 代码块）：{"upright":true}
说明：
- 文字行水平、从左到右读得通，题号在左上角 → true；
- 文字侧躺（需要把图片转一下才能读）、上下颠倒、或每列文字需要歪头读 → false；
- 图中几乎没有文字时，按字母标记（A、B、C）、坐标轴箭头、公式的朝向判断：正着写 → true。`
	out, err := v.dash.chat(ctx, prompt, image)
	if err != nil {
		return false, err
	}
	return parseUpright(out), nil
}

// parseUpright 解析正立判断结果；兼容 true/"true"，无法解析时按 false（交给调用方回退）。
func parseUpright(out string) bool {
	obj, err := extractJSON(out)
	if err != nil {
		return false
	}
	var r struct {
		Upright any `json:"upright"`
	}
	if err := json.Unmarshal([]byte(obj), &r); err != nil {
		return false
	}
	switch v := r.Upright.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

// parseOrientation 解析方向探测输出；非 JSON 或非法角度按 0（正立）处理，
// 方向判断失败不应该让识别链路报错。
func parseOrientation(out string) Rotation {
	obj, err := extractJSON(out)
	if err != nil {
		return 0
	}
	var r struct {
		Rotation Rotation `json:"rotation"`
	}
	if err := json.Unmarshal([]byte(obj), &r); err != nil {
		return 0
	}
	return NormalizeRotation(r.Rotation.Degrees())
}

// parseGeometry 解析几何识别返回的几何 JSON（容错处理非 JSON 输出）。
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
	// 防御 bbox 与描述自相矛盾：描述明确说没有几何图形时，以文字为准丢弃 bbox，
	// 避免纯文字/残缺碎片题触发重绘。
	if g.BoundingBox != nil && descriptionSaysNoGeometry(g.Description) {
		g.BoundingBox = nil
	}
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

// ExtractDocument 文档识别：本地解析 word/pdf 提取文本与内嵌图片，
// 图片项复用多模态大模型做视觉理解，产出结构化文本。
func (v *vlmCapabilities) ExtractDocument(ctx context.Context, data []byte, filename string) (*DocumentResult, error) {
	res, err := docparse.Parse(data, filename)
	if err != nil {
		return nil, err
	}

	items := make([]DocumentItem, 0, len(res.Items))
	for _, it := range res.Items {
		switch it.Kind {
		case "text":
			items = append(items, DocumentItem{Kind: "text", Text: it.Text})
		case "image":
			// 对文档内嵌图片调用多模态大模型做 OCR，提取图片中的题目文本。
			text, err := v.ocrImageText(ctx, it.Image)
			if err != nil {
				// 图片 OCR 失败时保留图片项，由上层降级处理。
				items = append(items, DocumentItem{Kind: "image", Image: it.Image})
				continue
			}
			items = append(items, DocumentItem{Kind: "text", Text: text})
		}
	}

	return &DocumentResult{Items: items, PageCount: res.PageCount}, nil
}

// ocrImageText 调用多模态大模型识别图片中的文字/题目内容。
func (v *vlmCapabilities) ocrImageText(ctx context.Context, image []byte) (string, error) {
	prompt := "识别图片中的全部文字与题目内容，按顺序输出纯文本，保留公式与几何描述。"
	out, err := v.dash.chat(ctx, prompt, image)
	if err != nil {
		return "", fmt.Errorf("vlm ocr: %w", err)
	}
	return out, nil
}
