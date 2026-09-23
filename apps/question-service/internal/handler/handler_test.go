package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/logger"

	"peak/libs/domain"

	"peak/apps/question-service/internal/export"
	"peak/apps/question-service/internal/repository"
	"peak/apps/question-service/internal/service"
)

func setupHandler(t *testing.T) *gin.Engine {
	t.Helper()
	return setupHandlerWithExporter(t, nil)
}

// setupHandlerWithExporter 构造带指定导出能力的处理器，exporter 为 nil 表示不启用导出。
func setupHandlerWithExporter(t *testing.T, exporter export.Service) *gin.Engine {
	t.Helper()
	db, err := domain.OpenDB(domain.DialectSQLite, filepath.Join(t.TempDir(), "h.db"), logger.Silent)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := domain.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.NewGormRepositories(db)
	svc := service.New(repos, exporter)
	h := New(svc)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.RegisterRoutes(r)
	return r
}

func doRequest(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestQuestionHandlerCRUD(t *testing.T) {
	r := setupHandler(t)

	// create
	w := doRequest(t, r, http.MethodPost, "/api/questions", map[string]any{
		"subject":   "math",
		"stem_text": "1+1=?",
		"answer":    "2",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var created struct {
		Data domain.Question `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}
	if created.Data.ID == 0 {
		t.Fatal("expected question id")
	}

	// get
	w = doRequest(t, r, http.MethodGet, "/api/questions/"+uintToString(created.Data.ID), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}

	// update
	w = doRequest(t, r, http.MethodPut, "/api/questions/"+uintToString(created.Data.ID), map[string]any{
		"stem_text": "updated",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", w.Code)
	}

	// list
	w = doRequest(t, r, http.MethodGet, "/api/questions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}

	// delete
	w = doRequest(t, r, http.MethodDelete, "/api/questions/"+uintToString(created.Data.ID), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", w.Code)
	}

	// get after delete -> 404
	w = doRequest(t, r, http.MethodGet, "/api/questions/"+uintToString(created.Data.ID), nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete: expected 404, got %d", w.Code)
	}
}

func TestQuestionHandlerInvalidID(t *testing.T) {
	r := setupHandler(t)
	w := doRequest(t, r, http.MethodGet, "/api/questions/not-a-number", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestQuestionHandlerInvalidJSON(t *testing.T) {
	r := setupHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/questions", bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestMistakeHandlerFlow(t *testing.T) {
	r := setupHandler(t)

	// 先创建题目。
	w := doRequest(t, r, http.MethodPost, "/api/questions", map[string]any{
		"subject": "math", "stem_text": "x=?", "answer": "1",
	})
	var resp struct {
		Data domain.Question `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal question: %v", err)
	}
	qid := resp.Data.ID

	// 创建错题（无 user_id 应该 400）。
	w = doRequest(t, r, http.MethodPost, "/api/mistakes", map[string]any{
		"question_id": qid,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing user_id, got %d", w.Code)
	}

	// 正常创建错题。
	w = doRequest(t, r, http.MethodPost, "/api/mistakes", map[string]any{
		"user_id":      1,
		"question_id":  qid,
		"wrong_reason": "careless",
		"source":       "期中考试",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create mistake: expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var mresp struct {
		Data domain.Mistake `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &mresp); err != nil {
		t.Fatalf("unmarshal mistake: %v", err)
	}
	mid := mresp.Data.ID
	// 服务端应兜底填充 recorded_at，避免列表展示 0001-01-01。
	if mresp.Data.RecordedAt.IsZero() {
		t.Fatal("expected recorded_at to be set, got zero value")
	}

	// 列表。
	w = doRequest(t, r, http.MethodGet, "/api/mistakes", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list mistakes: expected 200, got %d", w.Code)
	}

	// 更新。
	w = doRequest(t, r, http.MethodPut, "/api/mistakes/"+uintToString(mid), map[string]any{
		"wrong_reason": "concept",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update mistake: expected 200, got %d", w.Code)
	}

	// 删除。
	w = doRequest(t, r, http.MethodDelete, "/api/mistakes/"+uintToString(mid), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete mistake: expected 200, got %d", w.Code)
	}
}

// createQuestionAndMistake 创建一道题并挂一条错题，供列表筛选测试预置数据。
func createQuestionAndMistake(t *testing.T, r *gin.Engine, question map[string]any, mistakeSource string) {
	t.Helper()

	w := doRequest(t, r, http.MethodPost, "/api/questions", question)
	if w.Code != http.StatusOK {
		t.Fatalf("create question: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data domain.Question `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode question: %v", err)
	}

	w = doRequest(t, r, http.MethodPost, "/api/mistakes", map[string]any{
		"user_id": 1, "question_id": resp.Data.ID, "source": mistakeSource,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create mistake: %d %s", w.Code, w.Body.String())
	}
}

func TestMistakeListFilters(t *testing.T) {
	r := setupHandler(t)

	// 两条数据：题目来源分别为 期中考试 / 练习册，学科与题型不同。
	createQuestionAndMistake(t, r, map[string]any{
		"subject": "数学", "stem_text": "已知二次函数求顶点", "question_type": "解答题",
		"knowledge_points": `["二次函数"]`, "source": "期中考试",
	}, "错题本")
	createQuestionAndMistake(t, r, map[string]any{
		"subject": "物理", "stem_text": "物体做匀速直线运动", "question_type": "填空题",
		"knowledge_points": `["运动学"]`, "source": "练习册",
	}, "练习册")

	list := func(query string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/mistakes"+query, nil)
		req.Header.Set("X-User-Id", "1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("list %q: %d %s", query, w.Code, w.Body.String())
		}
		var body struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		return body.Data
	}

	all := list("")
	if all["total"].(float64) != 2 {
		t.Fatalf("total = %v, want 2", all["total"])
	}
	// 分面计数随列表返回，且来源优先取题目来源。
	subjects, _ := all["subject_counts"].(map[string]any)
	if subjects["数学"].(float64) != 1 || subjects["物理"].(float64) != 1 {
		t.Fatalf("subject_counts = %v", subjects)
	}
	sources, _ := all["source_counts"].(map[string]any)
	if sources["期中考试"].(float64) != 1 || sources["练习册"].(float64) != 1 {
		t.Fatalf("source_counts = %v", sources)
	}

	cases := []struct {
		name  string
		query string
		want  float64
	}{
		{"关键词命中题型", "?keyword=" + url.QueryEscape("填空题"), 1},
		{"关键词命中知识点", "?keyword=" + url.QueryEscape("二次函数"), 1},
		{"忽略大小写", "?keyword=math", 0}, // 预置数据无英文，验证参数被正常解析
		{"多关键词需全部命中", "?keyword=" + url.QueryEscape("二次函数 顶点"), 1},
		{"多关键词任一不命中则为空", "?keyword=" + url.QueryEscape("二次函数 物理"), 0},
		{"关键词命中学科", "?keyword=" + url.QueryEscape("数学"), 1},
		{"学科过滤", "?subject=" + url.QueryEscape("数学"), 1},
		{"来源过滤", "?source=" + url.QueryEscape("练习册"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := list(tc.query)["total"].(float64); got != tc.want {
				t.Fatalf("query %q: total = %v, want %v", tc.query, got, tc.want)
			}
		})
	}

	// 分页：total 仍是符合条件的总数，items 只返回当前页。
	paged := list("?offset=1&limit=1")
	if paged["total"].(float64) != 2 || len(paged["items"].([]any)) != 1 {
		t.Fatalf("paged: total = %v, items = %d", paged["total"], len(paged["items"].([]any)))
	}
}

func TestCategoryHandler(t *testing.T) {
	r := setupHandler(t)

	// 创建分类。
	w := doRequest(t, r, http.MethodPost, "/api/categories", map[string]any{
		"name": "math", "type": "subject",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create category: expected 200, got %d", w.Code)
	}

	// 列表。
	w = doRequest(t, r, http.MethodGet, "/api/categories", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list categories: expected 200, got %d", w.Code)
	}

	// 按类型过滤。
	w = doRequest(t, r, http.MethodGet, "/api/categories?type=subject", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list categories by type: expected 200, got %d", w.Code)
	}
}

func TestPageParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		offset, limit := pageParams(c)
		c.JSON(http.StatusOK, gin.H{"offset": offset, "limit": limit})
	})

	parse := func(w *httptest.ResponseRecorder) (int, int) {
		var resp struct {
			Offset int `json:"offset"`
			Limit  int `json:"limit"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return resp.Offset, resp.Limit
	}

	// 默认值。
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if off, lim := parse(w); off != 0 || lim != 20 {
		t.Fatalf("unexpected default: %d, %d", off, lim)
	}

	// 自定义值。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?offset=10&limit=50", nil))
	if off, lim := parse(w); off != 10 || lim != 50 {
		t.Fatalf("unexpected custom: %d, %d", off, lim)
	}

	// limit 超上限 -> 回退 20。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?limit=200", nil))
	if off, lim := parse(w); off != 0 || lim != 20 {
		t.Fatalf("unexpected limit clamp: %d, %d", off, lim)
	}

	// limit 非法 -> 回退 20。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?limit=abc", nil))
	if off, lim := parse(w); off != 0 || lim != 20 {
		t.Fatalf("unexpected invalid limit: %d, %d", off, lim)
	}
}

func uintToString(n uint64) string {
	return strconv.FormatUint(n, 10)
}

// ---- 以下为错误分支与遗漏路径的补充测试 ----

func TestQuestionGetNotFound(t *testing.T) {
	r := setupHandler(t)
	w := doRequest(t, r, http.MethodGet, "/api/questions/9999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestQuestionUpdateNotFound(t *testing.T) {
	r := setupHandler(t)
	w := doRequest(t, r, http.MethodPut, "/api/questions/9999", map[string]any{"stem_text": "x"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestQuestionUpdateInvalidJSON(t *testing.T) {
	r := setupHandler(t)
	req := httptest.NewRequest(http.MethodPut, "/api/questions/1", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestQuestionDeleteNotFound(t *testing.T) {
	r := setupHandler(t)
	w := doRequest(t, r, http.MethodDelete, "/api/questions/9999", nil)
	if w.Code != http.StatusOK {
		// GORM 的 Delete 对不存在的记录默认不报错，返回 200。
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMistakeGetNotFound(t *testing.T) {
	r := setupHandler(t)
	w := doRequest(t, r, http.MethodGet, "/api/mistakes/9999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMistakeGetSuccess(t *testing.T) {
	r := setupHandler(t)

	// 创建题目 + 错题。
	w := doRequest(t, r, http.MethodPost, "/api/questions", map[string]any{"subject": "math"})
	var qresp struct {
		Data domain.Question `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &qresp); err != nil {
		t.Fatal(err)
	}
	w = doRequest(t, r, http.MethodPost, "/api/mistakes", map[string]any{
		"user_id": 1, "question_id": qresp.Data.ID, "source": "期中考试",
	})
	var mresp struct {
		Data domain.Mistake `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &mresp); err != nil {
		t.Fatal(err)
	}

	// 查询错题详情。
	w = doRequest(t, r, http.MethodGet, "/api/mistakes/"+uintToString(mresp.Data.ID), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMistakeUpdateNotFound(t *testing.T) {
	r := setupHandler(t)
	w := doRequest(t, r, http.MethodPut, "/api/mistakes/9999", map[string]any{"wrong_reason": "x"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMistakeUpdateInvalidJSON(t *testing.T) {
	r := setupHandler(t)
	req := httptest.NewRequest(http.MethodPut, "/api/mistakes/1", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestMistakeCreateInvalidJSON(t *testing.T) {
	r := setupHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/mistakes", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestMistakeCreateRequiresSource(t *testing.T) {
	r := setupHandler(t)

	w := doRequest(t, r, http.MethodPost, "/api/questions", map[string]any{"subject": "math"})
	var qresp struct {
		Data domain.Question `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &qresp); err != nil {
		t.Fatal(err)
	}

	// 未传来源应 400（来源必填）。
	w = doRequest(t, r, http.MethodPost, "/api/mistakes", map[string]any{
		"user_id": 1, "question_id": qresp.Data.ID,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing source, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestMistakeDeleteNotFound(t *testing.T) {
	r := setupHandler(t)
	w := doRequest(t, r, http.MethodDelete, "/api/mistakes/9999", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMistakeInvalidID(t *testing.T) {
	r := setupHandler(t)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		w := doRequest(t, r, method, "/api/mistakes/abc", map[string]any{})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s invalid id: expected 400, got %d", method, w.Code)
		}
	}
}

func TestQuestionListEmpty(t *testing.T) {
	r := setupHandler(t)
	w := doRequest(t, r, http.MethodGet, "/api/questions?offset=0&limit=10", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCategoryCreateInvalidJSON(t *testing.T) {
	r := setupHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/categories", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCategoryListEmpty(t *testing.T) {
	r := setupHandler(t)
	w := doRequest(t, r, http.MethodGet, "/api/categories", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
