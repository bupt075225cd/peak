package geom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// 端点引用的两种写法：
//   - 点名（提示词要求的形态）："from":"A"
//   - 直接给坐标：{"from":{"x":15,"y":50.5}}、{"from":[22,58]}、
//     甚至 {"from":{"l1":[40,30]}}（模型把坐标包在标签里）
//
// 坐标写法在坐标系题里很常见（实测三轮里两轮都这样写），若直接按 string 反序列化
// 会整条线解析失败、连带整次重绘失败。这里统一转成 "x,y" 字符串，
// 再由 Spec.RepairCoordRefs 造隐藏构造点，保住模型已经定好的位置。

// Segment.UnmarshalJSON 容忍 from/to 写成坐标对象或数组。
func (s *Segment) UnmarshalJSON(data []byte) error {
	var raw struct {
		From   json.RawMessage `json:"from"`
		To     json.RawMessage `json:"to"`
		Dashed bool            `json:"dashed,omitempty"`
		Extend bool            `json:"extend,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	from, err := decodePointRef(raw.From)
	if err != nil {
		return fmt.Errorf("segments[].from: %w", err)
	}
	to, err := decodePointRef(raw.To)
	if err != nil {
		return fmt.Errorf("segments[].to: %w", err)
	}
	s.From, s.To = from, to
	s.Dashed, s.Extend = raw.Dashed, raw.Extend
	return nil
}

// decodePointRef 解析一个端点引用，返回点名或 "x,y" 坐标串。
func decodePointRef(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}
	var name string
	if err := json.Unmarshal(trimmed, &name); err == nil {
		return strings.TrimSpace(name), nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", fmt.Errorf("不是合法的点名或坐标：%s", shortJSON(trimmed))
	}
	if x, y, ok := coordFromJSON(v, 0); ok {
		return formatCoord(x, y), nil
	}
	return "", fmt.Errorf("必须是点名字符串（如 \"A\"），实际是 %s", shortJSON(trimmed))
}

// coordFromJSON 从 {"x":..,"y":..}、[x,y]、{"l1":[x,y]} 这类写法里取出一对坐标。
func coordFromJSON(v any, depth int) (float64, float64, bool) {
	if depth > 3 {
		return 0, 0, false
	}
	switch t := v.(type) {
	case []any:
		if len(t) >= 2 {
			x, okX := jsonNumber(t[0])
			y, okY := jsonNumber(t[1])
			if okX && okY {
				return x, y, true
			}
		}
		for _, e := range t {
			if x, y, ok := coordFromJSON(e, depth+1); ok {
				return x, y, true
			}
		}
	case map[string]any:
		if xv, ok := t["x"]; ok {
			if yv, ok2 := t["y"]; ok2 {
				x, okX := jsonNumber(xv)
				y, okY := jsonNumber(yv)
				if okX && okY {
					return x, y, true
				}
			}
		}
		for _, e := range t {
			if x, y, ok := coordFromJSON(e, depth+1); ok {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

func jsonNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	}
	return 0, false
}

// formatCoord 生成与 coordRefRe 匹配的 "x,y"（定点写法，不用指数）。
func formatCoord(x, y float64) string {
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		return ""
	}
	return strconv.FormatFloat(x, 'f', -1, 64) + "," + strconv.FormatFloat(y, 'f', -1, 64)
}

// Spec.UnmarshalJSON 容忍 points 写成对象（模型的常见违规写法）：
// 正常是 [{"name":"A","x":1,"y":2}]，模型有时写成 {"A":{"x":1,"y":2}} 或 {"A":[1,2]}。
func (s *Spec) UnmarshalJSON(data []byte) error {
	type specAlias Spec
	var raw struct {
		*specAlias
		Points json.RawMessage `json:"points"`
	}
	raw.specAlias = (*specAlias)(s)
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	pts, err := decodePoints(raw.Points)
	if err != nil {
		return err
	}
	if pts != nil {
		s.Points = pts
	}
	return nil
}

// decodePoints 解析 points 字段：数组是规范写法，对象则按“点名→坐标”补成点。
func decodePoints(raw json.RawMessage) ([]Point, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var pts []Point
		if err := json.Unmarshal(trimmed, &pts); err != nil {
			return nil, err
		}
		return pts, nil
	}
	if trimmed[0] != '{' {
		return nil, fmt.Errorf("points 必须是数组，实际是 %s", shortJSON(trimmed))
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(obj))
	for name := range obj {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Point, 0, len(names))
	for _, name := range names {
		p := Point{Name: name}
		v := bytes.TrimSpace(obj[name])
		// 坐标可以是 {"x":..,"y":..}，也可以是 [x,y]。
		dec := json.NewDecoder(bytes.NewReader(v))
		dec.UseNumber()
		var any1 any
		if err := dec.Decode(&any1); err != nil {
			return nil, err
		}
		if x, y, ok := coordFromJSON(any1, 0); ok {
			p.X, p.Y = x, y
		} else {
			return nil, fmt.Errorf("点 %s 的坐标无法解析：%s", name, shortJSON(v))
		}
		// 对象写法里可能同时带 hidden/label 等字段。
		if m, ok := any1.(map[string]any); ok {
			if h, ok := m["hidden"].(bool); ok {
				p.Hidden = h
			}
			if l, ok := m["label"].(string); ok {
				p.Label = l
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// shortJSON 截断过长的原始片段，用于错误信息（回喂模型时保持简短）。
func shortJSON(b []byte) string {
	const max = 60
	s := strings.TrimSpace(string(b))
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
