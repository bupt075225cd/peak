package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"peak/libs/domain"
	"peak/libs/filesign"
	"peak/libs/logger"
	"peak/libs/storage"

	"peak/apps/recognition-service/internal/provider"
	"peak/apps/recognition-service/internal/service"
)

const testFileSecret = "test-filesign-secret"

func setupHandler(t *testing.T) (*gin.Engine, *service.Service, storage.FileStorage, *gorm.DB) {
	t.Helper()
	db, err := domain.OpenDB(domain.DialectSQLite, filepath.Join(t.TempDir(), "h.db"), gormlogger.Silent)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := domain.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store, err := storage.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	svc := service.New(db, store, provider.NewMockProvider(), logger.NewNop())
	// 等待在途的异步识别流程结束，避免与 t.TempDir 清理竞态。
	t.Cleanup(svc.Wait)
	h := New(svc, db, store, testFileSecret)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.RegisterRoutes(r)
	return r, svc, store, db
}

func uploadRequest(t *testing.T, r *gin.Engine, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("image", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/recognition/tasks", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-User-Id", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateTask(t *testing.T) {
	r, _, _, _ := setupHandler(t)
	w := uploadRequest(t, r, "paper.jpg", []byte("fake-image-data"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data domain.RecognitionTask `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.ID == 0 {
		t.Fatal("expected task id")
	}
	if resp.Data.Provider != "mock" {
		t.Fatalf("expected provider mock, got %s", resp.Data.Provider)
	}
}

func TestCreateTaskMissingImage(t *testing.T) {
	r, _, _, _ := setupHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/recognition/tasks", bytes.NewBufferString(""))
	req.Header.Set("Content-Type", "multipart/form-data")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// uploadDocumentRequest 以 document 字段上传文件。
func uploadDocumentRequest(t *testing.T, r *gin.Engine, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("document", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/recognition/tasks", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-User-Id", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateTaskDocument(t *testing.T) {
	r, _, _, _ := setupHandler(t)
	w := uploadDocumentRequest(t, r, "paper.docx", []byte("fake-docx"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data domain.RecognitionTask `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.ID == 0 {
		t.Fatal("expected task id")
	}
}

func TestCreateTaskUnsupportedDocument(t *testing.T) {
	r, _, _, _ := setupHandler(t)
	w := uploadDocumentRequest(t, r, "paper.txt", []byte("not-a-doc"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetTask(t *testing.T) {
	r, svc, _, _ := setupHandler(t)

	// 先直接通过 service 创建一个任务（绕过上传），然后查询。
	task, err := svc.CreateTask(context.Background(), 1, 1, "original/x.jpg")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/recognition/tasks/"+strconv.FormatUint(task.ID, 10), nil)
	req.Header.Set("X-User-Id", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetTaskInvalidID(t *testing.T) {
	r, _, _, _ := setupHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/recognition/tasks/abc", nil)
	req.Header.Set("X-User-Id", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	r, _, _, _ := setupHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/recognition/tasks/99999", nil)
	req.Header.Set("X-User-Id", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRetryTask(t *testing.T) {
	r, svc, _, _ := setupHandler(t)

	task, err := svc.CreateTask(context.Background(), 1, 1, "original/y.jpg")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/recognition/tasks/"+strconv.FormatUint(task.ID, 10)+"/retry", nil)
	req.Header.Set("X-User-Id", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRetryTaskNotFound(t *testing.T) {
	r, _, _, _ := setupHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/recognition/tasks/99999/retry", nil)
	req.Header.Set("X-User-Id", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetFile(t *testing.T) {
	r, _, store, _ := setupHandler(t)
	ctx := context.Background()

	key := "transient/geometry/task_1_1.svg"
	if err := store.Put(ctx, key, []byte("image-bytes")); err != nil {
		t.Fatalf("put: %v", err)
	}

	// 携带有效签名的 URL 可访问（<img> 场景，无 JWT）。
	url := filesign.SignedURL(testFileSecret, "/api/recognition/files/", key, time.Now().Add(time.Minute))
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != "image-bytes" {
		t.Fatalf("unexpected body: %q", w.Body.String())
	}

	// 缺少签名 -> 401。
	req = httptest.NewRequest(http.MethodGet, "/api/recognition/files/"+key, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without signature, got %d", w.Code)
	}

	// 篡改签名 -> 401。
	req = httptest.NewRequest(http.MethodGet, "/api/recognition/files/"+key+"?exp=9999999999&sig=deadbeef", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with bad signature, got %d", w.Code)
	}
}

func TestGetFileEmptyKey(t *testing.T) {
	r, _, _, _ := setupHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/recognition/files/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetFileNotFound(t *testing.T) {
	r, _, _, _ := setupHandler(t)
	url := filesign.SignedURL(testFileSecret, "/api/recognition/files/", "missing.jpg", time.Now().Add(time.Minute))
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestFileURLs(t *testing.T) {
	r, svc, _, db := setupHandler(t)
	ctx := context.Background()

	// 准备任务（uid=1）及其原图记录。
	if err := db.Create(&domain.Image{StorageKey: "transient/original/123_a.jpg", ImageType: domain.ImageTypeOriginal}).Error; err != nil {
		t.Fatalf("create image: %v", err)
	}
	task, err := svc.CreateTask(ctx, 1, 1, "transient/original/123_a.jpg")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	fileURLs := func(uid string, taskID uint64, keys []string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"task_id": taskID, "keys": keys})
		req := httptest.NewRequest(http.MethodPost, "/api/recognition/file-urls", bytes.NewReader(body))
		req.Header.Set("X-User-Id", uid)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	keys := []string{
		"transient/geometry/task_" + strconv.FormatUint(task.ID, 10) + ".svg",
		"transient/geometry/task_" + strconv.FormatUint(task.ID, 10) + "_1.svg",
		"transient/original/123_a.jpg",
	}
	w := fileURLs("1", task.ID, keys)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Urls      map[string]string `json:"urls"`
			ExpiresAt int64             `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range keys {
		u, ok := resp.Data.Urls[key]
		if !ok || u == "" {
			t.Fatalf("missing url for key %s", key)
		}
	}

	// 他人签发 -> 404（不暴露存在性）。
	if w := fileURLs("2", task.ID, keys); w.Code != http.StatusNotFound {
		t.Fatalf("foreign user: expected 404, got %d", w.Code)
	}

	// 不属于该任务的 key -> 403。
	if w := fileURLs("1", task.ID, []string{"transient/geometry/task_999_1.svg"}); w.Code != http.StatusForbidden {
		t.Fatalf("foreign key: expected 403, got %d", w.Code)
	}
}


