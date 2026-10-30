import { describe, it, expect } from 'vitest'
import { buildSankeyOption, buildTrendOption } from './ledgerChartOptions'
import type { LedgerCategory, LedgerSnapshot, LedgerSummaryView } from '../../api/ledger'

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

/** 多分类视图：每个分类只有一个明细，金额即分类金额。 */
function multiView(amounts: [LedgerCategory, string, string, number][]): LedgerSummaryView {
  const groups = amounts.map(([category, label, color, amount_cents]) => ({
    category,
    label,
    kind: 'asset' as const,
    color,
    amount_cents,
    count: 1,
    children: [{ key: `${label}-c`, label: `${label}-明细`, amount_cents, count: 1, is_fixed: false }],
  }))
  const total = groups.reduce((sum, group) => sum + group.amount_cents, 0)
  return {
    groups,
    totalAssetCents: total,
    totalLiabilityCents: 0,
    netAssetCents: total,
    itemCount: groups.length,
    excludeFixed: false,
  }
}

type LabelFormatter = (params: unknown) => string

function sankey(option: ReturnType<typeof buildSankeyOption>) {
  const series = (option.series as unknown[])[0] as {
    data: {
      name: string
      display: string
      value?: number
      depth?: number
      tight?: boolean
      label?: { position: string; distance?: number; backgroundColor?: string; borderColor?: string; borderWidth?: number }
      itemStyle?: {
        color?: string
        borderWidth?: number
        opacity?: number
        shadowColor?: string
        shadowOffsetX?: number
      }
    }[]
    label: { formatter: LabelFormatter }
    links: { source: string; target: string; value: number; lineStyle?: { color?: string; opacity?: number } }[]
    lineStyle?: { color?: string; opacity?: number; curveness?: number }
    left?: number
    right?: number
    nodeWidth?: number
    nodeGap?: number
    nodeAlign?: string
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

  it('四列流带：负债在上、净资产在下，经总资产节点流向分类与明细', () => {
    const option = buildSankeyOption(view([30_000_000], [11_500_000, 500_000]), 'amount')
    const series = sankey(option)
    const names = series.data.map((n) => n.name)

    expect(names).toContain('净资产')
    expect(names).toContain('负债项0')
    expect(names).toContain('负债项1')
    expect(names).toContain('总资产')
    expect(names).not.toContain('负债来源')
    // 左列负债在上、净资产在下（对齐参考稿）。
    expect(names.indexOf('负债项0')).toBeLessThan(names.indexOf('净资产'))

    // 负债带先汇入自己的右端竖条（负债出口），再接入总资产；净资产不带
    // 自己的竖条，直接跨列接入总资产。流动资金/投资理财/应收款属于资产，
    // 与净资产无从属。
    expect(series.links).toContainEqual(
      expect.objectContaining({ source: '负债项0', target: '负债合计', value: 11_500_000 }),
    )
    expect(series.links).toContainEqual(
      expect.objectContaining({ source: '负债项1', target: '负债合计', value: 500_000 }),
    )
    expect(series.links).toContainEqual({
      source: '负债合计',
      target: '总资产',
      value: 12_000_000,
      // 衔接段透明：负债出口竖条与总资产竖条之间留窄空白（对齐参考稿）。
      lineStyle: { color: '#8E89A6', opacity: 0 },
    })
    // 净资产带在出口列收口（不带竖条），再经透明衔接段接入总资产竖条，
    // 与负债出口竖条共用同一列位，两者右缘垂直对齐。
    expect(series.links).toContainEqual(
      expect.objectContaining({ source: '净资产', target: '净资产出口', value: 30_000_000 }),
    )
    expect(series.links).toContainEqual({
      source: '净资产出口',
      target: '总资产',
      value: 30_000_000,
      lineStyle: { color: '#3E9E7E', opacity: 0 },
    })
    expect(series.links.some((l) => l.source === '总资产' && l.target === '流动资金')).toBe(true)
    expect(series.links.some((l) => l.source === '负债项0' && l.target === '流动资金')).toBe(false)
    expect(series.links.some((l) => l.source === '净资产' && l.target === '流动资金')).toBe(false)
  })

  it('流带为纯色填充（对齐参考稿），不使用渐变', () => {
    const option = buildSankeyOption(view([30_000_000], [11_500_000]), 'amount')
    const series = sankey(option)
    // 系列兜底色不再是渐变，可见带子逐条显式指定纯色。
    expect(series.lineStyle?.color).not.toBe('gradient')
    const visible = series.links.filter((l) => (l.lineStyle?.opacity ?? 1) > 0)
    expect(visible.length).toBeGreaterThan(0)
    for (const item of visible) {
      expect(item.lineStyle?.color).toMatch(/^#[0-9A-Fa-f]{6}$/)
      expect(item.lineStyle?.opacity).toBe(0.45)
    }
  })

  it('第一级流带统一用总资产色，第二级按分类同色（对齐参考稿）', () => {
    const option = buildSankeyOption(
      {
        groups: [
          { category: 'liquid', label: '流动资金', kind: 'asset', color: '#E9A66B', amount_cents: 10_000_000, count: 1, children: [{ key: 'a', label: '活期', amount_cents: 10_000_000, count: 1, is_fixed: false }] },
          { category: 'fixed', label: '固定资产', kind: 'asset', color: '#9DBCC4', amount_cents: 20_000_000, count: 1, children: [{ key: 'b', label: '房产', amount_cents: 20_000_000, count: 1, is_fixed: false }] },
        ],
        totalAssetCents: 30_000_000,
        totalLiabilityCents: 0,
        netAssetCents: 30_000_000,
        itemCount: 2,
        excludeFixed: false,
      },
      'amount',
    )
    const series = sankey(option)
    // 第一级（总资产 → 各分类）全部是总资产的蓝紫，不随分类变色。
    const firstLevel = series.links.filter((l) => l.source === '总资产')
    expect(firstLevel.length).toBe(2)
    for (const item of firstLevel) expect(item.lineStyle?.color).toBe('#4C5BB4')
    // 第二级（分类 → 明细）用所属分类的色，同一分类内同色。
    for (const item of series.links.filter((l) => l.source === '流动资金')) {
      expect(item.lineStyle?.color).toBe('#E9A66B')
    }
    for (const item of series.links.filter((l) => l.source === '固定资产')) {
      expect(item.lineStyle?.color).toBe('#9DBCC4')
    }
  })

  it('负债/总资产以方框标签渲染在节点上方（对齐参考稿）', () => {
    const option = buildSankeyOption(view([30_000_000], [11_500_000]), 'amount')
    const series = sankey(option)
    const at = (name: string) => series.data.find((n) => n.name === name)!

    // 金额模式：两行 rich 文本（名称 + 万，负债带负号），带背景方框、位于节点上方。
    expect(at('负债合计').display).toContain('{btName|负债}')
    expect(at('负债合计').display).toContain('{btValue|-11.50万}')
    expect(at('总资产').display).toContain('{btName|总资产}')
    expect(at('总资产').display).toContain('{btValue|41.50万}')
    expect(at('负债合计').label?.position).toBe('top')
    // 两个指标框是同一组同色卡片（参考稿），都不描边。
    expect(at('负债合计').label?.backgroundColor).toBe('#F1F3F7')
    expect(at('总资产').label?.backgroundColor).toBe('#F1F3F7')
    expect(at('总资产').label?.borderWidth).toBeUndefined()

    // 其他节点标签保持纯文字，无背景框。
    expect(at('净资产').label?.backgroundColor).toBeUndefined()
    expect(at('流动资金').label?.backgroundColor).toBeUndefined()
  })

  it('列内间隔改由占位节点提供：出口竖条与净资产带紧挨，其余列仍有空隙', () => {
    const option = buildSankeyOption(view([30_000_000], [11_500_000]), 'amount')
    const series = sankey(option)
    const depthOf = (name: string) => series.data.find((n) => n.name === name)!.depth ?? 0
    const at = (name: string) => series.data.find((n) => n.name === name)!

    // nodeGap 归零：全局间隔会让出口竖条与净资产带之间也留空。
    expect(series.nodeGap).toBe(0)
    // 出口列位：负债出口竖条与净资产出口同一列，净资产带经同色出口节点向右
    // 延伸一个竖条宽度，两条带的右缘因此始终齐平（与画布宽度无关）。
    expect(depthOf('净资产出口')).toBe(depthOf('负债合计'))
    expect(depthOf('负债合计')).toBeGreaterThan(0)
    expect(depthOf('净资产出口')).toBeLessThan(depthOf('总资产'))
    expect(depthOf('净资产出口')).toBeLessThan(depthOf('流动资金'))
    // 列距按 4/3 放大：总资产列在 1/3、分类列在 2/3、明细列正好落在最右缘
    // （maxDepth = 4），否则最右侧会凭空空出一整列。
    expect(depthOf('总资产')).toBeCloseTo(4 / 3, 5)
    expect(depthOf('流动资金')).toBeCloseTo(8 / 3, 5)
    expect(depthOf('子类0')).toBe(4)
    // 净资产出口紧接负债出口竖条下缘（tight），不再插入间隔占位。
    expect(at('净资产出口').tight).toBe(true)
    // 出口节点与流带同色同透明度：看起来就是带子多延伸了一个竖条宽度。
    expect(at('净资产出口').itemStyle?.color).toBe('#3E9E7E')
    expect(at('净资产出口').itemStyle?.opacity).toBe(0.45)
    expect(at('净资产出口').itemStyle?.borderWidth).toBe(0)
    // 原先用来把净资产出口顶下去的透明占位已不再需要。
    expect(series.data.some((n) => n.name === '负债出口占位')).toBe(false)

    // 同列相邻节点之间插入透明占位：来源列（负债项→净资产）、分类列、明细列都有。
    const spacers = series.data.filter((n) => n.display === '' && n.name.startsWith('间隔'))
    expect(spacers.length).toBeGreaterThan(0)
    for (const spacer of spacers) {
      expect(spacer.value).toBeGreaterThan(0)
      expect(spacer.itemStyle?.opacity).toBe(0)
    }
  })

  it('出口与总资产竖条更粗：同色阴影向外补厚度', () => {
    const option = buildSankeyOption(view([30_000_000], [11_500_000]), 'amount')
    const series = sankey(option)
    const at = (name: string) => series.data.find((n) => n.name === name)!

    expect(at('负债合计').itemStyle?.shadowOffsetX).toBeLessThan(0)
    expect(at('总资产').itemStyle?.shadowOffsetX).toBeGreaterThan(0)
    // 竖条宽度：出口/总资产 13（加粗，最粗 = 2×基准-1），源列与分类列同为 10，
    // 明细列最细 7。差额由同色阴影补出（偏移量 = 目标宽度 - 7）。
    expect(at('净资产').itemStyle?.shadowOffsetX).toBe(3)
    expect(at('流动资金').itemStyle?.shadowOffsetX).toBe(3)
    expect(at('负债合计').itemStyle?.shadowOffsetX).toBe(-6)
    expect(at('总资产').itemStyle?.shadowOffsetX).toBe(6)
    expect(at('子类0').itemStyle?.shadowOffsetX).toBeUndefined()
    // 总资产竖条比第一级流带淡一档，避免过亮。
    expect(at('总资产').itemStyle?.color).toBe('#7A85C9')
    expect(at('总资产').itemStyle?.shadowColor).toBe('#7A85C9')
  })

  it('比例/隐藏模式下指标方框只留名称行', () => {
    const ratio = sankey(buildSankeyOption(view([30_000_000], [11_500_000]), 'ratio'))
    expect(ratio.data.find((n) => n.name === '负债合计')!.display).toBe('负债')
    expect(ratio.data.find((n) => n.name === '总资产')!.display).toBe('总资产')

    const hidden = sankey(buildSankeyOption(view([30_000_000], [11_500_000]), 'hidden'))
    expect(hidden.data.find((n) => n.name === '负债合计')!.display).toBe('负债')
  })

  it('负债标签带负号，标签位置按列分布', () => {
    const option = buildSankeyOption(view([30_000_000], [11_500_000]), 'amount')
    const series = sankey(option)
    const at = (name: string) => series.data.find((n) => n.name === name)!

    expect(at('净资产').display).not.toContain('-')
    expect(at('负债项0').display).toContain('-11.50万')

    // 首列标签在左；金额最大的分类（大块流带）标签嵌在带内、贴节点左缘。
    expect(at('净资产').label?.position).toBe('left')
    expect(at('负债项0').label?.position).toBe('left')
    expect(at('流动资金').label?.position).toBe('left')
  })

  it('比例模式：资产类占总资产、负债项占负债合计、净资产不显百分比', () => {
    const option = buildSankeyOption(view([30_000_000], [12_000_000]), 'ratio')
    const series = sankey(option)
    const at = (name: string) => series.data.find((n) => n.name === name)!

    expect(at('净资产').display).not.toContain('%')
    // 比例模式下负债不带负号（对齐参考稿），负号只保留在金额模式。
    expect(at('负债项0').display).toBe('负债项0  100.00%')
    expect(at('流动资金').display).toBe('流动资金  71.43%')
  })

  it('竖条不带描边，流带与竖条之间不留白缝', () => {
    const option = buildSankeyOption(view([30_000_000], [11_500_000]), 'amount')
    const series = sankey(option)

    // 基准宽度取最细的明细列，其余列靠阴影补厚（逐级变粗）。
    expect(series.nodeWidth).toBe(7)
    expect(series.nodeAlign).toBe('justify')
    // 所有节点都不描边：白描边会在流带与竖条的接缝处留出一道白线。
    for (const node of series.data) {
      expect(node.itemStyle?.borderWidth).toBe(0)
    }
    // 纯布局节点（间隔占位）整体透明；净资产出口是带子的实色延伸，单独校验。
    const invisible = series.data.filter((n) => n.display === '' && n.name !== '净资产出口')
    expect(invisible.length).toBeGreaterThan(0)
    for (const node of invisible) {
      expect(node.itemStyle?.opacity).toBe(0)
    }
  })

  it('占比最大的分类排在中间，相邻分类之间都插间隔（第一级流带各自独立）', () => {
    const option = buildSankeyOption(
      multiView([
        ['liquid', '流动资金', '#E9A66B', 10_000_000],
        ['fixed', '固定资产', '#9DBCC4', 60_000_000],
        ['investment', '投资理财', '#A9A3D4', 20_000_000],
        ['receivable', '应收款', '#63C6A5', 5_000_000],
      ]),
      'amount',
    )
    const series = sankey(option)
    const names = series.data.map((n) => n.name)
    const categories = ['流动资金', '固定资产', '投资理财', '应收款'].filter((name) => names.includes(name))

    // 最大的固定资产落在中间位置。
    expect(categories[Math.floor((categories.length - 1) / 2)]).toBe('固定资产')
    // 分类节点不再设 tight：任意相邻两个分类之间都要留出白色缝隙，
    // 第一级流带像被撕开的一条条，而不是糊成一整块。
    const categoryNodes = series.data.filter((n) => n.name !== '' && categories.includes(n.name))
    for (const node of categoryNodes) expect(node.tight).toBeUndefined()

    // 只看分类这一列（同 depth）：分类之间交替出现间隔占位。
    const categoryDepth = series.data.find((n) => n.name === '固定资产')!.depth
    const column = series.data.filter((n) => n.depth === categoryDepth).map((n) => n.name)
    const realNames = column.filter((name) => !name.startsWith('间隔'))
    expect(realNames).toEqual(['流动资金', '固定资产', '投资理财', '应收款'])
    // 每个分类（除第一个）前面都有一个间隔占位。
    expect(column[0]).toBe('流动资金')
    for (let i = 1; i < column.length; i += 2) expect(column[i]).toMatch(/^间隔/)
  })

  it('出口列与总资产列之间保留一段空白', () => {
    const series = sankey(buildSankeyOption(view([30_000_000], [11_500_000]), 'amount'))
    const depthOf = (name: string) => series.data.find((n) => n.name === name)!.depth ?? 0
    const nodeWidth = series.nodeWidth ?? 0
    // 出口列在总资产列左侧，且间隔明显大于一个竖条宽度（不是贴齐）。
    const gapInColumnWidths = depthOf('总资产') - depthOf('净资产出口')
    expect(gapInColumnWidths).toBeGreaterThan(0.1)
    expect(gapInColumnWidths).toBeLessThan(0.2)
    // 净资产出口与负债出口竖条同列，两者右缘仍然齐平。
    expect(depthOf('净资产出口')).toBe(depthOf('负债合计'))
    expect(nodeWidth).toBeGreaterThan(0)
  })

  it('左右留白随最长标签自适应，标签与竖条间距拉大', () => {
    const short = sankey(buildSankeyOption(view([30_000_000]), 'hidden'))
    const long = sankey(buildSankeyOption(view([30_000_000], [11_500_000]), 'amount'))

    // 隐藏模式标签只有名称，右侧不该再留固定的大片空白。
    expect(short.right).toBeLessThan(120)
    // 金额模式左侧标签更长（名称 + 万），左边距随之变大。
    expect(long.left).toBeGreaterThan(short.left ?? 0)
    // 两侧都收紧到文字宽度附近，横向空间让给流带。
    expect(long.right).toBeLessThan(168)

    // 普通节点标签与竖条的间距大于 ECharts 默认的 5。
    const detail = long.data.find((n) => n.name === '子类0')!
    expect(detail.label?.distance).toBe(9)
    expect(long.data.find((n) => n.name === '净资产')!.label?.distance).toBe(9)
  })

  it('来源汇入总资产后流向分类，左右总量守恒', () => {
    const option = buildSankeyOption(view([25_000_000], [5_000_000]), 'amount')
    const series = sankey(option)
    const outflow = (source: string) =>
      series.links.filter((l) => l.source === source).reduce((sum, l) => sum + l.value, 0)
    const inflow = (target: string) =>
      series.links.filter((l) => l.target === target).reduce((sum, l) => sum + l.value, 0)

    // 净资产 25,000,000 + 负债 5,000,000 = 总资产 30,000,000
    expect(outflow('净资产')).toBe(25_000_000)
    expect(outflow('负债项0')).toBe(5_000_000)
    // 负债出口竖条全额转交负债流入。
    expect(inflow('负债合计')).toBe(5_000_000)
    expect(outflow('负债合计')).toBe(5_000_000)
    // 净资产出口同样全额转交。
    expect(inflow('净资产出口')).toBe(25_000_000)
    expect(outflow('净资产出口')).toBe(25_000_000)
    expect(inflow('总资产')).toBe(30_000_000)
    // 总资产出流等于自身金额，分类流入之和等于分类金额。
    expect(outflow('总资产')).toBe(30_000_000)
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
