package service

import "testing"

func TestDetectQuestionType(t *testing.T) {
	cases := []struct {
		name string
		stem string
		want string
	}{
		{"选择题-半角句点", "1. 下列计算正确的是（ ）A. x² B. x³ C. x⁴ D. x⁵", "选择题"},
		{"选择题-全角句点", "A．第一象限 B．第二象限 C．第三象限 D．第四象限", "选择题"},
		{"填空题-下划线", "若 x = ____，则 2x + 1 = 5", "填空题"},
		{"填空题-空括号", "计算 1 + 2 = （ ）", "填空题"},
		{"解答题", "如图，AB//CD，求 ∠BEO + ∠DFO 的值。", "解答题"},
		{"几何点不误判为选择题", "在△ABC中，点A、B、C分别在圆上，求∠ABC", "解答题"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detectQuestionType(c.stem); got != c.want {
				t.Fatalf("detectQuestionType(%q) = %q, want %q", c.stem, got, c.want)
			}
		})
	}
}
