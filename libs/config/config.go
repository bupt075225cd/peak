// Package config 提供统一的配置加载能力，支持 YAML 文件与环境变量覆盖。
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// envPattern 匹配 ${VAR} 或 ${VAR:-default} 形式的环境变量占位。
var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// Loader 配置加载器。
type Loader struct {
	data map[string]any
}

// Load 从 YAML 文件加载配置。
func Load(path string) (*Loader, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var data map[string]any
	if err := yaml.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if data == nil {
		data = map[string]any{}
	}
	return &Loader{data: data}, nil
}

// Get 按点分路径读取配置值（如 "server.port"）。
func (l *Loader) Get(key string) any {
	parts := strings.Split(key, ".")
	var cur any = l.data
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[p]
	}
	return cur
}

// String 读取字符串配置，支持 ${ENV_VAR} 形式的环境变量覆盖。
func (l *Loader) String(key string, def string) string {
	v := l.Get(key)
	if s, ok := v.(string); ok {
		return expandEnv(s)
	}
	return def
}

// Int 读取整数配置。
func (l *Loader) Int(key string, def int) int {
	v := l.Get(key)
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return def
}

// Bool 读取布尔配置。
func (l *Loader) Bool(key string, def bool) bool {
	v := l.Get(key)
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

// expandEnv 展开 ${VAR} 或 ${VAR:-default} 形式的环境变量。
// 若环境变量未设置：
//   - ${VAR}           → 空字符串
//   - ${VAR:-default}  → default
func expandEnv(s string) string {
	return envPattern.ReplaceAllStringFunc(s, func(m string) string {
		sub := envPattern.FindStringSubmatch(m)
		name := sub[1]
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		// 未设置时，若有默认值则返回默认值，否则返回空字符串
		if len(sub) >= 3 {
			return sub[2]
		}
		return ""
	})
}
