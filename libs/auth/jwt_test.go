package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret"

func TestJWTRoundtrip(t *testing.T) {
	token, err := Issue(42, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	uid, err := Parse(token, testSecret)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if uid != 42 {
		t.Fatalf("uid = %d, want 42", uid)
	}
}

func TestJWTRoundtripLargeUID(t *testing.T) {
	token, _ := Issue(1<<40, testSecret, time.Hour)
	uid, err := Parse(token, testSecret)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if uid != 1<<40 {
		t.Fatalf("uid = %d, want %d", uid, uint64(1)<<40)
	}
}

func TestJWTWrongSecret(t *testing.T) {
	token, _ := Issue(42, testSecret, time.Hour)
	if _, err := Parse(token, "other-secret"); err == nil {
		t.Fatal("parse with wrong secret should fail")
	}
}

func TestJWTExpired(t *testing.T) {
	token, _ := Issue(42, testSecret, -time.Minute)
	if _, err := Parse(token, testSecret); err == nil {
		t.Fatal("parse expired token should fail")
	}
}

func TestJWTTampered(t *testing.T) {
	token, _ := Issue(42, testSecret, time.Hour)
	if _, err := Parse(token+"x", testSecret); err == nil {
		t.Fatal("parse tampered token should fail")
	}
}

func TestJWTParseGarbage(t *testing.T) {
	if _, err := Parse("garbage", testSecret); err == nil {
		t.Fatal("parse garbage should fail")
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
	if uid, err := Parse(tok, testSecret); err != nil || uid != 7 {
		t.Fatalf("string claim: uid = %d, err = %v", uid, err)
	}

	// 缺少 user_id claim。
	tok = makeTokenWithClaims(t, jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()})
	if _, err := Parse(tok, testSecret); err == nil {
		t.Fatal("missing user_id claim should fail")
	}

	// user_id 类型非法。
	tok = makeTokenWithClaims(t, jwt.MapClaims{"user_id": true, "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := Parse(tok, testSecret); err == nil {
		t.Fatal("invalid user_id claim type should fail")
	}

	// 非预期签名算法。
	noneTok, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"user_id": 1, "exp": time.Now().Add(time.Hour).Unix()}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("make none token: %v", err)
	}
	if _, err := Parse(noneTok, testSecret); err == nil {
		t.Fatal("none signing method should be rejected")
	}
	if _, err := Parse(tok, testSecret); err == nil {
		t.Fatal("none signing method should be rejected")
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
