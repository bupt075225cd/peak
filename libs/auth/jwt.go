// Package auth 提供网关与 user-service 共用的 JWT 签发/解析
// 与登录发码凭证（ticket）能力。
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// claim 键名。
const (
	claimUserID = "user_id"
	claimAud    = "aud" // 受众：签发该令牌的应用 client_id（多应用隔离，可空=历史令牌）
)

// Issue 签发 HS256 JWT，claims 为 {user_id, aud?, exp}。
//
// aud 为签发该令牌的应用 client_id（应用受众）。多应用共用用户体系时，
// 各应用独立登录、令牌不通用：网关按路由前缀校验 aud 与应用匹配。
// aud 为空表示未按应用签发（历史令牌/内部调用），由网关侧兼容开关决定放行策略。
func Issue(userID uint64, aud, secret string, ttl time.Duration) (string, error) {
	claims := jwt.MapClaims{
		claimUserID: userID,
		"exp":       time.Now().Add(ttl).Unix(),
	}
	if aud != "" {
		claims[claimAud] = aud
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// Parse 校验并解析 JWT，返回 user_id 与 aud（应用受众，可能为空）；
// 过期/签名不符/格式非法均返回错误。
func Parse(tokenStr, secret string) (uint64, string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return 0, "", err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, "", errors.New("invalid token")
	}
	raw, ok := claims[claimUserID]
	if !ok {
		return 0, "", errors.New("missing user_id claim")
	}
	// JSON 数字解码为 float64。
	var uid uint64
	switch v := raw.(type) {
	case float64:
		uid = uint64(v)
	case string:
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return 0, "", fmt.Errorf("invalid user_id claim: %w", err)
		}
		uid = id
	default:
		return 0, "", errors.New("invalid user_id claim type")
	}
	// aud 为可选 claim：缺省返回空串。
	aud := ""
	if v, ok := claims[claimAud].(string); ok {
		aud = v
	}
	return uid, aud, nil
}

// DefaultTicketTTL 发码凭证有效期（需覆盖验证码有效期）。
const DefaultTicketTTL = 10 * time.Minute

// IssueTicket 签发登录凭证：HMAC-SHA256(phone|nonce|exp)，
// 绑定手机号与有效期，登录时必须携带，防止绕过发码直接爆破登录。
// 格式：<exp>.<nonce>.<base64url(signature)>。
func IssueTicket(phone, secret string, ttl time.Duration, now time.Time) string {
	exp := now.Add(ttl).Unix()
	nonce := fmt.Sprintf("%d", now.UnixNano())
	payload := phone + "|" + nonce + "|" + strconv.FormatInt(exp, 10)
	sig := hmacSHA256(secret, payload)
	return strconv.FormatInt(exp, 10) + "." + nonce + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// VerifyTicket 校验登录凭证：签名必须匹配（constant-time）、未过期，
// 且凭证中的手机号与登录手机号一致。
func VerifyTicket(phone, ticket, secret string, now time.Time) error {
	parts := strings.Split(ticket, ".")
	if len(parts) != 3 {
		return fmt.Errorf("登录凭证无效")
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return fmt.Errorf("登录凭证无效")
	}
	if now.Unix() > exp {
		return fmt.Errorf("登录凭证已过期，请重新获取验证码")
	}
	payload := phone + "|" + parts[1] + "|" + parts[0]
	want := hmacSHA256(secret, payload)
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(want, got) {
		return fmt.Errorf("登录凭证无效")
	}
	return nil
}

func hmacSHA256(secret, payload string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}
