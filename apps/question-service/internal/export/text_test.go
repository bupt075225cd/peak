package export

import "testing"

func TestNormalizeStemTextMergesLineBreaks(t *testing.T) {
	cases := map[string]string{
		"第一行\n第二行":     "第一行第二行",
		"4。\n求 AE 的长":  "4。求 AE 的长",
		"AB\n平行":       "AB平行",
		"a\r\nb":       "a b",
		"CF;\n(1) 求证":  "CF; (1) 求证",
		"多  个\t空白\n混排": "多 个 空白混排",
		"  首尾空白  ":    "首尾空白",
		"整段没有换行":      "整段没有换行",
		"全角\u3000空格":   "全角 空格",
	}
	for in, want := range cases {
		if got := normalizeStemText(in, true); got != want {
			t.Fatalf("normalizeStemText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeStemTextKeepsLineBreaksWhenDisabled(t *testing.T) {
	if got := normalizeStemText("  第一行\n第二行  ", false); got != "第一行\n第二行" {
		t.Fatalf("got %q, want line breaks preserved", got)
	}
}
