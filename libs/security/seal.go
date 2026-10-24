package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

// 密文封装参数。
const (
	// NonceSize GCM 标准 nonce 长度（12 字节）。
	NonceSize = 12
	// TagSize GCM 认证标签长度。
	TagSize = 16
)

var (
	// ErrDecrypt 解密失败（密钥不符、密文被篡改或 AAD 不匹配）。
	ErrDecrypt = errors.New("security: 解密失败")
	// ErrCipherShort 密文长度不足，无法包含 nonce 与认证标签。
	ErrCipherShort = errors.New("security: 密文长度非法")
)

// Seal 使用 AES-256-GCM 加密，输出 nonce || ciphertext || tag。
//
// aad 必须绑定 owner_ref、记录标识与字段名（如 "owner|rowuid|amount"），
// 使密文无法在某行与另一行、或某个字段与另一个字段之间被搬运。
func (k *Keys) Seal(dek, plaintext, aad []byte) ([]byte, error) {
	gcm, err := newGCM(dek)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, NonceSize+len(plaintext)+TagSize)
	out = append(out, nonce...)
	out = gcm.Seal(out, nonce, plaintext, aad)
	return out, nil
}

// Open 校验并解密 Seal 的输出。
func (k *Keys) Open(dek, sealed, aad []byte) ([]byte, error) {
	if len(sealed) < NonceSize+TagSize {
		return nil, ErrCipherShort
	}
	gcm, err := newGCM(dek)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, sealed[:NonceSize], sealed[NonceSize:], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}

// BuildAAD 构造字段级附加认证数据：主体 + 记录标识 + 字段名。
func BuildAAD(ownerRef, rowUID, field string) []byte {
	return Canonical(ownerRef, rowUID, field)
}

func newGCM(dek []byte) (cipher.AEAD, error) {
	if len(dek) != KeySize {
		return nil, fmt.Errorf("%w: DEK 长度必须为 %d 字节", ErrKeyInvalid, KeySize)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// randomBytes 生成密码学安全随机字节。
func randomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// NewRowUID 生成行级随机标识（16 字节十六进制），参与 AAD 与行 MAC，
// 使同一用户的不同记录之间密文不可互换。
func NewRowUID() (string, error) {
	buf, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf), nil
}

// hkdfSHA256 实现 RFC 5869 的 HKDF（extract + expand），零第三方依赖。
func hkdfSHA256(secret, salt, info []byte, length int) []byte {
	extract := hmac.New(sha256.New, salt)
	extract.Write(secret)
	prk := extract.Sum(nil)

	out := make([]byte, 0, length)
	var prev []byte
	for counter := byte(1); len(out) < length; counter++ {
		expander := hmac.New(sha256.New, prk)
		expander.Write(prev)
		expander.Write(info)
		expander.Write([]byte{counter})
		prev = expander.Sum(nil)
		out = append(out, prev...)
	}
	return out[:length]
}

// RandomToken 生成指定字节数的十六进制随机串（如 IP 哈希盐）。
func RandomToken(n int) string {
	buf, err := randomBytes(n)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", buf)
}

// HashIP 对来源 IP 做加盐哈希，仅用于同意留痕，避免明文存储网络标识。
func (k *Keys) HashIP(ip string) string {
	mac := hmac.New(sha256.New, k.macKey)
	mac.Write([]byte("ip|"))
	mac.Write([]byte(ip))
	return fmt.Sprintf("%x", mac.Sum(nil))[:32]
}
