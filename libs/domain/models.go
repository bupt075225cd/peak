// Package domain 定义核心领域模型与 GORM 表结构，供各服务复用。
package domain

import (
	"time"

	"gorm.io/gorm"
)

// User 用户（手机验证码登录，未注册手机号首次登录自动注册；
// 亦支持邮箱+密码注册与密码登录）。
//
// Email/Phone 为指针类型：手机用户无邮箱存 NULL，邮箱注册用户无手机号存
// NULL——MySQL/SQLite 的唯一索引允许多个 NULL，两类账号互不冲突；
// PasswordHash 为空表示未设置密码（仅验证码登录）。
type User struct {
	ID        uint64         `gorm:"primaryKey" json:"id"`
	Account   string         `gorm:"size:64;uniqueIndex" json:"account"`
	Phone     *string        `gorm:"size:16;uniqueIndex" json:"phone"`
	Email     *string        `gorm:"size:128;uniqueIndex" json:"email"`
	Name      string         `gorm:"size:64" json:"name"`
	// PasswordHash bcrypt 哈希，不对外序列化；空串表示未设置密码（仅验证码登录）。
	PasswordHash string        `gorm:"size:128" json:"-"`
	EmailVerified bool         `gorm:"default:false" json:"email_verified"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Question 题目（规范化后的题目本体，可被多道错题复用）。
type Question struct {
	ID              uint64         `gorm:"primaryKey" json:"id"`
	Subject         string         `gorm:"size:32;index" json:"subject"`
	Grade           string         `gorm:"size:32;index" json:"grade"` // 年级，如"七年级上"
	StemText        string         `gorm:"type:text" json:"stem_text"`
	Answer          string         `gorm:"type:text" json:"answer"`
	Analysis        string         `gorm:"type:text" json:"analysis"`
	QuestionType    string         `gorm:"size:32" json:"question_type"`      // 选择/填空/解答
	Image           string         `gorm:"type:json" json:"image"`            // 图片引用（image key 列表，含几何图与其它科目插图）
	// KnowledgePoints 可空指针：MySQL 的 JSON 列不接受空字符串，未设置时存 NULL。
	KnowledgePoints *string        `gorm:"type:json" json:"knowledge_points"` // 知识点标签数组
	Source          string         `gorm:"size:128" json:"source"`            // 题目来源（试卷/练习册等）
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	Categories []Category `gorm:"many2many:question_categories;" json:"categories,omitempty"`
}

// ReviewRecord 单条复习记录。
type ReviewRecord struct {
	ReviewedAt time.Time `json:"reviewed_at"` // 复习时间
	Result     string    `json:"result"`      // 复习结果
}

// Mistake 错题（用户维度对某道题的错题记录）。
type Mistake struct {
	ID            uint64         `gorm:"primaryKey" json:"id"`
	UserID        uint64         `gorm:"index" json:"user_id"`
	QuestionID    uint64         `gorm:"index" json:"question_id"`
	WrongReason   string         `gorm:"size:255" json:"wrong_reason"`
	Source        string         `gorm:"size:128" json:"source"`                          // 该次错题的来源
	ReviewRecords []ReviewRecord `gorm:"type:json;serializer:json" json:"review_records"` // 复习记录
	RecordedAt    time.Time      `json:"recorded_at"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

	Question *Question `gorm:"foreignKey:QuestionID" json:"question,omitempty"`
	Images   []Image   `gorm:"foreignKey:MistakeID" json:"images,omitempty"`
}

// Image 图片元信息（文件本身存本地/对象存储）。
//
// MistakeID 可空：识别服务上传原图/文档时错题尚未创建（前端先传 /recognition/tasks
// 再保存错题），此时图片暂不属于任何错题（mistake_id 为 NULL，外键允许 NULL）；
// 图片的实际归属由 questions.image 中的 storage_key 表达。
type Image struct {
	ID         uint64         `gorm:"primaryKey" json:"id"`
	MistakeID  *uint64        `gorm:"index" json:"mistake_id,omitempty"`
	StorageKey string         `gorm:"size:255" json:"storage_key"`
	ImageType  string         `gorm:"size:32" json:"image_type"` // original/erased/crop
	Width      int            `json:"width"`
	Height     int            `json:"height"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

// Category 分类（树形：学科/知识点/标签）。
type Category struct {
	ID        uint64         `gorm:"primaryKey" json:"id"`
	ParentID  *uint64        `gorm:"index" json:"parent_id"`
	Name      string         `gorm:"size:64" json:"name"`
	Type      string         `gorm:"size:32" json:"type"` // subject/knowledge/tag
	SortOrder int            `gorm:"default:0" json:"sort_order"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// QuestionCategory 题目-分类多对多关联表。
type QuestionCategory struct {
	QuestionID uint64 `gorm:"primaryKey" json:"question_id"`
	CategoryID uint64 `gorm:"primaryKey" json:"category_id"`
}

// RecognitionTask 识别任务（异步状态机）。
type RecognitionTask struct {
	ID           uint64         `gorm:"primaryKey" json:"id"`
	UserID       uint64         `gorm:"index" json:"user_id"` // 归属用户（网关注入 X-User-Id）
	ImageID      uint64         `gorm:"index" json:"image_id"`
	Status       string         `gorm:"size:32;index" json:"status"`   // pending/processing/success/failed
	Progress     int            `gorm:"default:0" json:"progress"`     // 0-100
	ProgressText string         `gorm:"size:128" json:"progress_text"` // 当前阶段文案（如"正在几何重绘…"）
	// ResultJSON 识别结果（JSON 列）。指针类型：任务创建时为 NULL——MySQL 的 JSON 列
	// 不接受空字符串，且前端以 falsy 判断"结果未就绪"。
	ResultJSON   *string        `gorm:"type:json" json:"result_json"`
	ErrorMessage string         `gorm:"size:512" json:"error_message"`
	Provider     string         `gorm:"size:32" json:"provider"`
	RetryCount   int            `gorm:"default:0" json:"retry_count"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// 识别任务状态常量。
const (
	TaskPending    = "pending"
	TaskProcessing = "processing"
	TaskSuccess    = "success"
	TaskFailed     = "failed"
)

// 图片类型常量。
const (
	ImageTypeOriginal = "original"
	ImageTypeErased   = "erased"
	ImageTypeCrop     = "crop"
	ImageTypeDocument = "document" // word/pdf 文档
)

// 分类类型常量。
const (
	CategoryTypeSubject   = "subject"
	CategoryTypeKnowledge = "knowledge"
	CategoryTypeTag       = "tag"
)
