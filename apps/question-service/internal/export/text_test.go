package export

import "testing"

func TestNormalizeStemTextMergesLineBreaks(t *testing.T) {
	cases := map[string]string{
		// 版式性换行：原卷按纸面宽度折行的痕迹，折叠后由渲染器按页宽重排。
		"第一行\n第二行":    "第一行第二行",
		"4。\n求 AE 的长": "4。求 AE 的长",
		"AB\n平行":      "AB平行",
		"a\r\nb":      "a b",
		"多  个\t空白\n混排": "多 个 空白混排",
		"  首尾空白  ":    "首尾空白",
		"整段没有换行":      "整段没有换行",
		"全角\u3000空格":  "全角 空格",
	}
	for in, want := range cases {
		if got := normalizeStemText(in, true); got != want {
			t.Fatalf("normalizeStemText(%q) = %q, want %q", in, got, want)
		}
	}
}

// 结构性换行必须保留：小问编号行之前、空行分段、Markdown 表格内部。
// 保留口径与前端 StemText 展示一致，保证网页与导出的段落划分相同。
func TestNormalizeStemTextKeepsStructuralBreaks(t *testing.T) {
	cases := map[string]string{
		// 小问编号（半角括号）。
		"CF;\n(1) 求证": "CF;\n(1) 求证",
		// 小问编号（全角括号、汉字编号、圈号）。
		"如图所示.\n（1）当220＜x≤300时":       "如图所示.\n（1）当220＜x≤300时",
		"设点M运动的时间为t（秒），\n①若ΔDMN的边与BC平行": "设点M运动的时间为t（秒），\n①若ΔDMN的边与BC平行",
		// 空行分段：保留一个段落分隔，连续空行压缩。
		"点C的坐标为\n\n(0, 6)，点P为线段上一动点":  "点C的坐标为\n\n(0, 6)，点P为线段上一动点",
		"第一段\n\n\n第二段":                "第一段\n\n第二段",
		// Markdown 表格：内部换行逐行保留，表格前后的文本独立成段。
		"如表所示：\n| a | b |\n| --- | --- |\n| 1 | 2 |\n(1) 求关系式": "如表所示：\n| a | b |\n| --- | --- |\n| 1 | 2 |\n(1) 求关系式",
		// 版式换行与结构换行混排：句中断行折叠、小问边界保留。
		"连接BE，将长方形ABCD沿BE\n翻折，点C落在点C处\n(1) 求面积": "连接BE，将长方形ABCD沿BE翻折，点C落在点C处\n(1) 求面积",
	}
	for in, want := range cases {
		if got := normalizeStemText(in, true); got != want {
			t.Fatalf("normalizeStemText(%q) = %q, want %q", in, got, want)
		}
	}
}

// 数字后跟点的裸编号不作为小问标记：避免把 "1.5 倍" 这类正文误判为结构边界。
func TestNormalizeStemTextDoesNotTreatDecimalAsMarker(t *testing.T) {
	if got := normalizeStemText("边长扩大\n1.5 倍后", true); got != "边长扩大1.5 倍后" {
		t.Fatalf("got %q, want folded", got)
	}
}

func TestNormalizeStemTextKeepsLineBreaksWhenDisabled(t *testing.T) {
	if got := normalizeStemText("  第一行\n第二行  ", false); got != "第一行\n第二行" {
		t.Fatalf("got %q, want line breaks preserved", got)
	}
}
