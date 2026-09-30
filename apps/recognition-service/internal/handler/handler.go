// Package handler 处理识别服务的 HTTP 请求。
package handler

import (
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"peak/libs/domain"
	"peak/libs/errors"
	"peak/libs/filesign"
	httpx "peak/libs/http"
	"peak/libs/storage"

	"peak/apps/recognition-service/internal/service"
)

// Handler 识别服务 HTTP 处理器。
type Handler struct {
	svc     *service.Service
	db      *gorm.DB
	storage storage.FileStorage
	// fileSecret 文件访问 URL 签名密钥（与签发、校验共用，取 JWT_SECRET 即可）。
	fileSecret string
}

// New 创建处理器。
func New(svc *service.Service, db *gorm.DB, store storage.FileStorage, fileSecret string) *Handler {
	return &Handler{svc: svc, db: db, storage: store, fileSecret: fileSecret}
}

// RegisterRoutes 注册路由。
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/recognition")
	{
		api.POST("/tasks", h.createTask)
		api.GET("/tasks/:id", h.getTask)
		api.POST("/tasks/:id/retry", h.retryTask)
		api.POST("/file-urls", h.fileURLs)
		api.GET("/files/*key", h.getFile)
	}
}

// userID 从网关注入的 X-User-Id 头解析用户 ID（0 表示未登录）。
func userID(c *gin.Context) uint64 {
	uid, _ := strconv.ParseUint(c.GetHeader("X-User-Id"), 10, 64)
	return uid
}

// requireUserID 校验登录态，未登录时写入错误响应并返回 false。
func requireUserID(c *gin.Context) (uint64, bool) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, errors.New(errors.CodeUnauthorized, "未登录"))
		return 0, false
	}
	return uid, true
}

// getFile 按存储 key 读取文件（几何重绘 SVG、文档内嵌图等），返回原始字节。
// 需要携带 file-urls 签发的短时签名（?exp=&sig=）：<img> 无法携带 JWT，
// 以签名 URL 代替登录态，泄露后随过期自动失效。
func (h *Handler) getFile(c *gin.Context) {
	key := strings.TrimPrefix(c.Param("key"), "/")
	if key == "" {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "empty key"))
		return
	}
	if err := filesign.VerifyRequest(h.fileSecret, key, c.Request, time.Now()); err != nil {
		httpx.Fail(c, errors.New(errors.CodeUnauthorized, "文件访问签名无效或已过期"))
		return
	}
	data, err := h.storage.Get(c.Request.Context(), key)
	if err != nil {
		httpx.Fail(c, errors.Wrap(errors.CodeNotFound, "file not found", err))
		return
	}
	c.Data(200, mimeByExt(key), data)
}

// mimeByExt 按扩展名返回响应 Content-Type（几何重绘产物为 SVG）。
func mimeByExt(key string) string {
	switch strings.ToLower(filepath.Ext(key)) {
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".pdf":
		return "application/pdf"
	default:
		return "image/jpeg"
	}
}

// createTask 接收图片或文档上传，保存文件并创建识别任务。
func (h *Handler) createTask(c *gin.Context) {
	file, imageType, filename, err := h.resolveUpload(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	src, err := file.Open()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer src.Close()
	data, err := io.ReadAll(src)
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	// 保存原始文件到存储（key 保留原始扩展名，便于 process 阶段判断格式）。
	// 识别产物统一写入 transient/ 前缀（临时区）：提交错题时由 question-service
		// 拷贝到 committed/ 正式区；transient/ 由对象存储生命周期规则定期清理。
		key := "transient/original/" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + filename
	if err := h.storage.Put(c.Request.Context(), key, data); err != nil {
		httpx.Fail(c, errors.Wrap(errors.CodeStorageFail, "store file failed", err))
		return
	}

	// 记录文件元信息。
	img := &domain.Image{
		StorageKey: key,
		ImageType:  imageType,
	}
	if err := h.db.Create(img).Error; err != nil {
		httpx.Fail(c, err)
		return
	}

	// 创建识别任务（需登录态）。
	uid, ok := requireUserID(c)
	if !ok {
		return
	}
	task, err := h.svc.CreateTask(c.Request.Context(), uid, img.ID, key)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, task)
}

// resolveUpload 解析上传字段：优先 image，其次 document，返回文件、类型与文件名。
func (h *Handler) resolveUpload(c *gin.Context) (file *multipart.FileHeader, imageType, filename string, err error) {
	// 图片上传。
	if f, e := c.FormFile("image"); e == nil {
		return f, domain.ImageTypeOriginal, f.Filename, nil
	}
	// 文档上传（word/pdf）。
	if f, e := c.FormFile("document"); e == nil {
		ext := strings.ToLower(filepath.Ext(f.Filename))
		switch ext {
		case ".docx", ".pdf":
			return f, domain.ImageTypeDocument, f.Filename, nil
		case ".doc":
			return nil, "", "", errors.New(errors.CodeInvalidArgument, "请将 .doc 转换为 .docx 后上传")
		default:
			return nil, "", "", errors.New(errors.CodeInvalidArgument, "仅支持 .docx 或 .pdf 文档")
		}
	}
	return nil, "", "", errors.New(errors.CodeInvalidArgument, "image or document is required")
}

// getTask 查询任务状态（仅任务属主可见）。
func (h *Handler) getTask(c *gin.Context) {
	uid, ok := requireUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "invalid id"))
		return
	}
	task, err := h.svc.GetTask(c.Request.Context(), uid, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, task)
}

// retryTask 重试失败任务（仅任务属主）。
func (h *Handler) retryTask(c *gin.Context) {
	uid, ok := requireUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "invalid id"))
		return
	}
	if err := h.svc.RetryTask(c.Request.Context(), uid, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

// fileURLs 为任务产物签发短时签名访问 URL。
// 请求体：{"task_id":13,"keys":["transient/geometry/task_13_1.svg",...]}
// 仅允许任务属主签发，且 key 必须属于该任务（原图 key 或几何重绘产物）。
func (h *Handler) fileURLs(c *gin.Context) {
	uid, ok := requireUserID(c)
	if !ok {
		return
	}
	var req struct {
		TaskID uint64   `json:"task_id"`
		Keys   []string `json:"keys"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Keys) == 0 {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "task_id and keys are required"))
		return
	}
	task, err := h.svc.GetTask(c.Request.Context(), uid, req.TaskID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	// 任务的原图 key（供文档内嵌图等场景）。
	var originalKey string
	var img domain.Image
	if err := h.db.WithContext(c.Request.Context()).First(&img, task.ImageID).Error; err == nil {
		originalKey = img.StorageKey
	}
	geometryPrefix := fmt.Sprintf("transient/geometry/task_%d_", task.ID)

	urls := make(map[string]string, len(req.Keys))
	exp := time.Now().Add(filesign.DefaultTTL)
	for _, key := range req.Keys {
		if key != originalKey && !strings.HasPrefix(key, geometryPrefix) {
			httpx.Fail(c, errors.New(errors.CodeForbidden, "文件不属于该任务"))
			return
		}
		urls[key] = filesign.SignedURL(h.fileSecret, "/api/recognition/files/", key, exp)
	}
	httpx.OK(c, gin.H{"urls": urls, "expires_at": exp.Unix()})
}
