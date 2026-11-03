import { describe, it, expect } from 'vitest'
import {
  splitStemByTables,
  splitStemBlocks,
  parseMarkdownTables,
  tableToMarkdown,
  inlineStemTables,
  serializeStemWithTables,
} from './stemTable'
import type { QuestionTable } from '../api'

const table: QuestionTable = {
  id: 'table1',
  headers: ['计费档', '用水量x/m³', '单价（元/m³）'],
  rows: [
    ['第一档', '0<x≤220', '3.5'],
    ['第二档', '220<x≤300', '5.0'],
  ],
}

describe('splitStemByTables', () => {
  it('按占位符切分文本与表格', () => {
    const segs = splitStemByTables('如表{{table1}}所示：\n(1) 求关系式', [table])
    expect(segs).toHaveLength(3)
    expect(segs[0].text).toContain('如表')
    expect(segs[1].table?.id).toBe('table1')
    expect(segs[2].text).toContain('(1)')
  })

  it('找不到对应表格的占位符按文本保留', () => {
    const segs = splitStemByTables('前{{table9}}后', [table])
    expect(segs.map((s) => s.text || '').join('')).toContain('{{table9}}')
  })
})

describe('parseMarkdownTables', () => {
  it('解析保存后的 Markdown 表格题干', () => {
    const segs = parseMarkdownTables('如表\n| a | b |\n| --- | --- |\n| 1 | 2 |\n后文')
    expect(segs).toHaveLength(3)
    expect(segs[1].table?.headers).toEqual(['a', 'b'])
    expect(segs[1].table?.rows).toEqual([['1', '2']])
  })

  it('没有分隔行时按普通文本处理', () => {
    const segs = parseMarkdownTables('第一档 | 0<x≤220 | 3.5')
    expect(segs.every((s) => !s.table)).toBe(true)
  })
})

describe('splitStemBlocks', () => {
  it('优先按占位符取结构化表格', () => {
    const segs = splitStemBlocks('如表{{table1}}所示', [table])
    expect(segs.some((s) => s.table?.id === 'table1')).toBe(true)
  })

  it('没有 tables 时按 Markdown 表格解析', () => {
    const segs = splitStemBlocks('如表\n| a | b |\n| --- | --- |\n| 1 | 2 |')
    expect(segs.some((s) => s.table?.headers[0] === 'a')).toBe(true)
  })
})

describe('tableToMarkdown', () => {
  it('生成带分隔行的 Markdown 表格', () => {
    const lines = tableToMarkdown(table).split('\n')
    expect(lines[0]).toBe('| 计费档 | 用水量x/m³ | 单价（元/m³） |')
    expect(lines[1]).toBe('| --- | --- | --- |')
    expect(lines).toHaveLength(4)
  })

  it('无表头时用首行数据充当表头', () => {
    const lines = tableToMarkdown({ id: 't', headers: [], rows: [['a', 'b'], ['1', '2']] }).split('\n')
    expect(lines[0]).toBe('| a | b |')
    expect(lines).toHaveLength(3)
  })

  it('转义单元格里的竖线', () => {
    expect(tableToMarkdown({ id: 't', headers: ['a|b'], rows: [] })).toContain('a\\|b')
  })
})

describe('serializeStemWithTables', () => {
  it('把占位符替换成 Markdown 表格', () => {
    const out = serializeStemWithTables('如表{{table1}}所示', [table])
    expect(out).toContain('| 计费档 |')
    expect(out).not.toContain('{{table1}}')
  })

  it('未被题干引用的表格追加到末尾', () => {
    const out = serializeStemWithTables('没有占位的题干', [table])
    expect(out.startsWith('没有占位的题干')).toBe(true)
    expect(out).toContain('| 第一档 | 0<x≤220 | 3.5 |')
  })

  it('没有表格时原样返回', () => {
    expect(serializeStemWithTables('纯文字', [])).toBe('纯文字')
  })

  it('已内联成 Markdown 表格的题干不会被重复追加', () => {
    const once = serializeStemWithTables('如表{{table1}}所示', [table])
    expect(serializeStemWithTables(once, [table])).toBe(once)
  })
})

describe('inlineStemTables', () => {
  it('把占位符替换成可读的 Markdown 表格，可重复调用', () => {
    const out = inlineStemTables('如表{{table1}}所示', [table])
    expect(out).toContain('| 计费档 |')
    expect(out).not.toContain('{{table1}}')
    expect(inlineStemTables(out, [table])).toBe(out)
  })

  it('没有对应表格时保留占位符原样', () => {
    expect(inlineStemTables('如{{table2}}所示', [table])).toBe('如{{table2}}所示')
  })
})
