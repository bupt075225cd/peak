package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SidecarRedraw geometry-sidecar 的 HTTP 客户端实现。
// sidecar 是 Python FastAPI 服务：POST /redraw（spec JSON）→ 求解 + 渲染 SVG。
//
// Deprecated: geometry-sidecar（apps/geometry-sidecar）已停用，不再接线与部署，
// 源码保留在此以便回滚。新链路由 service 层直接调用 internal/geom 渲染。
type SidecarRedraw struct {
	baseURL string
	client  *http.Client
}

// NewSidecarRedraw 创建 sidecar 客户端。baseURL 形如 http://geometry-sidecar:8090。
func NewSidecarRedraw(baseURL string, timeout time.Duration) *SidecarRedraw {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &SidecarRedraw{
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

// sidecarRedrawResponse /redraw 响应体。
type sidecarRedrawResponse struct {
	SVGs []struct {
		Title string `json:"title"`
		SVG   string `json:"svg"`
	} `json:"svgs"`
	Report GeometryRedrawReport `json:"report"`
	// Detail 错误响应体（FastAPI HTTPException 格式：{"detail": "..."}）。
	Detail string `json:"detail"`
}

// RedrawGeometry 调用 sidecar /redraw：求解约束并渲染 SVG。
func (s *SidecarRedraw) RedrawGeometry(ctx context.Context, spec string) (*GeometryRedrawResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.baseURL+"/redraw", bytes.NewReader([]byte(spec)))
	if err != nil {
		return nil, fmt.Errorf("sidecar new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sidecar request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sidecar read: %w", err)
	}

	// 400：spec 无法求解（未知约束类型、引用未定义点等），透传 sidecar 的 detail。
	if resp.StatusCode != http.StatusOK {
		var errResp sidecarRedrawResponse
		_ = json.Unmarshal(data, &errResp)
		msg := fmt.Sprintf("sidecar status %d", resp.StatusCode)
		if errResp.Detail != "" {
			msg += ": " + errResp.Detail
		}
		return nil, fmt.Errorf("%s", msg)
	}

	var out sidecarRedrawResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("sidecar parse: %w", err)
	}
	if len(out.SVGs) == 0 {
		return nil, fmt.Errorf("sidecar returned no svgs")
	}
	items := make([]RedrawSVGItem, 0, len(out.SVGs))
	for _, s := range out.SVGs {
		if s.SVG == "" {
			continue
		}
		items = append(items, RedrawSVGItem{Title: s.Title, Data: []byte(s.SVG)})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("sidecar returned empty svgs")
	}
	return &GeometryRedrawResult{
		SVGs:   items,
		Report: &out.Report,
	}, nil
}
