// Package service 家庭资产账本业务逻辑。
//
// 本层是唯一接触明文的地方：仓储返回密文行，服务在内存中校验行 MAC 后解密、
// 聚合、再按需重新加密写回。任何日志都不得输出金额、名称与成员名。
package service

import (
	"context"
	"regexp"
	"strings"
	"time"

	"peak/apps/asset-service/internal/repository"
	"peak/libs/domain"
	bizerr "peak/libs/errors"
	"peak/libs/security"
)

// 金额与字段边界。
const (
	// MaxAmountCents 单条金额上限（1 亿元），避免异常输入撑爆聚合。
	MaxAmountCents int64 = 100_000_000_00
	// MaxNameLen 名称长度上限（字符数）。
	MaxNameLen = 64
	// MaxNoteLen 备注长度上限。
	MaxNoteLen = 200
	// MaxMemberNameLen 成员昵称长度上限。
	MaxMemberNameLen = 32
	// MaxMembers 成员数量上限。
	MaxMembers = 20
	// MaxListLimit 列表单页上限。
	MaxListLimit = 100
)

// Options 服务可调参数。
type Options struct {
	ReauthWindow      time.Duration
	MaxItems          int
	DeletionGraceDays int
}

// Service 账本业务服务。
type Service struct {
	repo *repository.Repository
	keys *security.Keys
	opts Options
	now  func() time.Time
}

// New 创建服务。
func New(repo *repository.Repository, keys *security.Keys, opts Options) *Service {
	if opts.MaxItems <= 0 {
		opts.MaxItems = 2000
	}
	if opts.DeletionGraceDays <= 0 {
		opts.DeletionGraceDays = domain.LedgerDeletionGraceDays
	}
	if opts.ReauthWindow <= 0 {
		opts.ReauthWindow = 5 * time.Minute
	}
	return &Service{repo: repo, keys: keys, opts: opts, now: time.Now}
}

// OwnerRef 把登录用户映射为业务伪名。
func (s *Service) OwnerRef(userID uint64) string {
	return s.keys.OwnerRef(userID)
}

// ReauthWindow 返回敏感操作允许的令牌新鲜度窗口。
func (s *Service) ReauthWindow() time.Duration { return s.opts.ReauthWindow }

// Meta 返回枚举元数据与同意状态（不需要先同意即可读取）。
func (s *Service) Meta(ctx context.Context, owner string) (*domain.LedgerMeta, error) {
	consent, err := s.repo.GetConsent(ctx, owner, domain.LedgerConsentPolicyVersion)
	if err != nil {
		return nil, err
	}
	return &domain.LedgerMeta{
		Categories:           domain.LedgerCategories(),
		RiskTypes:            domain.LedgerRiskTypes(),
		Currencies:           domain.LedgerCurrencies(),
		ConsentPolicyVersion: domain.LedgerConsentPolicyVersion,
		ConsentRequired:      consent == nil,
	}, nil
}

// ensureConsent 数据读写前校验单独同意是否已留痕。
func (s *Service) ensureConsent(ctx context.Context, repo *repository.Repository, owner string) error {
	consent, err := repo.GetConsent(ctx, owner, domain.LedgerConsentPolicyVersion)
	if err != nil {
		return err
	}
	if consent == nil {
		return bizerr.New(bizerr.CodeForbidden, "请先阅读并同意敏感个人信息处理告知")
	}
	return nil
}

// dekFor 读取或创建每用户密钥盐并派生 DEK。
func (s *Service) dekFor(ctx context.Context, repo *repository.Repository, owner string) ([]byte, int, error) {
	keyring, err := repo.GetKeyring(ctx, owner)
	if err != nil {
		return nil, 0, err
	}
	if keyring == nil {
		salt, err := security.NewSalt()
		if err != nil {
			return nil, 0, err
		}
		candidate := &domain.LedgerKeyring{OwnerRef: owner, Salt: salt, KeyVersion: s.keys.Version()}
		if err := repo.CreateKeyring(ctx, candidate); err != nil {
			// 并发首次写入：唯一键冲突时回退读取已存在的盐。
			if keyring, err = repo.GetKeyring(ctx, owner); err != nil || keyring == nil {
				return nil, 0, err
			}
		} else {
			keyring = candidate
		}
	}
	dek, err := s.keys.DeriveDEK(keyring.Salt, keyring.KeyVersion)
	if err != nil {
		return nil, 0, err
	}
	return dek, keyring.KeyVersion, nil
}

// ── 输入校验 ──────────────────────────────────────────────

// ItemInput 新增/更新条目的输入。
type ItemInput struct {
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	SubType     string  `json:"sub_type"`
	AmountCents int64   `json:"amount_cents"`
	Currency    string  `json:"currency"`
	RiskType    string  `json:"risk_type"`
	MemberID    *uint64 `json:"member_id"`
	Note        string  `json:"note"`
}

// UpdateInput 更新输入，携带客户端版本号做乐观锁。
type UpdateInput struct {
	ItemInput
	Version uint32 `json:"version"`
}

// 直接标识符识别：命中即拒绝保存（从采集端杜绝身份证/银行卡/手机号入库）。
var (
	idCardRe   = regexp.MustCompile(`[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]`)
	bankCardRe = regexp.MustCompile(`\d{16,19}`)
	phoneRe    = regexp.MustCompile(`1[3-9]\d{9}`)
)

// validateItem 校验并归一化输入。
func validateItem(in *ItemInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > MaxNameLen {
		return bizerr.New(bizerr.CodeInvalidArgument, "名称需为 1~64 个字符")
	}
	if in.AmountCents <= 0 || in.AmountCents > MaxAmountCents {
		return bizerr.New(bizerr.CodeInvalidArgument, "金额需大于 0 且不超过 1 亿元")
	}
	if !isCategory(in.Category) {
		return bizerr.New(bizerr.CodeInvalidArgument, "资产分类不合法")
	}
	if in.SubType == "" {
		in.SubType = defaultSubType(in.Category)
	}
	if !domain.LedgerSubTypeBelongsTo(in.Category, in.SubType) {
		return bizerr.New(bizerr.CodeInvalidArgument, "资产子类与分类不匹配")
	}
	if in.Currency == "" {
		in.Currency = domain.LedgerCurrencyCNY
	}
	if !isCurrency(in.Currency) {
		return bizerr.New(bizerr.CodeInvalidArgument, "币种不合法")
	}
	if in.RiskType == "" {
		in.RiskType = domain.LedgerRiskNone
	}
	if !isRiskType(in.RiskType) {
		return bizerr.New(bizerr.CodeInvalidArgument, "资产类型不合法")
	}
	in.Note = strings.TrimSpace(in.Note)
	if len([]rune(in.Note)) > MaxNoteLen {
		return bizerr.New(bizerr.CodeInvalidArgument, "备注不超过 200 个字符")
	}
	if hit := containsDirectIdentifier(in.Name + " " + in.Note); hit != "" {
		return bizerr.New(bizerr.CodeInvalidArgument, "请勿填写身份证号、银行卡号或手机号等直接标识信息（"+hit+"）")
	}
	return nil
}

// containsDirectIdentifier 返回命中的敏感模式说明。
func containsDirectIdentifier(text string) string {
	switch {
	case idCardRe.MatchString(text):
		return "疑似身份证号"
	case bankCardRe.MatchString(text):
		return "疑似银行卡号"
	case phoneRe.MatchString(text):
		return "疑似手机号"
	default:
		return ""
	}
}

func isCategory(key string) bool {
	for _, c := range domain.LedgerCategories() {
		if c.Key == key {
			return true
		}
	}
	return false
}

func isRiskType(key string) bool {
	for _, r := range domain.LedgerRiskTypes() {
		if r.Key == key {
			return true
		}
	}
	return false
}

func isCurrency(key string) bool {
	for _, c := range domain.LedgerCurrencies() {
		if c.Key == key {
			return true
		}
	}
	return false
}

func defaultSubType(category string) string {
	for _, c := range domain.LedgerCategories() {
		if c.Key == category && len(c.SubTypes) > 0 {
			return c.SubTypes[0].Key
		}
	}
	return ""
}

// notFound 统一越权/不存在语义：不泄露资源是否存在。
func notFound() error {
	return bizerr.New(bizerr.CodeNotFound, "记录不存在")
}
