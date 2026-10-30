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

// RowTime 把参与行 MAC 的时间规整到数据库可精确存储的精度（微秒）。
//
// 时间戳必须进 MAC，否则"只改 UpdatedAt、不改内容"的篡改无法检出；
// 但 PostgreSQL timestamptz 只保留微秒，Go time.Time 带纳秒：直接用
// UnixNano() 会让写入时的值与本进程回读的值不同，导致所有行校验失败。
// 写入侧（service）与校验侧（Canonical）都先经此规整。
func RowTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Microsecond)
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
	return r.canonicalWith(strconv.FormatInt(RowTime(r.UpdatedAt).UnixNano(), 10))
}

// legacyCanonical 升级前的算法：直接用纳秒时间参与 MAC，供旧行校验兜底。
func (r ItemRow) legacyCanonical() []byte {
	return r.canonicalWith(strconv.FormatInt(r.UpdatedAt.UTC().UnixNano(), 10))
}

func (r ItemRow) canonicalWith(at string) []byte {
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
		at,
		Digest(r.NameCipher),
		Digest(r.SubCipher),
		Digest(r.AmtCipher),
		Digest(r.NoteCipher),
		Digest(r.MemCipher),
	)
}

// VerifyItemMAC 校验条目行 MAC。
//
// 兼容升级前的旧行：早期版本用纳秒时间参与 MAC，而数据库只存微秒。
// 库内时间仍带亚微秒部分（SQLite 旧数据）时按旧算法再校验一次，
// 避免升级后历史条目全部报"数据完整性校验失败"；两种算法同受 MAC_KEY 保护，
// 伪造任意一种仍需密钥，不降低防篡改强度。
func (k *Keys) VerifyItemMAC(row ItemRow, mac []byte) bool {
	if k.VerifyRowMAC(row.Canonical(), mac) {
		return true
	}
	if !hasSubMicrosecond(row.UpdatedAt) {
		return false
	}
	return k.VerifyRowMAC(row.legacyCanonical(), mac)
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
	return r.canonicalWith(strconv.FormatInt(RowTime(r.UpdatedAt).UnixNano(), 10))
}

// legacyCanonical 升级前的算法：直接用纳秒时间参与 MAC。
func (r MemberRow) legacyCanonical() []byte {
	return r.canonicalWith(strconv.FormatInt(r.UpdatedAt.UTC().UnixNano(), 10))
}

func (r MemberRow) canonicalWith(at string) []byte {
	return Canonical(
		"member",
		r.OwnerRef,
		r.RowUID,
		strconv.Itoa(r.SortOrder),
		strconv.Itoa(r.KeyVersion),
		at,
		Digest(r.NameCipher),
	)
}

// VerifyMemberMAC 校验成员行 MAC，规则同 VerifyItemMAC。
func (k *Keys) VerifyMemberMAC(row MemberRow, mac []byte) bool {
	if k.VerifyRowMAC(row.Canonical(), mac) {
		return true
	}
	if !hasSubMicrosecond(row.UpdatedAt) {
		return false
	}
	return k.VerifyRowMAC(row.legacyCanonical(), mac)
}

// hasSubMicrosecond 判断时间是否带亚微秒部分：只有 SQLite 旧数据会保留纳秒。
func hasSubMicrosecond(t time.Time) bool {
	return t.Nanosecond()%1000 != 0
}
