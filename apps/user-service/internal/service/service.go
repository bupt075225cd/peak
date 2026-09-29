// Package service 认证业务：发码编排、验证码登录（自动注册）与用户查询。
package service

import (
	stderrors "errors"
	"context"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"peak/apps/user-service/internal/code"
	"peak/apps/user-service/internal/repository"
	"peak/apps/user-service/internal/sms"
	"peak/libs/auth"
	"peak/libs/domain"
	"peak/libs/errors"
)

// 手机号格式：1 开头，第二位 3-9，共 11 位。
var phoneRe = regexp.MustCompile(`^1[3-9]\d{9}$`)

// MasterCode 超级验证码（仅开发模式 + 配置开启时生效）。
const MasterCode = "000000"

// Config 认证服务配置。
type Config struct {
	JWTSecret  string
	TokenTTL   time.Duration
	MasterCode bool // 超级验证码（仅开发模式且配置开启时由装配方传入 true）
	Debug      bool // 开发模式：发码响应附带 debug_code 便于联调
}

// Service 认证服务。
type Service struct {
	repos     repository.UserRepository
	codes     *code.Store
	sender    sms.Sender
	cfg       Config
	now       func() time.Time // 可注入时钟，便于测试
}

// New 创建认证服务。
func New(repos repository.UserRepository, codes *code.Store, sender sms.Sender, cfg Config) *Service {
	return &Service{repos: repos, codes: codes, sender: sender, cfg: cfg, now: time.Now}
}

// SendCodeResult 发码结果：ticket 供登录携带，debugCode 仅开发模式返回。
type SendCodeResult struct {
	Ticket    string
	DebugCode string // 仅开发模式非空，生产为空
}

// SendCode 发送验证码：限频校验 → 生成 → 存储 → 发送 → 签发登录凭证。
func (s *Service) SendCode(ctx context.Context, phone, ip string) (*SendCodeResult, error) {
	if !phoneRe.MatchString(phone) {
		return nil, errors.New(errors.CodeInvalidArgument, "手机号格式不正确")
	}

	codeStr, err := s.codes.Generate(phone, ip)
	if err != nil {
		return nil, errors.New(errors.CodeInvalidArgument, err.Error())
	}
	if err := s.sender.Send(phone, codeStr); err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "验证码发送失败", err)
	}

	res := &SendCodeResult{
		Ticket: auth.IssueTicket(phone, s.cfg.JWTSecret, auth.DefaultTicketTTL, s.now()),
	}
	if s.cfg.Debug { // 仅开发模式（装配方控制），附带提示便于联调
		res.DebugCode = codeStr
	}
	return res, nil
}

// LoginResult 登录结果。
type LoginResult struct {
	Token string
	User  *domain.User
}

// SmsLogin 验证码登录：校验凭证 → 校验验证码 → 查/建用户 → 签发 JWT。
// 未注册手机号自动注册（Account=手机号），中学生用户无需单独注册步骤。
func (s *Service) SmsLogin(ctx context.Context, phone, codeStr, ticket, ip string) (*LoginResult, error) {
	if !phoneRe.MatchString(phone) {
		return nil, errors.New(errors.CodeInvalidArgument, "手机号格式不正确")
	}
	if ticket == "" {
		return nil, errors.New(errors.CodeInvalidArgument, "缺少登录凭证，请先获取验证码")
	}
	if err := auth.VerifyTicket(phone, ticket, s.cfg.JWTSecret, s.now()); err != nil {
		return nil, errors.New(errors.CodeUnauthorized, err.Error())
	}

	// 超级验证码：配置开启 且 开发模式，双开关缺一不可（服务层硬性兜底）。
	if !(s.cfg.MasterCode && s.cfg.Debug && codeStr == MasterCode) {
		if err := s.codes.Verify(phone, codeStr); err != nil {
			return nil, errors.New(errors.CodeUnauthorized, err.Error())
		}
	}

	user, err := s.repos.GetByPhone(ctx, phone)
	if errors.CodeOf(err) == errors.CodeNotFound || stderrors.Is(err, gorm.ErrRecordNotFound) {
		// 自动注册。
		user = &domain.User{Account: phone, Phone: phone, Name: "同学" + phone[len(phone)-4:]}
		if err := s.repos.Create(ctx, user); err != nil {
			return nil, errors.Wrap(errors.CodeInternal, "创建用户失败", err)
		}
	} else if err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "查询用户失败", err)
	}


	token, err := auth.Issue(user.ID, s.cfg.JWTSecret, s.cfg.TokenTTL)
	if err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "签发令牌失败", err)
	}
	return &LoginResult{Token: token, User: user}, nil
}

// Me 查询当前用户信息。
func (s *Service) Me(ctx context.Context, userID uint64) (*domain.User, error) {
	u, err := s.repos.Get(ctx, userID)
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(errors.CodeNotFound, "user not found")
		}
		return nil, errors.Wrap(errors.CodeInternal, "查询用户失败", err)
	}
	return u, nil
}

// maxNameLen 昵称最大长度（字符数）。
const maxNameLen = 32

// UpdateName 修改当前用户昵称：去首尾空白，非空且不超过 32 字符。
func (s *Service) UpdateName(ctx context.Context, userID uint64, name string) (*domain.User, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New(errors.CodeInvalidArgument, "昵称不能为空")
	}
	if utf8.RuneCountInString(name) > maxNameLen {
		return nil, errors.New(errors.CodeInvalidArgument, "昵称不能超过 32 个字符")
	}
	u, err := s.repos.UpdateName(ctx, userID, name)
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(errors.CodeNotFound, "user not found")
		}
		return nil, errors.Wrap(errors.CodeInternal, "更新昵称失败", err)
	}
	return u, nil
}
