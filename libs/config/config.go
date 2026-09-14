// Package config 提供统一的配置加载能力，支持 YAML 文件与环境变量覆盖。
package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)



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
	// 递归展开所有字符串值中的环境变量占位（含嵌套 map，如 routes.<prefix>）。
	expandMap(data)
	return &Loader{data: data}, nil
}

// expandMap 递归展开 map/slice 中字符串值里的 ${VAR} / ${VAR:-default} 占位。
func expandMap(m map[string]any) {
	for k, v := range m {
		switch val := v.(type) {
		case string:
			m[k] = expandEnv(val)
		case map[string]any:
			expandMap(val)
		case []any:
			for i, item := range val {
				switch it := item.(type) {
				case string:
					val[i] = expandEnv(it)
				case map[string]any:
					expandMap(it)
				}
			}
		}
	}
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

// Int 读取整数配置。字符串值（含经 ${VAR:-default} 展开后的数字）会被解析。
func (l *Loader) Int(key string, def int) int {
	v := l.Get(key)
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
			return int(f)
		}
	}
	return def
}

// Bool 读取布尔配置。字符串值（含经 ${VAR:-default} 展开后的布尔字面量）会被解析。
func (l *Loader) Bool(key string, def bool) bool {
	v := l.Get(key)
	switch b := v.(type) {
	case bool:
		return b
	case string:
		switch strings.ToLower(strings.TrimSpace(b)) {
		case "true", "1", "yes", "on", "y":
			return true
		case "false", "0", "no", "off", "n", "":
			return false
		}
	}
	return def
}

// envNameRe 校验环境变量名（${...} 占位符内）。
var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// expandEnv 展开 ${VAR} 或 ${VAR:-default} 形式的环境变量。
// 若环境变量未设置：
//   - ${VAR}           → 空字符串
//   - ${VAR:-default}  → default
//
// default 中支持再嵌套占位符（如 "${A:-${B:-}}"：A 未设置时回退到 B）。
func expandEnv(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			if end, ok := matchPlaceholder(s, i); ok {
				inner := s[i+2 : end]
				if name, def, hasDef := strings.Cut(inner, ":-"); envNameRe.MatchString(name) {
					if v, ok := os.LookupEnv(name); ok {
						sb.WriteString(v)
					} else if hasDef {
						sb.WriteString(expandEnv(def))
					}
					i = end + 1
					continue
				}
			}
		}
		sb.WriteByte(s[i])
		i++
	}
	return sb.String()
}

// matchPlaceholder 返回与 start（指向 '$'）处 "${" 配对的 '}' 下标，
// 按 {} 配对计数，因此 default 中可嵌套 "${...}"。找不到返回 false。
func matchPlaceholder(s string, start int) (int, bool) {
	depth := 0
	for j := start + 1; j < len(s); j++ {
		switch s[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j, true
			}
		}
	}
	return 0, false
}
