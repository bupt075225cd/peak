package handler

import (
	"net/http"
	"net/url"
	"path"
	"strconv"

	"github.com/gin-gonic/gin"

	"peak/libs/errors"
	httpx "peak/libs/http"

	"peak/apps/question-service/internal/export"
)

// exportRequest 导出请求体。
//
// 用 POST 承载 id 列表而非查询参数：筛选结果可能包含较多 id，URL 会超长。
type exportRequest struct {
	IDs    []uint64 `json:"ids"`
	Format string   `json:"format"`
}

// exportMistakes 导出错题为 PDF 或 Word，直接返回文件流。
func (h *Handler) exportMistakes(c *gin.Context) {
	var req exportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "invalid export request body"))
		return
	}

	format, err := export.ParseFormat(req.Format)
	if err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "unsupported export format"))
		return
	}
	if len(req.IDs) == 0 {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "ids is required"))
		return
	}

	userID, _ := strconv.ParseUint(c.GetHeader("X-User-Id"), 10, 64)
	if userID == 0 {
		httpx.Fail(c, errors.New(errors.CodeUnauthorized, "missing user id"))
		return
	}

	result, err := h.svc.ExportMistakes(c.Request.Context(), userID, req.IDs, format)
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	// 二进制响应不套 JSON 封装，直接写文件流。
	c.Header("Content-Disposition", contentDisposition(result.Filename))
	c.Data(http.StatusOK, format.ContentType(), result.Data)
}

// contentDisposition 生成兼容中文文件名的响应头。
//
// 同时给出 ASCII 兜底名与 RFC 5987 编码的 UTF-8 名，避免中文名在某些客户端乱码。
func contentDisposition(filename string) string {
	fallback := "mistakes" + path.Ext(filename)
	return `attachment; filename="` + fallback + `"; filename*=UTF-8''` + url.PathEscape(filename)
}
