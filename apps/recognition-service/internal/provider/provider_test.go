package provider

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	content := "recognition:\n  provider: aliyun\n  aliyun:\n    dash_key: dk\n"
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
	if a.dash.model != "qwen-vl-max" {
		t.Fatalf("expected default model, got %s", a.dash.model)
	}
	if !strings.Contains(a.dash.endpoint, "compatible-mode/v1/chat/completions") {
		t.Fatalf("expected default dash endpoint, got %s", a.dash.endpoint)
	}
}

// dashTestServer 返回一个模拟 DashScope OpenAI 兼容接口的 server，
// 以及记录请求的 helper。
func dashTestServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected bearer auth")
		}
		w.Header().Set("Content-Type", "application/json")
		// 正确序列化 content，避免内嵌 JSON/引号破坏响应结构。
		body, _ := json.Marshal(map[string]any{
			"choices": []any{
				map[string]any{"message": map[string]any{"content": content}},
			},
		})
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAliyunRecognizeText(t *testing.T) {
	srv := dashTestServer(t, "这是一道题目")
	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: srv.URL})

	res, err := a.RecognizeText(context.Background(), []byte("img"))
	if err != nil {
		t.Fatalf("recognize text: %v", err)
	}
	if res.Text != "这是一道题目" {
		t.Fatalf("unexpected text: %s", res.Text)
	}
}

func TestAliyunRecognizeFormula(t *testing.T) {
	srv := dashTestServer(t, "x^2 + y^2 = r^2")
	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: srv.URL})

	res, err := a.RecognizeFormula(context.Background(), []byte("img"))
	if err != nil {
		t.Fatalf("recognize formula: %v", err)
	}
	if res.LaTeX != "x^2 + y^2 = r^2" {
		t.Fatalf("unexpected latex: %s", res.LaTeX)
	}
}

func TestAliyunRecognizeGeometryJSON(t *testing.T) {
	srv := dashTestServer(t, `{"shape_type":"triangle","properties":{"角度":"90"},"description":"直角三角形"}`)
	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: srv.URL})

	res, err := a.RecognizeGeometry(context.Background(), []byte("img"))
	if err != nil {
		t.Fatalf("recognize geometry: %v", err)
	}
	if res.ShapeType != "triangle" {
		t.Fatalf("expected triangle, got %s", res.ShapeType)
	}
	if res.Description != "直角三角形" {
		t.Fatalf("unexpected description: %s", res.Description)
	}
}

func TestAliyunRecognizeGeometryFallback(t *testing.T) {
	// 模型未按 JSON 输出时，回退到 description。
	srv := dashTestServer(t, "这是一个三角形描述")
	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: srv.URL})

	res, err := a.RecognizeGeometry(context.Background(), []byte("img"))
	if err != nil {
		t.Fatalf("recognize geometry: %v", err)
	}
	if res.ShapeType != "unknown" {
		t.Fatalf("expected unknown shape, got %s", res.ShapeType)
	}
	if res.Description != "这是一个三角形描述" {
		t.Fatalf("unexpected description: %s", res.Description)
	}
}

func TestAliyunRequestError(t *testing.T) {
	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: "http://127.0.0.1:1"})
	if _, err := a.RecognizeText(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected request error")
	}
}

func TestAliyunBadEndpoint(t *testing.T) {
	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: "://bad"})
	if _, err := a.RecognizeText(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected error for bad endpoint")
	}
}

func TestAliyunErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key","code":"invalid_api_key"}}`))
	}))
	defer srv.Close()

	a := NewAliyunProvider(AliyunConfig{DashKey: "bad", DashEndpoint: srv.URL})
	if _, err := a.RecognizeText(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected error response")
	}
}

func TestAliyunEraseHandwritingFallback(t *testing.T) {
	// 手写擦除降级：直接返回原图，不发起网络调用。
	a := NewAliyunProvider(AliyunConfig{DashKey: "k"})
	res, err := a.EraseHandwriting(context.Background(), []byte("orig"))
	if err != nil {
		t.Fatalf("erase: %v", err)
	}
	if string(res.ImageData) != "orig" {
		t.Fatalf("expected original image, got %s", res.ImageData)
	}
}

func TestParseGeometry(t *testing.T) {
	g := parseGeometry("some description")
	if g.ShapeType != "unknown" || g.Description != "some description" {
		t.Fatalf("unexpected geometry: %+v", g)
	}
}

func TestParseGeometryWithBBox(t *testing.T) {
	g := parseGeometry(`{"shape_type":"triangle","description":"直角三角形","bounding_box":{"x":0.1,"y":0.2,"width":0.5,"height":0.4}}`)
	if g.ShapeType != "triangle" {
		t.Fatalf("unexpected shape: %s", g.ShapeType)
	}
	if g.BoundingBox == nil {
		t.Fatal("expected bounding box")
	}
	if g.BoundingBox.X != 0.1 || g.BoundingBox.Y != 0.2 || g.BoundingBox.Width != 0.5 || g.BoundingBox.Height != 0.4 {
		t.Fatalf("unexpected bbox: %+v", g.BoundingBox)
	}
}

func TestNormalizeBBox(t *testing.T) {
	tests := []struct {
		name string
		in   *BoundingBox
		want *BoundingBox // nil 表示期望无效
	}{
		{"nil", nil, nil},
		{"zero size", &BoundingBox{X: 0, Y: 0, Width: 0, Height: 0.5}, nil},
		{"negative width", &BoundingBox{X: 0, Y: 0, Width: -1, Height: 0.5}, nil},
		{"x out of range", &BoundingBox{X: 2, Y: 0, Width: 0.5, Height: 0.5}, nil},
		{"valid", &BoundingBox{X: 0.1, Y: 0.2, Width: 0.5, Height: 0.4}, &BoundingBox{X: 0.1, Y: 0.2, Width: 0.5, Height: 0.4}},
		{"clamp overflow", &BoundingBox{X: 0.8, Y: 0.8, Width: 0.5, Height: 0.5}, &BoundingBox{X: 0.8, Y: 0.8, Width: 0.2, Height: 0.2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeBBox(tt.in)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil bbox")
			}
			if !approxEqual(got.X, tt.want.X) || !approxEqual(got.Y, tt.want.Y) ||
				!approxEqual(got.Width, tt.want.Width) || !approxEqual(got.Height, tt.want.Height) {
				t.Fatalf("got %+v, want %+v", *got, *tt.want)
			}
		})
	}
}

// approxEqual 浮点近似比较，容忍 1e-9 误差。
func approxEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}

func TestParseStructured(t *testing.T) {
	// 模型可能带 ```json 代码块，需正确剥离并解析。
	in := "```json\n" +
		`[{"stem_text":"18. 如图，AB//CD…","sub_questions":[{"label":"(2)","text":"如图2…求值","geometry_desc":"直线MN交角平分线"}]}]` +
		"\n```"
	items, err := parseStructured(in)
	if err != nil {
		t.Fatalf("parse structured: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].StemText == "" {
		t.Fatal("expected stem_text")
	}
	if len(items[0].SubQuestions) != 1 {
		t.Fatalf("expected 1 sub-question, got %d", len(items[0].SubQuestions))
	}
	if items[0].SubQuestions[0].Label != "(2)" {
		t.Fatalf("unexpected sub-question label: %s", items[0].SubQuestions[0].Label)
	}
	if items[0].SubQuestions[0].GeometryDesc == "" {
		t.Fatal("expected geometry_desc")
	}
}

func TestParseStructuredPlain(t *testing.T) {
	// 无代码块的纯 JSON 数组。
	in := `[{"stem_text":"1. 题","sub_questions":[]}]`
	items, err := parseStructured(in)
	if err != nil {
		t.Fatalf("parse structured: %v", err)
	}
	if len(items) != 1 || items[0].StemText != "1. 题" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestParseStructuredInvalid(t *testing.T) {
	if _, err := parseStructured("not json at all"); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestParseOcrGeo(t *testing.T) {
	out := "文字：18. 如图，AB//CD\n几何：三角形ABC，∠EOF=100°"
	text, geo := parseOcrGeo(out)
	if text != "18. 如图，AB//CD" {
		t.Fatalf("unexpected text: %q", text)
	}
	if geo != "三角形ABC，∠EOF=100°" {
		t.Fatalf("unexpected geo: %q", geo)
	}
}

// buildTestDocx 构造含文本段落的 docx（无内嵌图片，避免图片 OCR 干扰）。
func buildTestDocx(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>相交线与平行线（角度计算与证明）</w:t></w:r></w:p>
    <w:p><w:r><w:t>18. 如图，AB//CD，点E、F分别在直线AB、CD上。</w:t></w:r></w:p>
  </w:body>
</w:document>`
	w, _ := zw.Create("word/document.xml")
	w.Write([]byte(docXML))
	zw.Close()
	return buf.Bytes()
}

// buildTestDocxWithImage 构造含一段文本 + 一张内嵌图片（drawing + rels）的 docx。
func buildTestDocxWithImage(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>18. 如图，AB//CD。</w:t></w:r></w:p>
    <w:p><w:r><w:drawing><a:blip r:embed="rId1"/></w:drawing></w:r></w:p>
  </w:body>
</w:document>`
	w1, _ := zw.Create("word/document.xml")
	w1.Write([]byte(docXML))
	w2, _ := zw.Create("word/media/image1.png")
	w2.Write([]byte("fake-png"))
	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/>
</Relationships>`
	w3, _ := zw.Create("word/_rels/document.xml.rels")
	w3.Write([]byte(rels))
	zw.Close()
	return buf.Bytes()
}

func TestExtractStructured(t *testing.T) {
	// mock dashscope：纯文本调用返回结构化 JSON。
	structuredJSON := `[{"stem_text":"18. 如图，AB//CD，点E、F分别在直线AB、CD上。","sub_questions":[{"label":"(2)","text":"如图2，直线MN交角平分线，求值。","geometry_desc":"直线MN交两角平分线"}]}]`
	srv := dashTestServer(t, structuredJSON)
	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: srv.URL})

	res, err := a.ExtractStructured(context.Background(), buildTestDocx(t), "paper.docx")
	if err != nil {
		t.Fatalf("extract structured: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
	if res.Items[0].StemText == "" {
		t.Fatal("expected stem_text")
	}
	if len(res.Items[0].SubQuestions) != 1 {
		t.Fatalf("expected 1 sub-question, got %d", len(res.Items[0].SubQuestions))
	}
}

func TestExtractStructuredWithImage(t *testing.T) {
	// 含内嵌图片的 docx：图片调用返回“文字/几何”两行，纯文本调用返回结构化 JSON。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		var content string
		if bytes.Contains(body, []byte("image_url")) {
			// 图片 OCR 调用。
			content = "文字：18. 如图，AB//CD\n几何：三角形ABC，∠EOF=100°"
		} else {
			// 纯文本结构化拆题调用。
			content = `[{"stem_text":"18. 如图，AB//CD，点E、F分别在直线AB、CD上。","sub_questions":[{"label":"(2)","text":"求∠EMN-∠FNM的值","geometry_desc":"三角形ABC，∠EOF=100°"}]}]`
		}
		resp, _ := json.Marshal(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": content}}},
		})
		w.Write(resp)
	}))
	defer srv.Close()

	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: srv.URL})

	res, err := a.ExtractStructured(context.Background(), buildTestDocxWithImage(t), "paper.docx")
	if err != nil {
		t.Fatalf("extract structured with image: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
	if len(res.Items[0].SubQuestions) != 1 {
		t.Fatalf("expected 1 sub-question, got %d", len(res.Items[0].SubQuestions))
	}
}

func TestExtractStructuredFallback(t *testing.T) {
	// 模型返回非 JSON 时，整体作为一道题兜底。
	srv := dashTestServer(t, "这不是 JSON")
	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: srv.URL})

	res, err := a.ExtractStructured(context.Background(), buildTestDocx(t), "paper.docx")
	if err != nil {
		t.Fatalf("extract structured should not error on fallback: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 fallback item, got %d", len(res.Items))
	}
}

func TestExtractStructuredUnsupported(t *testing.T) {
	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: "http://127.0.0.1:1"})
	if _, err := a.ExtractStructured(context.Background(), []byte("x"), "a.doc"); err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestExtractDocumentAliyun(t *testing.T) {
	// 含文本 + 内嵌图片的 docx：图片走 OCR（mock dashscope），文本直接透传。
	srv := dashTestServer(t, "图片OCR文字")
	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: srv.URL})

	res, err := a.ExtractDocument(context.Background(), buildTestDocxWithImage(t), "paper.docx")
	if err != nil {
		t.Fatalf("extract document: %v", err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("expected 2 items (1 text + 1 image->text), got %d", len(res.Items))
	}
	// 图片项已被 OCR 替换为文本项。
	if res.Items[1].Kind != "text" || res.Items[1].Text != "图片OCR文字" {
		t.Fatalf("unexpected image item: %+v", res.Items[1])
	}
}

func TestExtractDocumentMock(t *testing.T) {
	m := NewMockProvider()
	res, err := m.ExtractDocument(context.Background(), buildTestDocx(t), "paper.docx")
	if err != nil {
		t.Fatalf("extract document mock: %v", err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("expected 2 text items, got %d", len(res.Items))
	}
}

func TestExtractStructuredMock(t *testing.T) {
	m := NewMockProvider()
	res, err := m.ExtractStructured(context.Background(), buildTestDocx(t), "paper.docx")
	if err != nil {
		t.Fatalf("extract structured mock: %v", err)
	}
	if len(res.Items) == 0 {
		t.Fatal("expected at least 1 item")
	}
}

func TestDashEmptyResponse(t *testing.T) {
	// 响应无 choices 时应返回错误。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: srv.URL})
	if _, err := a.RecognizeText(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestDashParseError(t *testing.T) {
	// 响应非 JSON 时应返回解析错误。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: srv.URL})
	if _, err := a.RecognizeText(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestExtractDocumentImageOCRFail(t *testing.T) {
	// 图片 OCR 失败时，ExtractDocument 应保留原始 image 项（降级），而非报错。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"boom","code":"500"}}`))
	}))
	defer srv.Close()

	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: srv.URL})
	res, err := a.ExtractDocument(context.Background(), buildTestDocxWithImage(t), "paper.docx")
	if err != nil {
		t.Fatalf("extract document should not fail: %v", err)
	}
	// 应包含 1 个文本项 + 1 个降级保留的 image 项。
	if len(res.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(res.Items))
	}
	if res.Items[1].Kind != "image" {
		t.Fatalf("expected image item preserved, got kind=%s", res.Items[1].Kind)
	}
}

func TestExtractStructuredImageOCRFail(t *testing.T) {
	// 图片 OCR 失败（chat 返回错误）但结构化拆题成功时，仍应产出结果（图片 OCR 文本为空）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		var content string
		if bytes.Contains(body, []byte("image_url")) {
			// 图片 OCR 调用：返回错误。
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":{"message":"boom","code":"500"}}`))
			return
		}
		// 纯文本结构化拆题：返回 JSON。
		content = `[{"stem_text":"18. 如图，AB//CD。","sub_questions":[]}]`
		resp, _ := json.Marshal(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": content}}},
		})
		w.Write(resp)
	}))
	defer srv.Close()

	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: srv.URL})
	res, err := a.ExtractStructured(context.Background(), buildTestDocxWithImage(t), "paper.docx")
	if err != nil {
		t.Fatalf("extract structured should tolerate image ocr failure: %v", err)
	}
	if res == nil || len(res.Items) == 0 {
		t.Fatal("expected at least 1 item")
	}
}

func TestRecognizeFormulaError(t *testing.T) {
	// dashscope 返回错误时 RecognizeFormula 应透传错误。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"boom","code":"500"}}`))
	}))
	defer srv.Close()

	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: srv.URL})
	if _, err := a.RecognizeFormula(context.Background(), []byte("img")); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseOcrGeoWithColonAndEmpty(t *testing.T) {
	// 覆盖 "文字:" / "几何:" 半角冒号分支，以及无匹配行。
	text, geo := parseOcrGeo("文字:abc\n几何:def\n无关行")
	if text != "abc" {
		t.Fatalf("unexpected text: %q", text)
	}
	if geo != "def" {
		t.Fatalf("unexpected geo: %q", geo)
	}
}
