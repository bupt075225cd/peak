package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// wanxTestServer 返回模拟 wanx 异步任务（提交 -> 轮询 -> 下载）的 server，
// 结果图固定为 resultImg 字节。
func wanxTestServer(t *testing.T, resultImg string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "image-synthesis"):
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("expected bearer auth, got %q", r.Header.Get("Authorization"))
			}
			if r.Header.Get("X-DashScope-Async") != "enable" {
				t.Errorf("expected async header, got %q", r.Header.Get("X-DashScope-Async"))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output": map[string]any{"task_id": "task-123", "task_status": "PENDING"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/tasks/task-123"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output": map[string]any{
					"task_id":     "task-123",
					"task_status": "SUCCEEDED",
					"results":     []any{map[string]any{"url": srv.URL + "/result.jpg"}},
				},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/result.jpg"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte(resultImg))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestWanxEraseHandwriting(t *testing.T) {
	srv := wanxTestServer(t, "erased-bytes")
	c := NewWanxClient("test-key", "", srv.URL)

	data, err := c.EraseHandwriting(context.Background(), []byte("img"))
	if err != nil {
		t.Fatalf("erase: %v", err)
	}
	if string(data) != "erased-bytes" {
		t.Fatalf("expected erased-bytes, got %s", data)
	}
}

func TestWanxSubmitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"invalid_api_key","message":"bad key"}`))
	}))
	defer srv.Close()

	c := NewWanxClient("k", "", srv.URL)
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected submit error")
	}
}

func TestWanxSubmitEmptyTaskID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"task_id":""}}`))
	}))
	defer srv.Close()

	c := NewWanxClient("k", "", srv.URL)
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected empty task id error")
	}
}

func TestWanxTaskFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"PENDING"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"FAILED"},"code":"500","message":"boom"}`))
	}))
	defer srv.Close()

	c := NewWanxClient("k", "", srv.URL)
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected task failed error")
	}
}

func TestWanxSucceededNoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"PENDING"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"SUCCEEDED","results":[]}}`))
	}))
	defer srv.Close()

	c := NewWanxClient("k", "", srv.URL)
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected no results error")
	}
}

func TestWanxDownloadError(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"PENDING"}}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/tasks/t1") {
			_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"SUCCEEDED","results":[{"url":"` + srv.URL + `/missing.jpg"}]}}`))
			return
		}
		// 下载路径返回 404。
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewWanxClient("k", "", srv.URL)
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected download error")
	}
}

func TestWanxDefaults(t *testing.T) {
	c := NewWanxClient("k", "", "")
	if c.model != wanxDefaultModel {
		t.Fatalf("expected default model, got %s", c.model)
	}
	if c.endpoint != wanxDefaultEndpoint {
		t.Fatalf("expected default endpoint, got %s", c.endpoint)
	}
}

func TestWanxSubmitBadEndpoint(t *testing.T) {
	c := NewWanxClient("k", "", "://bad")
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected bad endpoint error")
	}
}

func TestWanxSubmitRequestError(t *testing.T) {
	c := NewWanxClient("k", "", "http://127.0.0.1:1")
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected request error")
	}
}

func TestWanxSubmitParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	c := NewWanxClient("k", "", srv.URL)
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestWanxWaitUnknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"PENDING"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"UNKNOWN"}}`))
	}))
	defer srv.Close()

	c := NewWanxClient("k", "", srv.URL)
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected unknown status error")
	}
}

func TestWanxWaitEmptyResultURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"PENDING"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"SUCCEEDED","results":[{"url":""}]}}`))
	}))
	defer srv.Close()

	c := NewWanxClient("k", "", srv.URL)
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected empty result url error")
	}
}

func TestWanxGetTaskNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"PENDING"}}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	c := NewWanxClient("k", "", srv.URL)
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected get task non-200 error")
	}
}

func TestWanxGetTaskParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"PENDING"}}`))
			return
		}
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	c := NewWanxClient("k", "", srv.URL)
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected get task parse error")
	}
}

func TestWanxDownloadEmpty(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"PENDING"}}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/tasks/t1") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"SUCCEEDED","results":[{"url":"` + srv.URL + `/empty.jpg"}]}}`))
			return
		}
		// 200 + 空 body，触发 download 空结果错误。
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewWanxClient("k", "", srv.URL)
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected download empty error")
	}
}

func TestWanxDownloadBadURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"PENDING"}}`))
			return
		}
		// 返回非法结果 URL，触发 download 的 NewRequest 错误。
		_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"SUCCEEDED","results":[{"url":"://bad"}]}}`))
	}))
	defer srv.Close()

	c := NewWanxClient("k", "", srv.URL)
	if _, err := c.EraseHandwriting(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected download bad url error")
	}
}
