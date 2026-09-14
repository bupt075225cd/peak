package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// GeometrySpecSystemPrompt 几何描述提取的系统提示词。
//
// 与旧方案（输出约束 → sidecar 数值求解反解坐标）不同，本提示词要求模型
// **直接给出各点坐标与图元结构**，由服务内 Go 渲染器（internal/geom）绘制 SVG，
// 不再做任何数值求解。因此图形度量不保证数学精确，以"比例协调、不违背题意"为准。
// schema 对齐 internal/geom 的 Spec 类型与多子图 panels 包装。
const GeometrySpecSystemPrompt = `你是初中数学几何配图解析器：看图与题干，输出"可直接绘制"的几何描述 JSON（坐标直出，不需要求解）。
你的输入是"几何区域图 + 题干文字"。仅输出一个 JSON 对象，不解释、不用 Markdown 代码块包裹。

输出结构：
- 只有 1 张几何图时：{"title":"图1","canvas":{"width":100,"height":70},"points":[...],"segments":[...]}
- 题目含"图1/图2/图3"等多个子图时：{"panels":[{每个子图各自一份上述结构，独立坐标系},...]}
  （严禁把多个子图的点塞进同一套 points。）

字段说明：
- canvas：画布尺寸，建议 100x100；宽扁图可用 height=60~80，竖高图可用 height=120~140。
- points：所有点的坐标，name 用 A、B、C、O 等。坐标系与 SVG 一致：画布左上角为原点，x 向右，y 向下。
  坐标保留 1~2 位小数即可；图形整体居中，四周留 8~15 个单位空白。
  纯构造点（只用于连线、不画实心点）可设 "hidden": true。
- segments：线段，from/to 必须是同一张图 points 中已有的 name；dashed 表示虚线（辅助线）；
  extend 表示向两端延长（用于表达直线/射线）。
- polygons：多边形，points 为点名数组，fill 为 true 时淡色填充。
- circles：圆，center 为圆心点名；radius 与 through 二选一（through 指圆上一点）。
- arcs：圆弧，radius 必填；角度单位为度，0° 指向 x 轴正方向，角度增大方向与 y 轴正向一致。
- right_angles：直角标记，vertex 为直角顶点，a/b 为两条边上的点。
- angle_marks：角的弧线标记，vertex/a/b 为角的顶点与两条边上的点，count 为弧线条数（1~3），不要填 label。
- ticks / parallels：等长、平行标记，from/to 为线段两端点，count 为标记数量（1~3）。
- labels：自由文本标注（边长、代数式、结论等），x/y 为落点，anchor 取 start|middle|end。

硬性规则：
1. 只还原图中真实存在的几何要素，绝不臆造题目未给出的点、线、圆或结论。
2. 由题干条件决定的垂直、平行、共线、中点、等腰、等边、角平分线、垂直平分线、中线、高线、
   切线、圆心角与圆周角、圆内接等关系必须如实体现；比例要协调，看上去不能与题意矛盾。
   已知量（角度/长度）只信题干文字，不要采信示意图目测值。
3. segments/polygons/circles/arcs/right_angles/angle_marks/ticks/parallels 引用的每个点
   都必须在该张图的 points 中出现。
4. 图上不标注任何角度文字：不写度数、不写角名、不写 ∠ 表达式；
   角一律用 angle_marks 的弧线表示（不要填 label），直角用 right_angles 表示。
   题干里的度数照常保留在题干中，只是不要画到图上。
5. labels 只用于边长、代数式、结论等非角度文字（如 "AD=2BD"），坐标要放在空白处；
   角平分线、中线、垂直平分线等不要用 ticks 表示。
6. 辅助线、延长线用 dashed 或 extend 表达。
7. 每个子图用独立 panel 与独立坐标系，标题写"图1/图2/图3"以与题干对应；
   每张图各自的作图条件必须在该张图里如实画出，不得省略、也不得挪到别的图上。
8. 文字（labels 与点的字母）由程序自动避让，你只需把 labels 放在大概正确的空白处。
9. 可选字段按需出现，没有的内容直接省略，不要填 null。

仅当某张图完全无法用上述结构化字段表达时，才在该张图里额外提供 "svg" 字段，
内容是一段完整的 <svg viewBox="0 0 100 100">...</svg> 字符串（必须自带 stroke、fill 等样式属性，
禁止出现 script、事件属性 on* 与任何外部链接）。优先使用结构化字段。`

// ExtractGeometrySpec 调用多模态大模型把几何子图 + 题干文本翻译为坐标直出的几何描述 JSON。
// 题干文本与图形分开输入：已知量（角度/长度）从题干文本读取，不依赖模型从整图识读。
// correction 非空时为上一轮结构校验的问题清单，要求模型据此修正后重新输出完整 JSON。
func (v *vlmCapabilities) ExtractGeometrySpec(ctx context.Context, geoImage []byte, stemText, correction string) (string, error) {
	var sb strings.Builder
	sb.WriteString("请解析此几何图并输出可绘制的几何描述 JSON（坐标直出）。")
	if t := strings.TrimSpace(stemText); t != "" {
		sb.WriteString("\n【题干文字（已知量以此为准，不要采信图形目测值）】\n")
		sb.WriteString(t)
	}
	if correction != "" {
		sb.WriteString("\n【上一轮输出的几何描述存在结构问题，请修正后重新输出完整 JSON】\n")
		sb.WriteString(correction)
	}
	ctx, cancel := context.WithTimeout(ctx, 240*time.Second)
	defer cancel()
	out, err := v.dash.chatSystem(ctx, GeometrySpecSystemPrompt, sb.String(), geoImage)
	if err != nil {
		return "", err
	}
	return extractJSON(out)
}

// extractJSON 从模型输出中提取 JSON 文本（容忍 ```json 包裹、前后噪声）。
func extractJSON(text string) (string, error) {
	t := strings.TrimSpace(text)
	if strings.HasPrefix(t, "```") {
		parts := strings.Split(t, "```")
		if len(parts) > 1 {
			t = parts[1]
			if strings.HasPrefix(strings.TrimSpace(t), "json") {
				t = strings.TrimSpace(t)[4:]
			}
		}
	}
	start, end := strings.Index(t, "{"), strings.LastIndex(t, "}")
	if start < 0 || end <= start {
		return "", fmt.Errorf("model output contains no JSON object")
	}
	obj := t[start : end+1]
	// 校验是合法 JSON 后原样返回。
	if !json.Valid([]byte(obj)) {
		return "", fmt.Errorf("model output is not valid JSON")
	}
	return obj, nil
}
