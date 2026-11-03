package provider

import "testing"

// TestParseOrientation 方向探测输出的容错解析：非法角度取最近的 90 的倍数，
// 非 JSON 一律按 0（正立）处理，方向判读失败不应该让识别链路报错。
func TestParseOrientation(t *testing.T) {
	cases := map[string]Rotation{
		`{"rotation":0}`:      0,
		`{"rotation":90}`:     90,
		`{"rotation":180.0}`:  180,
		`{"rotation":"270"}`:  270,
		`{"rotation":"null"}`: 0,
		`{"rotation":87}`:     90,
		`{"rotation":400}`:    0,
		`not json`:            0,
		``:                    0,
	}
	for in, want := range cases {
		if got := parseOrientation(in); got != want {
			t.Fatalf("parseOrientation(%q) = %d, want %d", in, got, want)
		}
	}
}
