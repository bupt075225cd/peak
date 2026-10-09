package geom

import "strings"

// semicircleHint 题干提到半圆而描述只有整圆时，回喂给模型的修正指引。
const semicircleHint = "题干提到\"半圆\"，但几何描述中只有整圆 circles、没有任何弧 arcs。" +
	"半圆必须用 arcs 表达，不要用 circles 画整圆：先在 points 中补充直径的中点（hidden:true）作为圆心，" +
	"radius 取直径长度的一半，start_angle/end_angle 只覆盖半圆弧" +
	"（水平直径、弧朝上时为 180→360，弧朝下时为 0→180，竖直直径、弧朝右时为 270→90）。"

// CheckSemicircle 题干-图形语义核对（启发式），弥补结构校验无法发现的语义偏差：
// VLM 提取几何描述时常把半圆误画成整圆（circles），而整圆在结构上完全合法，
// Validate 检测不到。当题干含"半圆"、各子图存在 circles 且没有任何 arcs 时，
// 给含圆的子图追加修正问题（进入 Issues，随回喂修正与 consistent 判定生效）。
//
// 采用保守触发：仅在"有 circles 且无 arcs"时报告，避免题干文字与配图无关时误报。
func CheckSemicircle(panels []Panel, stemText string) {
	if !strings.Contains(stemText, "半圆") {
		return
	}
	hasCircle, hasArc := false, false
	for i := range panels {
		if len(panels[i].Spec.Circles) > 0 {
			hasCircle = true
		}
		if len(panels[i].Spec.Arcs) > 0 {
			hasArc = true
		}
	}
	if !hasCircle || hasArc {
		return
	}
	for i := range panels {
		if len(panels[i].Spec.Circles) > 0 {
			panels[i].Issues = append(panels[i].Issues, semicircleHint)
		}
	}
}
