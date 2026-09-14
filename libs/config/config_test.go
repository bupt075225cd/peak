package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
server:
  port: "8080"
  timeout: 30
  debug: true
routes:
  /api/questions: "http://localhost:8081"
  /api/recognition: "http://localhost:8082"
nested:
  value: "hello"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got := cfg.String("server.port", "x"); got != "8080" {
		t.Fatalf("expected 8080, got %s", got)
	}
	if got := cfg.Int("server.timeout", 0); got != 30 {
		t.Fatalf("expected 30, got %d", got)
	}
	if got := cfg.Bool("server.debug", false); got != true {
		t.Fatalf("expected true, got %v", got)
	}
	if got := cfg.String("nested.value", ""); got != "hello" {
		t.Fatalf("expected hello, got %s", got)
	}

	// routes 是 map，通过 Get 获取。
	if v := cfg.Get("routes"); v == nil {
		t.Fatal("expected routes map")
	} else if m, ok := v.(map[string]any); !ok {
		t.Fatalf("expected map, got %T", v)
	} else if m["/api/questions"] != "http://localhost:8081" {
		t.Fatalf("unexpected routes: %v", m)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "none.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte(":\n\t- bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid yaml")
	}
}

func TestDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.String("missing", "default"); got != "default" {
		t.Fatalf("expected default, got %s", got)
	}
	if got := cfg.Int("missing", 42); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	if got := cfg.Bool("missing", true); got != true {
		t.Fatalf("expected true, got %v", got)
	}
	if got := cfg.Get("a.b.c"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestExpandEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.yaml")
	if err := os.WriteFile(path, []byte("token: ${TEST_TOKEN_VAR}"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_TOKEN_VAR", "secret-123")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.String("token", ""); got != "secret-123" {
		t.Fatalf("expected env expanded, got %s", got)
	}
}

func TestExpandEnvWithDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "envdefault.yaml")
	content := "a: ${UNSET_VAR:-fallback}\nb: ${SET_VAR:-ignored}\nc: ${EMPTY_VAR:-d}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SET_VAR", "from-env")
	// UNSET_VAR 未设置
	cfg, _ := Load(path)
	if got := cfg.String("a", ""); got != "fallback" {
		t.Fatalf("expected fallback for unset, got %s", got)
	}
	if got := cfg.String("b", ""); got != "from-env" {
		t.Fatalf("expected from-env for set, got %s", got)
	}
	if got := cfg.String("c", ""); got != "d" {
		t.Fatalf("expected default d, got %s", got)
	}
}

func TestExpandEnvInNestedMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nestedmap.yaml")
	content := `routes:
  /api/questions: "${QUESTION_SERVICE_URL:-http://localhost:8081}"
  /api/recognition: "${RECOGNITION_SERVICE_URL:-http://localhost:8082}"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// 环境变量未设置，应展开为默认值。
	cfg, _ := Load(path)
	v := cfg.Get("routes")
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", v)
	}
	if m["/api/questions"] != "http://localhost:8081" {
		t.Fatalf("expected default expansion, got %v", m["/api/questions"])
	}
	if m["/api/recognition"] != "http://localhost:8082" {
		t.Fatalf("expected default expansion, got %v", m["/api/recognition"])
	}
}

func TestExpandEnvNoDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "envnodefault.yaml")
	if err := os.WriteFile(path, []byte("a: ${TOTALLY_UNSET_VAR}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := Load(path)
	if got := cfg.String("a", "keep"); got != "" {
		t.Fatalf("expected empty for unset var without default, got %s", got)
	}
}

func TestIntTypeConversions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "int.yaml")
	content := "a: 10\nb: 20\nc: 30.0\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := Load(path)
	if got := cfg.Int("a", 0); got != 10 {
		t.Fatalf("expected 10, got %d", got)
	}
	if got := cfg.Int("b", 0); got != 20 {
		t.Fatalf("expected 20, got %d", got)
	}
	if got := cfg.Int("c", 0); got != 30 {
		t.Fatalf("expected 30, got %d", got)
	}
}

func TestExpandEnvInSlice(t *testing.T) {
	// 覆盖 expandMap 的 []any 分支：slice 中的字符串与嵌套 map 都会被展开。
	dir := t.TempDir()
	path := filepath.Join(dir, "slice.yaml")
	content := `items:
  - "${SLICE_VAR:-a}"
  - name: "${SLICE_VAR2:-b}"
    nested: "${SLICE_VAR3:-c}"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := Load(path)
	v := cfg.Get("items")
	items, ok := v.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", v)
	}
	if items[0] != "a" {
		t.Fatalf("expected 'a', got %v", items[0])
	}
	m := items[1].(map[string]any)
	if m["name"] != "b" || m["nested"] != "c" {
		t.Fatalf("unexpected nested map: %v", m)
	}
}

func TestGetNonMapPath(t *testing.T) {
	// Get 中间路径非 map 时应返回 nil（覆盖 cur.(map[string]any) 失败分支）。
	dir := t.TempDir()
	path := filepath.Join(dir, "scalar.yaml")
	if err := os.WriteFile(path, []byte("a: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := Load(path)
	if got := cfg.Get("a.b.c"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestExpandEnvNested(t *testing.T) {
	// 嵌套回退写法：A 未设置时回退到 B。
	// 注意占位符在 Load 时即展开，因此先设置环境变量再加载配置（与真实启动顺序一致）。
	t.Setenv("NESTED_B", "from-b")
	dir := t.TempDir()
	path := filepath.Join(dir, "nested.yaml")
	content := "key: \"${NESTED_A:-${NESTED_B:-}}\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.String("key", ""); got != "from-b" {
		t.Fatalf("expected fallback to NESTED_B, got %q", got)
	}

	// A 与 B 同时设置时 A 优先（占位符在 Load 时展开，需重新加载）。
	t.Setenv("NESTED_A", "from-a")
	cfg2, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg2.String("key", ""); got != "from-a" {
		t.Fatalf("expected NESTED_A to take precedence, got %q", got)
	}
}

func TestStringCoercedIntAndBool(t *testing.T) {
	// 形如 "${VAR:-true}" 的占位符展开后是字符串，Int/Bool 必须能解析字符串值，
	// 否则环境变量形式的数字/布尔配置（如 GEOMETRY_ENABLED）不生效。
	dir := t.TempDir()
	path := filepath.Join(dir, "coerce.yaml")
	content := "enabled: \"${FLAG:-true}\"\n" +
		"attempts: \"${COUNT:-3}\"\n" +
		"native_bool: true\n" +
		"native_int: 42\n" +
		"bad_bool: \"not-a-bool\"\n" +
		"bad_int: \"abc\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.Bool("enabled", false) {
		t.Fatal("expected string \"true\" to coerce to true")
	}
	if got := cfg.Int("attempts", 0); got != 3 {
		t.Fatalf("expected string \"3\" to coerce to 3, got %d", got)
	}
	if !cfg.Bool("native_bool", false) || cfg.Int("native_int", 0) != 42 {
		t.Fatal("native yaml types should keep working")
	}
	if cfg.Bool("bad_bool", true) != true {
		t.Fatal("unparsable bool should fall back to default")
	}
	if got := cfg.Int("bad_int", 7); got != 7 {
		t.Fatalf("unparsable int should fall back to default, got %d", got)
	}
}
