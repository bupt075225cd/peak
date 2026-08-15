package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"peak/libs/config"
)

func mustLoad(t *testing.T, path string) *config.Loader {
	t.Helper()
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

func TestMockProvider(t *testing.T) {
	m := NewMockProvider()
	ctx := context.Background()

	if m.Name() != "mock" {
		t.Fatalf("unexpected name: %s", m.Name())
	}

	text, err := m.RecognizeText(ctx, []byte("abc"))
	if err != nil || text.Text == "" || text.Confidence != 0.99 {
		t.Fatalf("unexpected text result: %+v err=%v", text, err)
	}

	f, err := m.RecognizeFormula(ctx, []byte("abc"))
	if err != nil || f.LaTeX == "" {
		t.Fatalf("unexpected formula: %+v err=%v", f, err)
	}

	e, err := m.EraseHandwriting(ctx, []byte("abc"))
	if err != nil || string(e.ImageData) != "abc" {
		t.Fatalf("unexpected erasure: %+v err=%v", e, err)
	}

	g, err := m.RecognizeGeometry(ctx, []byte("abc"))
	if err != nil || g.ShapeType != "triangle" {
		t.Fatalf("unexpected geometry: %+v err=%v", g, err)
	}
}

func TestFactoryMock(t *testing.T) {
	// 临时写一个 mock 配置。
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(path, []byte("recognition:\n  provider: mock\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoad(t, path)

	p, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if p.Name() != "mock" {
		t.Fatalf("expected mock, got %s", p.Name())
	}
}

func TestFactoryAliyun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	content := "recognition:\n  provider: aliyun\n  aliyun:\n    access_key_id: ak\n    dash_key: dk\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoad(t, path)

	p, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if p.Name() != "aliyun" {
		t.Fatalf("expected aliyun, got %s", p.Name())
	}
}

func TestFactoryUnsupported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(path, []byte("recognition:\n  provider: aws\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoad(t, path)
	if _, err := NewFromConfig(cfg); err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestAliyunProviderDefaults(t *testing.T) {
	a := NewAliyunProvider(AliyunConfig{})
	if a.Name() != "aliyun" {
		t.Fatalf("expected aliyun, got %s", a.Name())
	}
	if a.dashModel != "qwen-vl-max" {
		t.Fatalf("expected default model, got %s", a.dashModel)
	}
	if a.ocrEndpoint == "" {
		t.Fatal("expected default ocr endpoint")
	}
}

func TestAliyunRecognizeText(t *testing.T) {
	// 模拟阿里云 OCR 服务。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"text":"hello formula"}`))
	}))
	defer srv.Close()

	a := NewAliyunProvider(AliyunConfig{OCREndpoint: srv.URL})
	res, err := a.RecognizeText(context.Background(), []byte("img"))
	if err != nil {
		t.Fatalf("recognize text: %v", err)
	}
	if res.Text != "hello formula" {
		t.Fatalf("unexpected text: %s", res.Text)
	}
}

func TestAliyunRecognizeFormula(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"latex":"x^2","text":"x squared"}`))
	}))
	defer srv.Close()

	a := NewAliyunProvider(AliyunConfig{OCREndpoint: srv.URL})
	res, err := a.RecognizeFormula(context.Background(), []byte("img"))
	if err != nil {
		t.Fatalf("recognize formula: %v", err)
	}
	if res.LaTeX != "x^2" || res.RawText != "x squared" {
		t.Fatalf("unexpected formula result: %+v", res)
	}
}

func TestAliyunOCRError(t *testing.T) {
	// 返回非 JSON 内容，触发解析错误。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	a := NewAliyunProvider(AliyunConfig{OCREndpoint: srv.URL})
	if _, err := a.RecognizeText(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestAliyunOCRRequestError(t *testing.T) {
	// 不可达地址触发网络错误。
	a := NewAliyunProvider(AliyunConfig{OCREndpoint: "http://127.0.0.1:1"})
	if _, err := a.RecognizeText(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected request error")
	}
}

func TestAliyunRecognizeFormulaError(t *testing.T) {
	// 不可达地址触发公式识别的错误分支。
	a := NewAliyunProvider(AliyunConfig{OCREndpoint: "http://127.0.0.1:1"})
	if _, err := a.RecognizeFormula(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected error for formula recognition")
	}
}

func TestAliyunOCRBadEndpoint(t *testing.T) {
	// 非法 URL 使 NewRequestWithContext 失败。
	a := NewAliyunProvider(AliyunConfig{OCREndpoint: "://bad"})
	if _, err := a.RecognizeText(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected error for bad ocr endpoint")
	}
}

func TestDecodeImage(t *testing.T) {
	if _, e := decodeImage("no image"); e == nil {
		t.Fatal("decodeImage should error for non-image output")
	}
}

func TestAliyunProviderDashEndpointDefault(t *testing.T) {
	a := NewAliyunProvider(AliyunConfig{})
	if a.dashEndpoint == "" {
		t.Fatal("expected default dash endpoint")
	}
}

func TestEraseHandwritingFallbackOnNonImage(t *testing.T) {
	// DashScope 返回无法解析出图片的内容，应回退返回原图。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"output":{"text":"no image here"}}`))
	}))
	defer srv.Close()

	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: srv.URL})
	res, err := a.EraseHandwriting(context.Background(), []byte("orig"))
	if err != nil {
		t.Fatalf("erase: %v", err)
	}
	if string(res.ImageData) != "orig" {
		t.Fatalf("expected fallback to original image, got %s", res.ImageData)
	}
}

func TestEraseHandwritingRequestError(t *testing.T) {
	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: "http://127.0.0.1:1"})
	if _, err := a.EraseHandwriting(context.Background(), []byte("orig")); err == nil {
		t.Fatal("expected request error")
	}
}

func TestRecognizeGeometry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"output":{"text":"triangle description"}}`))
	}))
	defer srv.Close()

	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: srv.URL})
	res, err := a.RecognizeGeometry(context.Background(), []byte("img"))
	if err != nil {
		t.Fatalf("geometry: %v", err)
	}
	if res.ShapeType != "unknown" {
		t.Fatalf("expected unknown shape, got %s", res.ShapeType)
	}
	if res.Description == "" {
		t.Fatal("expected description from output")
	}
}

func TestRecognizeGeometryRequestError(t *testing.T) {
	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: "http://127.0.0.1:1"})
	if _, err := a.RecognizeGeometry(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected request error")
	}
}

func TestCallDashVLBadEndpoint(t *testing.T) {
	// endpoint 为非法 URL，NewRequest 会失败。
	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: "://bad"})
	if _, err := a.callDashVL(context.Background(), "p", []byte("i")); err == nil {
		t.Fatal("expected error for bad endpoint")
	}
}

func TestParseGeometry(t *testing.T) {
	g := parseGeometry("some description")
	if g.ShapeType != "unknown" || g.Description != "some description" {
		t.Fatalf("unexpected geometry: %+v", g)
	}
}

func TestExtractText(t *testing.T) {
	if extractText(map[string]any{"text": "hi"}) != "hi" {
		t.Fatal("expected hi")
	}
	if extractText(map[string]any{}) != "" {
		t.Fatal("expected empty")
	}
}

func TestExtractLaTeX(t *testing.T) {
	if extractLaTeX(map[string]any{"latex": "x"}) != "x" {
		t.Fatal("expected x")
	}
	if extractLaTeX(map[string]any{}) != "" {
		t.Fatal("expected empty")
	}
}
