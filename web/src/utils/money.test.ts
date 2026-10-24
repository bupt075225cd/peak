import { describe, it, expect } from 'vitest'
import {
  debtRatio,
  deltaPercent,
  formatCents,
  formatDate,
  formatMonthLabel,
  formatPercent,
  formatWan,
  maskMoney,
  MONEY_MASK,
  parseAmountToCents,
  share,
} from './money'

describe('formatCents', () => {
  it('整数分转千分位金额', () => {
    expect(formatCents(1_277_250)).toBe('12,772.50')
    expect(formatCents(0)).toBe('0.00')
    expect(formatCents(5)).toBe('0.05')
    expect(formatCents(1_000_000_00)).toBe('1,000,000.00')
  })

  it('负数保留符号，非法值按 0 处理', () => {
    expect(formatCents(-123_45)).toBe('-123.45')
    expect(formatCents(Number.NaN)).toBe('0.00')
  })
})

describe('formatWan', () => {
  it('以万为单位保留两位', () => {
    expect(formatWan(588_290_000)).toBe('588.29万')
    expect(formatWan(0)).toBe('0.00万')
  })
})

describe('parseAmountToCents', () => {
  it('接受整数、两位小数与千分位', () => {
    expect(parseAmountToCents('1200')).toBe(120_000)
    expect(parseAmountToCents('1,200.5')).toBe(120_050)
    expect(parseAmountToCents(' 88.88 ')).toBe(8_888)
    expect(parseAmountToCents('1，200')).toBe(120_000)
  })

  it('拒绝空值、负数、超两位小数与非法字符', () => {
    expect(parseAmountToCents('')).toBeNull()
    expect(parseAmountToCents('0')).toBeNull()
    expect(parseAmountToCents('-5')).toBeNull()
    expect(parseAmountToCents('1.234')).toBeNull()
    expect(parseAmountToCents('abc')).toBeNull()
    expect(parseAmountToCents('1.2.3')).toBeNull()
  })
})

describe('比例与负债率', () => {
  it('负债率分母为 0 时返回 0', () => {
    expect(debtRatio(100, 0)).toBe(0)
    expect(debtRatio(150, 300)).toBe(0.5)
  })

  it('占比与百分比', () => {
    expect(share(25, 100)).toBe(0.25)
    expect(share(25, 0)).toBe(0)
    expect(formatPercent(0.2597)).toBe('25.97%')
    expect(formatPercent(Number.NaN)).toBe('0.00%')
  })

  it('环比在上期为 0 时返回 null', () => {
    expect(deltaPercent(120, 100)).toBeCloseTo(0.2)
    expect(deltaPercent(80, 100)).toBeCloseTo(-0.2)
    expect(deltaPercent(80, 0)).toBeNull()
  })
})

describe('时间与脱敏', () => {
  it('格式化日期与月份标签', () => {
    expect(formatDate('2026-02-28T10:00:00Z')).toBe('2026.02.28')
    expect(formatDate(null)).toBe('—')
    expect(formatDate('not-a-date')).toBe('—')
    expect(formatMonthLabel('2026-01')).toBe('1月')
    expect(formatMonthLabel('bad')).toBe('bad')
  })

  it('隐藏态统一占位符', () => {
    expect(MONEY_MASK).toBe('••••••')
    expect(maskMoney('1,200.00', true)).toBe(MONEY_MASK)
    expect(maskMoney('1,200.00', false)).toBe('1,200.00')
  })
})
