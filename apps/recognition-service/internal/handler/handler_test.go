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

	"github.com/gin-gonic/gin"
	gormlogger "gorm.io/gorm/logger"

	"peak/libs/domain"
	"peak/libs/logger"
	"peak/libs/storage"

	"peak/apps/recognition-service/internal/provider"
	"peak/apps/recognition-service/internal/service"
)

func setupHandler(t *testing.T) (*gin.Engine, *service.Service, storage.FileStorage) {
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
	h := New(svc, db, store)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.RegisterRoutes(r)
	return r, svc, store
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
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateTask(t *testing.T) {
	r, _, _ := setupHandler(t)
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

// uploadFileRequest 以 file 字段上传任意图片（手动裁剪几何图后上传）。
func uploadFileRequest(t *testing.T, r *gin.Engine, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/recognition/files", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestUploadFile(t *testing.T) {
	r, _, store := setupHandler(t)
	w := uploadFileRequest(t, r, "geo.jpg", []byte("fake-geometry-bytes"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Key == "" {
		t.Fatal("expected storage key")
	}
	// 验证存储中确有该文件。
	data, err := store.Get(context.Background(), resp.Data.Key)
	if err != nil {
		t.Fatalf("get uploaded file: %v", err)
	}
	if string(data) != "fake-geometry-bytes" {
		t.Fatalf("unexpected stored content: %q", string(data))
	}
}

func TestUploadFileMissing(t *testing.T) {
	r, _, _ := setupHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/recognition/files", bytes.NewBufferString(""))
	req.Header.Set("Content-Type", "multipart/form-data")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateTaskMissingImage(t *testing.T) {
	r, _, _ := setupHandler(t)
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
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateTaskDocument(t *testing.T) {
	r, _, _ := setupHandler(t)
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
	r, _, _ := setupHandler(t)
	w := uploadDocumentRequest(t, r, "paper.txt", []byte("not-a-doc"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetTask(t *testing.T) {
	r, svc, _ := setupHandler(t)

	// 先直接通过 service 创建一个任务（绕过上传），然后查询。
	task, err := svc.CreateTask(context.Background(), 1, "original/x.jpg")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/recognition/tasks/"+strconv.FormatUint(task.ID, 10), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetTaskInvalidID(t *testing.T) {
	r, _, _ := setupHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/recognition/tasks/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	r, _, _ := setupHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/recognition/tasks/99999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRetryTask(t *testing.T) {
	r, svc, _ := setupHandler(t)

	task, err := svc.CreateTask(context.Background(), 1, "original/y.jpg")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/recognition/tasks/"+strconv.FormatUint(task.ID, 10)+"/retry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRetryTaskNotFound(t *testing.T) {
	r, _, _ := setupHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/recognition/tasks/99999/retry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
