import { use } from 'echarts/core'
import { SankeyChart, LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { EChartsOption } from 'echarts'
import type { LedgerSnapshot, LedgerSummaryView } from '../../api/ledger'
import { formatCents, formatPercent, formatWan, share } from '../../utils/money'

// 账本图表按需注册，避免把整包 echarts 带进古诗文页面。
let registered = false

export function ensureLedgerECharts(): void {
  if (registered) return
  use([SankeyChart, LineChart, GridComponent, TooltipComponent, CanvasRenderer])
  registered = true
}

/** 桑基图展示模式：金额 / 比例 / 隐藏金额。 */
export type CompositionMode = 'amount' | 'ratio' | 'hidden'

interface SankeyNode {
  name: string
  display: string
  // 带上 value 让标签 formatter 能判断节点占比（ECharts 内部同样会用它）。
  value: number
  itemStyle: { color: string }
}

interface SankeyLink {
  source: string
  target: string
  value: number
}

const COLORS = {
  net: '#3E9E7E',
  debt: '#B9B4C7',
  liquid: '#E9A66B',
  fixed: '#9DBCC4',
  investment: '#A9A3D4',
  receivable: '#63C6A5',
}

/**
 * 构建资产组成桑基数据。
 *
 * 资金来源 → 资金运用两段式：左侧是净资产与每一笔负债（逐项展开，不再聚合成
 * 「负债来源」），右侧是资产分类与其明细。总资产不占节点，由页面顶部的方框展示。
 * 每个来源按各分类占比分摊到右侧，保证左右总量守恒。
 */
export function buildSankeyOption(view: LedgerSummaryView, mode: CompositionMode): EChartsOption {
  const nodes: SankeyNode[] = []
  const links: SankeyLink[] = []
  const totalAsset = view.totalAssetCents
  const net = view.netAssetCents

  const label = (name: string, value: number): string => {
    if (mode === 'hidden') return name
    if (mode === 'ratio') return `${name}  ${formatPercent(share(value, Math.max(totalAsset, 1)))}`
    return `${name}  ${formatWan(value)}`
  }

  // 明细列中占比极小的节点（如 5.6 万的货币基金）在比例模式下会被压成一条线，
  // 标签必然互相压叠，这里直接不画标签，值仍可通过悬停 tooltip 查看。
  const detailNames = new Set<string>()
  const MIN_DETAIL_LABEL_RATIO = 0.012

  const addNode = (name: string, value: number, color: string) => {
    nodes.push({ name, display: label(name, value), value, itemStyle: { color } })
  }

  const assetGroups = view.groups.filter((g) => g.kind === 'asset' && g.amount_cents > 0)

  // 来源列：净资产 + 负债逐项（房屋贷款、车贷……各自一条，不合并）。
  const sources: { name: string; value: number; color: string }[] = []
  if (net > 0) sources.push({ name: '净资产', value: net, color: COLORS.net })
  for (const group of view.groups) {
    if (group.kind !== 'liability') continue
    for (const child of group.children) {
      if (child.amount_cents <= 0) continue
      // 负债项与资产分类重名时补后缀，避免 ECharts 合并同名节点。
      const name = assetGroups.some((g) => g.label === child.label) ? `${child.label}（负债）` : child.label
      sources.push({ name, value: child.amount_cents, color: COLORS.debt })
    }
  }

  for (const source of sources) addNode(source.name, source.value, source.color)

  for (const group of assetGroups) {
    const color = COLORS[group.category as keyof typeof COLORS] ?? group.color
    addNode(group.label, group.amount_cents, color)

    // 明细列只保留金额最大的 3 项，其余合并为「其他」：避免小额节点标签互相压叠。
    const children = group.children.filter((child) => child.amount_cents > 0).sort((a, b) => b.amount_cents - a.amount_cents)
    const head = children.slice(0, 3)
    const tail = children.slice(3)
    for (const child of head) {
      // 父子同名时补后缀，避免 ECharts 节点重名。
      const childName = child.label === group.label ? `${child.label}（明细）` : child.label
      addNode(childName, child.amount_cents, color)
      detailNames.add(childName)
      links.push({ source: group.label, target: childName, value: child.amount_cents })
    }
    if (tail.length) {
      const restValue = tail.reduce((sum, child) => sum + child.amount_cents, 0)
      const restName = `${group.label} 其他 ${tail.length} 项`
      addNode(restName, restValue, color)
      detailNames.add(restName)
      links.push({ source: group.label, target: restName, value: restValue })
    }
  }

  // 来源 → 分类：按各分类在总资产中的占比分摊，最后一项吃下舍入余量，
  // 保证每条来源的流出之和恰好等于它的金额、每个分类的流入之和等于该分类金额。
  for (const source of sources) {
    let allocated = 0
    assetGroups.forEach((group, index) => {
      const isLast = index === assetGroups.length - 1
      const value = isLast
        ? source.value - allocated
        : Math.round(source.value * (group.amount_cents / Math.max(totalAsset, 1)))
      allocated += value
      if (value > 0) links.push({ source: source.name, target: group.label, value })
    })
  }

  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      formatter: (params: unknown) => {
        const data = params as { dataType?: string; data?: { display?: string; value?: number }; name?: string }
        if (mode === 'hidden') return data.name ?? ''
        if (data.dataType === 'edge') return `${formatCents(data.data?.value ?? 0)} 元`
        return data.data?.display ?? data.name ?? ''
      },
    },
    series: [
      {
        type: 'sankey',
        left: 8,
        right: 168,
        top: 24,
        bottom: 24,
        // 参考稿：细长节点 + 更宽的列间距，靠留白撑出层次。
        nodeWidth: 8,
        nodeGap: 16,
        nodeAlign: 'left',
        draggable: false,
        emphasis: { focus: 'adjacency' },
        label: {
          color: '#475569',
          fontSize: 13,
          formatter: (params: unknown) => {
            const data = (params as { data?: { name?: string; value?: number; display?: string } }).data ?? {}
            const name = data.name ?? ''
            if (detailNames.has(name) && totalAsset > 0 && (data.value ?? 0) / totalAsset < MIN_DETAIL_LABEL_RATIO) {
              return ''
            }
            return data.display ?? name
          },
        },
        // 兜底：即使阈值判断漏掉个别节点，也隐藏重叠标签而不是糊成一团。
        labelLayout: { hideOverlap: true },
        lineStyle: { color: 'gradient', opacity: 0.55, curveness: 0.55 },
        data: nodes,
        links,
      },
    ],
  }
}

/** 近 12 个月净资产趋势（渐变面积）。 */
export function buildTrendOption(snapshots: LedgerSnapshot[], hidden: boolean): EChartsOption {
  const points = snapshots.map((s) => ({
    label: `${Number(s.period.split('-')[1])}月`,
    value: s.payload.net_asset_cents,
  }))
  return {
    grid: { left: 8, right: 16, top: 24, bottom: 8, containLabel: true },
    tooltip: {
      trigger: 'axis',
      formatter: (params: unknown) => {
        const list = params as { name: string; value: number }[]
        const first = list[0]
        if (!first) return ''
        const text = hidden ? '••••••' : `${formatCents(first.value)} 元`
        return `${first.name} · 净资产 ${text}`
      },
    },
    xAxis: {
      type: 'category',
      data: points.map((p) => p.label),
      axisLine: { lineStyle: { color: '#E2E8F0' } },
      axisTick: { show: false },
      axisLabel: { color: '#94A3B8', fontSize: 11 },
    },
    yAxis: {
      type: 'value',
      splitLine: { lineStyle: { color: '#F1F5F9' } },
      axisLabel: {
        color: '#94A3B8',
        fontSize: 11,
        formatter: (value: number) => (hidden ? '••' : `${Math.round(value / 1_000_000)}万`),
      },
    },
    series: [
      {
        type: 'line',
        smooth: true,
        // 只有 1~2 个数据点时折线不可见，改为显示数据点，避免看起来像空图。
        showSymbol: points.length <= 2,
        symbolSize: 6,
        data: points.map((p) => p.value),
        lineStyle: { width: 2.5, color: '#3E9E7E' },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(62,158,126,0.28)' },
              { offset: 1, color: 'rgba(62,158,126,0.02)' },
            ],
          },
        },
      },
    ],
  }
}
