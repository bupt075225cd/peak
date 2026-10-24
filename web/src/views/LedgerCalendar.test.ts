import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import LedgerCalendar from './LedgerCalendar.vue'
import { resetLedgerMetaCache } from '../composables/useLedgerMeta'
import { useLedgerPrivacy } from '../composables/useLedgerPrivacy'
import { MONEY_MASK } from '../utils/money'

const { getMeta, getConsent, listMembers, listSnapshots } = vi.hoisted(() => ({
  getMeta: vi.fn(),
  getConsent: vi.fn(),
  listMembers: vi.fn(),
  listSnapshots: vi.fn(),
}))

vi.mock('../api/ledger', async () => {
  const actual = await vi.importActual<typeof import('../api/ledger')>('../api/ledger')
  return { ...actual, getMeta, getConsent, listMembers, listSnapshots }
})

vi.mock('vue-echarts', () => ({ default: { name: 'VChart', template: '<div data-test="trend-chart" />' } }))

const meta = {
  categories: [
    { key: 'liquid', label: '流动资金', kind: 'asset', color: '#E9A66B', sub_types: [] },
    { key: 'liability', label: '负债', kind: 'liability', color: '#B9B4C7', sub_types: [] },
  ],
  risk_types: [],
  currencies: [],
  consent_policy_version: 'v1',
  consent_required: false,
}

function snapshot(period: string, net: number, asset: number, liability: number) {
  return {
    period,
    payload: {
      total_asset_cents: asset,
      total_liability_cents: liability,
      net_asset_cents: net,
      breakdown: [
        { category: 'liquid', label: '流动资金', amount_cents: asset - 100_000, count: 2 },
        { category: 'liability', label: '负债', amount_cents: liability, count: 1 },
      ],
    },
    updated_at: `${period}-01T00:00:00Z`,
  }
}

const snapshots = [snapshot('2026-08', 1_000_000, 1_200_000, 200_000), snapshot('2026-09', 1_500_000, 1_800_000, 300_000)]

async function mountView() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/ledger/calendar', name: 'ledger-calendar', component: LedgerCalendar },
      { path: '/ledger', name: 'ledger-dashboard', component: { template: '<div />' } },
      { path: '/ledger/items', name: 'ledger-items', component: { template: '<div />' } },
      { path: '/ledger/composition', name: 'ledger-composition', component: { template: '<div />' } },
    ],
  })
  router.push('/ledger/calendar')
  await router.isReady()
  const wrapper = mount(LedgerCalendar, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 9, 23))
  resetLedgerMetaCache()
  sessionStorage.clear()
  useLedgerPrivacy().show()
  ;[getMeta, getConsent, listMembers, listSnapshots].forEach((fn) => fn.mockReset())
  getMeta.mockResolvedValue(meta)
  getConsent.mockResolvedValue({ policy_version: 'v1', required: false, consented_at: '2026-10-01T00:00:00Z' })
  listMembers.mockResolvedValue([])
  listSnapshots.mockResolvedValue(snapshots)
})

afterEach(() => {
  vi.useRealTimers()
})

describe('LedgerCalendar.vue 资产月历', () => {
  it('渲染 12 个月网格与年度汇总', async () => {
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="current-year"]').text()).toBe('2026 年')
    expect(wrapper.findAll('[data-test^="month-2026-"]')).toHaveLength(12)
    // 年度指标取当年最新一期快照（9 月）
    expect(wrapper.find('[data-test="year-net"]').text()).toBe('15,000.00')
    expect(wrapper.find('[data-test="year-total"]').text()).toBe('18,000.00')
    expect(wrapper.find('[data-test="year-liability"]').text()).toBe('3,000.00')
  })

  it('默认选中最新有快照的月份并展示分类构成', async () => {
    const wrapper = await mountView()
    expect(wrapper.text()).toContain('2026-09 分类构成')
    expect(wrapper.text()).toContain('流动资金')
    expect(wrapper.text()).toContain('负债项，不计入资产占比')
  })

  it('点击其它月份切换构成详情', async () => {
    const wrapper = await mountView()
    await wrapper.find('[data-test="month-2026-08"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('2026-08 分类构成')
  })

  it('向上一年切换时按年份重新拉取', async () => {
    const wrapper = await mountView()
    listSnapshots.mockClear()
    listSnapshots.mockResolvedValue([])
    await wrapper.find('[data-test="prev-year"]').trigger('click')
    await flushPromises()
    expect(listSnapshots).toHaveBeenCalledWith('2025')
    expect(wrapper.find('[data-test="current-year"]').text()).toBe('2025 年')
    expect(wrapper.text()).toContain('还没有月度快照')
  })

  it('当年时禁止切换到下一年', async () => {
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="next-year"]').attributes('disabled')).toBeDefined()
  })

  it('隐藏金额后月历与汇总统一脱敏', async () => {
    useLedgerPrivacy().hide()
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="year-net"]').text()).toBe(MONEY_MASK)
    expect(wrapper.find('[data-test="month-2026-09"]').text()).toContain(MONEY_MASK)
  })

  it('加载失败展示错误提示', async () => {
    listSnapshots.mockRejectedValue({ response: { data: { message: '快照解密失败' } } })
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="calendar-error"]').text()).toContain('快照解密失败')
  })
})
