package geom

import (
	"encoding/json"
	"fmt"
	"strings"
)

// panelJSON 模型返回的单张子图（标题 + 几何描述）。
// 注：Spec 实现了 UnmarshalJSON 并接管整段 JSON 解析，因此外层不能新增字段
// （会被 Spec.UnmarshalJSON 忽略），has_angle_marks 等面板级字段须放进 Spec。
type panelJSON struct {
	Title string `json:"title,omitempty"`
	Spec
}

// panelsJSON 模型返回的多子图包装。
type panelsJSON struct {
	Panels []panelJSON `json:"panels"`
}

// Panel 解析、校验、清洗后的单张子图。
type Panel struct {
	Title  string
	Spec   Spec
	Issues []string // 结构校验发现的问题（清洗前），空表示结构完整
}

// PanelResult 单张子图的渲染结果。
type PanelResult struct {
	Title  string
	SVG    []byte
	Issues []string
}

// ParsePanels 解析模型返回的 spec JSON，支持两种形态：
//   - 多子图：{"panels":[{"title":"图1",...},{"title":"图2",...}]}
//   - 单子图：{"title":"图1","canvas":{...},"points":[...],...}
//
// 逐子图先做结构校验（issues 保留在清洗前的原始输出上，供回喂修正），
// 再做 Normalize 清洗，并丢弃没有任何可绘制内容的子图。
func ParsePanels(raw string) ([]Panel, error) {
	body := strings.TrimSpace(raw)
	if body == "" {
		return nil, fmt.Errorf("geom: empty spec")
	}

	var multi panelsJSON
	haveMulti := false
	// 通过显式存在 "panels" 键判定多子图形态（单子图 JSON 也能 unmarshal 进
	// panelsJSON 但 panels 为 nil，不能据此判定）。
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &probe); err == nil {
		if rawPanels, ok := probe["panels"]; ok {
			var ps []panelJSON
			if err := json.Unmarshal(rawPanels, &ps); err != nil {
				return nil, fmt.Errorf("geom: parse spec json: %w", err)
			}
			haveMulti = true
			multi.Panels = ps
		}
	}
	if !haveMulti {
		var one panelJSON
		if err := json.Unmarshal([]byte(body), &one); err != nil {
			return nil, fmt.Errorf("geom: parse spec json: %w", err)
		}
		multi.Panels = []panelJSON{one}
	}

	// 模型显式返回 {"panels":[]}（判定输入不是几何示意图、拒绝绘制）→
	// 合法返回空结果，由调用方干净跳过重绘（不算错误、不回喂重试）。
	if haveMulti && len(multi.Panels) == 0 {
		return []Panel{}, nil
	}

	panels := make([]Panel, 0, len(multi.Panels))
	for i, pj := range multi.Panels {
		spec := pj.Spec
		// 合并的角标记核对：模型明确声明原图没有弧线标记（has_angle_marks=false）时，
		// 直接剔除其臆造的 angle_marks，避免非法引用在结构校验里误报、也避免画到图上。
		if spec.HasAngleMarks != nil && !*spec.HasAngleMarks {
			spec.AngleMarks = nil
		}
		// 先把"端点写成坐标字符串"的引用补成隐藏点，再做结构校验：
		// 这类写法在网格图里很常见，不修就会被判成悬空引用、整条线丢掉。
		spec.RepairCoordRefs()
		issues := spec.Validate()
		spec.Normalize()
		// 渲染前做点-线-轴对齐：模型给的坐标是目测值，常出现"点浮在直线旁边""直线
		// 没穿过它该穿过的点"。这里只做小幅吸附，不改变图元的拓扑关系。
		spec.Reconcile()
		if spec.Empty() {
			continue
		}
		title := strings.TrimSpace(pj.Title)
		if title == "" {
			title = fmt.Sprintf("图%d", i+1)
		}
		panels = append(panels, Panel{Title: title, Spec: spec, Issues: issues})
	}
	if len(panels) == 0 {
		return nil, fmt.Errorf("geom: spec has no drawable panel")
	}
	return panels, nil
}

// RenderPanels 解析并渲染模型返回的 spec JSON，逐子图产出独立 SVG。
func RenderPanels(raw string) ([]PanelResult, error) {
	panels, err := ParsePanels(raw)
	if err != nil {
		return nil, err
	}
	results := make([]PanelResult, 0, len(panels))
	for i := range panels {
		svg, rerr := Render(&panels[i].Spec)
		if rerr != nil {
			return nil, fmt.Errorf("geom: render %s: %w", panels[i].Title, rerr)
		}
		results = append(results, PanelResult{
			Title:  panels[i].Title,
			SVG:    svg,
			Issues: panels[i].Issues,
		})
	}
	return results, nil
}

// AllConsistent 报告是否所有子图都通过了结构校验。
// 映射到对前端的 redraw_report.consistent 字段。
func AllConsistent(results []PanelResult) bool {
	for _, r := range results {
		if len(r.Issues) > 0 {
			return false
		}
	}
	return true
}

// CollectIssues 汇总所有子图的结构校验问题（供回喂模型修正与告警）。
func CollectIssues(results []PanelResult) []string {
	var out []string
	for _, r := range results {
		for _, s := range r.Issues {
			out = append(out, r.Title+"："+s)
		}
	}
	return out
}
