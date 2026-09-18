package service

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"peak/libs/domain"
	"peak/libs/errors"

	"peak/apps/question-service/internal/export"
)

// fakeExporter 记录收到的导出请求，返回预设结果。
type fakeExporter struct {
	mu     sync.Mutex
	items  []export.ExportItem
	format export.Format
	result export.Result
	err    error
}

func (f *fakeExporter) Export(_ context.Context, items []export.ExportItem, format export.Format) (export.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items = append([]export.ExportItem(nil), items...)
	f.format = format
	return f.result, f.err
}

func (f *fakeExporter) captured() []export.ExportItem {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.items
}

var errExportBoom = fmt.Errorf("boom")

// seedExportData 为指定用户造 3 道错题，返回题目与错题 id（顺序一致）。
func seedExportData(t *testing.T, svc *Service, userID uint64) (questionIDs, mistakeIDs []uint64) {
	t.Helper()
	ctx := context.Background()

	for _, stem := range []string{"第一题", "第二题", "第三题"} {
		q := &domain.Question{
			Subject: "数学", Grade: "七年级上", QuestionType: "解答题",
			StemText: stem, Source: "期中试卷", Image: `["a.svg"]`,
		}
		if err := svc.CreateQuestion(ctx, q); err != nil {
			t.Fatalf("create question: %v", err)
		}
		questionIDs = append(questionIDs, q.ID)

		m := &domain.Mistake{UserID: userID, QuestionID: q.ID, Source: "错题本"}
		if err := svc.CreateMistake(ctx, m); err != nil {
			t.Fatalf("create mistake: %v", err)
		}
		mistakeIDs = append(mistakeIDs, m.ID)
	}
	return questionIDs, mistakeIDs
}

func TestExportMistakesWithoutExporter(t *testing.T) {
	svc := setupService(t)

	_, err := svc.ExportMistakes(context.Background(), 1, []uint64{1}, export.FormatPDF)
	if errors.CodeOf(err) != errors.CodeInternal {
		t.Fatalf("expected CodeInternal, got %v", err)
	}
}

func TestExportMistakesValidatesInput(t *testing.T) {
	svc := setupService(t)
	svc.exporter = &fakeExporter{}

	if _, err := svc.ExportMistakes(context.Background(), 0, []uint64{1}, export.FormatPDF); errors.CodeOf(err) != errors.CodeInvalidArgument {
		t.Fatalf("missing user: expected CodeInvalidArgument, got %v", err)
	}
	if _, err := svc.ExportMistakes(context.Background(), 1, nil, export.FormatPDF); errors.CodeOf(err) != errors.CodeInvalidArgument {
		t.Fatalf("missing ids: expected CodeInvalidArgument, got %v", err)
	}
}

func TestExportMistakesNotFoundForUnknownIDs(t *testing.T) {
	svc := setupService(t)
	svc.exporter = &fakeExporter{}

	_, err := svc.ExportMistakes(context.Background(), 1, []uint64{9999}, export.FormatPDF)
	if errors.CodeOf(err) != errors.CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %v", err)
	}
}

func TestExportMistakesKeepsRequestedOrder(t *testing.T) {
	svc := setupService(t)
	fake := &fakeExporter{result: export.Result{Data: []byte("payload"), Filename: "我的错题本.pdf"}}
	svc.exporter = fake

	_, mistakeIDs := seedExportData(t, svc, 1)

	reversed := []uint64{mistakeIDs[2], mistakeIDs[0], mistakeIDs[1]}
	res, err := svc.ExportMistakes(context.Background(), 1, reversed, export.FormatPDF)
	if err != nil {
		t.Fatalf("ExportMistakes: %v", err)
	}

	items := fake.captured()
	if len(items) != 3 {
		t.Fatalf("exported %d items, want 3", len(items))
	}
	for i, want := range []string{"第三题", "第一题", "第二题"} {
		if items[i].StemText != want {
			t.Fatalf("item %d stem = %q, want %q", i, items[i].StemText, want)
		}
	}
	if items[0].Source != "期中试卷" {
		t.Fatalf("source = %q, want 期中试卷", items[0].Source)
	}
	if items[0].Grade != "七年级上" || items[0].Subject != "数学" || items[0].QuestionType != "解答题" {
		t.Fatalf("unexpected meta: %+v", items[0])
	}
	if len(items[0].ImageKeys) != 1 || items[0].ImageKeys[0] != "a.svg" {
		t.Fatalf("unexpected image keys: %v", items[0].ImageKeys)
	}

	if fake.format != export.FormatPDF {
		t.Fatalf("format = %q, want pdf", fake.format)
	}
	if res.Filename != "我的错题本.pdf" || string(res.Data) != "payload" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestExportMistakesSkipsForeignMistakes(t *testing.T) {
	svc := setupService(t)
	fake := &fakeExporter{result: export.Result{Data: []byte("x")}}
	svc.exporter = fake

	_, mine := seedExportData(t, svc, 1)
	_, others := seedExportData(t, svc, 2)

	// 混入他人错题：只应导出自己的那一条。
	if _, err := svc.ExportMistakes(context.Background(), 1, []uint64{mine[0], others[0]}, export.FormatPDF); err != nil {
		t.Fatalf("ExportMistakes: %v", err)
	}
	if got := len(fake.captured()); got != 1 {
		t.Fatalf("exported %d items, want 1", got)
	}

	// 全部是他人错题：应视为未找到可导出内容。
	if _, err := svc.ExportMistakes(context.Background(), 1, others, export.FormatPDF); errors.CodeOf(err) != errors.CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %v", err)
	}
}

func TestExportMistakesWrapsExporterError(t *testing.T) {
	svc := setupService(t)
	svc.exporter = &fakeExporter{err: errExportBoom}

	_, mistakeIDs := seedExportData(t, svc, 1)

	_, err := svc.ExportMistakes(context.Background(), 1, mistakeIDs, export.FormatPDF)
	if errors.CodeOf(err) != errors.CodeInternal {
		t.Fatalf("expected CodeInternal, got %v", err)
	}
}

func TestParseImageKeys(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"two keys", `["a.svg","b.png"]`, 2},
		{"skips empty", `["a.svg",""]`, 1},
		{"empty array", `[]`, 0},
		{"blank", `   `, 0},
		{"invalid json", `not json`, 0},
		{"wrong type", `{"a":1}`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseImageKeys(tc.raw); len(got) != tc.want {
				t.Fatalf("parseImageKeys(%q) = %v, want %d items", tc.raw, got, tc.want)
			}
		})
	}
}

func TestMistakeSourcePrefersQuestionSource(t *testing.T) {
	withQuestionSource := &domain.Mistake{
		Source:   "错题本",
		Question: &domain.Question{Source: " 期中试卷 "},
	}
	if got := mistakeSource(withQuestionSource); got != "期中试卷" {
		t.Fatalf("source = %q, want 期中试卷", got)
	}

	emptyQuestionSource := &domain.Mistake{
		Source:   "错题本",
		Question: &domain.Question{},
	}
	if got := mistakeSource(emptyQuestionSource); got != "错题本" {
		t.Fatalf("source = %q, want 错题本", got)
	}

	noQuestion := &domain.Mistake{Source: "  "}
	if got := mistakeSource(noQuestion); got != "" {
		t.Fatalf("source = %q, want empty", got)
	}
}
