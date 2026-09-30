// Package filesign 提供文件访问 URL 的短时签名与校验。
//
// 背景：前端以 <img src> 加载文件，无法携带 Authorization 头；为避免把
// 文件端点完全公开，采用「签发端点校验属主后签发短时签名 URL」的模式：
//  1. 受保护的签发端点（file-urls）先校验用户对文件的属主关系；
//  2. 签发 ?exp=<unix秒>&sig=<hmac> 形式的 URL，签名覆盖 key 与过期时间；
//  3. 文件流式端点校验签名与有效期后放行，无需 JWT。
package filesign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Query 参数名。
const (
	ParamExp = "exp"
	ParamSig = "sig"
)

// DefaultTTL 签名默认有效期。
const DefaultTTL = 15 * time.Minute

var (
	ErrMissingSig = errors.New("filesign: missing signature")
	ErrExpired    = errors.New("filesign: link expired")
	ErrBadSig     = errors.New("filesign: invalid signature")
)

// Sign 计算 key 在 exp 时刻过期时的签名（hex 编码 HMAC-SHA256）。
func Sign(secret, key string, exp time.Time) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s|%d", key, exp.Unix())
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify 校验请求中的签名与有效期。任一失败返回对应错误。
func Verify(secret, key string, expUnix int64, sig string, now time.Time) error {
	if sig == "" || expUnix == 0 {
		return ErrMissingSig
	}
	if now.Unix() >= expUnix {
		return ErrExpired
	}
	want := Sign(secret, key, time.Unix(expUnix, 0))
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return ErrBadSig
	}
	return nil
}

// VerifyRequest 从 HTTP 请求的 query 中取出 exp/sig 并校验。
func VerifyRequest(secret, key string, r *http.Request, now time.Time) error {
	exp, _ := strconv.ParseInt(r.URL.Query().Get(ParamExp), 10, 64)
	return Verify(secret, key, exp, r.URL.Query().Get(ParamSig), now)
}

// SignedURL 为流式端点构造带签名的访问路径：
// <basePath><key>?exp=<unix>&sig=<hex>。basePath 形如 "/api/recognition/files/"。
func SignedURL(secret, basePath, key string, exp time.Time) string {
	return fmt.Sprintf("%s%s?%s=%d&%s=%s",
		basePath, key, ParamExp, exp.Unix(), ParamSig, Sign(secret, key, exp))
}
