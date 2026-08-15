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
