package filesign

import (
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

const secret = "test-secret"

func TestSignVerifyRoundTrip(t *testing.T) {
	key := "transient/geometry/task_13_1.svg"
	exp := time.Now().Add(DefaultTTL)

	sig := Sign(secret, key, exp)
	if err := Verify(secret, key, exp.Unix(), sig, time.Now()); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// 通过 HTTP query 的完整链路。
	req := httptest.NewRequest("GET", "/f/"+key+"?exp="+strconv.FormatInt(exp.Unix(), 10)+"&sig="+sig, nil)
	if err := VerifyRequest(secret, key, req, time.Now()); err != nil {
		t.Fatalf("verify request: %v", err)
	}
}

func TestVerifyFailures(t *testing.T) {
	key := "committed/1.png"
	exp := time.Now().Add(time.Minute)

	// 篡改 key。
	if err := Verify(secret, "committed/2.png", exp.Unix(), Sign(secret, key, exp), time.Now()); err != ErrBadSig {
		t.Fatalf("forged key: err = %v, want ErrBadSig", err)
	}
	// 错误密钥。
	if err := Verify("other-secret", key, exp.Unix(), Sign(secret, key, exp), time.Now()); err != ErrBadSig {
		t.Fatalf("wrong secret: err = %v, want ErrBadSig", err)
	}
	// 过期。
	past := time.Now().Add(-time.Minute)
	if err := Verify(secret, key, past.Unix(), Sign(secret, key, past), time.Now()); err != ErrExpired {
		t.Fatalf("expired: err = %v, want ErrExpired", err)
	}
	// 缺参。
	if err := Verify(secret, key, 0, "", time.Now()); err != ErrMissingSig {
		t.Fatalf("missing: err = %v, want ErrMissingSig", err)
	}
}

func TestSignedURL(t *testing.T) {
	key := "transient/original/123_a.png"
	exp := time.Now().Add(DefaultTTL)
	u := SignedURL(secret, "/api/recognition/files/", key, exp)

	req := httptest.NewRequest("GET", u, nil)
	if err := VerifyRequest(secret, key, req, time.Now()); err != nil {
		t.Fatalf("verify signed url %q: %v", u, err)
	}
}
