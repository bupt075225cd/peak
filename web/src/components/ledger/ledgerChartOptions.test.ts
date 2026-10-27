import { describe, it, expect } from 'vitest'
import { buildSankeyOption, buildTrendOption } from './ledgerChartOptions'
import type { LedgerSnapshot, LedgerSummaryView } from '../../api/ledger'

function view(amounts: number[], debts: number[] = []): LedgerSummaryView {
  const children = amounts.map((amount, i) => ({
    key: `k${i}`,
    label: `子类${i}`,
    amount_cents: amount,
    count: 1,
    is_fixed: false,
  }))
  const total = amounts.reduce((a, b) => a + b, 0)
  const liability = debts.reduce((a, b) => a + b, 0)
  const debtChildren = debts.map((amount, i) => ({
    key: `d${i}`,
    label: `负债项${i}`,
    amount_cents: amount,
    count: 1,
    is_fixed: false,
  }))
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
      ...(debts.length
        ? [
            {
              category: 'liability' as const,
              label: '负债',
              kind: 'liability' as const,
              color: '#B9B4C7',
              amount_cents: liability,
              count: debts.length,
              children: debtChildren,
            },
          ]
        : []),
    ],
    // 总资产 = 净资产 + 负债
    totalAssetCents: total + liability,
    totalLiabilityCents: liability,
    netAssetCents: total,
    itemCount: amounts.length + debts.length,
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
    expect(names).toContain('净资产')
    expect(names).toContain('流动资金')
    expect(names.join(' ')).not.toContain('万')
  })

  it('左侧是净资产与每一笔负债，不再出现「负债来源」与「总资产」节点', () => {
    const option = buildSankeyOption(view([30_000_000], [11_500_000, 500_000]), 'amount')
    const names = sankey(option).data.map((n) => n.name)

    expect(names).toContain('净资产')
    expect(names).toContain('负债项0')
    expect(names).toContain('负债项1')
    expect(names).not.toContain('负债来源')
    expect(names).not.toContain('负债')
    expect(names).not.toContain('总资产')
  })

  it('来源按分类占比分摊，左右总量守恒', () => {
    const option = buildSankeyOption(view([25_000_000], [5_000_000]), 'amount')
    const series = sankey(option) as unknown as { links: { source: string; target: string; value: number }[] }
    const total = (source: string) =>
      series.links.filter((l) => l.source === source).reduce((sum, l) => sum + l.value, 0)

    // 净资产 25,000,000 + 负债 5,000,000 = 总资产 30,000,000
    expect(total('净资产')).toBe(25_000_000)
    expect(total('负债项0')).toBe(5_000_000)
    // 单一分类时一条来源的全部金额都流向该分类。
    expect(series.links.filter((l) => l.target === '流动资金').reduce((s, l) => s + l.value, 0)).toBe(30_000_000)
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

  it('负债项与资产分类重名时补后缀，避免节点合并', () => {
    const v = view([100_000], [100_000])
    v.groups[1].children[0].label = '流动资金'
    const option = buildSankeyOption(v, 'amount')
    const names = sankey(option).data.map((n) => n.name)
    expect(names).toContain('流动资金（负债）')
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
