package export

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPFetcherFetchesFile(t *testing.T) {
	payload := []byte("image-bytes")
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	f := NewHTTPFetcher(srv.URL+"/", 5*time.Second)
	data, err := f.Fetch(context.Background(), "geometry/task_1.svg")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("payload = %q, want %q", data, payload)
	}
	if want := "/api/recognition/files/geometry/task_1.svg"; gotPath != want {
		t.Fatalf("request path = %q, want %q", gotPath, want)
	}
}

func TestHTTPFetcherErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/recognition/files/missing.png" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	f := NewHTTPFetcher(srv.URL, time.Second)

	if _, err := f.Fetch(context.Background(), "  "); err == nil {
		t.Fatal("expected error for empty key")
	}
	if _, err := f.Fetch(context.Background(), "missing.png"); err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestHTTPFetcherRejectsEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	f := NewHTTPFetcher(srv.URL, time.Second)
	if _, err := f.Fetch(context.Background(), "a.png"); err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestHTTPFetcherWithoutBaseURL(t *testing.T) {
	f := NewHTTPFetcher("", time.Second)
	if _, err := f.Fetch(context.Background(), "a.png"); err == nil {
		t.Fatal("expected error when base url is missing")
	}
}

func TestHTTPFetcherRespectsContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	f := NewHTTPFetcher(srv.URL, time.Second)
	if _, err := f.Fetch(ctx, "a.png"); err == nil {
		t.Fatal("expected error for canceled context")
	}
}
