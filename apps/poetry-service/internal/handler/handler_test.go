package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"peak/apps/poetry-service/internal/importer"
	"peak/apps/poetry-service/internal/repository"
	"peak/apps/poetry-service/internal/service"
	"peak/libs/domain"
	httpx "peak/libs/http"
)

// setupRouter 以 SQLite 空库 + 真实语料构建路由（沿用 seed，覆盖 importer 链路）。
func setupRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := domain.OpenDB(domain.DialectSQLite, filepath.Join(t.TempDir(), "handler.db"), logger.Silent)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := domain.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, _, err := importAndSeed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := service.New(repository.NewGormPoemRepository(db))
	h := New(svc)

	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) { httpx.OK(c, gin.H{"status": "ok"}) })
	h.RegisterRoutes(r)
	return r
}

// apiResponse 与 libs/http 的统一响应结构对应。
type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func doGet(t *testing.T, r *gin.Engine, path string) (*httptest.ResponseRecorder, apiResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(w, req)
	var resp apiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	return w, resp
}

func TestListPoemsAPI(t *testing.T) {
	r := setupRouter(t)

	w, resp := doGet(t, r, "/api/poetry/pieces?limit=10")
	if w.Code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("list failed: %d %s", w.Code, resp.Message)
	}
	var data struct {
		Items []domain.Poem `json:"items"`
		Total int64         `json:"total"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data.Total != 124 || len(data.Items) != 10 {
		t.Fatalf("list = %d items, total %d, want 10/124", len(data.Items), data.Total)
	}

	// 筛选 + 关键词（语料中仅《白雪歌送武判官归京》正文含"琵琶"）。
	_, resp = doGet(t, r, "/api/poetry/pieces?dynasty=唐&keyword=琵琶&limit=100")
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data.Total != 1 || data.Items[0].Title != "白雪歌送武判官归京" {
		t.Errorf("filtered list unexpected: total=%d first=%v", data.Total, data.Items[0].Title)
	}

	// 分页越界返回空列表而非报错。
	w, resp = doGet(t, r, "/api/poetry/pieces?offset=999&limit=10")
	if w.Code != http.StatusOK {
		t.Fatalf("offset beyond end should still be 200, got %d", w.Code)
	}
}

func TestGetPoemAPI(t *testing.T) {
	r := setupRouter(t)

	_, resp := doGet(t, r, "/api/poetry/pieces/1")
	var poem domain.Poem
	if err := json.Unmarshal(resp.Data, &poem); err != nil {
		t.Fatalf("decode poem: %v", err)
	}
	if poem.Title != "关雎" || poem.Content == "" {
		t.Errorf("poem 1 = %+v", poem)
	}

	// 不存在的篇目返回 404。
	w, _ := doGet(t, r, "/api/poetry/pieces/99999")
	if w.Code != http.StatusNotFound {
		t.Errorf("missing poem: http=%d, want 404", w.Code)
	}

	// 非法 ID 返回 400。
	w, _ = doGet(t, r, "/api/poetry/pieces/abc")
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid id should return 400, got %d", w.Code)
	}
}

func TestRelatedAPI(t *testing.T) {
	r := setupRouter(t)
	_, resp := doGet(t, r, "/api/poetry/pieces/1/related")
	var related []domain.Poem
	if err := json.Unmarshal(resp.Data, &related); err != nil {
		t.Fatalf("decode related: %v", err)
	}
	if len(related) != 4 {
		t.Errorf("related = %d, want 4", len(related))
	}
}

func TestStatsAPI(t *testing.T) {
	r := setupRouter(t)

	// 朝代统计。
	_, resp := doGet(t, r, "/api/poetry/stats/dynasties")
	var dyn []service.DynastyStat
	if err := json.Unmarshal(resp.Data, &dyn); err != nil {
		t.Fatalf("decode dynasties: %v", err)
	}
	if len(dyn) != 10 || dyn[0].Dynasty != "先秦" {
		t.Fatalf("dynasties = %d rows, first=%+v", len(dyn), dyn[0])
	}

	// 册数统计。
	_, resp = doGet(t, r, "/api/poetry/stats/volumes")
	var vols []service.VolumeStat
	if err := json.Unmarshal(resp.Data, &vols); err != nil {
		t.Fatalf("decode volumes: %v", err)
	}
	if len(vols) < 6 {
		t.Fatalf("volumes = %d rows", len(vols))
	}

	// 作者排行。
	_, resp = doGet(t, r, "/api/poetry/stats/authors?top=3")
	var authors []repository.AuthorCount
	if err := json.Unmarshal(resp.Data, &authors); err != nil {
		t.Fatalf("decode authors: %v", err)
	}
	if len(authors) != 3 {
		t.Errorf("authors top3 = %d", len(authors))
	}

	// 词频。
	_, resp = doGet(t, r, "/api/poetry/stats/words?top=20")
	var words struct {
		Items      []service.WordStat `json:"items"`
		TotalChars int                `json:"total_chars"`
	}
	if err := json.Unmarshal(resp.Data, &words); err != nil {
		t.Fatalf("decode words: %v", err)
	}
	if len(words.Items) != 20 || words.TotalChars == 0 {
		t.Errorf("words = %d, total=%d", len(words.Items), words.TotalChars)
	}
}

func TestChatAPI(t *testing.T) {
	r := setupRouter(t)

	body := `{"question":"陶渊明写过哪些篇目？"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/poetry/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	var resp apiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode chat: %v", err)
	}
	var chat service.ChatResponse
	if err := json.Unmarshal(resp.Data, &chat); err != nil {
		t.Fatalf("decode chat data: %v", err)
	}
	if chat.Enabled || chat.Notice == "" {
		t.Errorf("chat placeholder = %+v", chat)
	}

	// 非法请求体返回 400 参数错误。
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/poetry/chat", strings.NewReader(`{invalid`))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid body should return 400, got %d", w.Code)
	}
}

// importAndSeed 复用 importer.Seed 完成语料导入（与生产链路一致）。
func importAndSeed(db *gorm.DB) (int, int, error) {
	return importer.Seed(db)
}
