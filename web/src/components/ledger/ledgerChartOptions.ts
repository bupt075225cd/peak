import { use } from 'echarts/core'
import { SankeyChart, LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { EChartsOption } from 'echarts'
import type { LedgerSnapshot, LedgerSummaryView } from '../../api/ledger'
import { formatCents, formatPercent, formatWan, share, MONEY_MASK } from '../../utils/money'
import { isLedgerHidden } from '../../composables/useLedgerPrivacy'

// 账本图表按需注册，避免把整包 echarts 带进古诗文页面。
let registered = false

export function ensureLedgerECharts(): void {
  if (registered) return
  use([SankeyChart, LineChart, GridComponent, TooltipComponent, CanvasRenderer])
  registered = true
}

/** 桑基图展示模式：金额 / 比例 / 隐藏金额。 */
export type CompositionMode = 'amount' | 'ratio' | 'hidden'

type LabelPosition = 'left' | 'right' | 'top' | 'bottom'

interface SankeyNode {
  name: string
  display: string
  // 带上 value 让标签 formatter 能判断节点占比（ECharts 内部同样会用它）。
  value: number
  negative?: boolean
  // 手动指定列位置：负债出口用小数 depth 靠近总资产竖条，留出窄空白。
  depth?: number
  // 上侧指标方框节点：display 含 rich 标记，tooltip 需要走独立分支。
  boxed?: boolean
  // 与同列前一个节点紧挨（不插入间隔占位），用于负债出口竖条与净资产带的衔接。
  tight?: boolean
  itemStyle: {
    color: string
    borderColor: string
    borderWidth: number
    opacity?: number
    shadowColor?: string
    shadowBlur?: number
    shadowOffsetX?: number
  }
  label: {
    position: LabelPosition
    // 文字与竖条之间的间距（ECharts 标签 distance，默认 5）。
    distance?: number
    backgroundColor?: string
    borderColor?: string
    borderWidth?: number
    borderRadius?: number
    padding?: number | number[]
    rich?: Record<string, { color: string; fontSize: number; fontWeight?: number; lineHeight?: number; align?: 'left' | 'center' | 'right' }>
  }
}

interface SankeyLink {
  source: string
  target: string
  value: number
  // 参考稿里每条流带通体一色（不是渐变），颜色显式指定在该连接的 lineStyle 上，
  // 透明度统一由 LINK_OPACITY 决定；个别连接只承担数据守恒、不做视觉呈现
  // （如负债出口 → 总资产的衔接段），把 opacity 置 0。
  lineStyle: { color: string; opacity: number }
}

const COLORS = {
  net: '#3E9E7E',
  // 第一级流带的颜色（总资产的蓝紫），也是总资产竖条阴影的底色。
  total: '#4C5BB4',
  // 总资产竖条本身更淡一档：它比流带更醒目，但不宜过亮压过两侧内容。
  totalBar: '#7A85C9',
  debt: '#B9B4C7',
  debtExit: '#8E89A6',
  liquid: '#E9A66B',
  fixed: '#9DBCC4',
  investment: '#A9A3D4',
  receivable: '#63C6A5',
}

// 流带统一为纯色填充，透明度只用来压淡，确保每条带子从头到尾一个色。
// 整体偏淡（参考稿里流带比竖条浅很多），竖条保持不透明以拉开层次。
const LINK_OPACITY = 0.45

// 列距换算：ECharts 桑基的列宽按 maxDepth 均分，而 maxDepth 取拓扑层数
// （源 → 出口 → 总资产 → 分类 → 明细共 5 层，减 1 得 4），画面上却只有 3 段
// 列距。显式 depth 因此要按 4/3 放大，明细列才会正好落在画布右缘；否则最右侧
// 会凭空多出一整列（约画布宽度的 1/4）空白。
const MAX_DEPTH = 4
const COLUMN_STEP = MAX_DEPTH / 3
// 出口列与总资产列之间留的空白（depth 单位）：约一列宽度的 12%，随画布等比缩放。
const EXIT_GAP = 0.12

// 竖条宽度：基准取最细的明细列，其余列依次加粗（参考稿里越靠近源头竖条越粗）。
// 桑基节点宽度是全局值，只能靠同色阴影向外补厚度；阴影与本体至少要重叠 1px
// （否则接缝处会透出 1px 白线），所以最粗只能补到 2 × 基准 - 1。
const NODE_WIDTH = 7
const BAR_WIDTH = {
  source: 10, // 最左侧：负债项 / 净资产
  exit: 13, // 负债出口竖条（加粗，最粗 2×7-1）
  total: 13, // 总资产竖条（与出口竖条同宽）
  group: 10, // 分类列：与源列同宽
  detail: NODE_WIDTH, // 最右侧：明细列（最细）
}
/** 竖条要补出的厚度（相对基准宽度），留 1px 与本体重叠，避免接缝白线。 */
const barExtra = (width: number): number => Math.min(Math.max(width - NODE_WIDTH, 0), NODE_WIDTH - 1)
// 标签排版基准：与桑基 label.fontSize 保持一致，用于估算文字宽度。
const LABEL_FONT_SIZE = 12.5
// 文字与竖条的间距（ECharts 默认 5），拉大后标签不再紧贴竖条。
const LABEL_DISTANCE = 9
// 画布边缘到文字的最小留白：保证标签不被裁切，又不留出大片空白。
const LABEL_MARGIN = 10

/**
 * 粗略估算标签的像素宽度：中日韩字符按一个字号宽，西文与数字按半个字号宽，
 * 空格更窄。只用于决定左右外边距，略偏高估以免最后一个字被裁掉。
 */
function labelTextWidth(text: string): number {
  let width = 0
  for (const ch of text) {
    if (ch === ' ') width += LABEL_FONT_SIZE * 0.3
    else if (/[\u2e80-\u9fff\uff00-\uffef]/.test(ch)) width += LABEL_FONT_SIZE
    else width += LABEL_FONT_SIZE * 0.56
  }
  return width
}

const link = (source: string, target: string, value: number, color: string, opacity = LINK_OPACITY): SankeyLink => ({
  source,
  target,
  value,
  lineStyle: { color, opacity },
})

/**
 * 构建资产组成桑基数据（对齐参考稿形态）。
 *
 * 左侧是资金的两个来源——负债（逐项展开，在上）与净资产（在下）。负债带右端
 * 有自己的深灰紫竖条（负债出口）；净资产不带竖条，但流带在同列收口，右缘与
 * 负债出口竖条的右缘垂直对齐。出口列与总资产列之间留一段窄空白，再经透明衔接段
 * 接入总资产。总资产 = 净资产 + 负债，金额由页面上方的方框展示，节点本身不带
 * 文字。分类列把占比最大的分类放在中间；相邻分类之间一律插入透明间隔占位，
 * 于是每条第一级流带都被一道白色缝隙隔开、各自像撕下来的一条（带子两端落点不同，
 * 自然带一点倾斜）。负债标签统一带负号（金额模式）；总量在总资产处守恒。
 */
export function buildSankeyOption(view: LedgerSummaryView, mode: CompositionMode): EChartsOption {
  const nodes: SankeyNode[] = []
  const links: SankeyLink[] = []
  const totalAsset = view.totalAssetCents
  const net = view.netAssetCents
  const denom = Math.max(totalAsset, 1)

  type NodeKind = 'net' | 'debt' | 'asset'

  const assetGroups = view.groups.filter((g) => g.kind === 'asset' && g.amount_cents > 0)
  const debtItems: { name: string; value: number }[] = []
  for (const group of view.groups) {
    if (group.kind !== 'liability') continue
    for (const child of group.children) {
      if (child.amount_cents <= 0) continue
      // 负债项与资产分类重名时补后缀，避免 ECharts 合并同名节点。
      const name = assetGroups.some((g) => g.label === child.label) ? `${child.label}（负债）` : child.label
      debtItems.push({ name, value: child.amount_cents })
    }
  }
  const totalDebt = debtItems.reduce((sum, item) => sum + item.value, 0)

  // 分类列的视觉顺序：占比最大的分类挪到中间，其余保持原顺序。
  const groupOrder = [...assetGroups]
  let biggestIndex = 0
  groupOrder.forEach((group, index) => {
    if (group.amount_cents > groupOrder[biggestIndex].amount_cents) biggestIndex = index
  })
  if (groupOrder.length > 0) {
    const [biggest] = groupOrder.splice(biggestIndex, 1)
    biggestIndex = Math.min(Math.floor((assetGroups.length - 1) / 2), groupOrder.length)
    groupOrder.splice(biggestIndex, 0, biggest)
  }

  // 标签文案（对齐参考稿口径）：金额模式带「万」、负债负号；比例模式下
  // 资产类占总资产、负债项占负债合计（不带负号）、净资产只留名称不显百分比。
  const label = (name: string, value: number, kind: NodeKind): string => {
    if (mode === 'hidden') return name
    if (mode === 'ratio') {
      if (kind === 'net') return name
      const base = kind === 'debt' ? Math.max(totalDebt, 1) : denom
      return `${name}  ${formatPercent(share(value, base))}`
    }
    return `${name}  ${kind === 'debt' ? '-' : ''}${formatWan(value)}`
  }

  // 明细列中占比极小的节点（如 5.6 万的货币基金）在比例模式下会被压成一条线，
  // 标签必然互相压叠，这里直接不画标签，值仍可通过悬停 tooltip 查看。
  const detailNames = new Set<string>()
  const MIN_DETAIL_LABEL_RATIO = 0.012

  const addNode = (
    name: string,
    value: number,
    color: string,
    position: LabelPosition,
    kind: NodeKind,
    opts: { noLabel?: boolean; depth?: number; barWidth?: number; tight?: boolean } = {},
  ) => {
    // 竖条比基准宽时，用同色阴影向右补出厚度（阴影不改变纵向范围）。
    const extra = barExtra(opts.barWidth ?? NODE_WIDTH)
    nodes.push({
      name,
      display: opts.noLabel ? '' : label(name, value, kind),
      value,
      negative: kind === 'debt',
      ...(opts.depth !== undefined ? { depth: opts.depth } : {}),
      ...(opts.tight ? { tight: true } : {}),
      // 不加描边：白描边会在流带与竖条的接缝处留出一道白线（参考稿里两者是
      // 严丝合缝的）；同一列的竖条之间靠透明占位节点拉开距离，不需要描边分隔。
      itemStyle: {
        color,
        borderColor: 'transparent',
        borderWidth: 0,
        ...(extra > 0 ? { shadowColor: color, shadowBlur: 0, shadowOffsetX: extra } : {}),
      },
      label: { position, distance: LABEL_DISTANCE },
    })
  }

  // 列一（左）：负债在上、净资产在下；负债逐项展开不合并。手动 depth 控制列位。
  for (const item of debtItems) {
    addNode(item.name, item.value, COLORS.debt, 'left', 'debt', { depth: 0, barWidth: BAR_WIDTH.source })
  }
  if (net > 0) addNode('净资产', net, COLORS.net, 'left', 'net', { depth: 0, barWidth: BAR_WIDTH.source })

  // 上侧指标方框（负债/总资产）：作为对应节点的顶部标签渲染，天然与竖条
  // 垂直居中对齐。金额模式显示两行（名称 + 万，负债负号）；比例/隐藏模式
  // 只留名称行，方框收缩。全局隐藏金额态沿用脱敏占位符。
  const boxValue = (kind: 'asset' | 'liability'): string => {
    const cents = kind === 'liability' ? totalDebt : totalAsset
    if (isLedgerHidden()) return MONEY_MASK
    return kind === 'liability' ? `-${formatWan(cents)}` : formatWan(cents)
  }
  // 两个指标框共用同一底色（参考稿里是一组同色卡片）。
  const BOX_BG = '#F1F3F7'
  // 方框相对节点顶部上抬的像素（拉开方框与流带的间隔，对齐参考稿）。
  const BOX_LIFT = 26
  const boxRich = (valueColor: string) => ({
    btName: { color: '#94A3B8', fontSize: 11, lineHeight: 16, align: 'left' as const },
    btValue: { color: valueColor, fontSize: 17, fontWeight: 600 as const, lineHeight: 24, align: 'left' as const },
  })
  const boxDisplay = (name: string, kind: 'asset' | 'liability'): string =>
    mode === 'amount' ? `{btName|${name}}\n{btValue|${boxValue(kind)}}` : name

  // 列二：负债带的右端竖条（对齐参考稿：负债有自己的右侧竖条，颜色比带更深，
  // 竖条即负债带的末端）。净资产不带竖条，但流带在同一列收口：出口节点用与
  // 流带完全相同的颜色和透明度绘制，视觉上等于把净资产带向右延长一个竖条宽度，
  // 两条带的右缘因此始终齐平——不依赖画布宽度，图表缩放时也不会错位。
  const DEBT_EXIT = '负债合计'
  const NET_EXIT = '净资产出口'
  // 出口列：与总资产列之间留一段窄空白（参考稿），空白宽度按列距等比缩放。
  const EXIT_DEPTH = COLUMN_STEP - EXIT_GAP
  if (totalDebt > 0) {
    const exitNode: SankeyNode = {
      name: DEBT_EXIT,
      display: boxDisplay('负债', 'liability'),
      value: totalDebt,
      negative: true,
      boxed: true,
      depth: EXIT_DEPTH,
      // 出口竖条比基准略粗：桑基节点宽度是全局值，这里用同色阴影向左补出厚度
      // （阴影不改变纵向范围，不会破坏上下对齐，右缘仍与净资产带齐平）。
      itemStyle: {
        color: COLORS.debtExit,
        // 不加白描边：描边会在补出的厚度与本体之间留下白色缝。
        borderColor: 'transparent',
        borderWidth: 0,
        shadowColor: COLORS.debtExit,
        shadowBlur: 0,
        shadowOffsetX: -barExtra(BAR_WIDTH.exit),
      },
      label: {
        position: 'top',
        backgroundColor: BOX_BG,
        borderRadius: 12,
        padding: [8, 16],
        rich: boxRich('#0F172A'),
      },
    }
    nodes.push(exitNode)
    for (const item of debtItems) links.push(link(item.name, DEBT_EXIT, item.value, COLORS.debt))
  }
  if (net > 0) {
    // 出口节点与负债出口竖条同列、紧接其下缘（tight，不插间隔占位），并落在与
    // 净资产带相同的纵向区间。用带子的同色同透明度绘制：流带在节点左缘收口，
    // 节点再向右延续一个竖条宽度，右缘正好与负债出口竖条的右缘对齐。
    const exitNode: SankeyNode = {
      name: NET_EXIT,
      display: '',
      value: net,
      depth: EXIT_DEPTH,
      tight: true,
      itemStyle: { color: COLORS.net, borderColor: 'transparent', borderWidth: 0, opacity: LINK_OPACITY },
      label: { position: 'left' },
    }
    nodes.push(exitNode)
    links.push(link('净资产', NET_EXIT, net, COLORS.net))
  }

  // 列三（中）：总资产节点（参考稿大块左缘的窄竖条），是左右两侧的枢纽。
  // 出口列与总资产列之间留一段窄空白：负债出口 / 净资产出口 → 总资产的衔接段
  // 只承担数据守恒，透明不渲染；总资产竖条与右侧大块之间则无缝紧贴。
  const TOTAL = '总资产'
  if (totalAsset > 0) {
    nodes.push({
      name: TOTAL,
      display: boxDisplay('总资产', 'asset'),
      value: totalAsset,
      boxed: true,
      // 总资产列固定在 1/3 画布处：显式 depth 按 4/3 放大后仍是「一列」。
      depth: COLUMN_STEP,
      itemStyle: {
        // 竖条用比流带更淡一档的蓝紫，避免整块太亮压过两侧内容。
        color: COLORS.totalBar,
        borderColor: 'transparent',
        borderWidth: 0,
        shadowColor: COLORS.totalBar,
        shadowBlur: 0,
        // 总资产竖条向右补厚度：左缘（与指标方框对齐）和左侧空白都保持不变，
        // 右侧多出的部分压在固定资产大块的起始处，视觉上仍是无缝的大块边缘。
        shadowOffsetX: barExtra(BAR_WIDTH.total),
      },
      label: {
        position: 'top',
        // 与负债方框同底色（对齐参考稿：两个指标框是一组同色卡片，不描边）。
        backgroundColor: BOX_BG,
        borderRadius: 12,
        padding: [8, 16],
        rich: boxRich('#4C5BB4'),
      },
    })
    if (totalDebt > 0) {
      links.push(link(DEBT_EXIT, TOTAL, totalDebt, COLORS.debtExit, 0))
    }
    if (net > 0) links.push(link(NET_EXIT, TOTAL, net, COLORS.net, 0))
  }

  // 列四：资产分类；列五：明细（只保留金额最大的 3 项，其余合并为「其他」）。
  // 分类标签统一嵌在各自流带内、贴节点左缘（对齐参考稿），只有最后一级
  // 明细的标签放在节点右侧。
  const groupColor = (group: { category: string; color: string }): string =>
    COLORS[group.category as keyof typeof COLORS] ?? group.color

  groupOrder.forEach((group) => {
    const color = groupColor(group)
    // 不设 tight：每个分类前都插间隔占位，相邻第一级流带之间留出白色缝隙。
    addNode(group.label, group.amount_cents, color, 'left', 'asset', {
      depth: COLUMN_STEP * 2,
      barWidth: BAR_WIDTH.group,
    })

    const children = group.children.filter((child) => child.amount_cents > 0).sort((a, b) => b.amount_cents - a.amount_cents)
    const head = children.slice(0, 3)
    const tail = children.slice(3)
    for (const child of head) {
      // 父子同名时补后缀，避免 ECharts 节点重名。
      const childName = child.label === group.label ? `${child.label}（明细）` : child.label
      addNode(childName, child.amount_cents, color, 'right', 'asset', { depth: COLUMN_STEP * 3 })
      detailNames.add(childName)
      links.push(link(group.label, childName, child.amount_cents, color))
    }
    if (tail.length) {
      const restValue = tail.reduce((sum, child) => sum + child.amount_cents, 0)
      const restName = `${group.label} 其他 ${tail.length} 项`
      addNode(restName, restValue, color, 'right', 'asset', { depth: COLUMN_STEP * 3 })
      detailNames.add(restName)
      links.push(link(group.label, restName, restValue, color))
    }
  })

  // 来源 → 分类：按各分类在总资产中的占比分摊，最后一项吃下舍入余量，
  // 保证每条来源的流出之和恰好等于它的金额、每个分类的流入之和等于该分类金额。
  // 参考稿里第一级流带（总资产 → 各分类）整体是同一个色（总资产的蓝紫），
  // 到第二级（分类 → 明细）才各自换成所属分类的颜色。
  const allocate = (sourceName: string, amount: number) => {
    let allocated = 0
    groupOrder.forEach((group, index) => {
      const isLast = index === groupOrder.length - 1
      const value = isLast
        ? Math.max(amount - allocated, 0)
        : Math.round(amount * (group.amount_cents / denom))
      allocated += value
      if (value > 0) links.push(link(sourceName, group.label, value, COLORS.total))
    })
  }
  // 总资产 → 分类：全部总量从总资产节点流出，按分类顺序堆叠，不交叉。
  if (totalAsset > 0) allocate(TOTAL, totalAsset)

  // 左右外边距按“最长的可见标签”自适应：标签短时不至于像固定 168px 那样留出
  // 一大片空白，标签长时也不会被画布边缘裁掉。方框标签（position: top）走
  // labelLayout 定位，不参与这里的计算。
  const labelWidthOf = (node: SankeyNode): number => {
    if (!node.display || node.label.position === 'top') return 0
    if (detailNames.has(node.name) && totalAsset > 0 && node.value / totalAsset < MIN_DETAIL_LABEL_RATIO) return 0
    return labelTextWidth(node.display)
  }
  let maxLeftLabel = 0
  let maxRightLabel = 0
  for (const node of nodes) {
    if (node.label.position === 'left') maxLeftLabel = Math.max(maxLeftLabel, labelWidthOf(node))
    else if (node.label.position === 'right') maxRightLabel = Math.max(maxRightLabel, labelWidthOf(node))
  }
  // 留白 = 文字宽度 + 与竖条的间距 + 边缘最小留白；下限 48 保证空数据时也有呼吸空间。
  const marginLeft = Math.max(48, Math.ceil(maxLeftLabel + LABEL_DISTANCE + LABEL_MARGIN))
  const marginRight = Math.max(48, Math.ceil(maxRightLabel + LABEL_DISTANCE + LABEL_MARGIN))

  // 列内间隔：nodeGap 已置 0，改为在相邻同级节点之间插入透明占位节点（金额按
  // 总资产比例给，间隔随图表尺寸缩放）。这样出口竖条与净资产带可以紧挨，而来源
  // 列、分类列、明细列仍保留空隙。
  const GAP_VALUE = Math.max(Math.round(totalAsset * 0.034), 1)
  const spacedNodes: SankeyNode[] = []
  const lastAtDepth = new Map<number, SankeyNode>()
  for (const node of nodes) {
    const depth = node.depth
    if (depth !== undefined) {
      const previous = lastAtDepth.get(depth)
      if (previous && !node.tight) {
        spacedNodes.push({
          name: `间隔${depth}-${spacedNodes.length}`,
          display: '',
          value: GAP_VALUE,
          depth,
          itemStyle: { color: 'transparent', borderColor: 'transparent', borderWidth: 0, opacity: 0 },
          label: { position: 'left' },
        })
      }
      lastAtDepth.set(depth, node)
    }
    spacedNodes.push(node)
  }

  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      formatter: (params: unknown) => {
        const data = params as {
          dataType?: string
          data?: { display?: string; value?: number; boxed?: boolean }
          name?: string
        }
        if (mode === 'hidden') return data.name ?? ''
        if (data.dataType === 'edge') return `${formatCents(data.data?.value ?? 0)} 元`
        // 指标方框节点的 display 含 rich 标记，tooltip 用纯文本。
        if (data.data?.boxed) return `${data.name ?? ''}  ${formatCents(data.data?.value ?? 0)} 元`
        // 不可见节点（净资产出口 / 负债占位）只用于布局，不弹提示。
        if (data.data?.display === '') return ''
        return data.data?.display || data.name || ''
      },
    },
    series: [
      {
        type: 'sankey',
        // 左右留白按最长标签自适应（见 marginLeft / marginRight），横向空间尽量给流带；
        // 顶部留出负债/总资产方框的高度，四列按 justify 均分列距。
        left: marginLeft,
        right: marginRight,
        top: 104,
        bottom: 30,
        nodeWidth: NODE_WIDTH,
        // 列内间隔由显式占位节点提供（见 buildSankeyOption 末尾的 insertGaps）：
        // 桑基的 nodeGap 是全局的，无法让出口竖条与净资产带紧挨、又保留来源列间隔。
        nodeGap: 0,
        nodeAlign: 'justify',
        // 禁用布局迭代：迭代会按金额重排序，导致净资产跑到负债上方；
        // 置 0 后按数据顺序布局，交叉通过 links 的添加顺序控制。
        layoutIterations: 0,
        draggable: false,
        emphasis: { focus: 'adjacency' },
        label: {
          color: '#334155',
          fontSize: 12.5,
          // 参考稿的标签是纯文字（左侧来源与右侧分类/明细一致），不加背景框。
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
        // 指标方框（负债/总资产）向两侧偏移展开并豁免隐藏：两竖条相距很近，
        // 居中渲染的两个方框会互相重叠（对齐参考稿的并排布局）。
        // 偏移量由回调参数里的两组矩形精确计算（金额位数变化时仍保持对齐）：
        // rect = 竖条节点矩形，labelRect = 方框标签包围盒（同一全局坐标系）。
        // 总资产框左缘对齐竖条左缘，负债框右缘对齐负债出口竖条右缘。
        labelLayout: (params: unknown) => {
          const p = params as {
            dataIndex?: number
            rect?: { x?: number; width?: number }
            labelRect?: { x?: number; width?: number }
          }
          // dataIndex 对应最终渲染的 spacedNodes（列内已插入间隔占位）。
          const node = typeof p.dataIndex === 'number' ? spacedNodes[p.dataIndex] : undefined
          if (!node?.boxed) return { hideOverlap: true }
          const { rect, labelRect } = p
          let dx = 0
          if (rect?.x != null && rect.width != null && labelRect?.x != null && labelRect.width != null) {
            dx =
              node.name === '总资产'
                ? rect.x - labelRect.x
                : rect.x + rect.width - (labelRect.x + labelRect.width)
          }
          // 方框整体上抬，拉开与下方流带的间隔。
          const dy = -BOX_LIFT
          return { dx, dy, hideOverlap: false }
        },
        // 低弯曲度让「总资产→固定资产」的大流带近似矩形，紧贴总资产节点，
        // 对齐参考稿的大块形态；细带仍保留柔和的 S 弯。
        // 参考稿的流带是纯色而非渐变：每条连接的角标 color 已在 link() 里逐条指定，
        // 这里的 color 只是兜底（未指定时取目标节点色），不会产生渐变。
        lineStyle: { color: 'target', opacity: LINK_OPACITY, curveness: 0.3 },
        data: spacedNodes,
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
