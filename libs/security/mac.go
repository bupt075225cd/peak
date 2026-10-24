package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// Canonical 以不可出现在业务文本中的分隔符拼接规范化字段。
func Canonical(parts ...string) []byte {
	return []byte(strings.Join(parts, "\x1f"))
}

// RowMAC 计算行级完整性校验码（HMAC-SHA256）。
//
// canonical 需覆盖所有明文列、密文摘要、密钥版本、记录版本与更新时间：
// 攻击者即使能写数据库，也无法在不掌握 MAC_KEY 的情况下改值而不被发现。
func (k *Keys) RowMAC(canonical []byte) []byte {
	mac := hmac.New(sha256.New, k.macKey)
	mac.Write(canonical)
	return mac.Sum(nil)
}

// VerifyRowMAC 以常量时间比较校验码。
func (k *Keys) VerifyRowMAC(canonical, mac []byte) bool {
	if len(mac) == 0 {
		return false
	}
	return hmac.Equal(k.RowMAC(canonical), mac)
}

// Digest 返回密文的 SHA-256 摘要（十六进制），供行 MAC 引用而不必重复哈希明文。
func Digest(ciphertext []byte) string {
	sum := sha256.Sum256(ciphertext)
	return hex.EncodeToString(sum[:])
}

// ItemRow 账本条目参与行 MAC 的字段集合。
type ItemRow struct {
	OwnerRef   string
	RowUID     string
	Category   string
	Currency   string
	RiskType   string
	MemberID   uint64
	KeyVersion int
	Version    uint32
	UpdatedAt  time.Time
	NameCipher []byte
	SubCipher  []byte
	AmtCipher  []byte
	NoteCipher []byte
	MemCipher  []byte
}

// Canonical 生成条目行的规范化字节串。
func (r ItemRow) Canonical() []byte {
	return Canonical(
		"item",
		r.OwnerRef,
		r.RowUID,
		r.Category,
		r.Currency,
		r.RiskType,
		strconv.FormatUint(r.MemberID, 10),
		strconv.Itoa(r.KeyVersion),
		strconv.FormatUint(uint64(r.Version), 10),
		strconv.FormatInt(r.UpdatedAt.UTC().UnixNano(), 10),
		Digest(r.NameCipher),
		Digest(r.SubCipher),
		Digest(r.AmtCipher),
		Digest(r.NoteCipher),
		Digest(r.MemCipher),
	)
}

// MemberRow 成员记录参与行 MAC 的字段集合。
type MemberRow struct {
	OwnerRef   string
	RowUID     string
	SortOrder  int
	KeyVersion int
	UpdatedAt  time.Time
	NameCipher []byte
}

// Canonical 生成成员行的规范化字节串。
func (r MemberRow) Canonical() []byte {
	return Canonical(
		"member",
		r.OwnerRef,
		r.RowUID,
		strconv.Itoa(r.SortOrder),
		strconv.Itoa(r.KeyVersion),
		strconv.FormatInt(r.UpdatedAt.UTC().UnixNano(), 10),
		Digest(r.NameCipher),
	)
}
