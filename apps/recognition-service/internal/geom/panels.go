package geom

import (
	"encoding/json"
	"fmt"
	"strings"
)

// panelJSON 模型返回的单张子图（标题 + 几何描述）。
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
	if err := json.Unmarshal([]byte(body), &multi); err == nil && len(multi.Panels) > 0 {
		haveMulti = true
	}
	if !haveMulti {
		var one panelJSON
		if err := json.Unmarshal([]byte(body), &one); err != nil {
			return nil, fmt.Errorf("geom: parse spec json: %w", err)
		}
		multi.Panels = []panelJSON{one}
	}

	panels := make([]Panel, 0, len(multi.Panels))
	for i, pj := range multi.Panels {
		issues := pj.Spec.Validate()
		spec := pj.Spec
		spec.Normalize()
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
