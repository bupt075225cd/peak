package mail

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestSender 基于 httptest 端点构造 ResendSender，并捕获请求供断言。
func newTestSender(t *testing.T, handler http.HandlerFunc) (*ResendSender, *recordedRequest) {
	t.Helper()
	rec := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.capture(t, r)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	// BaseURL 为完整端点（含路径），与生产默认 https://api.resend.com/emails 一致。
	return NewResendSender(ResendConfig{APIKey: "re_test_key", From: "Peak <no-reply@peak.local>", BaseURL: srv.URL + "/emails"}), rec
}

type recordedRequest struct {
	method string
	path   string
	auth   string
	body   resendPayload
}

func (rec *recordedRequest) capture(t *testing.T, r *http.Request) {
	t.Helper()
	rec.method = r.Method
	rec.path = r.URL.Path
	rec.auth = r.Header.Get("Authorization")
	if err := json.NewDecoder(r.Body).Decode(&rec.body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
}

func TestResendSendSuccess(t *testing.T) {
	sender, rec := newTestSender(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"4ef9a417"}`))
	})

	if err := sender.Send("stu@peak.local", "Peak 验证码", "你的验证码是 654321，5 分钟内有效。"); err != nil {
		t.Fatalf("send: %v", err)
	}

	if rec.method != http.MethodPost || rec.path != "/emails" {
		t.Fatalf("request = %s %s, want POST /emails", rec.method, rec.path)
	}
	if rec.auth != "Bearer re_test_key" {
		t.Fatalf("authorization = %q", rec.auth)
	}
	if rec.body.From != "Peak <no-reply@peak.local>" {
		t.Fatalf("from = %q", rec.body.From)
	}
	if len(rec.body.To) != 1 || rec.body.To[0] != "stu@peak.local" {
		t.Fatalf("to = %v", rec.body.To)
	}
	if rec.body.Subject != "Peak 验证码" || rec.body.Text == "" {
		t.Fatalf("subject = %q, text = %q", rec.body.Subject, rec.body.Text)
	}
}

func TestResendSendAPIError(t *testing.T) {
	sender, _ := newTestSender(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"name":"validation_error","message":"The 'from' domain is not verified"}`))
	})

	err := sender.Send("stu@peak.local", "Peak 验证码", "code 654321")
	if err == nil {
		t.Fatal("non-2xx should return error")
	}
	// 错误含状态码与响应摘要，便于定位；不含 API Key。
	for _, want := range []string{"422", "not verified"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q should contain %q", err.Error(), want)
		}
	}
	if strings.Contains(err.Error(), "re_test_key") {
		t.Fatalf("error %q must not leak API key", err.Error())
	}
}

func TestResendSendNetworkError(t *testing.T) {
	// 指向已关闭端口模拟网络错误。
	sender := NewResendSender(ResendConfig{
		APIKey: "k", From: "f",
		BaseURL: "http://127.0.0.1:1/emails",
		Client:  &http.Client{Timeout: time.Second},
	})
	if err := sender.Send("a@b.c", "s", "t"); err == nil {
		t.Fatal("network error should return error")
	}
}

func TestResendSendLongErrorBodyTruncated(t *testing.T) {
	sender, _ := newTestSender(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		long := make([]byte, 4096)
		for i := range long {
			long[i] = 'x'
		}
		_, _ = w.Write(long)
	})
	err := sender.Send("a@b.c", "s", "t")
	if err == nil {
		t.Fatal("500 should return error")
	}
	// 摘要被截断：错误信息远短于原始 4KB 响应体。
	if len(err.Error()) > maxErrorBodyChars+64 {
		t.Fatalf("error too long: %d chars", len(err.Error()))
	}
}
