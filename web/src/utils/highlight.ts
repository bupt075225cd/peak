export interface HighlightPart {
  text: string
  hit: boolean
}

// 关键词高亮切分：把命中片段拆成数组交给模板渲染（不使用 v-html，避免 XSS）。
// 多关键词以空白分隔，全部命中片段都标黄。
export function highlightParts(text: string, keyword: string): HighlightPart[] {
  const terms = (keyword || '').trim().toLowerCase().split(/\s+/).filter(Boolean)
  if (!text || terms.length === 0) return [{ text: text || '', hit: false }]

  const lower = text.toLowerCase()
  const marks = new Array<boolean>(text.length).fill(false)
  for (const term of terms) {
    for (let from = 0; ; ) {
      const idx = lower.indexOf(term, from)
      if (idx < 0) break
      for (let i = idx; i < idx + term.length; i++) marks[i] = true
      from = idx + term.length
    }
  }

  const parts: HighlightPart[] = []
  let start = 0
  for (let i = 1; i <= text.length; i++) {
    if (i === text.length || (marks[i] ?? false) !== (marks[start] ?? false)) {
      parts.push({ text: text.slice(start, i), hit: marks[start] ?? false })
      start = i
    }
  }
  return parts
}
