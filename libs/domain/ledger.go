package domain

import (
	"time"

	"gorm.io/gorm"
)

// 账本分类（准标识符，明文存储用于索引与分组；子类与金额等敏感字段一律密文）。
const (
	LedgerCategoryLiquid     = "liquid"     // 流动资金
	LedgerCategoryFixed      = "fixed"      // 固定资产
	LedgerCategoryInvestment = "investment" // 投资理财
	LedgerCategoryReceivable = "receivable" // 应收款
	LedgerCategoryLiability  = "liability"  // 负债
)

// 资产类型（风险/流动性标签），仅作展示与筛选。
const (
	LedgerRiskCash   = "cash"   // 活钱
	LedgerRiskStable = "stable" // 稳健
	LedgerRiskLong   = "long"   // 长期
	LedgerRiskNone   = "none"   // 不标记
)

// 币种（仅标注，不做汇率换算）。
const (
	LedgerCurrencyCNY = "CNY"
	LedgerCurrencyUSD = "USD"
	LedgerCurrencyHKD = "HKD"
	LedgerCurrencyEUR = "EUR"
	LedgerCurrencyJPY = "JPY"
)

// 注销申请状态。
const (
	LedgerDeletionPending   = "pending"
	LedgerDeletionCancelled = "cancelled"
	LedgerDeletionCompleted = "completed"
)

// LedgerConsentPolicyVersion 敏感个人信息处理告知版本；
// 版本升级后用户需重新同意（compliant 留痕）。
const LedgerConsentPolicyVersion = "ledger-consent-v1-2026-10"

// LedgerDeletionGraceDays 注销冷静期（天）：期间可撤销，到期执行密码学删除。
const LedgerDeletionGraceDays = 30

// LedgerItem 账本条目。
//
// 隐私设计：表内不含 user_id 等直接标识符，只存 HMAC 伪名 OwnerRef；
// 金额、名称、子类、备注、成员名均为 AES-256-GCM 密文，AAD 绑定
// OwnerRef + RowUID + 字段名，防止跨行/跨字段重放。Category 与时间戳
// 为明文枚举，仅用于索引与按月聚合（残余风险已在 PIA 中声明）。
type LedgerItem struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	OwnerRef string `gorm:"size:64;index:idx_ledger_items_owner_category,priority:1;index:idx_ledger_items_owner_updated,priority:1" json:"-"`
	// RowUID 行级随机标识，参与 AAD 与行 MAC。
	RowUID   string `gorm:"size:32;index" json:"-"`
	Category string `gorm:"size:16;index:idx_ledger_items_owner_category,priority:2" json:"category"`

	NameCipher []byte `json:"-"`
	SubCipher  []byte `json:"-"`
	AmtCipher  []byte `json:"-"`
	NoteCipher []byte `json:"-"`
	MemCipher  []byte `json:"-"`

	// 非敏感展示字段：币种与资产类型为有限枚举，无标识性。
	Currency string `gorm:"size:8;default:CNY" json:"currency"`
	RiskType string `gorm:"size:16;default:none" json:"risk_type"`

	// MemberID 指向本用户成员标签（可为空）；成员名以密文快照保存，
	// 删除成员后历史条目仍可展示归属。
	MemberID *uint64 `gorm:"index" json:"member_id"`

	KeyVersion int    `gorm:"not null;default:1" json:"-"`
	Version    uint32 `gorm:"not null;default:1" json:"version"`
	RowMAC     []byte `json:"-"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `gorm:"index:idx_ledger_items_owner_updated,priority:2" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// LedgerMember 成员标签（仅昵称/角色，禁止填写真实姓名与证件信息）。
//
// Hidden 表示"不在家庭资产记账中显示该成员"：只影响总览与筛选里的成员入口，
// 成员本身与其名下条目都保留，避免切换显示状态时丢失数据。
// 列名取 hidden 而非 visible，是为了避开 GORM 对带 default 标签的布尔零值
// 直接跳过写入的问题（false 会被数据库默认值覆盖）。
// 该字段与加密、行 MAC 无关：它只是展示偏好，不承载敏感信息。
type LedgerMember struct {
	ID         uint64         `gorm:"primaryKey" json:"id"`
	OwnerRef   string         `gorm:"size:64;index:idx_ledger_members_owner,priority:1" json:"-"`
	RowUID     string         `gorm:"size:32;index" json:"-"`
	NameCipher []byte         `json:"-"`
	SortOrder  int            `gorm:"default:0" json:"sort_order"`
	Hidden     bool           `gorm:"not null;default:false" json:"-"`
	KeyVersion int            `gorm:"not null;default:1" json:"-"`
	RowMAC     []byte         `json:"-"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

// LedgerKeyring 每用户密钥盐：DEK = HKDF(KEK, Salt)。
// 删除本行即"密码学删除"——所有密文永久不可解，即使备份仍在。
type LedgerKeyring struct {
	OwnerRef   string    `gorm:"size:64;primaryKey" json:"-"`
	Salt       []byte    `json:"-"`
	KeyVersion int       `gorm:"not null;default:1" json:"-"`
	CreatedAt  time.Time `json:"-"`
}

// LedgerSnapshot 月度快照：PayloadCipher 为加密后的
// {total_asset_cents,total_liability_cents,net_asset_cents,breakdown[]}。
type LedgerSnapshot struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	OwnerRef      string    `gorm:"size:64;uniqueIndex:idx_ledger_snapshot_owner_period,priority:1" json:"-"`
	Period        string    `gorm:"size:7;uniqueIndex:idx_ledger_snapshot_owner_period,priority:2" json:"period"`
	PayloadCipher []byte    `json:"-"`
	KeyVersion    int       `gorm:"not null;default:1" json:"-"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// LedgerAuditLog 追加式审计日志：只记动作，不记金额与名称。
// Hash = SHA256(PrevHash || 事件字段)，PrevHash 指向上一条，
// 形成可检测篡改/截断的哈希链。
type LedgerAuditLog struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	OwnerRef   string    `gorm:"size:64;index:idx_ledger_audit_owner,priority:1" json:"-"`
	Action     string    `gorm:"size:32" json:"action"`
	TargetType string    `gorm:"size:16" json:"target_type"`
	TargetID   string    `gorm:"size:64" json:"target_id"`
	TraceID    string    `gorm:"size:64" json:"trace_id"`
	PrevHash   string    `gorm:"size:64" json:"-"`
	Hash       string    `gorm:"size:64" json:"-"`
	CreatedAt  time.Time `gorm:"index:idx_ledger_audit_owner,priority:2" json:"created_at"`
}

// LedgerAuditHead 审计链头：按 OwnerRef 记录最后一条哈希，
// 追加时以乐观重试保证并发下链不分裂。
type LedgerAuditHead struct {
	OwnerRef  string    `gorm:"size:64;primaryKey" json:"-"`
	LastHash  string    `gorm:"size:64" json:"-"`
	LastID    uint64    `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

// LedgerConsent 敏感个人信息处理单独同意留痕（IP 只存加盐哈希）。
type LedgerConsent struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	OwnerRef      string    `gorm:"size:64;uniqueIndex:idx_ledger_consent_owner_version,priority:1" json:"-"`
	PolicyVersion string    `gorm:"size:64;uniqueIndex:idx_ledger_consent_owner_version,priority:2" json:"policy_version"`
	IPHash        string    `gorm:"size:64" json:"-"`
	ConsentedAt   time.Time `json:"consented_at"`
}

// LedgerDeletionRequest 注销申请（30 天冷静期，到期执行密码学删除）。
type LedgerDeletionRequest struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	OwnerRef     string    `gorm:"size:64;uniqueIndex" json:"-"`
	Status       string    `gorm:"size:16;index" json:"status"`
	RequestedAt  time.Time `json:"requested_at"`
	ExecuteAfter time.Time `gorm:"index" json:"execute_after"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// LedgerSubTypeMeta 子类元数据（前后端单一真源，经 /meta 下发）。
type LedgerSubTypeMeta struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Icon  string `json:"icon"` // lucide 图标名，前端按名映射
	// IsFixed 参与"排除固定资产"的计算口径。
	IsFixed bool `json:"is_fixed"`
}

// LedgerCategoryMeta 分类元数据。
type LedgerCategoryMeta struct {
	Key      string              `json:"key"`
	Label    string              `json:"label"`
	Kind     string              `json:"kind"` // asset / liability
	Color    string              `json:"color"`
	SubTypes []LedgerSubTypeMeta `json:"sub_types"`
	// Description 分类口径说明：更新流程里的「什么是{分类}」与子类弹层顶部文案，
	// 与标签一样由服务端下发，避免前后端各写一份口径。
	Description string `json:"description"`
}

// LedgerRiskMeta 资产类型元数据。
type LedgerRiskMeta struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// LedgerCurrencyMeta 币种元数据。
type LedgerCurrencyMeta struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Symbol string `json:"symbol"`
}

// LedgerMeta 账本枚举元数据。
type LedgerMeta struct {
	Categories           []LedgerCategoryMeta `json:"categories"`
	RiskTypes            []LedgerRiskMeta     `json:"risk_types"`
	Currencies           []LedgerCurrencyMeta `json:"currencies"`
	ConsentPolicyVersion string               `json:"consent_policy_version"`
	ConsentRequired      bool                 `json:"consent_required"`
}

// LedgerCategories 返回分类与子类的唯一真源。
func LedgerCategories() []LedgerCategoryMeta {
	return []LedgerCategoryMeta{
		{
			Key: LedgerCategoryLiquid, Label: "流动资金", Kind: "asset", Color: "#E9A66B",
			Description: "可随用随取、即时变现的钱；大额存单、定期存单、基金、股票等流动性更低的资产推荐记入投资理财。",
			SubTypes: []LedgerSubTypeMeta{
				{Key: "bank_current", Label: "银行活期", Icon: "Landmark"},
				{Key: "bank_deposit", Label: "银行定期", Icon: "PiggyBank"},
				{Key: "cash", Label: "现金", Icon: "Wallet"},
				{Key: "wallet_app", Label: "支付宝/微信", Icon: "Smartphone"},
				{Key: "money_fund", Label: "货币基金", Icon: "Coins"},
				{Key: "liquid_custom", Label: "自定义流动资金", Icon: "CircleEllipsis"},
			},
		},
		{
			Key: LedgerCategoryFixed, Label: "固定资产", Kind: "asset", Color: "#9DBCC4",
			Description: "用于投资或自用的、流动性低的实物类资产。",
			SubTypes: []LedgerSubTypeMeta{
				{Key: "house_self", Label: "房产（自住）", Icon: "House", IsFixed: true},
				{Key: "house_invest", Label: "房产（投资）", Icon: "Building2", IsFixed: true},
				{Key: "car", Label: "汽车", Icon: "Car", IsFixed: true},
				{Key: "parking", Label: "车位", Icon: "MapPin", IsFixed: true},
				{Key: "jewelry", Label: "金银珠宝", Icon: "Gem", IsFixed: true},
				{Key: "collectible", Label: "收藏品", Icon: "Palette", IsFixed: true},
				{Key: "fixed_custom", Label: "自定义固定资产", Icon: "CircleEllipsis", IsFixed: true},
			},
		},
		{
			Key: LedgerCategoryInvestment, Label: "投资理财", Kind: "asset", Color: "#A9A3D4",
			Description: "投资于金融产品、追求保值增值的钱；房产、收藏品等实物类投资资产推荐记入固定资产。",
			SubTypes: []LedgerSubTypeMeta{
				{Key: "fund", Label: "基金", Icon: "LineChart"},
				{Key: "stock", Label: "股票", Icon: "TrendingUp"},
				{Key: "bond", Label: "债券", Icon: "Receipt"},
				{Key: "insurance", Label: "保险", Icon: "Shield"},
				{Key: "pension", Label: "养老金", Icon: "PiggyBank"},
				{Key: "investment_custom", Label: "自定义投资理财", Icon: "CircleEllipsis"},
			},
		},
		{
			Key: LedgerCategoryReceivable, Label: "应收款", Kind: "asset", Color: "#63C6A5",
			Description: "家庭资产中应收未收的款项，如借给他人的钱、为他人垫付的资金。",
			SubTypes: []LedgerSubTypeMeta{
				{Key: "loan_out", Label: "借出款", Icon: "HandCoins"},
				{Key: "other_receivable", Label: "其他应收", Icon: "FileText"},
				{Key: "receivable_custom", Label: "自定义应收款", Icon: "CircleEllipsis"},
			},
		},
		{
			Key: LedgerCategoryLiability, Label: "负债", Kind: "liability", Color: "#B9B4C7",
			Description: "家庭需要偿还的债务与欠款，如信用卡账单、房屋贷款、汽车贷款等。",
			SubTypes: []LedgerSubTypeMeta{
				{Key: "mortgage", Label: "房屋贷款", Icon: "Landmark"},
				{Key: "car_loan", Label: "车贷", Icon: "Car"},
				{Key: "credit_card", Label: "信用卡", Icon: "CreditCard"},
				{Key: "consumer_loan", Label: "消费贷", Icon: "Banknote"},
				{Key: "other_liability", Label: "其他负债", Icon: "FileText"},
				{Key: "liability_custom", Label: "自定义负债", Icon: "CircleEllipsis"},
			},
		},
	}
}

// LedgerRiskTypes 返回资产类型枚举。
func LedgerRiskTypes() []LedgerRiskMeta {
	return []LedgerRiskMeta{
		{Key: LedgerRiskCash, Label: "活钱"},
		{Key: LedgerRiskStable, Label: "稳健"},
		{Key: LedgerRiskLong, Label: "长期"},
		{Key: LedgerRiskNone, Label: "不标记"},
	}
}

// LedgerCurrencies 返回币种枚举（仅标注，不换算）。
func LedgerCurrencies() []LedgerCurrencyMeta {
	return []LedgerCurrencyMeta{
		{Key: LedgerCurrencyCNY, Label: "人民币", Symbol: "¥"},
		{Key: LedgerCurrencyUSD, Label: "美元", Symbol: "$"},
		{Key: LedgerCurrencyHKD, Label: "港币", Symbol: "HK$"},
		{Key: LedgerCurrencyEUR, Label: "欧元", Symbol: "€"},
		{Key: LedgerCurrencyJPY, Label: "日元", Symbol: "¥"},
	}
}

// LedgerCategoryLabel 返回分类中文名（未知返回原值）。
func LedgerCategoryLabel(key string) string {
	for _, c := range LedgerCategories() {
		if c.Key == key {
			return c.Label
		}
	}
	return key
}

// LedgerSubTypeLabel 返回子类中文名（未知返回原值）。
func LedgerSubTypeLabel(key string) string {
	for _, c := range LedgerCategories() {
		for _, s := range c.SubTypes {
			if s.Key == key {
				return s.Label
			}
		}
	}
	return key
}

// LedgerSubTypeBelongsTo 判断子类是否属于指定分类。
func LedgerSubTypeBelongsTo(category, subType string) bool {
	for _, c := range LedgerCategories() {
		if c.Key != category {
			continue
		}
		for _, s := range c.SubTypes {
			if s.Key == subType {
				return true
			}
		}
	}
	return false
}

// LedgerIsLiability 判断分类是否属于负债。
func LedgerIsLiability(category string) bool {
	return category == LedgerCategoryLiability
}

// LedgerPeriod 把时间格式化为快照周期（YYYY-MM）。
func LedgerPeriod(t time.Time) string {
	return t.Format("2006-01")
}
