package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"peak/apps/user-service/internal/code"
	"peak/apps/user-service/internal/guard"
	"peak/apps/user-service/internal/mail"
	"peak/apps/user-service/internal/repository"
	"peak/apps/user-service/internal/service"
	"peak/libs/auth"
	"peak/libs/domain"
)

const testSecret = "test-secret"

// recordingSender 记录最近一次发送的验证码，供测试断言。
type recordingSender struct {
	mu   sync.Mutex
	code string
}

func (s *recordingSender) Send(phone, codeStr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.code = codeStr
	return nil
}

func (s *recordingSender) last() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.code
}

// recordingMailer 记录最近一次发送的邮件内容，供测试断言。
type recordingMailer struct {
	mu   sync.Mutex
	code string
}

func (m *recordingMailer) Send(to, subject, body string) error {
	// 提取正文中 6 位验证码（mock 发码正文唯一包含 6 位数字）。
	for i := 0; i+6 <= len(body); i++ {
		if isDigits(body[i : i+6]) {
			m.mu.Lock()
			m.code = body[i : i+6]
			m.mu.Unlock()
			return nil
		}
	}
	return nil
}

func (m *recordingMailer) last() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.code
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// 编译期断言：recordingMailer 满足 mail.Sender 接口。
var _ mail.Sender = (*recordingMailer)(nil)

type fixture struct {
	r      *gin.Engine
	sender *recordingSender
	mailer *recordingMailer
}

// setup 构建带 SQLite 存储的测试服务与路由（默认防爆破参数）。
func setup(t *testing.T, cfg service.Config) *fixture {
	return setupWithGuard(t, cfg, guard.Config{})
}

// setupWithGuard 同 setup，但允许自定义防爆破参数（便于用小阈值快速测 IP 限频）。
func setupWithGuard(t *testing.T, cfg service.Config, gcfg guard.Config) *fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := domain.OpenDB(domain.DialectSQLite, filepath.Join(t.TempDir(), "test.db"), 2)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := domain.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repos := repository.NewUserRepository(db)
	codes := code.NewStore(code.Config{})
	sender := &recordingSender{}
	mailer := &recordingMailer{}
	svc := service.New(repos, codes, sender, mailer, guard.NewGuard(gcfg), cfg)

	r := gin.New()
	New(svc).RegisterRoutes(r)
	return &fixture{r: r, sender: sender, mailer: mailer}
}

func devConfig(masterCode bool) service.Config {
	return service.Config{JWTSecret: testSecret, TokenTTL: time.Hour, MasterCode: masterCode, Debug: true}
}

func post(t *testing.T, r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-IP", "10.0.0.1")
	r.ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	return res
}

func sendCode(t *testing.T, f *fixture, phone string) (ticket, debugCode string) {
	t.Helper()
	w := post(t, f.r, "/api/users/auth/sms/code", map[string]string{"phone": phone})
	if w.Code != http.StatusOK {
		t.Fatalf("send code: status = %d, body = %s", w.Code, w.Body.String())
	}
	data := decode(t, w)["data"].(map[string]any)
	ticket, _ = data["ticket"].(string)
	debugCode, _ = data["debug_code"].(string)
	if ticket == "" {
		t.Fatal("ticket is empty")
	}
	return ticket, debugCode
}

func login(t *testing.T, f *fixture, phone, codeStr, ticket string) *httptest.ResponseRecorder {
	t.Helper()
	return post(t, f.r, "/api/users/auth/sms/login", map[string]string{
		"phone": phone, "code": codeStr, "ticket": ticket,
	})
}

func TestSendCodeInvalidPhone(t *testing.T) {
	f := setup(t, devConfig(false))
	w := post(t, f.r, "/api/users/auth/sms/code", map[string]string{"phone": "12345"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}

func TestLoginFullFlowWithRealCode(t *testing.T) {
	f := setup(t, devConfig(false))
	phone := "13800001234"

	ticket, debugCode := sendCode(t, f, phone)
	if debugCode == "" {
		t.Fatal("debug_code should be present in dev mode")
	}
	realCode := f.sender.last()
	if realCode == "" {
		t.Fatal("mock sender should have recorded the code")
	}

	// 验证码错误 → 401（消耗一次尝试机会，但验证码仍有效）。
	if w := login(t, f, phone, "000000", ticket); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong code: status = %d, want 401", w.Code)
	}

	// 用正确验证码登录 → 自动注册成功。
	w := login(t, f, phone, realCode, ticket)
	if w.Code != http.StatusOK {
		t.Fatalf("login: status = %d, body = %s", w.Code, w.Body.String())
	}

	// 自动注册：Account = 手机号，Name 含手机尾号。
	data := decode(t, w)["data"].(map[string]any)
	user := data["user"].(map[string]any)
	if user["phone"] != phone {
		t.Fatalf("user.phone = %v, want %s", user["phone"], phone)
	}
	if user["account"] != phone {
		t.Fatalf("auto-register account = %v, want %s", user["account"], phone)
	}
	if name, _ := user["name"].(string); name != "同学"+phone[len(phone)-4:] {
		t.Fatalf("name = %v, want 同学+尾号", user["name"])
	}

	// 签发的 JWT 可解析回同一用户。
	uid, err := auth.Parse(data["token"].(string), testSecret)
	if err != nil || uid == 0 {
		t.Fatalf("parse issued token: uid = %d, err = %v", uid, err)
	}

	// 验证码一次性消费：重放登录 → 401。
	if w := login(t, f, phone, realCode, ticket); w.Code != http.StatusUnauthorized {
		t.Fatalf("replay login: status = %d, want 401", w.Code)
	}
}

func TestLoginWithoutTicket(t *testing.T) {
	f := setup(t, devConfig(false))
	w := post(t, f.r, "/api/users/auth/sms/login", map[string]string{
		"phone": "13800001234", "code": "123456",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing ticket: status = %d, want 400", w.Code)
	}
}

func TestLoginWithMasterCode(t *testing.T) {
	f := setup(t, devConfig(true)) // 开发模式 + 超级验证码
	phone := "13900005678"

	ticket, _ := sendCode(t, f, phone)
	// 超级验证码登录成功（无需知道真实验证码）。
	w := login(t, f, phone, "000000", ticket)
	if w.Code != http.StatusOK {
		t.Fatalf("master code login: status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestMasterCodeDisabledInProd(t *testing.T) {
	// 生产模式（Debug=false）：超级验证码不可用。
	f := setup(t, service.Config{JWTSecret: testSecret, TokenTTL: time.Hour, MasterCode: true, Debug: false})
	phone := "13900005678"

	ticket, debugCode := sendCode(t, f, phone)
	if debugCode != "" {
		t.Fatalf("debug_code must be empty in prod, got %q", debugCode)
	}
	if w := login(t, f, phone, "000000", ticket); w.Code != http.StatusUnauthorized {
		t.Fatalf("master code in prod: status = %d, want 401", w.Code)
	}
}

func TestMeEndpoint(t *testing.T) {
	f := setup(t, devConfig(true))
	phone := "13900001111"
	ticket, _ := sendCode(t, f, phone)
	w := login(t, f, phone, "000000", ticket)
	uid := decode(t, w)["data"].(map[string]any)["user"].(map[string]any)["id"].(float64)
	if uid != 1 {
		t.Fatalf("first auto-registered uid = %v, want 1", uid)
	}

	// 带 X-User-Id 查询成功。
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req.Header.Set("X-User-Id", "1")
	f.r.ServeHTTP(w2, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("me: status = %d, body = %s", w2.Code, w2.Body.String())
	}
	if got := decode(t, w2)["data"].(map[string]any)["phone"]; got != phone {
		t.Fatalf("me phone = %v, want %s", got, phone)
	}

	// 缺少身份头 → 401。
	w3 := httptest.NewRecorder()
	f.r.ServeHTTP(w3, httptest.NewRequest(http.MethodGet, "/api/users/me", nil))
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("me without uid: status = %d, want 401", w3.Code)
	}
}

func TestUpdateName(t *testing.T) {
	f := setup(t, devConfig(true))
	phone := "13900002222"
	ticket, _ := sendCode(t, f, phone)
	if w := login(t, f, phone, "000000", ticket); w.Code != http.StatusOK {
		t.Fatalf("login: %d", w.Code)
	}

	doPut := func(body map[string]string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, "/api/users/me", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Id", "1")
		f.r.ServeHTTP(w, req)
		return w
	}

	// 正常修改 → 返回更新后的用户。
	w := doPut(map[string]string{"name": "小明同学"})
	if w.Code != http.StatusOK {
		t.Fatalf("update name: status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := decode(t, w)["data"].(map[string]any)["name"]; got != "小明同学" {
		t.Fatalf("name = %v, want 小明同学", got)
	}

	// GET /me 同步展示新昵称。
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req.Header.Set("X-User-Id", "1")
	f.r.ServeHTTP(w2, req)
	if got := decode(t, w2)["data"].(map[string]any)["name"]; got != "小明同学" {
		t.Fatalf("me name = %v, want 小明同学", got)
	}

	// 空昵称 → 400。
	if w := doPut(map[string]string{"name": "   "}); w.Code != http.StatusBadRequest {
		t.Fatalf("blank name: status = %d, want 400", w.Code)
	}

	// 超长昵称（33 字符）→ 400。
	if w := doPut(map[string]string{"name": strings.Repeat("名", 33)}); w.Code != http.StatusBadRequest {
		t.Fatalf("long name: status = %d, want 400", w.Code)
	}

	// 缺少身份头 → 401。
	w3 := httptest.NewRecorder()
	raw, _ := json.Marshal(map[string]string{"name": "x"})
	f.r.ServeHTTP(w3, httptest.NewRequest(http.MethodPut, "/api/users/me", bytes.NewReader(raw)))
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("without uid: status = %d, want 401", w3.Code)
	}
}

func TestRateLimitPerPhone(t *testing.T) {
	f := setup(t, devConfig(false))
	if w := post(t, f.r, "/api/users/auth/sms/code", map[string]string{"phone": "13800009999"}); w.Code != http.StatusOK {
		t.Fatalf("first send: %d", w.Code)
	}
	w := post(t, f.r, "/api/users/auth/sms/code", map[string]string{"phone": "13800009999"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("second send: status = %d, want 400 (rate limited)", w.Code)
	}
	if msg := decode(t, w)["message"]; msg != "发送过于频繁，请稍后再试" {
		t.Fatalf("message = %v", msg)
	}
}

// sendEmailCode 发送邮箱验证码并返回响应 recorder。
func sendEmailCode(t *testing.T, f *fixture, email, purpose string) *httptest.ResponseRecorder {
	t.Helper()
	return post(t, f.r, "/api/users/auth/email/code", map[string]string{"email": email, "purpose": purpose})
}

// emailCode 从 mock 邮件通道取最近一次发送的验证码。
func emailCode(t *testing.T, f *fixture) string {
	t.Helper()
	c := f.mailer.last()
	if c == "" {
		t.Fatal("mock mailer should have recorded the code")
	}
	return c
}

func TestEmailRegisterAndPasswordLogin(t *testing.T) {
	f := setup(t, devConfig(false))
	email := "student@peak.local"

	// 注册验证码：debug_code 返回（开发模式）。
	w := sendEmailCode(t, f, email, "register")
	if w.Code != http.StatusOK {
		t.Fatalf("send email code: status = %d, body = %s", w.Code, w.Body.String())
	}
	debugCode, _ := decode(t, w)["data"].(map[string]any)["debug_code"].(string)
	if debugCode == "" {
		t.Fatal("debug_code should be present in dev mode")
	}

	// 错误验证码注册 → 401。
	if w := post(t, f.r, "/api/users/auth/email/register", map[string]string{
		"email": email, "password": "password123", "code": "000000",
	}); w.Code != http.StatusUnauthorized {
		t.Fatalf("register with wrong code: status = %d, want 401", w.Code)
	}

	// 正确验证码注册成功，签发 JWT。
	codeStr := emailCode(t, f)
	w = post(t, f.r, "/api/users/auth/email/register", map[string]string{
		"email": email, "password": "password123", "code": codeStr,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("register: status = %d, body = %s", w.Code, w.Body.String())
	}
	data := decode(t, w)["data"].(map[string]any)
	user := data["user"].(map[string]any)
	if user["email"] != email {
		t.Fatalf("user.email = %v, want %s", user["email"], email)
	}
	if user["phone"] != nil {
		t.Fatalf("email user phone = %v, want null", user["phone"])
	}
	if _, err := auth.Parse(data["token"].(string), testSecret); err != nil {
		t.Fatalf("parse issued token: %v", err)
	}

	// 邮箱注册用户可用密码登录。
	if w := post(t, f.r, "/api/users/auth/password/login", map[string]string{
		"account": email, "password": "password123",
	}); w.Code != http.StatusOK {
		t.Fatalf("password login: status = %d, body = %s", w.Code, w.Body.String())
	}

	// 密码错误 → 401 且文案统一（不泄露具体原因）。
	w = post(t, f.r, "/api/users/auth/password/login", map[string]string{
		"account": email, "password": "wrong-password",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: status = %d, want 401", w.Code)
	}
	if msg := decode(t, w)["message"]; msg != "账号或密码不正确" {
		t.Fatalf("message = %v", msg)
	}

	// 不存在的账号同样返回"账号或密码不正确"。
	w = post(t, f.r, "/api/users/auth/password/login", map[string]string{
		"account": "nobody@peak.local", "password": "password123",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown account: status = %d, want 401", w.Code)
	}
	if msg := decode(t, w)["message"]; msg != "账号或密码不正确" {
		t.Fatalf("unknown account message = %v", msg)
	}

	// 重复注册同一邮箱：发码阶段即被拒绝。
	if w := sendEmailCode(t, f, email, "register"); w.Code != http.StatusBadRequest {
		t.Fatalf("duplicate email code: status = %d, want 400", w.Code)
	}
}

func TestPasswordResetFlow(t *testing.T) {
	f := setup(t, devConfig(false))
	email := "reset@peak.local"

	// 先注册。
	if w := sendEmailCode(t, f, email, "register"); w.Code != http.StatusOK {
		t.Fatalf("send register code: %d", w.Code)
	}
	if w := post(t, f.r, "/api/users/auth/email/register", map[string]string{
		"email": email, "password": "old-password-1", "code": emailCode(t, f),
	}); w.Code != http.StatusOK {
		t.Fatalf("register: %d, body = %s", w.Code, w.Body.String())
	}

	// 申请重置验证码 → mock 通道收到 → 重置为新密码。
	if w := sendEmailCode(t, f, email, "reset"); w.Code != http.StatusOK {
		t.Fatalf("send reset code: %d, body = %s", w.Code, w.Body.String())
	}
	if w := post(t, f.r, "/api/users/auth/password/reset", map[string]string{
		"email": email, "code": "000000", "password": "new-password-9",
	}); w.Code != http.StatusUnauthorized {
		t.Fatalf("reset with wrong code: status = %d, want 401", w.Code)
	}
	if w := post(t, f.r, "/api/users/auth/password/reset", map[string]string{
		"email": email, "code": emailCode(t, f), "password": "new-password-9",
	}); w.Code != http.StatusOK {
		t.Fatalf("reset: status = %d, body = %s", w.Code, w.Body.String())
	}

	// 旧密码失效，新密码可登录。
	if w := post(t, f.r, "/api/users/auth/password/login", map[string]string{
		"account": email, "password": "old-password-1",
	}); w.Code != http.StatusUnauthorized {
		t.Fatalf("old password after reset: status = %d, want 401", w.Code)
	}
	if w := post(t, f.r, "/api/users/auth/password/login", map[string]string{
		"account": email, "password": "new-password-9",
	}); w.Code != http.StatusOK {
		t.Fatalf("new password login: status = %d, body = %s", w.Code, w.Body.String())
	}

	// 未注册邮箱申请重置 → 404。
	if w := sendEmailCode(t, f, "ghost@peak.local", "reset"); w.Code != http.StatusNotFound {
		t.Fatalf("reset unregistered email: status = %d, want 404", w.Code)
	}
}

func TestPasswordValidation(t *testing.T) {
	f := setup(t, devConfig(false))
	email := "short@peak.local"
	if w := sendEmailCode(t, f, email, "register"); w.Code != http.StatusOK {
		t.Fatalf("send code: %d", w.Code)
	}

	// 短密码 → 400。
	if w := post(t, f.r, "/api/users/auth/email/register", map[string]string{
		"email": email, "password": "short", "code": emailCode(t, f),
	}); w.Code != http.StatusBadRequest {
		t.Fatalf("short password: status = %d, want 400", w.Code)
	}
}

// registerEmailUser 注册一个邮箱用户供密码登录用例使用。
func registerEmailUser(t *testing.T, f *fixture, email, password string) {
	t.Helper()
	if w := sendEmailCode(t, f, email, "register"); w.Code != http.StatusOK {
		t.Fatalf("send register code: %d", w.Code)
	}
	if w := post(t, f.r, "/api/users/auth/email/register", map[string]string{
		"email": email, "password": password, "code": emailCode(t, f),
	}); w.Code != http.StatusOK {
		t.Fatalf("register: %d, body = %s", w.Code, w.Body.String())
	}
}

func TestPasswordLoginLockoutAfterFiveFails(t *testing.T) {
	f := setup(t, devConfig(false))
	email := "lockout@peak.local"
	registerEmailUser(t, f, email, "correct-pass-1")

	// 连续 4 次错误密码：401（未锁定）。
	for i := 1; i <= 4; i++ {
		w := post(t, f.r, "/api/users/auth/password/login", map[string]string{
			"account": email, "password": "wrong-pass-000",
		})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("wrong attempt %d: status = %d, want 401, body = %s", i, w.Code, w.Body.String())
		}
	}

	// 第 5 次失败触发锁定。
	if w := post(t, f.r, "/api/users/auth/password/login", map[string]string{
		"account": email, "password": "wrong-pass-000",
	}); w.Code != http.StatusUnauthorized {
		t.Fatalf("5th wrong attempt: status = %d, want 401", w.Code)
	}

	// 锁定期间即使密码正确也被拒绝 → 429 + 明确提示。
	w := post(t, f.r, "/api/users/auth/password/login", map[string]string{
		"account": email, "password": "correct-pass-1",
	})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("locked login: status = %d, want 429, body = %s", w.Code, w.Body.String())
	}
	if msg := decode(t, w)["message"]; msg != "尝试次数过多，账号已锁定，请约 15 分钟后再试" {
		t.Fatalf("locked message = %v", msg)
	}
}

func TestPasswordLoginSuccessResetsFailCounter(t *testing.T) {
	f := setup(t, devConfig(false))
	email := "reset-counter@peak.local"
	registerEmailUser(t, f, email, "good-password-9")

	// 4 次失败（未达 5 次阈值）。
	for i := 0; i < 4; i++ {
		post(t, f.r, "/api/users/auth/password/login", map[string]string{
			"account": email, "password": "wrong-pass-000",
		})
	}
	// 登录成功清零计数。
	if w := post(t, f.r, "/api/users/auth/password/login", map[string]string{
		"account": email, "password": "good-password-9",
	}); w.Code != http.StatusOK {
		t.Fatalf("correct login after 4 fails: status = %d, body = %s", w.Code, w.Body.String())
	}
	// 再次 4 次失败仍不锁定（计数已清零）。
	for i := 0; i < 4; i++ {
		w := post(t, f.r, "/api/users/auth/password/login", map[string]string{
			"account": email, "password": "wrong-pass-000",
		})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("fail %d after reset: status = %d, want 401", i+1, w.Code)
		}
	}
	if w := post(t, f.r, "/api/users/auth/password/login", map[string]string{
		"account": email, "password": "good-password-9",
	}); w.Code != http.StatusOK {
		t.Fatalf("correct login after second round: status = %d, want 200", w.Code)
	}
}

func TestPasswordLoginIPRateLimit(t *testing.T) {
	// 小阈值快速验证：同 IP 3 次尝试后第 4 次拒绝（成败均计入）。
	f := setupWithGuard(t, devConfig(false), guard.Config{
		MaxFails: 100, IPWindow: 15 * time.Minute, IPMaxAttempts: 3,
	})

	for i := 1; i <= 3; i++ {
		w := post(t, f.r, "/api/users/auth/password/login", map[string]string{
			"account": "nobody@peak.local", "password": "whatever-123",
		})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i, w.Code)
		}
	}
	w := post(t, f.r, "/api/users/auth/password/login", map[string]string{
		"account": "nobody@peak.local", "password": "whatever-123",
	})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("4th attempt: status = %d, want 429, body = %s", w.Code, w.Body.String())
	}
	if msg := decode(t, w)["message"]; msg != "操作过于频繁，请稍后再试" {
		t.Fatalf("ip limited message = %v", msg)
	}
}

func TestPasswordResetGuardedByLock(t *testing.T) {
	f := setup(t, devConfig(false))
	email := "reset-lock@peak.local"
	registerEmailUser(t, f, email, "before-reset-1")

	// 连续失败 5 次密码登录触发账号锁定。
	for i := 0; i < 5; i++ {
		post(t, f.r, "/api/users/auth/password/login", map[string]string{
			"account": email, "password": "wrong-pass-000",
		})
	}
	// 锁定期间密码重置同样被拒（密码类接口共用防爆破）。
	if w := sendEmailCode(t, f, email, "reset"); w.Code != http.StatusOK {
		t.Fatalf("send reset code: %d", w.Code)
	}
	w := post(t, f.r, "/api/users/auth/password/reset", map[string]string{
		"email": email, "code": emailCode(t, f), "password": "after-reset-2",
	})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("reset while locked: status = %d, want 429, body = %s", w.Code, w.Body.String())
	}
}

func TestPasswordLoginWithPhoneUser(t *testing.T) {
	// 手机验证码注册的存量用户，也可用手机号+密码登录的前提是有密码；
	// 无密码手机用户密码登录 → 401（文案与不存在账号一致）。
	f := setup(t, devConfig(true))
	phone := "13811112222"
	ticket, _ := sendCode(t, f, phone)
	if w := login(t, f, phone, "000000", ticket); w.Code != http.StatusOK {
		t.Fatalf("sms login: %d", w.Code)
	}
	w := post(t, f.r, "/api/users/auth/password/login", map[string]string{
		"account": phone, "password": "whatever-123",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no-password phone user: status = %d, want 401", w.Code)
	}
	if msg := decode(t, w)["message"]; msg != "账号或密码不正确" {
		t.Fatalf("message = %v", msg)
	}
}

func TestSendEmailCodeRateLimit(t *testing.T) {
	f := setup(t, devConfig(false))
	email := "ratelimit@peak.local"
	if w := sendEmailCode(t, f, email, "register"); w.Code != http.StatusOK {
		t.Fatalf("first send: %d", w.Code)
	}
	// 60 秒内重发 → 400。
	if w := sendEmailCode(t, f, email, "register"); w.Code != http.StatusBadRequest {
		t.Fatalf("second send: status = %d, want 400 (rate limited)", w.Code)
	}
	// 非法邮箱 → 400。
	if w := sendEmailCode(t, f, "not-an-email", "register"); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid email: status = %d, want 400", w.Code)
	}
	// 非法用途 → 400。
	if w := sendEmailCode(t, f, "x@peak.local", "hack"); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid purpose: status = %d, want 400", w.Code)
	}
}
