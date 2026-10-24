// Package domain 定义核心领域模型与 GORM 表结构，供各服务复用。
package domain

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// User 用户（手机验证码登录，未注册手机号首次登录自动注册；
// 亦支持邮箱+密码注册与密码登录）。
//
// Email/Phone 为指针类型：手机用户无邮箱存 NULL，邮箱注册用户无手机号存
// NULL——PostgreSQL/SQLite 的唯一索引允许多个 NULL，两类账号互不冲突；
// PasswordHash 为空表示未设置密码（仅验证码登录）。
type User struct {
	ID      uint64  `gorm:"primaryKey" json:"id"`
	Account string  `gorm:"size:64;uniqueIndex" json:"account"`
	Phone   *string `gorm:"size:16;uniqueIndex" json:"phone"`
	Email   *string `gorm:"size:128;uniqueIndex" json:"email"`
	Name    string  `gorm:"size:64" json:"name"`
	// PasswordHash bcrypt 哈希，不对外序列化；空串表示未设置密码（仅验证码登录）。
	PasswordHash  string         `gorm:"size:128" json:"-"`
	EmailVerified bool           `gorm:"default:false" json:"email_verified"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// AppClient 应用（前端客户端）注册表：多应用共用用户体系的基础设施。
//
// 各独立域名应用在用户中心注册为唯一 client（OAuth2/OIDC 多客户端模型）：
// 登录时前端携带 X-Client-Id，user-service 校验注册表后把 client_id 写入
// JWT 的 aud claim，网关按路由前缀校验 aud 与应用匹配——应用间令牌不通用，
// 登录状态互不共享。新应用接入仅需在注册表新增一条记录。
type AppClient struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	ClientID string `gorm:"size:64;uniqueIndex" json:"client_id"` // 应用标识（JWT aud）
	Name     string `gorm:"size:64" json:"name"`                  // 展示名（如"错题本"）
	// AllowedOrigins 允许的前端来源（逗号分隔，含协议，如 https://mistake.example.com）。
	AllowedOrigins string         `gorm:"size:512" json:"allowed_origins"`
	Status         string         `gorm:"size:16;default:enabled" json:"status"` // enabled / disabled
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// 应用状态常量。
const (
	AppStatusEnabled  = "enabled"
	AppStatusDisabled = "disabled"
)

// DefaultAppClients 默认注册的应用：与现有前端/网关配置保持一致。
// app-peak 为统一前端过渡期客户端（单 SPA 同时承载错题本与古诗文页面），
// 前端按域名拆分后可分别使用 app-mistake / app-poetry 并收紧网关映射。
var DefaultAppClients = []AppClient{
	{ClientID: "app-mistake", Name: "错题本", AllowedOrigins: "http://localhost:5173,http://localhost:5174", Status: AppStatusEnabled},
	{ClientID: "app-poetry", Name: "古诗文", AllowedOrigins: "http://localhost:5173,http://localhost:5174", Status: AppStatusEnabled},
	{ClientID: "app-ledger", Name: "家庭资产账本", AllowedOrigins: "http://localhost:5173,http://localhost:5174", Status: AppStatusEnabled},
	{ClientID: "app-peak", Name: "统一前端（过渡）", AllowedOrigins: "http://localhost:5173,http://localhost:5174", Status: AppStatusEnabled},
}

// Question 题目（规范化后的题目本体，可被多道错题复用）。
type Question struct {
	ID           uint64 `gorm:"primaryKey" json:"id"`
	Subject      string `gorm:"size:32;index" json:"subject"`
	Grade        string `gorm:"size:32;index" json:"grade"` // 年级，如"七年级上"
	StemText     string `gorm:"type:text" json:"stem_text"`
	Answer       string `gorm:"type:text" json:"answer"`
	Analysis     string `gorm:"type:text" json:"analysis"`
	QuestionType string `gorm:"size:32" json:"question_type"` // 选择/填空/解答
	Image        string `gorm:"type:json" json:"image"`       // 图片引用（image key 列表，含几何图与其它科目插图）
	// KnowledgePoints 可空指针：JSON 列不接受空字符串，未设置时存 NULL。
	KnowledgePoints *string        `gorm:"type:json" json:"knowledge_points"` // 知识点标签数组
	Source          string         `gorm:"size:128" json:"source"`            // 题目来源（试卷/练习册等）
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	Categories []Category `gorm:"many2many:question_categories;" json:"categories,omitempty"`
}

// BeforeSave 归一化 JSON 列，保证写入值一定是合法 JSON：
// 空字符串在 PostgreSQL 的 json 列会直接报 invalid input syntax，故无图存空数组、
// 空知识点存 NULL（与指针语义一致）。此前依赖 MySQL JSON 列的宽松行为与调用方
// 手工兜底，切换 PostgreSQL 后统一在模型层收敛，避免各地漏改。
func (q *Question) BeforeSave(tx *gorm.DB) error {
	if strings.TrimSpace(q.Image) == "" {
		q.Image = "[]"
	}
	if q.KnowledgePoints != nil && strings.TrimSpace(*q.KnowledgePoints) == "" {
		q.KnowledgePoints = nil
	}
	return nil
}

// ReviewRecord 单条复习记录。
type ReviewRecord struct {
	ReviewedAt  time.Time `json:"reviewed_at"`            // 复习时间
	Result      string    `json:"result"`                 // 复习结果
	WrongReason string    `json:"wrong_reason,omitempty"` // 本次重做仍出错的原因（结果为「正确」时为空）
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
	ID           uint64 `gorm:"primaryKey" json:"id"`
	UserID       uint64 `gorm:"index" json:"user_id"` // 归属用户（网关注入 X-User-Id）
	ImageID      uint64 `gorm:"index" json:"image_id"`
	Status       string `gorm:"size:32;index" json:"status"`   // pending/processing/success/failed
	Progress     int    `gorm:"default:0" json:"progress"`     // 0-100
	ProgressText string `gorm:"size:128" json:"progress_text"` // 当前阶段文案（如"正在几何重绘…"）
	// ResultJSON 识别结果（JSON 列）。指针类型：任务创建时为 NULL——JSON 列
	// 不接受空字符串，且前端以 falsy 判断"结果未就绪"。
	ResultJSON   *string        `gorm:"type:json" json:"result_json"`
	ErrorMessage string         `gorm:"size:512" json:"error_message"`
	Provider     string         `gorm:"size:32" json:"provider"`
	RetryCount   int            `gorm:"default:0" json:"retry_count"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// Poem 古诗文篇目（统编版初中教材，由 Markdown 语料在服务启动时幂等导入）。
// 唯一键为 (title, dynasty)：同朝代篇名不重复，跨朝代允许重名。
type Poem struct {
	ID           uint64         `gorm:"primaryKey" json:"id"`
	Title        string         `gorm:"size:128;uniqueIndex:idx_poems_title_dynasty" json:"title"`
	Dynasty      string         `gorm:"size:32;uniqueIndex:idx_poems_title_dynasty" json:"dynasty"`
	DynastyOrder int            `gorm:"index" json:"dynasty_order"` // 朝代序号（先秦=0 → 清=9），用于时间轴排序
	Author       string         `gorm:"size:128;index" json:"author"`
	Volume       string         `gorm:"size:64;index" json:"volume"`   // 册数，如"八年级下册"
	Category     string         `gorm:"size:16;index" json:"category"` // 课内 / 课外（旧版未收录篇目为空）
	Content      string         `gorm:"type:text" json:"content"`      // 正文（段落以换行分隔）
	WordCount    int            `json:"word_count"`                    // 正文汉字数（去标点空白）
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// PoemDynasties 朝代固定顺序（先秦 → 清），对应统编版教材语料章节；
// poems.dynasty_order 即该切片下标，用于时间轴与有序统计。
var PoemDynasties = []string{
	"先秦", "汉", "三国", "晋", "南北朝", "唐", "宋", "元", "明", "清",
}

// PoemDynastyOrder 返回朝代序号；未知朝代排在末尾。
func PoemDynastyOrder(dynasty string) int {
	for i, d := range PoemDynasties {
		if d == dynasty {
			return i
		}
	}
	return len(PoemDynasties)
}

// 识别任务状态常量。
const (
	TaskPending    = "pending"
	TaskProcessing = "processing"
	TaskSuccess    = "success"
	TaskFailed     = "failed"
)

// 存储分区前缀：识别产物先入 transient/ 临时区，提交错题时晋升为 committed/ 正式区。
const (
	TransientPrefix = "transient/"
	CommittedPrefix = "committed/"
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
