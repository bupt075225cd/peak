// Package security 提供账本数据的信封加密、主体伪名与行级完整性校验能力。
//
// 三把密钥职责分离：
//   - KEK（主密钥）：仅用于派生每用户 DEK，不直接加密数据；
//   - ID_KEY：把 user_id 映射为不可逆伪名 owner_ref，业务库不落任何直接标识符；
//   - MAC_KEY：行级 HMAC，用于检出数据库层静默篡改。
//
// 三者任一单独泄露都不足以还原数据或关联到自然人；生产环境必须经环境变量注入，
// 缺失或仍为内置开发密钥时进程拒绝启动（fail-fast）。
package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
)

// 密钥与派生长度约束。
const (
	// KeySize 三类密钥的字节长度（AES-256 / HMAC-SHA256）。
	KeySize = 32
	// SaltSize 每用户密钥盐的字节长度。
	SaltSize = 32
	// DEKInfo HKDF 扩展阶段的上下文标签，隔离不同用途的派生结果。
	DEKInfo = "peak-ledger-dek-v1"
)

var (
	// ErrKeyMissing 缺少必需的密钥（生产环境直接拒绝启动）。
	ErrKeyMissing = errors.New("security: 缺少必需的加密密钥")
	// ErrKeyInvalid 密钥解码失败或长度不符。
	ErrKeyInvalid = errors.New("security: 密钥必须为 32 字节的 base64 编码")
	// ErrDevKeyInProd 生产环境仍使用内置开发密钥。
	ErrDevKeyInProd = errors.New("security: 生产环境禁止使用内置开发密钥")
	// ErrKeyVersion 请求的密钥版本不存在。
	ErrKeyVersion = errors.New("security: 密钥版本不存在")
	// ErrSaltWeak 每用户盐长度不足。
	ErrSaltWeak = errors.New("security: 每用户盐长度不足")
)

// KeyConfig 密钥装载配置。
//
// MasterKeys 支持多版本（版本号 -> base64 密钥）：新数据用 Version 加密，
// 旧版本保留用于解密历史数据，便于灰度轮换。
type KeyConfig struct {
	MasterKeys map[int]string
	IDKey      string
	MACKey     string
	Version    int
	// AllowDev 为 true 时允许缺省并使用内置开发密钥（仅本地开发/测试）。
	AllowDev bool
}

// Keys 完成校验与解码后的密钥集合。
type Keys struct {
	version int
	keks    map[int][]byte
	idKey   []byte
	macKey  []byte
	dev     bool
}

// LoadKeys 校验并装载密钥。
//
// 生产（AllowDev=false）要求三把密钥齐全、长度正确且不等于内置开发密钥；
// 开发（AllowDev=true）缺失时回退到固定开发密钥，保证重启后历史数据仍可解密。
func LoadKeys(cfg KeyConfig) (*Keys, error) {
	version := cfg.Version
	if version <= 0 {
		version = 1
	}

	k := &Keys{version: version, keks: map[int][]byte{}}

	if len(cfg.MasterKeys) == 0 {
		if !cfg.AllowDev {
			return nil, fmt.Errorf("%w: LEDGER_MASTER_KEY", ErrKeyMissing)
		}
		k.keks[version] = devKey("master", version)
		k.dev = true
	}
	for v, raw := range cfg.MasterKeys {
		if v <= 0 {
			return nil, fmt.Errorf("%w: 密钥版本号必须为正整数", ErrKeyInvalid)
		}
		key, err := decodeKey(raw)
		if err != nil {
			return nil, fmt.Errorf("master key v%d: %w", v, err)
		}
		if !cfg.AllowDev && hmac.Equal(key, devKey("master", v)) {
			return nil, fmt.Errorf("%w: master key v%d", ErrDevKeyInProd, v)
		}
		k.keks[v] = key
	}
	if _, ok := k.keks[version]; !ok {
		return nil, fmt.Errorf("%w: v%d", ErrKeyVersion, version)
	}

	idKey, err := resolveSecret("LEDGER_ID_KEY", cfg.IDKey, cfg.AllowDev, "id")
	if err != nil {
		return nil, err
	}
	macKey, err := resolveSecret("LEDGER_MAC_KEY", cfg.MACKey, cfg.AllowDev, "mac")
	if err != nil {
		return nil, err
	}
	k.idKey = idKey
	k.macKey = macKey
	return k, nil
}

// Version 返回当前加密版本。
func (k *Keys) Version() int { return k.version }

// IsDev 表示是否使用了内置开发密钥（供启动日志告警）。
func (k *Keys) IsDev() bool { return k.dev }

// OwnerRef 把用户 ID 映射为稳定、不可逆的主体伪名（64 位十六进制）。
//
// 同一用户恒定，因此可作业务表主键维度；但仅持有伪名无法反查账号——
// 反查需要独立的 ID_KEY，而该密钥不在数据库中。
func (k *Keys) OwnerRef(userID uint64) string {
	mac := hmac.New(sha256.New, k.idKey)
	mac.Write([]byte("peak-ledger|"))
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], userID)
	mac.Write(buf[:])
	return hex.EncodeToString(mac.Sum(nil))
}

// DeriveDEK 由主密钥与每用户盐派生 32 字节数据密钥。
//
// 盐随每用户随机生成并存于 ledger_keyrings；删除该行即"密码学删除"：
// 密文仍在但已无法解出明文。
func (k *Keys) DeriveDEK(salt []byte, version int) ([]byte, error) {
	if len(salt) < SaltSize {
		return nil, ErrSaltWeak
	}
	if version <= 0 {
		version = k.version
	}
	kek, ok := k.keks[version]
	if !ok {
		return nil, fmt.Errorf("%w: v%d", ErrKeyVersion, version)
	}
	return hkdfSHA256(kek, salt, []byte(DEKInfo), KeySize), nil
}

// NewSalt 生成每用户密钥盐。
func NewSalt() ([]byte, error) {
	return randomBytes(SaltSize)
}

// SortedMasterVersions 返回已装载的主密钥版本（升序），用于启动日志与运维核对。
func (k *Keys) SortedMasterVersions() []int {
	versions := make([]int, 0, len(k.keks))
	for v := range k.keks {
		versions = append(versions, v)
	}
	sort.Ints(versions)
	return versions
}

// resolveSecret 解析 32 字节密钥；开发环境缺失时回退内置开发密钥。
func resolveSecret(name, raw string, allowDev bool, kind string) ([]byte, error) {
	if raw == "" {
		if !allowDev {
			return nil, fmt.Errorf("%w: %s", ErrKeyMissing, name)
		}
		return devKey(kind, 1), nil
	}
	key, err := decodeKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if !allowDev && hmac.Equal(key, devKey(kind, 1)) {
		return nil, fmt.Errorf("%w: %s", ErrDevKeyInProd, name)
	}
	return key, nil
}

func decodeKey(raw string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) != KeySize {
		return nil, ErrKeyInvalid
	}
	return key, nil
}

// devKey 生成确定性的内置开发密钥：仅用于本地开发与测试。
func devKey(kind string, version int) []byte {
	sum := sha256.Sum256([]byte(fmt.Sprintf("peak-ledger-dev-%s-key-v%d", kind, version)))
	return sum[:]
}

// DevKeys 返回内置开发密钥集合（供 Makefile / 本地 SQLite 调测显式注入）。
func DevKeys() []string {
	encode := func(kind string) string {
		return base64.StdEncoding.EncodeToString(devKey(kind, 1))
	}
	return []string{encode("master"), encode("id"), encode("mac")}
}
