// Package service 认证业务：发码编排、验证码登录（自动注册）与用户查询。
package service

import (
	stderrors "errors"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"peak/apps/user-service/internal/code"
	"peak/apps/user-service/internal/guard"
	"peak/apps/user-service/internal/mail"
	"peak/apps/user-service/internal/repository"
	"peak/apps/user-service/internal/sms"
	"peak/libs/auth"
	"peak/libs/domain"
	"peak/libs/errors"
)

// 手机号格式：1 开头，第二位 3-9，共 11 位。
var phoneRe = regexp.MustCompile(`^1[3-9]\d{9}$`)

// 邮箱格式：常规 local@domain.tld，覆盖常见字符集。
var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// 密码长度限制：最短 8 位，最长 64 位（bcrypt 上限 72 字节，留余量）。
const (
	minPasswordLen = 8
	maxPasswordLen = 64
)

// 邮箱验证码用途：注册确认与密码重置。验证码存储 key 加用途前缀，
// 同一邮箱在两条流程中的验证码互不干扰。
const (
	emailPurposeRegister = "register"
	emailPurposeReset    = "reset"
)

// MasterCode 超级验证码（仅开发模式 + 配置开启时生效）。
const MasterCode = "000000"

// Config 认证服务配置。
type Config struct {
	JWTSecret  string
	TokenTTL   time.Duration
	MasterCode bool // 超级验证码（仅开发模式且配置开启时由装配方传入 true）
	Debug      bool // 开发模式：发码响应附带 debug_code 便于联调
	Log        *zap.Logger // 结构化日志（密码类认证失败记录，供异常检测）；nil 时静默
}

// Service 认证服务。
type Service struct {
	repos     repository.UserRepository
	codes     *code.Store
	sender    sms.Sender
	mailer    mail.Sender
	guard     *guard.Guard
	cfg       Config
	now       func() time.Time // 可注入时钟，便于测试
}

// New 创建认证服务。
func New(repos repository.UserRepository, codes *code.Store, sender sms.Sender, mailer mail.Sender, g *guard.Guard, cfg Config) *Service {
	return &Service{repos: repos, codes: codes, sender: sender, mailer: mailer, guard: g, cfg: cfg, now: time.Now}
}

// log 获取日志器，未注入时静默。
func (s *Service) log() *zap.Logger {
	if s.cfg.Log == nil {
		return zap.NewNop()
	}
	return s.cfg.Log
}

// checkGuard 密码类认证的防爆破前置检查：账号锁定或 IP 超限均返回 429 语义。
func (s *Service) checkGuard(account, ip string) error {
	if err := s.guard.Check(account, ip); err != nil {
		return errors.New(errors.CodeRateLimited, err.Error())
	}
	return nil
}

// recordAuthFailure 记录一次密码类认证失败：累计锁定计数并输出结构化 warn 日志。
func (s *Service) recordAuthFailure(op, account, ip, reason string) {
	locked := s.guard.OnFailure(account, ip)
	s.log().Warn("auth failure",
		zap.String("op", op),
		zap.String("account", account),
		zap.String("ip", ip),
		zap.String("reason", reason),
		zap.Bool("account_locked", locked))
}

// recordAuthSuccess 记录一次密码类认证成功：清零失败计数并输出 info 日志。
func (s *Service) recordAuthSuccess(op, account, ip string) {
	s.guard.OnSuccess(account)
	s.log().Info("auth success",
		zap.String("op", op),
		zap.String("account", account),
		zap.String("ip", ip))
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
		user = &domain.User{Account: phone, Phone: &phone, Name: "同学" + phone[len(phone)-4:]}
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

// SendEmailCodeResult 发送邮箱验证码结果：debugCode 仅开发模式返回。
type SendEmailCodeResult struct {
	DebugCode string
}

// SendEmailCode 发送邮箱验证码（注册确认 / 密码重置共用入口，按用途区分）：
// 格式与占用校验 → 限频 → 生成 → 邮件发送。
func (s *Service) SendEmailCode(ctx context.Context, email, purpose, ip string) (*SendEmailCodeResult, error) {
	if !emailRe.MatchString(email) {
		return nil, errors.New(errors.CodeInvalidArgument, "邮箱格式不正确")
	}
	switch purpose {
	case emailPurposeRegister:
		// 注册：邮箱必须未被占用（业务必要提示，便于用户改走登录）。
		if _, err := s.repos.GetByEmail(ctx, email); err == nil {
			return nil, errors.New(errors.CodeInvalidArgument, "该邮箱已注册，请直接登录")
		} else if !isNotFound(err) {
			return nil, errors.Wrap(errors.CodeInternal, "查询用户失败", err)
		}
	case emailPurposeReset:
		// 重置：邮箱必须已注册。
		if _, err := s.repos.GetByEmail(ctx, email); err != nil {
			if isNotFound(err) {
				return nil, errors.New(errors.CodeNotFound, "该邮箱未注册")
			}
			return nil, errors.Wrap(errors.CodeInternal, "查询用户失败", err)
		}
	default:
		return nil, errors.New(errors.CodeInvalidArgument, "不支持的验证码用途")
	}

	codeStr, err := s.codes.Generate(emailCodeKey(purpose, email), ip)
	if err != nil {
		return nil, errors.New(errors.CodeInvalidArgument, err.Error())
	}
	if err := s.mailer.Send(email, "Peak 验证码",
		fmt.Sprintf("你的验证码是 %s，5 分钟内有效。若非本人操作，请忽略本邮件。", codeStr)); err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "邮件发送失败", err)
	}

	res := &SendEmailCodeResult{}
	if s.cfg.Debug { // 仅开发模式附带验证码便于联调（mock 通道不真正发信）
		res.DebugCode = codeStr
	}
	return res, nil
}

// EmailRegister 邮箱+密码注册：校验 → 消费注册验证码 → 建 User（Account=邮箱，
// EmailVerified=true）→ 签发 JWT。注册成功即登录态。
func (s *Service) EmailRegister(ctx context.Context, email, password, codeStr, ip string) (*LoginResult, error) {
	if !emailRe.MatchString(email) {
		return nil, errors.New(errors.CodeInvalidArgument, "邮箱格式不正确")
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}
	if err := s.codes.Verify(emailCodeKey(emailPurposeRegister, email), codeStr); err != nil {
		return nil, errors.New(errors.CodeUnauthorized, err.Error())
	}
	// 双重校验：发码后到注册前，邮箱可能已被并发注册。
	if _, err := s.repos.GetByEmail(ctx, email); err == nil {
		return nil, errors.New(errors.CodeInvalidArgument, "该邮箱已注册，请直接登录")
	} else if !isNotFound(err) {
		return nil, errors.Wrap(errors.CodeInternal, "查询用户失败", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "密码加密失败", err)
	}
	local := email[:strings.IndexByte(email, '@')]
	if utf8.RuneCountInString(local) > 8 {
		local = string([]rune(local)[:8])
	}
	user := &domain.User{
		Account:       email,
		Email:         &email,
		PasswordHash:  string(hash),
		Name:          "同学" + local,
		EmailVerified: true, // 注册验证码已确认邮箱有效性
	}
	if err := s.repos.Create(ctx, user); err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "创建用户失败", err)
	}

	token, err := auth.Issue(user.ID, s.cfg.JWTSecret, s.cfg.TokenTTL)
	if err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "签发令牌失败", err)
	}
	return &LoginResult{Token: token, User: user}, nil
}

// dummyHash 用户不存在时的等价 bcrypt 比较，抹平时序差异防账号探测。
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("peak-timing-equalizer"), bcrypt.DefaultCost)

// PasswordLogin 密码登录：账号可为手机号或邮箱（与验证码登录并存）。
// 统一返回"账号或密码不正确"，不区分账号不存在与密码错误；
// 前置防爆破检查（账号锁定/IP 限频），失败累计锁定并记录结构化日志。
func (s *Service) PasswordLogin(ctx context.Context, account, password, ip string) (*LoginResult, error) {
	account = strings.TrimSpace(account)
	if err := s.checkGuard(account, ip); err != nil {
		return nil, err
	}
	var (
		user *domain.User
		err  error
	)
	switch {
	case phoneRe.MatchString(account):
		user, err = s.repos.GetByPhone(ctx, account)
	case emailRe.MatchString(account):
		user, err = s.repos.GetByEmail(ctx, account)
	default:
		return nil, errors.New(errors.CodeInvalidArgument, "请输入正确的手机号或邮箱")
	}
	if err != nil {
		if isNotFound(err) {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password)) // 时序抹平
			s.recordAuthFailure("password_login", account, ip, "account not found")
			return nil, errors.New(errors.CodeUnauthorized, "账号或密码不正确")
		}
		return nil, errors.Wrap(errors.CodeInternal, "查询用户失败", err)
	}
	if user.PasswordHash == "" ||
		bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		s.recordAuthFailure("password_login", account, ip, "wrong password")
		return nil, errors.New(errors.CodeUnauthorized, "账号或密码不正确")
	}

	s.recordAuthSuccess("password_login", account, ip)
	token, err := auth.Issue(user.ID, s.cfg.JWTSecret, s.cfg.TokenTTL)
	if err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "签发令牌失败", err)
	}
	return &LoginResult{Token: token, User: user}, nil
}

// ResetPassword 通过邮箱验证码重置密码：防爆破前置检查 → 消费重置验证码 →
// 更新密码哈希 → 标记邮箱已验证（验证码即邮箱所有权证明）。
func (s *Service) ResetPassword(ctx context.Context, email, codeStr, newPassword, ip string) error {
	if !emailRe.MatchString(email) {
		return errors.New(errors.CodeInvalidArgument, "邮箱格式不正确")
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	if err := s.checkGuard(email, ip); err != nil {
		return err
	}
	if err := s.codes.Verify(emailCodeKey(emailPurposeReset, email), codeStr); err != nil {
		s.recordAuthFailure("password_reset", email, ip, err.Error())
		return errors.New(errors.CodeUnauthorized, err.Error())
	}
	user, err := s.repos.GetByEmail(ctx, email)
	if err != nil {
		if isNotFound(err) {
			return errors.New(errors.CodeNotFound, "该邮箱未注册")
		}
		return errors.Wrap(errors.CodeInternal, "查询用户失败", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.Wrap(errors.CodeInternal, "密码加密失败", err)
	}
	if _, err := s.repos.UpdatePassword(ctx, user.ID, string(hash)); err != nil {
		return errors.Wrap(errors.CodeInternal, "更新密码失败", err)
	}
	if !user.EmailVerified {
		if err := s.repos.MarkEmailVerified(ctx, user.ID); err != nil {
			return errors.Wrap(errors.CodeInternal, "更新邮箱状态失败", err)
		}
	}
	s.recordAuthSuccess("password_reset", email, ip)
	return nil
}

// emailCodeKey 验证码存储 key：用途前缀隔离注册与重置两条流程。
func emailCodeKey(purpose, email string) string { return purpose + "|" + email }

// validatePassword 校验密码长度（8-64 字符）。
func validatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if n < minPasswordLen {
		return errors.New(errors.CodeInvalidArgument, "密码至少 8 位")
	}
	if n > maxPasswordLen {
		return errors.New(errors.CodeInvalidArgument, "密码不能超过 64 位")
	}
	return nil
}

// isNotFound 判断仓储错误是否为记录不存在。
func isNotFound(err error) bool {
	return errors.CodeOf(err) == errors.CodeNotFound || stderrors.Is(err, gorm.ErrRecordNotFound)
}
