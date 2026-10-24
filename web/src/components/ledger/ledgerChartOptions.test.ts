import { describe, it, expect } from 'vitest'
import { buildSankeyOption, buildTrendOption, COMPOSITION_LEGEND } from './ledgerChartOptions'
import type { LedgerSnapshot, LedgerSummaryView } from '../../api/ledger'

function view(amounts: number[]): LedgerSummaryView {
  const children = amounts.map((amount, i) => ({
    key: `k${i}`,
    label: `子类${i}`,
    amount_cents: amount,
    count: 1,
    is_fixed: false,
  }))
  const total = amounts.reduce((a, b) => a + b, 0)
  return {
    groups: [
      {
        category: 'liquid',
        label: '流动资金',
        kind: 'asset',
        color: '#E9A66B',
        amount_cents: total,
        count: amounts.length,
        children,
      },
    ],
    totalAssetCents: total,
    totalLiabilityCents: 0,
    netAssetCents: total,
    itemCount: amounts.length,
    excludeFixed: false,
  }
}

type LabelFormatter = (params: unknown) => string

function sankey(option: ReturnType<typeof buildSankeyOption>) {
  const series = (option.series as unknown[])[0] as {
    data: { name: string; display: string; value?: number }[]
    label: { formatter: LabelFormatter }
  }
  return series
}

describe('buildSankeyOption', () => {
  it('隐藏金额模式下标签只保留名称', () => {
    const option = buildSankeyOption(view([100_000, 900_000]), 'hidden')
    const names = sankey(option).data.map((n) => n.display)
    expect(names).toContain('总资产')
    expect(names.join(' ')).not.toContain('万')
  })

  it('不绘制占比过小的明细标签，避免文字压叠', () => {
    const option = buildSankeyOption(view([99_500_000, 500_000]), 'amount')
    const { data, label } = sankey(option)
    const big = data.find((n) => n.name === '子类0')!
    const tiny = data.find((n) => n.name === '子类1')!

    expect(label.formatter({ data: big })).toBe(big.display)
    expect(label.formatter({ data: tiny })).toBe('')
    // 值本身没有丢失，仍可通过 tooltip 查看。
    expect(tiny.value ?? tiny.display).toBeTruthy()
  })

  it('分类节点即使占比很小也保留标签', () => {
    const option = buildSankeyOption(view([99_500_000, 500_000]), 'ratio')
    const { data, label } = sankey(option)
    const group = data.find((n) => n.name === '流动资金')!
    expect(label.formatter({ data: group })).toContain('流动资金')
  })

  it('明细与分类同名时补后缀，避免 ECharts 节点重名', () => {
    const v = view([100_000])
    v.groups[0].children[0].label = '流动资金'
    const option = buildSankeyOption(v, 'amount')
    const names = sankey(option).data.map((n) => n.name)
    expect(names).toContain('流动资金（明细）')
  })

  it('图例与分类配色一一对应', () => {
    expect(COMPOSITION_LEGEND.map((l) => l.key)).toEqual(['liquid', 'fixed', 'investment', 'receivable', 'liability'])
    expect(new Set(COMPOSITION_LEGEND.map((l) => l.color)).size).toBeGreaterThan(3)
  })
})

describe('buildTrendOption', () => {
  const snap = (period: string, net: number): LedgerSnapshot =>
    ({
      period,
      updated_at: `${period}-01T00:00:00Z`,
      payload: { total_asset_cents: net, total_liability_cents: 0, net_asset_cents: net, breakdown: [] },
    }) as LedgerSnapshot

  it('单个月份也显示数据点，避免看起来像空图', () => {
    const option = buildTrendOption([snap('2026-10', 2_674_500_00)], false)
    const series = (option.series as unknown[])[0] as { showSymbol: boolean; data: number[] }
    expect(series.showSymbol).toBe(true)
    expect(series.data).toEqual([267_450_000])
  })

  it('多个月份时用平滑折线，不显示数据点', () => {
    const option = buildTrendOption([snap('2026-09', 100), snap('2026-10', 200)], false)
    const series = (option.series as unknown[])[0] as { showSymbol: boolean }
    expect(series.showSymbol).toBe(true) // 2 个点仍显示，保证可读

    const many = buildTrendOption([snap('2026-08', 1), snap('2026-09', 2), snap('2026-10', 3)], false)
    expect((many.series as unknown[])[0]).toMatchObject({ showSymbol: false })
  })
})
