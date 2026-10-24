package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret"

func TestJWTRoundtrip(t *testing.T) {
	token, err := Issue(42, "", testSecret, time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	uid, aud, err := Parse(token, testSecret)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if uid != 42 {
		t.Fatalf("uid = %d, want 42", uid)
	}
	if aud != "" {
		t.Fatalf("aud = %q, want empty", aud)
	}
}

func TestJWTRoundtripLargeUID(t *testing.T) {
	token, _ := Issue(1<<40, "", testSecret, time.Hour)
	uid, _, err := Parse(token, testSecret)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if uid != 1<<40 {
		t.Fatalf("uid = %d, want %d", uid, uint64(1)<<40)
	}
}

func TestJWTRoundtripWithAudience(t *testing.T) {
	// 按应用签发：aud 应完整保留在令牌中。
	token, err := Issue(7, "app-poetry", testSecret, time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	uid, aud, err := Parse(token, testSecret)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if uid != 7 || aud != "app-poetry" {
		t.Fatalf("uid = %d, aud = %q, want 7/app-poetry", uid, aud)
	}
}

func TestJWTWrongSecret(t *testing.T) {
	token, _ := Issue(42, "", testSecret, time.Hour)
	if _, _, err := Parse(token, "other-secret"); err == nil {
		t.Fatal("parse with wrong secret should fail")
	}
}

func TestJWTExpired(t *testing.T) {
	token, _ := Issue(42, "", testSecret, -time.Minute)
	if _, _, err := Parse(token, testSecret); err == nil {
		t.Fatal("parse expired token should fail")
	}
}

func TestJWTTampered(t *testing.T) {
	token, _ := Issue(42, "", testSecret, time.Hour)
	if _, _, err := Parse(token+"x", testSecret); err == nil {
		t.Fatal("parse tampered token should fail")
	}
}

func TestJWTParseGarbage(t *testing.T) {
	if _, _, err := Parse("garbage", testSecret); err == nil {
		t.Fatal("parse garbage should fail")
	}
}

func TestJWTUnusedAudienceNotShared(t *testing.T) {
	// 令牌签名覆盖 aud：为应用 A 签发的令牌无法被改写为应用 B 的受众。
	tokenA, _ := Issue(7, "app-a", testSecret, time.Hour)
	// 同一令牌在合法签名下 aud 固定为 app-a，不可篡改（签名校验兜底）。
	_, aud, err := Parse(tokenA, testSecret)
	if err != nil || aud != "app-a" {
		t.Fatalf("aud = %q, err = %v, want app-a", aud, err)
	}
}

// makeTokenWithClaims 构造指定 claims 的合法签名 token（覆盖解析分支用）。
func makeTokenWithClaims(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("make token: %v", err)
	}
	return tok
}

func TestParseUnusualClaims(t *testing.T) {
	// user_id 为字符串（非常规但应兼容）。
	tok := makeTokenWithClaims(t, jwt.MapClaims{"user_id": "7", "exp": time.Now().Add(time.Hour).Unix()})
	if uid, _, err := Parse(tok, testSecret); err != nil || uid != 7 {
		t.Fatalf("string claim: uid = %d, err = %v", uid, err)
	}

	// 缺少 user_id claim。
	tok = makeTokenWithClaims(t, jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()})
	if _, _, err := Parse(tok, testSecret); err == nil {
		t.Fatal("missing user_id claim should fail")
	}

	// user_id 类型非法。
	tok = makeTokenWithClaims(t, jwt.MapClaims{"user_id": true, "exp": time.Now().Add(time.Hour).Unix()})
	if _, _, err := Parse(tok, testSecret); err == nil {
		t.Fatal("invalid user_id claim type should fail")
	}

	// aud 为非字符串类型：忽略该 claim（返回空 aud）。
	tok = makeTokenWithClaims(t, jwt.MapClaims{"user_id": 1, "aud": 42, "exp": time.Now().Add(time.Hour).Unix()})
	if uid, aud, err := Parse(tok, testSecret); err != nil || uid != 1 || aud != "" {
		t.Fatalf("non-string aud: uid = %d, aud = %q, err = %v", uid, aud, err)
	}

	// 非预期签名算法。
	noneTok, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"user_id": 1, "exp": time.Now().Add(time.Hour).Unix()}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("make none token: %v", err)
	}
	if _, _, err := Parse(noneTok, testSecret); err == nil {
		t.Fatal("none signing method should be rejected")
	}
}

func TestParseClaimsIssuedAt(t *testing.T) {
	before := time.Now().Add(-time.Second)
	token, err := Issue(9, "app-ledger", testSecret, time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := ParseClaims(token, testSecret)
	if err != nil {
		t.Fatalf("parse claims: %v", err)
	}
	if claims.UserID != 9 || claims.Audience != "app-ledger" {
		t.Fatalf("claims = %+v", claims)
	}
	if claims.IssuedAt.Before(before) || claims.IssuedAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("iat = %v, want around now", claims.IssuedAt)
	}
	if claims.ExpiresAt.IsZero() {
		t.Fatal("exp should be parsed")
	}
}

func TestParseClaimsLegacyTokenWithoutIat(t *testing.T) {
	// 历史令牌无 iat：解析成功但签发时间为零值，敏感操作需重新登录。
	tok := makeTokenWithClaims(t, jwt.MapClaims{
		"user_id": 3,
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	claims, err := ParseClaims(tok, testSecret)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !claims.IssuedAt.IsZero() {
		t.Fatalf("iat should be zero for legacy token, got %v", claims.IssuedAt)
	}
}

func TestParseClaimsRejectsWrongSecret(t *testing.T) {
	token, _ := Issue(1, "", testSecret, time.Hour)
	if _, err := ParseClaims(token, "other"); err == nil {
		t.Fatal("wrong secret should fail")
	}
}

func TestTicketRoundtrip(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	ticket := IssueTicket("13800000001", testSecret, DefaultTicketTTL, now)
	if err := VerifyTicket("13800000001", ticket, testSecret, now.Add(time.Minute)); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestTicketWrongPhone(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	ticket := IssueTicket("13800000001", testSecret, DefaultTicketTTL, now)
	if err := VerifyTicket("13800000002", ticket, testSecret, now.Add(time.Minute)); err == nil {
		t.Fatal("ticket for another phone should fail")
	}
}

func TestTicketExpired(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	ticket := IssueTicket("13800000001", testSecret, time.Minute, now)
	if err := VerifyTicket("13800000001", ticket, testSecret, now.Add(2*time.Minute)); err == nil || !strings.Contains(err.Error(), "过期") {
		t.Fatalf("expired ticket: err = %v", err)
	}
}

func TestTicketTampered(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	ticket := IssueTicket("13800000001", testSecret, DefaultTicketTTL, now)
	if err := VerifyTicket("13800000001", ticket+"x", testSecret, now.Add(time.Minute)); err == nil {
		t.Fatal("tampered ticket should fail")
	}
	if err := VerifyTicket("13800000001", "bad-ticket", testSecret, now.Add(time.Minute)); err == nil {
		t.Fatal("malformed ticket should fail")
	}
}
