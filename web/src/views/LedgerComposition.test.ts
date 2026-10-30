import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import LedgerComposition from './LedgerComposition.vue'
import { resetLedgerMetaCache } from '../composables/useLedgerMeta'
import { useLedgerPrivacy } from '../composables/useLedgerPrivacy'

const { getMeta, getConsent, getSummary, listMembers, downloadBlob, watermarkDataURL } = vi.hoisted(() => ({
  getMeta: vi.fn(),
  getConsent: vi.fn(),
  getSummary: vi.fn(),
  listMembers: vi.fn(),
  downloadBlob: vi.fn(),
  watermarkDataURL: vi.fn(),
}))

vi.mock('../api/ledger', async () => {
  const actual = await vi.importActual<typeof import('../api/ledger')>('../api/ledger')
  return { ...actual, getMeta, getConsent, getSummary, listMembers }
})

vi.mock('../utils/watermark', () => ({
  downloadBlob,
  watermarkDataURL,
  dataURLToBlob: () => new Blob(['x'], { type: 'image/png' }),
  ledgerMark: () => 'AB12CD · 2026.10.23',
}))

// 图表依赖 canvas，测试用 stub 暴露导出方法。
vi.mock('vue-echarts', () => ({
  default: {
    name: 'VChart',
    setup(_: unknown, { expose }: { expose: (api: Record<string, unknown>) => void }) {
      expose({ getDataURL: () => 'data:image/png;base64,QQ==' })
      return () => null
    },
  },
}))

const meta = {
  categories: [
    { key: 'liquid', label: '流动资金', kind: 'asset', color: '#E9A66B', sub_types: [] },
    { key: 'fixed', label: '固定资产', kind: 'asset', color: '#9DBCC4', sub_types: [] },
    { key: 'liability', label: '负债', kind: 'liability', color: '#B9B4C7', sub_types: [] },
  ],
  risk_types: [],
  currencies: [],
  consent_policy_version: 'v1',
  consent_required: false,
}

const summary = {
  groups: [
    { category: 'liquid', label: '流动资金', kind: 'asset', color: '#E9A66B', amount_cents: 1_000_000, count: 2, children: [] },
    { category: 'fixed', label: '固定资产', kind: 'asset', color: '#9DBCC4', amount_cents: 2_000_000, count: 1, children: [] },
    { category: 'liability', label: '负债', kind: 'liability', color: '#B9B4C7', amount_cents: 500_000, count: 1, children: [] },
  ],
  total_asset_cents: 3_000_000,
  total_liability_cents: 500_000,
  net_asset_cents: 2_500_000,
  item_count: 4,
  updated_at: '2026-10-01T10:00:00Z',
  has_foreign: false,
  consent_required: false,
}

async function mountView() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/ledger/composition', name: 'ledger-composition', component: LedgerComposition },
      { path: '/ledger', name: 'ledger-dashboard', component: { template: '<div />' } },
      { path: '/ledger/items', name: 'ledger-items', component: { template: '<div />' } },
    ],
  })
  router.push('/ledger/composition')
  await router.isReady()
  const wrapper = mount(LedgerComposition, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  resetLedgerMetaCache()
  sessionStorage.clear()
  useLedgerPrivacy().show()
  ;[getMeta, getConsent, getSummary, listMembers, downloadBlob, watermarkDataURL].forEach((fn) => fn.mockReset())
  getMeta.mockResolvedValue(meta)
  getConsent.mockResolvedValue({ policy_version: 'v1', required: false, consented_at: '2026-10-01T00:00:00Z' })
  getSummary.mockResolvedValue(summary)
  listMembers.mockResolvedValue([])
  watermarkDataURL.mockResolvedValue('data:image/png;base64,QQ==')
})

describe('LedgerComposition.vue 资产组成', () => {
  it('只展示桑基图（含负债/总资产指标方框），不出现右侧汇总卡、图例与口径说明', async () => {
    const wrapper = await mountView()

    expect(wrapper.find('[data-test="composition-chart"]').exists()).toBe(true)
    // 方框由 ECharts 节点标签渲染（数据层断言见 ledgerChartOptions.test.ts）。

    expect(wrapper.find('[data-test="composition-total"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="composition-liability"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="composition-net"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('金额按录入币种面值计入')
    // 副标题与五色图例都按反馈移除。
    expect(wrapper.text()).not.toContain('从净资产与负债出发')
    expect(wrapper.text()).not.toContain('应收款')
  })

  it('不再提供排除固定资产的范围切换', async () => {
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="scope-all"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="scope-exclude-fixed"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('排除固定资产')
  })

  it('切换隐藏金额后同步全局隐藏态', async () => {
    const wrapper = await mountView()
    await wrapper.find('[data-test="mode-hidden"]').trigger('click')
    await flushPromises()
    expect(useLedgerPrivacy().hidden.value).toBe(true)
  })

  it('下载图片时叠加会话水印', async () => {
    const wrapper = await mountView()
    await wrapper.find('[data-test="download-composition"]').trigger('click')
    await flushPromises()
    expect(watermarkDataURL).toHaveBeenCalledWith('data:image/png;base64,QQ==', 'AB12CD · 2026.10.23')
    expect(downloadBlob).toHaveBeenCalledWith(expect.any(Blob), expect.stringContaining('资产组成-'))
  })

  it('刷新按钮已移除，只保留三态切换与下载', async () => {
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="refresh-composition"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="mode-amount"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="download-composition"]').exists()).toBe(true)
  })

  it('三态顺序为比例、金额、隐藏金额，默认选中比例', async () => {
    const wrapper = await mountView()
    const buttons = wrapper.findAll('[data-test^="mode-"]')
    expect(buttons.map((b) => b.attributes('data-test'))).toEqual([
      'mode-ratio',
      'mode-amount',
      'mode-hidden',
    ])
    // 默认比例：首个按钮高亮，图表按比例模式渲染。
    expect(buttons[0].classes()).toContain('bg-ledger-ink')
    expect(buttons[1].classes()).not.toContain('bg-ledger-ink')
    // 图表组件收到 ratio 模式（CompositionChart 占位组件透传 props）。
    const chart = wrapper.findComponent({ name: 'CompositionChart' })
    expect(chart.exists() && (chart.props('mode') as string)).toBe('ratio')
  })

  it('标题右侧成员下拉：切换成员后按 member_id 重新聚合', async () => {
    listMembers.mockResolvedValue([{ id: 5, name: '配偶', sort_order: 1, visible: true }])
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('家庭资产组成')
    expect(wrapper.find('[data-test="member-select-trigger"]').text()).toContain('全部')

    await wrapper.find('[data-test="member-select-trigger"]').trigger('click')
    expect(wrapper.find('[data-test="member-option-self"]').text()).toContain('我')
    expect(wrapper.find('[data-test="member-option-5"]').text()).toContain('配偶')

    await wrapper.find('[data-test="member-option-5"]').trigger('click')
    await flushPromises()

    expect(getSummary).toHaveBeenLastCalledWith('5')
    expect(wrapper.find('[data-test="member-select-trigger"]').text()).toContain('配偶')
    expect(wrapper.find('[data-test="member-select-menu"]').exists()).toBe(false)
  })

  it('隐藏成员不出现在下拉里', async () => {
    listMembers.mockResolvedValue([
      { id: 5, name: '配偶', sort_order: 1, visible: true },
      { id: 6, name: '已隐藏', sort_order: 2, visible: false },
    ])
    const wrapper = await mountView()
    await wrapper.find('[data-test="member-select-trigger"]').trigger('click')

    expect(wrapper.find('[data-test="member-option-5"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="member-option-6"]').exists()).toBe(false)
  })

  it('加载失败展示错误提示', async () => {
    getSummary.mockRejectedValue({ response: { data: { message: '解密失败' } } })
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="composition-error"]').text()).toContain('解密失败')
  })
})
