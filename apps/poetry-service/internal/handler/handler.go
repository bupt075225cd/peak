// Package handler 古诗文 HTTP 处理器：路由注册、参数解析与响应编码。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"peak/apps/poetry-service/internal/repository"
	"peak/apps/poetry-service/internal/service"
	"peak/libs/errors"
	"peak/libs/http"
)

// Handler HTTP 处理器。
type Handler struct {
	svc *service.Service
}

// New 创建处理器实例。
func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes 注册路由（网关不剥离前缀，路径与后端一致）。
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/poetry")
	{
		api.GET("/pieces", h.listPoems)
		api.GET("/pieces/:id", h.getPoem)
		api.GET("/pieces/:id/related", h.relatedPoems)
		api.GET("/stats/dynasties", h.dynastyStats)
		api.GET("/stats/volumes", h.volumeStats)
		api.GET("/stats/authors", h.authorStats)
		api.GET("/stats/words", h.wordStats)
		api.POST("/chat", h.chat)
	}
}

// listPoems GET /api/poetry/pieces?dynasty=&author=&volume=&category=&keyword=&offset=&limit=
func (h *Handler) listPoems(c *gin.Context) {
	offset, limit := pageParams(c)
	poems, total, err := h.svc.ListPoems(c.Request.Context(), filterParams(c, offset, limit))
	if err != nil {
		http.Fail(c, err)
		return
	}
	http.OK(c, gin.H{"items": poems, "total": total})
}

// getPoem GET /api/poetry/pieces/:id
func (h *Handler) getPoem(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		http.Fail(c, err)
		return
	}
	poem, err := h.svc.GetPoem(c.Request.Context(), id)
	if err != nil {
		http.Fail(c, err)
		return
	}
	http.OK(c, poem)
}

// relatedPoems GET /api/poetry/pieces/:id/related
func (h *Handler) relatedPoems(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		http.Fail(c, err)
		return
	}
	poem, err := h.svc.GetPoem(c.Request.Context(), id)
	if err != nil {
		http.Fail(c, err)
		return
	}
	related, err := h.svc.RelatedPoems(c.Request.Context(), poem, 4)
	if err != nil {
		http.Fail(c, err)
		return
	}
	http.OK(c, related)
}

// dynastyStats GET /api/poetry/stats/dynasties
func (h *Handler) dynastyStats(c *gin.Context) {
	stats, err := h.svc.DynastyStats(c.Request.Context())
	if err != nil {
		http.Fail(c, err)
		return
	}
	http.OK(c, stats)
}

// volumeStats GET /api/poetry/stats/volumes
func (h *Handler) volumeStats(c *gin.Context) {
	stats, err := h.svc.VolumeStats(c.Request.Context())
	if err != nil {
		http.Fail(c, err)
		return
	}
	http.OK(c, stats)
}

// authorStats GET /api/poetry/stats/authors?top=10
func (h *Handler) authorStats(c *gin.Context) {
	stats, err := h.svc.Authors(c.Request.Context(), intParam(c, "top", 10))
	if err != nil {
		http.Fail(c, err)
		return
	}
	http.OK(c, stats)
}

// wordStats GET /api/poetry/stats/words?top=100
func (h *Handler) wordStats(c *gin.Context) {
	words, total, err := h.svc.WordFreq(c.Request.Context(), intParam(c, "top", 100))
	if err != nil {
		http.Fail(c, err)
		return
	}
	http.OK(c, gin.H{"items": words, "total_chars": total})
}

// chat POST /api/poetry/chat —— RAG 智能对话占位接口（契约先行）。
func (h *Handler) chat(c *gin.Context) {
	var req service.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		http.Fail(c, errors.New(errors.CodeInvalidArgument, err.Error()))
		return
	}
	http.OK(c, h.svc.Chat(c.Request.Context(), req))
}

// parseID 解析路径参数 :id。
func parseID(c *gin.Context) (uint64, error) {
	id, err := service.ParseUint(c.Param("id"))
	if err != nil || id == 0 {
		return 0, errors.New(errors.CodeInvalidArgument, "invalid id")
	}
	return id, nil
}

// pageParams 解析分页参数（offset/limit，limit 上限 100）。
func pageParams(c *gin.Context) (offset, limit int) {
	return intParam(c, "offset", 0), intParam(c, "limit", 20)
}

// intParam 解析整型查询参数，非法或缺省时返回 def。
func intParam(c *gin.Context, key string, def int) int {
	raw := c.Query(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if n < 0 {
		return def
	}
	if key == "limit" && n > 100 {
		return 100
	}
	return n
}

// filterParams 构建列表筛选条件。
func filterParams(c *gin.Context, offset, limit int) repository.Filter {
	return repository.Filter{
		Dynasty:  c.Query("dynasty"),
		Author:   c.Query("author"),
		Volume:   c.Query("volume"),
		Category: c.Query("category"),
		Keyword:  c.Query("keyword"),
		Offset:   offset,
		Limit:    limit,
	}
}
