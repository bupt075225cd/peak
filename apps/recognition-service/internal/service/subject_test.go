package service

import "testing"

func TestDetectSubject(t *testing.T) {
	cases := []struct {
		name string
		stem string
		want string
	}{
		{"数学-几何", "如图，AB//CD，点E、F分别在直线AB、CD上，求∠BEO+∠DFO的值", "数学"},
		{"数学-函数", "已知二次函数 y = x² - 2x - 3，求其顶点坐标", "数学"},
		{"语文-文言阅读", "阅读下面的文言文，完成下列各题", "语文"},
		{"语文-古诗默写", "默写古诗文中的名句名篇", "语文"},
		{"英语", "What is the meaning of the underlined word?", "英语"},
		{"物理-加速度", "一个质量为 2kg 的物体受到 10N 拉力，求加速度", "物理"},
		{"化学-酸", "下列物质中，属于酸的是", "化学"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detectSubject(c.stem); got != c.want {
				t.Fatalf("detectSubject(%q) = %q, want %q", c.stem, got, c.want)
			}
		})
	}
}
