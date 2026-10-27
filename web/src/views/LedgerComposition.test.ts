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
      { path: '/ledger/calendar', name: 'ledger-calendar', component: { template: '<div />' } },
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
  it('只展示桑基图与图例，不出现右侧汇总卡与口径说明', async () => {
    const wrapper = await mountView()

    expect(wrapper.find('[data-test="composition-chart"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('流动资金')
    expect(wrapper.text()).toContain('固定资产')

    // 需求：用户只看桑基图，右侧三项汇总与"面值/汇率"口径说明都不要。
    expect(wrapper.find('[data-test="composition-total"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="composition-liability"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="composition-net"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('金额按录入币种面值计入')
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

  it('刷新按钮重新拉取聚合', async () => {
    const wrapper = await mountView()
    getSummary.mockClear()
    await wrapper.find('[data-test="refresh-composition"]').trigger('click')
    await flushPromises()
    expect(getSummary).toHaveBeenCalledTimes(1)
  })

  it('加载失败展示错误提示', async () => {
    getSummary.mockRejectedValue({ response: { data: { message: '解密失败' } } })
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="composition-error"]').text()).toContain('解密失败')
  })
})
