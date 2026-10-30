package security

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

func mustKey(t *testing.T, kind string) string {
	t.Helper()
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func prodConfig(t *testing.T) KeyConfig {
	t.Helper()
	return KeyConfig{
		MasterKeys: map[int]string{1: mustKey(t, "master")},
		IDKey:      mustKey(t, "id"),
		MACKey:     mustKey(t, "mac"),
		Version:    1,
	}
}

func TestLoadKeysDevFallback(t *testing.T) {
	keys, err := LoadKeys(KeyConfig{AllowDev: true})
	if err != nil {
		t.Fatalf("dev fallback should succeed: %v", err)
	}
	if !keys.IsDev() || keys.Version() != 1 {
		t.Fatalf("unexpected dev state: dev=%v version=%d", keys.IsDev(), keys.Version())
	}
	// 开发密钥确定性：重启后历史数据仍可解密。
	again, err := LoadKeys(KeyConfig{AllowDev: true})
	if err != nil {
		t.Fatalf("reload dev keys: %v", err)
	}
	if again.OwnerRef(42) != keys.OwnerRef(42) {
		t.Fatal("dev keys must be deterministic")
	}
}

func TestLoadKeysRejectsMissingInProd(t *testing.T) {
	if _, err := LoadKeys(KeyConfig{AllowDev: false}); !errors.Is(err, ErrKeyMissing) {
		t.Fatalf("want ErrKeyMissing, got %v", err)
	}
}

func TestLoadKeysRejectsDevKeyInProd(t *testing.T) {
	dev := DevKeys()
	cfg := KeyConfig{
		MasterKeys: map[int]string{1: dev[0]},
		IDKey:      dev[1],
		MACKey:     dev[2],
		Version:    1,
	}
	if _, err := LoadKeys(cfg); !errors.Is(err, ErrDevKeyInProd) {
		t.Fatalf("want ErrDevKeyInProd, got %v", err)
	}
}

func TestLoadKeysRejectsBadLength(t *testing.T) {
	cfg := prodConfig(t)
	cfg.MACKey = base64.StdEncoding.EncodeToString([]byte("short"))
	if _, err := LoadKeys(cfg); !errors.Is(err, ErrKeyInvalid) {
		t.Fatalf("want ErrKeyInvalid, got %v", err)
	}
}

func TestOwnerRefStableAndOpaque(t *testing.T) {
	keys, err := LoadKeys(prodConfig(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ref := keys.OwnerRef(1001)
	if len(ref) != 64 {
		t.Fatalf("owner_ref length = %d, want 64", len(ref))
	}
	if keys.OwnerRef(1001) != ref {
		t.Fatal("owner_ref must be stable")
	}
	if keys.OwnerRef(1002) == ref {
		t.Fatal("owner_ref must differ across users")
	}
	if strings.Contains(ref, "1001") {
		t.Fatal("owner_ref must not embed raw user id")
	}
}

func TestDeriveDEK(t *testing.T) {
	keys, err := LoadKeys(prodConfig(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	salt := bytes.Repeat([]byte{7}, SaltSize)
	dek, err := keys.DeriveDEK(salt, 1)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(dek) != KeySize {
		t.Fatalf("dek length = %d", len(dek))
	}
	again, _ := keys.DeriveDEK(salt, 1)
	if !bytes.Equal(dek, again) {
		t.Fatal("dek derivation must be deterministic")
	}
	other, _ := keys.DeriveDEK(bytes.Repeat([]byte{8}, SaltSize), 1)
	if bytes.Equal(dek, other) {
		t.Fatal("different salt must yield different dek")
	}
	if _, err := keys.DeriveDEK([]byte("short"), 1); !errors.Is(err, ErrSaltWeak) {
		t.Fatalf("want ErrSaltWeak, got %v", err)
	}
	if _, err := keys.DeriveDEK(salt, 99); !errors.Is(err, ErrKeyVersion) {
		t.Fatalf("want ErrKeyVersion, got %v", err)
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	keys, _ := LoadKeys(prodConfig(t))
	dek, _ := keys.DeriveDEK(bytes.Repeat([]byte{1}, SaltSize), 1)
	aad := BuildAAD("owner", "row", "amount")
	sealed, err := keys.Seal(dek, []byte("123456"), aad)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Contains(sealed, []byte("123456")) {
		t.Fatal("ciphertext must not contain plaintext")
	}
	got, err := keys.Open(dek, sealed, aad)
	if err != nil || string(got) != "123456" {
		t.Fatalf("open = %q, %v", got, err)
	}
}

func TestSealRejectsTamperAndAADMismatch(t *testing.T) {
	keys, _ := LoadKeys(prodConfig(t))
	dek, _ := keys.DeriveDEK(bytes.Repeat([]byte{2}, SaltSize), 1)
	aad := BuildAAD("owner", "row", "amount")
	sealed, _ := keys.Seal(dek, []byte("100"), aad)

	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 0xff
	if _, err := keys.Open(dek, tampered, aad); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("tampered ciphertext should fail, got %v", err)
	}
	if _, err := keys.Open(dek, sealed, BuildAAD("owner", "row", "name")); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("aad mismatch should fail, got %v", err)
	}
	if _, err := keys.Open(dek, []byte("short"), aad); !errors.Is(err, ErrCipherShort) {
		t.Fatalf("short ciphertext should fail, got %v", err)
	}
}

func TestRowMAC(t *testing.T) {
	keys, _ := LoadKeys(prodConfig(t))
	now := time.Unix(1700000000, 0).UTC()
	row := ItemRow{
		OwnerRef: "owner", RowUID: "row", Category: "liquid", Currency: "CNY",
		RiskType: "cash", MemberID: 3, KeyVersion: 1, Version: 2, UpdatedAt: now,
		NameCipher: []byte("n"), SubCipher: []byte("s"), AmtCipher: []byte("a"),
		NoteCipher: []byte("t"), MemCipher: []byte("m"),
	}
	mac := keys.RowMAC(row.Canonical())
	if !keys.VerifyRowMAC(row.Canonical(), mac) {
		t.Fatal("mac should verify")
	}
	row.Version = 3
	if keys.VerifyRowMAC(row.Canonical(), mac) {
		t.Fatal("tampered version must be detected")
	}
	if keys.VerifyRowMAC(row.Canonical(), nil) {
		t.Fatal("empty mac must not verify")
	}
}

// 行 MAC 覆盖 UpdatedAt，但数据库（PostgreSQL timestamptz）只保留微秒：
// 写入时带纳秒、回读时被截断的行必须仍能通过校验，否则生产环境所有
// 账本行都会报"数据完整性校验失败"。
func TestRowMACIgnoresSubMicrosecondTime(t *testing.T) {
	keys, _ := LoadKeys(prodConfig(t))
	stored := time.Now().UTC()
	row := ItemRow{
		OwnerRef: "owner", RowUID: "row", Category: "liquid", Currency: "CNY",
		RiskType: "cash", KeyVersion: 1, Version: 1, UpdatedAt: stored,
		NameCipher: []byte("n"), SubCipher: []byte("s"), AmtCipher: []byte("a"),
		NoteCipher: []byte("t"), MemCipher: []byte("m"),
	}
	mac := keys.RowMAC(row.Canonical())

	row.UpdatedAt = stored.Truncate(time.Microsecond)
	if !keys.VerifyRowMAC(row.Canonical(), mac) {
		t.Fatal("microsecond truncation must not break row mac")
	}

	member := MemberRow{OwnerRef: "o", RowUID: "r", KeyVersion: 1, UpdatedAt: stored, NameCipher: []byte("x")}
	memberMAC := keys.RowMAC(member.Canonical())
	member.UpdatedAt = stored.Truncate(time.Microsecond)
	if !keys.VerifyRowMAC(member.Canonical(), memberMAC) {
		t.Fatal("microsecond truncation must not break member mac")
	}
}

// 升级前的行 MAC 用纳秒时间计算：SQLite 旧库保留纳秒，旧 MAC 必须仍可校验，
// 同时任何字段被改写都要继续被检出。
func TestVerifyMACAcceptsLegacyNanoForm(t *testing.T) {
	keys, _ := LoadKeys(prodConfig(t))
	stored := time.Now().UTC()
	row := ItemRow{
		OwnerRef: "owner", RowUID: "row", Category: "liquid", Currency: "CNY",
		RiskType: "cash", KeyVersion: 1, Version: 1, UpdatedAt: stored,
		NameCipher: []byte("n"), SubCipher: []byte("s"), AmtCipher: []byte("a"),
		NoteCipher: []byte("t"), MemCipher: []byte("m"),
	}
	legacy := keys.RowMAC(row.legacyCanonical())
	if !keys.VerifyItemMAC(row, legacy) {
		t.Fatal("legacy nanosecond row mac must still verify")
	}
	row.Version = 2
	if keys.VerifyItemMAC(row, legacy) {
		t.Fatal("tampered legacy row must be detected")
	}

	member := MemberRow{OwnerRef: "o", RowUID: "r", KeyVersion: 1, UpdatedAt: stored, NameCipher: []byte("x")}
	memberLegacy := keys.RowMAC(member.legacyCanonical())
	if !keys.VerifyMemberMAC(member, memberLegacy) {
		t.Fatal("legacy member mac must still verify")
	}
	member.SortOrder = 9
	if keys.VerifyMemberMAC(member, memberLegacy) {
		t.Fatal("tampered legacy member must be detected")
	}
}

func TestMemberRowCanonical(t *testing.T) {
	row := MemberRow{OwnerRef: "o", RowUID: "r", SortOrder: 1, KeyVersion: 1, UpdatedAt: time.Unix(1, 0), NameCipher: []byte("x")}
	if !bytes.Contains(row.Canonical(), []byte("member")) {
		t.Fatal("member canonical must be tagged")
	}
}

func TestNewRowUIDUnique(t *testing.T) {
	a, err := NewRowUID()
	if err != nil {
		t.Fatalf("uid: %v", err)
	}
	b, _ := NewRowUID()
	if len(a) != 32 || a == b {
		t.Fatalf("unexpected row uid: %q %q", a, b)
	}
}

func TestLoadKeysVersionValidation(t *testing.T) {
	cfg := prodConfig(t)
	cfg.MasterKeys = map[int]string{0: mustKey(t, "master")}
	if _, err := LoadKeys(cfg); !errors.Is(err, ErrKeyInvalid) {
		t.Fatalf("zero version: want ErrKeyInvalid, got %v", err)
	}

	cfg = prodConfig(t)
	cfg.MasterKeys = map[int]string{2: mustKey(t, "master")}
	cfg.Version = 1
	if _, err := LoadKeys(cfg); !errors.Is(err, ErrKeyVersion) {
		t.Fatalf("missing version: want ErrKeyVersion, got %v", err)
	}
}

func TestLoadKeysRejectsDevSecretInProd(t *testing.T) {
	dev := DevKeys()
	cfg := prodConfig(t)
	cfg.MACKey = dev[2]
	if _, err := LoadKeys(cfg); !errors.Is(err, ErrDevKeyInProd) {
		t.Fatalf("want ErrDevKeyInProd for mac key, got %v", err)
	}

	cfg = prodConfig(t)
	cfg.IDKey = dev[1]
	if _, err := LoadKeys(cfg); !errors.Is(err, ErrDevKeyInProd) {
		t.Fatalf("want ErrDevKeyInProd for id key, got %v", err)
	}
}

func TestSortedMasterVersions(t *testing.T) {
	cfg := prodConfig(t)
	cfg.MasterKeys[3] = mustKey(t, "master")
	keys, err := LoadKeys(cfg)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := keys.SortedMasterVersions()
	if len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("versions = %v, want [1 3]", got)
	}
}

func TestSealRejectsInvalidDEK(t *testing.T) {
	keys, _ := LoadKeys(prodConfig(t))
	if _, err := keys.Seal([]byte("short"), []byte("x"), nil); !errors.Is(err, ErrKeyInvalid) {
		t.Fatalf("want ErrKeyInvalid, got %v", err)
	}
}

func TestRandomToken(t *testing.T) {
	a := RandomToken(8)
	b := RandomToken(8)
	if len(a) != 16 || a == b {
		t.Fatalf("unexpected tokens: %q %q", a, b)
	}
}

func TestNewSalt(t *testing.T) {
	salt, err := NewSalt()
	if err != nil || len(salt) != SaltSize {
		t.Fatalf("salt = %d bytes, err = %v", len(salt), err)
	}
}

func TestHashIP(t *testing.T) {
	keys, _ := LoadKeys(prodConfig(t))
	h := keys.HashIP("203.0.113.7")
	if len(h) != 32 || strings.Contains(h, "203.0.113.7") {
		t.Fatalf("unexpected ip hash: %q", h)
	}
	if keys.HashIP("203.0.113.7") != h {
		t.Fatal("ip hash must be stable")
	}
	if keys.HashIP("203.0.113.8") == h {
		t.Fatal("ip hash must differ per address")
	}
}
