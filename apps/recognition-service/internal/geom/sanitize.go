package geom

import (
	"encoding/xml"
	"io"
	"strings"
)

// maxSanitizedSVGBytes 限制清洗后的 SVG 体积，防止超大 payload 落库。
const maxSanitizedSVGBytes = 512 << 10

// allowedSVGElements SVG 安全清洗的白名单标签（小写）。
// 刻意排除 script/foreignObject/iframe/image/use/animate/set/handler
// 等脚本载体或可引用外部资源的标签。
var allowedSVGElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "title": true, "desc": true,
	"path": true, "rect": true, "circle": true, "ellipse": true, "line": true,
	"polyline": true, "polygon": true, "text": true, "tspan": true, "textpath": true,
	"marker": true, "lineargradient": true, "radialgradient": true, "stop": true,
	"clippath": true, "pattern": true, "symbol": true,
}

// allowedSVGAttrs SVG 安全清洗的白名单属性（小写、不含命名空间前缀）。
var allowedSVGAttrs = map[string]bool{
	"id": true, "class": true, "style": true, "transform": true,
	"d": true, "points": true,
	"x": true, "y": true, "x1": true, "y1": true, "x2": true, "y2": true,
	"cx": true, "cy": true, "r": true, "rx": true, "ry": true,
	"dx": true, "dy": true, "rotate": true,
	"width": true, "height": true, "viewbox": true, "preserveaspectratio": true, "version": true,
	"fill": true, "fill-opacity": true, "fill-rule": true,
	"stroke": true, "stroke-width": true, "stroke-linecap": true,
	"stroke-linejoin": true, "stroke-dasharray": true, "stroke-dashoffset": true,
	"stroke-opacity": true, "stroke-miterlimit": true,
	"opacity": true, "offset": true,
	"font-family": true, "font-size": true, "font-weight": true, "font-style": true,
	"text-anchor": true, "dominant-baseline": true, "paint-order": true,
	"text-decoration": true, "letter-spacing": true,
	"marker-start": true, "marker-mid": true, "marker-end": true,
	"markerunits": true, "markerwidth": true, "markerheight": true,
	"refx": true, "refy": true, "orient": true,
	"clip-path": true, "clip-rule": true, "mask": true,
	"gradientunits": true, "gradienttransform": true,
	"stop-color": true, "stop-opacity": true,
	"patternunits": true, "patterncontentunits": true, "patterntransform": true,
	"startoffset": true, "method": true, "spacing": true,
}

// SanitizeSVG 对外部/模型产出的 SVG 文本做服务端安全清洗并归一化文档头。
//
// 重绘产物会以 image/svg+xml 直接对外提供，若不做清洗，攻击者可借
// 模型输出注入 <script> 或事件属性造成存储型 XSS，因此这里采用白名单：
// 只保留白名单标签与属性，剥离脚本载体、on* 事件属性、非本地片段引用
// （url(...)/href/xlink:href）以及 javascript: 等伪协议。
// 输入非法（不是 <svg> 文档、无法解析、体积超限）时返回空串。
func SanitizeSVG(raw string) string {
	src := strings.TrimSpace(raw)
	if src == "" || len(src) > maxSanitizedSVGBytes {
		return ""
	}
	dec := xml.NewDecoder(strings.NewReader(src))
	dec.Strict = false // 容忍个别非法实体，但结构性错误仍会报错

	var out strings.Builder
	enc := xml.NewEncoder(&out)

	skip := -1      // >=0 表示正处于被丢弃的子树内，值为该子树的相对深度
	rootSeen := false
	depth := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ""
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if skip >= 0 {
				skip++
				continue
			}
			name := strings.ToLower(t.Name.Local)
			if !rootSeen {
				if name != "svg" {
					return ""
				}
				rootSeen = true
			}
			if !allowedSVGElements[name] {
				skip = 0
				continue
			}
			attrs := sanitizeAttrs(t.Attr)
			if name == "svg" {
				attrs = withDocHeader(attrs)
			}
			if err := enc.EncodeToken(xml.StartElement{Name: xml.Name{Local: name}, Attr: attrs}); err != nil {
				return ""
			}
			depth++
		case xml.EndElement:
			if skip > 0 {
				skip--
				continue
			}
			if skip == 0 {
				skip = -1
				continue
			}
			depth--
			if err := enc.EncodeToken(xml.EndElement{Name: xml.Name{Local: strings.ToLower(t.Name.Local)}}); err != nil {
				return ""
			}
		case xml.CharData:
			if skip >= 0 {
				continue
			}
			if err := enc.EncodeToken(xml.CharData(stripInvalidXML(string(t)))); err != nil {
				return ""
			}
		default: // Comment / ProcInst / Directive 一律丢弃
			continue
		}
	}
	if !rootSeen || depth != 0 {
		return ""
	}
	if err := enc.Flush(); err != nil {
		return ""
	}
	if out.Len() > maxSanitizedSVGBytes {
		return ""
	}
	return out.String()
}

// sanitizeAttrs 过滤单个元素的属性：白名单 + 值安全检查。
func sanitizeAttrs(all []xml.Attr) []xml.Attr {
	out := make([]xml.Attr, 0, len(all))
	for _, a := range all {
		name := strings.ToLower(a.Name.Local)
		// 命名空间声明统一重写，不透传。
		if name == "xmlns" || a.Name.Space == "xmlns" {
			continue
		}
		// 事件处理器属性（onclick/onload/...）一律剥离。
		if strings.HasPrefix(name, "on") && len(name) > 2 {
			continue
		}
		if !allowedSVGAttrs[name] {
			continue
		}
		if !safeSVGValue(a.Value) {
			continue
		}
		out = append(out, xml.Attr{Name: xml.Name{Local: name}, Value: a.Value})
	}
	return out
}

// withDocHeader 归一化 <svg> 根节点的文档头：补 xmlns / viewBox / preserveAspectRatio。
func withDocHeader(attrs []xml.Attr) []xml.Attr {
	viewBox, par := "", ""
	out := make([]xml.Attr, 0, len(attrs)+3)
	out = append(out, xml.Attr{Name: xml.Name{Local: "xmlns"}, Value: svgNS})
	for _, a := range attrs {
		switch strings.ToLower(a.Name.Local) {
		case "viewbox":
			viewBox = a.Value
			continue
		case "preserveaspectratio":
			par = a.Value
			continue
		}
		out = append(out, a)
	}
	if strings.TrimSpace(viewBox) == "" {
		viewBox = "0 0 100 100"
	}
	if strings.TrimSpace(par) == "" {
		par = "xMidYMid meet"
	}
	out = append(out,
		xml.Attr{Name: xml.Name{Local: "viewBox"}, Value: viewBox},
		xml.Attr{Name: xml.Name{Local: "preserveAspectRatio"}, Value: par})
	return out
}

// safeSVGValue 判断属性值是否安全：不含脚本伪协议，且所有 url(...) 引用都是本地片段。
func safeSVGValue(v string) bool {
	low := stripSpaces(strings.ToLower(v))
	if strings.Contains(low, "javascript:") || strings.Contains(low, "vbscript:") ||
		strings.Contains(low, "data:") || strings.Contains(low, "expression(") {
		return false
	}
	for {
		i := strings.Index(low, "url(")
		if i < 0 {
			return true
		}
		rest := low[i+4:]
		j := strings.Index(rest, ")")
		if j < 0 {
			return false
		}
		ref := strings.Trim(strings.TrimSpace(rest[:j]), `"'`)
		if !strings.HasPrefix(ref, "#") {
			return false
		}
		low = rest[j+1:]
	}
}

// stripSpaces 去掉全部空白与控制字符，用于识别被拆写的伪协议（如 "java\tscript:"）。
func stripSpaces(s string) string {
	return strings.Map(func(r rune) rune {
		if r <= 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, s)
}
