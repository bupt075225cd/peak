package service

import (
	"context"
	"testing"

	"peak/apps/recognition-service/internal/provider"
)

func TestSplitQuestionsByNumber(t *testing.T) {
	prov := provider.NewMockProvider()
	items := []provider.DocumentItem{
		{Kind: "text", Text: "1. 已知集合 A={1,2,3}，求子集个数。"},
		{Kind: "text", Text: "  这是一道选择题。"},
		{Kind: "text", Text: "2. 计算 lim(x→0) sin(x)/x。"},
		{Kind: "text", Text: "3、解方程 x^2 - 5x + 6 = 0。"},
	}

	qs := splitQuestions(context.Background(), prov, items)
	if len(qs) != 3 {
		t.Fatalf("expected 3 questions, got %d", len(qs))
	}
	if qs[0].StemText != "1. 已知集合 A={1,2,3}，求子集个数。\n这是一道选择题。" {
		t.Fatalf("unexpected q0: %q", qs[0].StemText)
	}
}

func TestSplitQuestionsFallbackSingle(t *testing.T) {
	prov := provider.NewMockProvider()
	items := []provider.DocumentItem{
		{Kind: "text", Text: "这是一段没有题号的文本。"},
	}

	qs := splitQuestions(context.Background(), prov, items)
	if len(qs) != 1 {
		t.Fatalf("expected 1 question, got %d", len(qs))
	}
}

func TestSplitQuestionsEmpty(t *testing.T) {
	prov := provider.NewMockProvider()
	qs := splitQuestions(context.Background(), prov, nil)
	if len(qs) != 0 {
		t.Fatalf("expected 0 questions, got %d", len(qs))
	}
}

func TestFromStructured(t *testing.T) {
	res := &provider.StructuredResult{
		Items: []provider.StructuredItem{
			{
				StemText: "18. 如图，AB//CD，点E、F分别在直线AB、CD上…",
				SubQuestions: []provider.SubQuestion{
					{Label: "(2)", Text: "如图2，直线MN…求∠EMN-∠FNM的值", GeometryDesc: "直线MN交角平分线"},
					{Label: "(3)", Text: "如图3…求n的值", GeometryDesc: "EG在∠AEO内"},
				},
			},
		},
	}
	qs := fromStructured(res, nil)
	if len(qs) != 1 {
		t.Fatalf("expected 1 question, got %d", len(qs))
	}
	if qs[0].StemText == "" {
		t.Fatal("expected stem_text")
	}
	if len(qs[0].SubQuestions) != 2 {
		t.Fatalf("expected 2 sub-questions, got %d", len(qs[0].SubQuestions))
	}
}

func TestFromStructuredWithGeometryKeys(t *testing.T) {
	// 验证 geometry_refs 正确映射为 geometry_keys。
	res := &provider.StructuredResult{
		Items: []provider.StructuredItem{
			{
				StemText: "18. 如图…",
				SubQuestions: []provider.SubQuestion{
					{Label: "(2)", Text: "求值", GeometryRefs: []int{2}},
				},
			},
		},
	}
	imgKeys := []string{"doc/img_1.jpg", "doc/img_2.jpg"}
	qs := fromStructured(res, imgKeys)
	if len(qs) != 1 {
		t.Fatalf("expected 1 question, got %d", len(qs))
	}
	sub := qs[0].SubQuestions[0]
	if len(sub.GeometryKeys) != 1 || sub.GeometryKeys[0] != "doc/img_2.jpg" {
		t.Fatalf("unexpected geometry_keys: %v", sub.GeometryKeys)
	}
}

func TestFromStructuredEmptyStemUsesSub(t *testing.T) {
	// 模型只给了子问、没给题干时，用子问拼出题干。
	res := &provider.StructuredResult{
		Items: []provider.StructuredItem{
			{
				StemText: "",
				SubQuestions: []provider.SubQuestion{
					{Label: "(1)", Text: "求角度"},
				},
			},
		},
	}
	qs := fromStructured(res, nil)
	if len(qs) != 1 {
		t.Fatalf("expected 1 question, got %d", len(qs))
	}
	if qs[0].StemText != "(1) 求角度" {
		t.Fatalf("unexpected stem_text: %q", qs[0].StemText)
	}
}

func TestFilenameFromKey(t *testing.T) {
	cases := map[string]string{
		"original/123_paper.docx": "paper.docx",
		"original/456_a.b.pdf":    "a.b.pdf",
		"no-underscore.jpg":       "no-underscore.jpg",
	}
	for key, want := range cases {
		if got := filenameFromKey(key); got != want {
			t.Fatalf("filenameFromKey(%q)=%q, want %q", key, got, want)
		}
	}
}
