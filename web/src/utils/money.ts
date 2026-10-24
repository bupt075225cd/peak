// 金额与时间格式化工具（纯函数，便于单测）。
//
// 全链路金额以「分」为整数传输与存储，避免浮点误差；展示成"元"时
// 用整数拆分整数位与小数位，绝不做 toFixed 之前的浮点除法。

/** 千分位整数。 */
function groupThousands(value: number): string {
  return value.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',')
}

/** 1277250 → "12,772.50"；负数保留符号。 */
export function formatCents(cents: number): string {
  const rounded = Math.round(Number.isFinite(cents) ? cents : 0)
  const negative = rounded < 0
  const abs = Math.abs(rounded)
  const yuan = Math.floor(abs / 100)
  const fen = abs % 100
  return `${negative ? '-' : ''}${groupThousands(yuan)}.${String(fen).padStart(2, '0')}`
}

/** 以「万」为单位展示：588290000 → "588.29万"。 */
export function formatWan(cents: number): string {
  const rounded = Math.round(Number.isFinite(cents) ? cents : 0)
  const wan = rounded / 1_000_000
  return `${wan.toFixed(2)}万`
}

/** 隐藏态统一占位符。 */
export const MONEY_MASK = '••••••'

/** 按隐私开关返回金额文本。 */
export function maskMoney(text: string, hidden: boolean): string {
  return hidden ? MONEY_MASK : text
}

/**
 * 解析用户输入的金额为分。
 * 支持千分位与最多两位小数；非法/负数/超精度返回 null。
 */
export function parseAmountToCents(raw: string): number | null {
  const text = raw.trim().replace(/,/g, '').replace(/，/g, '')
  if (!text) return null
  if (!/^\d+(\.\d{0,2})?$/.test(text)) return null
  const [yuanPart, fenPart = ''] = text.split('.')
  const yuan = Number(yuanPart)
  if (!Number.isFinite(yuan)) return null
  const fen = Number(fenPart.padEnd(2, '0'))
  const cents = yuan * 100 + fen
  if (!Number.isSafeInteger(cents) || cents <= 0) return null
  return cents
}

/** 比率（0~1）转百分比字符串：0.2597 → "25.97%"。 */
export function formatPercent(ratio: number, digits = 2): string {
  if (!Number.isFinite(ratio)) return '0.00%'
  return `${(ratio * 100).toFixed(digits)}%`
}

/** 负债率 = 总负债 / 总资产；分母为 0 时返回 0。 */
export function debtRatio(liabilityCents: number, assetCents: number): number {
  if (assetCents <= 0) return 0
  return liabilityCents / assetCents
}

/** ISO 时间 → "2026.02.28"。 */
export function formatDate(iso: string | null | undefined): string {
  if (!iso) return '—'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '—'
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${date.getFullYear()}.${month}.${day}`
}

/** "2026-01" → "1月"。 */
export function formatMonthLabel(period: string): string {
  const month = period.split('-')[1] ?? ''
  return month ? `${Number(month)}月` : period
}

/** 环比增长率；上期为 0 时返回 null（避免除零与无穷大）。 */
export function deltaPercent(current: number, previous: number): number | null {
  if (!previous) return null
  return (current - previous) / Math.abs(previous)
}

/** 金额等级配色用的占比（0~1）。 */
export function share(part: number, total: number): number {
  if (total <= 0) return 0
  return part / total
}
