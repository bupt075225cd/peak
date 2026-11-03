import type { QuestionTable } from '../api'

// 题干里的表格占位符：{{table1}}、{{table2}}…
const TABLE_PLACEHOLDER = /\{\{\s*(table\d+)\s*\}\}/g

export interface StemSegment {
  text?: string
  table?: QuestionTable
}

// 把题干按表格占位符切成"文本段 + 表格段"，供录入页渲染表格。
// 找不到对应表格的占位符按普通文本保留，避免内容凭空消失。
export function splitStemByTables(stem: string, tables: QuestionTable[]): StemSegment[] {
  const byId = new Map((tables || []).map((t) => [t.id, t]))
  const out: StemSegment[] = []
  let last = 0
  for (const m of stem.matchAll(TABLE_PLACEHOLDER)) {
    const start = m.index ?? 0
    if (start > last) out.push({ text: stem.slice(last, start) })
    const table = byId.get(m[1])
    out.push(table ? { table } : { text: m[0] })
    last = start + m[0].length
  }
  if (last < stem.length) out.push({ text: stem.slice(last) })
  return out.filter((s) => (s.table ? true : (s.text ?? '').trim() !== ''))
}

// isTableRow 是否为 Markdown 表格行（以 | 开头/结尾，且含分隔符）。
function isTableRow(line: string): boolean {
  const s = line.trim()
  if (!s || !s.includes('|')) return false
  return s.startsWith('|') || s.endsWith('|')
}

// isSeparatorRow 是否为 Markdown 表格的分隔行，如 |---|:--:|
function isSeparatorRow(line: string): boolean {
  const s = line.trim()
  if (!s.includes('|') || !s.includes('-')) return false
  const body = s.replace(/^\|/, '').replace(/\|$/, '')
  const cells = body.split('|')
  return cells.length > 0 && cells.every((c) => /^:?-+:?$/.test(c.trim()))
}

// splitCells 拆分表格行为单元格（去掉首尾竖线、还原 \| 转义）。
function splitCells(line: string): string[] {
  const s = line.trim().replace(/^\|/, '').replace(/\|$/, '')
  return s.split(/(?<!\\)\|/).map((c) => c.replace(/\\\|/g, '|').trim())
}

// 解析题干里的 Markdown 表格（保存后的题干就是这种形式，与导出侧 parseStemBlocks 同语法）。
export function parseMarkdownTables(text: string): StemSegment[] {
  const lines = (text || '').split('\n')
  const out: StemSegment[] = []
  let buf: string[] = []
  const flush = () => {
    const t = buf.join('\n').trim()
    if (t) out.push({ text: t })
    buf = []
  }
  for (let i = 0; i < lines.length; ) {
    if (isTableRow(lines[i]) && i + 1 < lines.length && isSeparatorRow(lines[i + 1])) {
      flush()
      const headers = splitCells(lines[i])
      const rows: string[][] = []
      let j = i + 2
      while (j < lines.length && isTableRow(lines[j])) {
        rows.push(splitCells(lines[j]))
        j++
      }
      out.push({ table: { id: `md${out.length}`, headers, rows } })
      i = j
      continue
    }
    buf.push(lines[i])
    i++
  }
  flush()
  return out
}

// 题干展示的统一入口：先按 {{tableN}} 占位取结构化表格，再处理段落里的 Markdown 表格。
// 识别结果带 tables（录入页），保存后的题干只有 Markdown 表格（列表页/编辑弹窗），两种都要能渲染。
export function splitStemBlocks(stem: string, tables: QuestionTable[] = []): StemSegment[] {
  const src = stem || ''
  const hasPlaceholder = /\{\{\s*table\d+\s*\}\}/.test(src)
  const initial: StemSegment[] =
    hasPlaceholder && tables.length > 0 ? splitStemByTables(src, tables) : [{ text: src }]

  const out: StemSegment[] = []
  for (const seg of initial) {
    if (seg.table) {
      out.push(seg)
      continue
    }
    out.push(...parseMarkdownTables(seg.text || ''))
  }
  return out
}

// 单元格转义：竖线是 Markdown 表格的列分隔符，换行会打断表格行。
function cellText(v: string): string {
  return (v || '').replace(/\|/g, '\\|').replace(/\r?\n/g, ' ').trim()
}

// 把表格序列化成 Markdown 表格（保存题干时使用，导出侧按它还原成表格）。
// Markdown 必须有表头：原表没有表头时用第一行数据充当，保证行列不错位。
export function tableToMarkdown(table: QuestionTable): string {
  let headers = (table.headers || []).map(cellText)
  let rows = (table.rows || []).map((r) => r.map(cellText))
  if (headers.length === 0) {
    if (rows.length === 0) return ''
    headers = rows[0]
    rows = rows.slice(1)
  }
  const cols = Math.max(headers.length, ...rows.map((r) => r.length), 1)
  const pad = (cells: string[]): string[] => {
    const out = cells.slice(0, cols)
    while (out.length < cols) out.push('')
    return out
  }
  const line = (cells: string[]): string => `| ${pad(cells).join(' | ')} |`
  const sep = `| ${Array.from({ length: cols }, () => '---').join(' | ')} |`
  return [line(headers), sep, ...rows.map(line)].join('\n')
}

// 把题干里的 {{tableN}} 占位替换成 Markdown 表格（编辑框展示用）：
// 用户看到的应是表格数据本身，而不是 {{table1}} 这种占位符。可重复调用（无占位时原样返回）。
export function inlineStemTables(stem: string, tables: QuestionTable[]): string {
  if (!tables || tables.length === 0) return stem
  if (!/\{\{\s*table\d+\s*\}\}/.test(stem)) return stem
  const byId = new Map(tables.map((t) => [t.id, t]))
  const out = stem.replace(TABLE_PLACEHOLDER, (raw: string, id: string) => {
    const table = byId.get(id)
    if (!table) return raw
    const md = tableToMarkdown(table)
    // 表格必须独占一行：粘在句子里（"如表<表格>所示"）会与正文混行，行列解析错位。
    return md ? `\n${md}\n` : raw
  })
  return out.replace(/\n{3,}/g, '\n\n').trim()
}

// 保存题干：占位符替换成 Markdown 表格；既未引用、也没出现在题干里的表格追加到末尾。
// 编辑框已内联过的题干再调用不会重复追加。
export function serializeStemWithTables(stem: string, tables: QuestionTable[]): string {
  if (!tables || tables.length === 0) return stem
  let out = inlineStemTables(stem, tables)
  for (const table of tables) {
    if (new RegExp(`\\{\\{\\s*${table.id}\\s*\\}\\}`).test(stem)) continue
    const md = tableToMarkdown(table)
    if (!md) continue
    if (out.includes(md.split('\n')[0])) continue // 题干里已有这张表
    out = `${out.trimEnd()}\n${md}`
  }
  return out
}
