// Package handler 处理识别服务的 HTTP 请求。
package handler

import (
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
	httpx "peak/libs/http"
	"peak/libs/storage"

	"peak/apps/recognition-service/internal/service"
)

// Handler 识别服务 HTTP 处理器。
type Handler struct {
	svc     *service.Service
	db      *gorm.DB
	storage storage.FileStorage
}

// New 创建处理器。
func New(svc *service.Service, db *gorm.DB, store storage.FileStorage) *Handler {
	return &Handler{svc: svc, db: db, storage: store}
}

// RegisterRoutes 注册路由。
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/recognition")
	{
		api.POST("/tasks", h.createTask)
		api.GET("/tasks/:id", h.getTask)
		api.POST("/tasks/:id/retry", h.retryTask)
		api.GET("/files/*key", h.getFile)
		api.POST("/files", h.uploadFile)
	}
}

// getFile 按存储 key 读取文件（擦除图、文档内嵌图等），返回原始字节。
func (h *Handler) getFile(c *gin.Context) {
	key := strings.TrimPrefix(c.Param("key"), "/")
	if key == "" {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "empty key"))
		return
	}
	data, err := h.storage.Get(c.Request.Context(), key)
	if err != nil {
		httpx.Fail(c, errors.Wrap(errors.CodeNotFound, "file not found", err))
		return
	}
	c.Data(200, "image/jpeg", data)
}

// uploadFile 上传任意图片字节（供前端手动裁剪几何图后上传），返回 storage key。
// 不创建识别任务，仅存储文件，便于录入错题时关联。
func (h *Handler) uploadFile(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "file is required"))
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
	if len(data) == 0 {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "empty file"))
		return
	}

	key := "geometry/" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + filepath.Base(file.Filename)
	if err := h.storage.Put(c.Request.Context(), key, data); err != nil {
		httpx.Fail(c, errors.Wrap(errors.CodeStorageFail, "store file failed", err))
		return
	}

	httpx.OK(c, gin.H{"key": key})
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
	key := "original/" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + filename
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

	// 创建识别任务。
	task, err := h.svc.CreateTask(c.Request.Context(), img.ID, key)
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

// getTask 查询任务状态。
func (h *Handler) getTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "invalid id"))
		return
	}
	task, err := h.svc.GetTask(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, task)
}

// retryTask 重试失败任务。
func (h *Handler) retryTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "invalid id"))
		return
	}
	if err := h.svc.RetryTask(c.Request.Context(), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}
