package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"peak/apps/question-service/internal/export"
)

const docxContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

// newExportEngine 构造启用了真实导出能力的路由（内置字体，测试中不涉及图片）。
func newExportEngine(t *testing.T) *gin.Engine {
	t.Helper()
	exporter, err := export.NewDefault(export.DefaultConfig())
	if err != nil {
		t.Fatalf("create exporter: %v", err)
	}
	return setupHandlerWithExporter(t, exporter)
}

func createQuestionForExport(t *testing.T, r *gin.Engine, stem string) uint64 {
	t.Helper()

	w := doRequest(t, r, http.MethodPost, "/api/questions", map[string]any{
		"subject":       "数学",
		"grade":         "七年级上",
		"stem_text":     stem,
		"question_type": "解答题",
		"source":        "期中试卷",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create question: %d %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID uint64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode question response: %v", err)
	}
	return resp.Data.ID
}

func createMistakeForExport(t *testing.T, r *gin.Engine, questionID uint64) uint64 {
	t.Helper()

	w := doRequest(t, r, http.MethodPost, "/api/mistakes", map[string]any{
		"user_id":     1,
		"question_id": questionID,
		"source":      "错题本",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create mistake: %d %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID uint64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode mistake response: %v", err)
	}
	return resp.Data.ID
}

// doExportRequest 发送导出请求，userID 为空时不带用户头。
func doExportRequest(t *testing.T, r *gin.Engine, userID string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/mistakes/export", &buf)
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		req.Header.Set("X-User-Id", userID)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestExportMistakesReturnsPDF(t *testing.T) {
	r := newExportEngine(t)
	qID := createQuestionForExport(t, r, "已知 x^2 = 4，求 x。")
	mID := createMistakeForExport(t, r, qID)

	w := doExportRequest(t, r, "1", map[string]any{"ids": []uint64{mID}, "format": "pdf"})
	if w.Code != http.StatusOK {
		t.Fatalf("export pdf: %d %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("content-type = %q, want application/pdf", ct)
	}

	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, "filename*=UTF-8''") {
		t.Fatalf("content-disposition = %q", cd)
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-")) {
		t.Fatal("expected pdf payload")
	}
}

func TestExportMistakesReturnsDocx(t *testing.T) {
	r := newExportEngine(t)
	qID := createQuestionForExport(t, r, "已知 x^2 = 4，求 x。")
	mID := createMistakeForExport(t, r, qID)

	w := doExportRequest(t, r, "1", map[string]any{"ids": []uint64{mID}, "format": "docx"})
	if w.Code != http.StatusOK {
		t.Fatalf("export docx: %d %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != docxContentType {
		t.Fatalf("content-type = %q, want %q", ct, docxContentType)
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("PK")) {
		t.Fatal("expected zip (docx) payload")
	}
}

func TestExportMistakesRequiresUserHeader(t *testing.T) {
	r := newExportEngine(t)
	w := doExportRequest(t, r, "", map[string]any{"ids": []uint64{1}, "format": "pdf"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", w.Code)
	}
}

func TestExportMistakesRejectsUnsupportedFormat(t *testing.T) {
	r := newExportEngine(t)
	w := doExportRequest(t, r, "1", map[string]any{"ids": []uint64{1}, "format": "txt"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", w.Code)
	}
}

func TestExportMistakesRejectsEmptyIDs(t *testing.T) {
	r := newExportEngine(t)
	w := doExportRequest(t, r, "1", map[string]any{"ids": []uint64{}, "format": "pdf"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", w.Code)
	}
}

func TestExportMistakesRejectsInvalidBody(t *testing.T) {
	r := newExportEngine(t)

	req := httptest.NewRequest(http.MethodPost, "/api/mistakes/export",
		strings.NewReader("definitely not json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "1")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", w.Code)
	}
}

func TestExportMistakesScopedToCurrentUser(t *testing.T) {
	r := newExportEngine(t)
	qID := createQuestionForExport(t, r, "题干")
	mID := createMistakeForExport(t, r, qID) // 属于 user 1

	w := doExportRequest(t, r, "2", map[string]any{"ids": []uint64{mID}, "format": "pdf"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("code = %d body=%s, want 404", w.Code, w.Body.String())
	}
}

func TestExportMistakesWithoutExporter(t *testing.T) {
	r := setupHandler(t) // 未注入导出能力
	w := doExportRequest(t, r, "1", map[string]any{"ids": []uint64{1}, "format": "pdf"})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", w.Code)
	}
}

func TestContentDispositionEncodesChineseFilename(t *testing.T) {
	got := contentDisposition("我的错题本 2026-09-17.pdf")
	if !strings.Contains(got, `filename="mistakes.pdf"`) {
		t.Fatalf("missing ascii fallback: %s", got)
	}
	if !strings.Contains(got, "filename*=UTF-8''") {
		t.Fatalf("missing utf-8 filename: %s", got)
	}
	if strings.Contains(got, "我的错题本") {
		t.Fatalf("raw chinese should be url-encoded: %s", got)
	}
}
