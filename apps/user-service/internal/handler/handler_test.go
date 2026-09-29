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

type fixture struct {
	r      *gin.Engine
	sender *recordingSender
}

// setup 构建带 SQLite 存储的测试服务与路由。
func setup(t *testing.T, cfg service.Config) *fixture {
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
	svc := service.New(repos, codes, sender, cfg)

	r := gin.New()
	New(svc).RegisterRoutes(r)
	return &fixture{r: r, sender: sender}
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
