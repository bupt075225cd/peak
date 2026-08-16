// Package domain 定义核心领域模型与 GORM 表结构，供各服务复用。
package domain

import (
	"time"

	"gorm.io/gorm"
)

// User 用户（预留，后续迭代实现完整逻辑）。
type User struct {
	ID        uint64         `gorm:"primaryKey" json:"id"`
	Account   string         `gorm:"size:64;uniqueIndex" json:"account"`
	Name      string         `gorm:"size:64" json:"name"`
	ClassName string         `gorm:"size:64" json:"class_name"`
	Grade     string         `gorm:"size:32" json:"grade"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Question 题目（规范化后的题目本体，可被多道错题复用）。
type Question struct {
	ID           uint64         `gorm:"primaryKey" json:"id"`
	Subject      string         `gorm:"size:32;index" json:"subject"`
	StemText     string         `gorm:"type:text" json:"stem_text"`
	StemFormula  string         `gorm:"type:json" json:"stem_formula"`   // LaTeX 公式结构 JSON
	GeometryRefs string         `gorm:"type:json" json:"geometry_refs"`  // 几何图形引用（image key 列表）
	Answer       string         `gorm:"type:text" json:"answer"`
	Analysis     string         `gorm:"type:text" json:"analysis"`
	Difficulty   int            `gorm:"default:1" json:"difficulty"` // 1-5
	QuestionType string         `gorm:"size:32" json:"question_type"` // 选择/填空/解答
	SourcePaper  string         `gorm:"size:128" json:"source_paper"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	Categories []Category `gorm:"many2many:question_categories;" json:"categories,omitempty"`
}

// Mistake 错题（用户维度对某道题的错题记录）。
type Mistake struct {
	ID           uint64         `gorm:"primaryKey" json:"id"`
	UserID       uint64         `gorm:"index" json:"user_id"`
	QuestionID   uint64         `gorm:"index" json:"question_id"`
	WrongReason  string         `gorm:"size:255" json:"wrong_reason"`
	MasteryLevel int            `gorm:"default:0" json:"mastery_level"` // 0-未掌握 1-部分掌握 2-已掌握
	SourcePaper  string         `gorm:"size:128" json:"source_paper"`
	RecordedAt   time.Time      `json:"recorded_at"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	Question *Question `gorm:"foreignKey:QuestionID" json:"question,omitempty"`
	Images   []Image   `gorm:"foreignKey:MistakeID" json:"images,omitempty"`
}

// Image 图片元信息（文件本身存本地/对象存储）。
type Image struct {
	ID         uint64         `gorm:"primaryKey" json:"id"`
	MistakeID  uint64         `gorm:"index" json:"mistake_id"`
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
	ImageID      uint64         `gorm:"index" json:"image_id"`
	Status       string         `gorm:"size:32;index" json:"status"` // pending/processing/success/failed
	Progress     int            `gorm:"default:0" json:"progress"`   // 0-100
	ResultJSON   string         `gorm:"type:json" json:"result_json"`
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
