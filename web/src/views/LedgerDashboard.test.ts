import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import LedgerDashboard from './LedgerDashboard.vue'
import { resetLedgerMetaCache } from '../composables/useLedgerMeta'
import { useLedgerPrivacy } from '../composables/useLedgerPrivacy'
import { MONEY_MASK } from '../utils/money'

const { getMeta, getConsent, acceptConsent, getSummary, listItems, listMembers, createItem, createMember } = vi.hoisted(() => ({
  getMeta: vi.fn(),
  getConsent: vi.fn(),
  acceptConsent: vi.fn(),
  getSummary: vi.fn(),
  listItems: vi.fn(),
  listMembers: vi.fn(),
  createItem: vi.fn(),
  createMember: vi.fn(),
}))

vi.mock('../api/ledger', async () => {
  const actual = await vi.importActual<typeof import('../api/ledger')>('../api/ledger')
  return {
    ...actual,
    getMeta,
    getConsent,
    acceptConsent,
    getSummary,
    listItems,
    listMembers,
    createItem,
    createMember,
  }
})

const meta = {
  categories: [
    {
      key: 'liquid',
      label: '流动资金',
      kind: 'asset',
      color: '#E9A66B',
      sub_types: [{ key: 'bank_current', label: '银行活期', icon: 'Landmark', is_fixed: false }],
    },
    {
      key: 'fixed',
      label: '固定资产',
      kind: 'asset',
      color: '#9DBCC4',
      sub_types: [{ key: 'house_self', label: '房产（自住）', icon: 'House', is_fixed: true }],
    },
    {
      key: 'liability',
      label: '负债',
      kind: 'liability',
      color: '#B9B4C7',
      sub_types: [{ key: 'mortgage', label: '房屋贷款', icon: 'Landmark', is_fixed: false }],
    },
  ],
  risk_types: [
    { key: 'cash', label: '活钱' },
    { key: 'none', label: '不标记' },
  ],
  currencies: [{ key: 'CNY', label: '人民币', symbol: '¥' }],
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

const recentItems = {
  items: [
    {
      id: 1,
      name: '招行活期',
      category: 'liquid',
      sub_type: 'bank_current',
      amount_cents: 1_000_000,
      currency: 'CNY',
      risk_type: 'cash',
      member_id: null,
      member_name: '',
      note: '',
      version: 1,
      updated_at: '2026-10-01T10:00:00Z',
    },
  ],
  total: 1,
  sum_cents: 1_000_000,
  category_counts: { liquid: 1 },
}

function buildRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/ledger', name: 'ledger-dashboard', component: LedgerDashboard },
      { path: '/ledger/items', name: 'ledger-items', component: { template: '<div />' } },
      { path: '/ledger/calendar', name: 'ledger-calendar', component: { template: '<div />' } },
      { path: '/ledger/composition', name: 'ledger-composition', component: { template: '<div />' } },
    ],
  })
}

async function mountView() {
  const router = buildRouter()
  router.push('/ledger')
  await router.isReady()
  const wrapper = mount(LedgerDashboard, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  resetLedgerMetaCache()
  useLedgerPrivacy().show()
  sessionStorage.clear()
  getMeta.mockReset()
  getConsent.mockReset()
  acceptConsent.mockReset()
  getSummary.mockReset()
  listItems.mockReset()
  listMembers.mockReset()
  createItem.mockReset()
  createMember.mockReset()

  getMeta.mockResolvedValue(meta)
  getConsent.mockResolvedValue({ policy_version: 'v1', required: false, consented_at: '2026-10-01T00:00:00Z' })
  listMembers.mockResolvedValue([])
  getSummary.mockResolvedValue(summary)
  listItems.mockResolvedValue(recentItems)
  createItem.mockResolvedValue({ id: 9 })
})

describe('LedgerDashboard.vue 总览', () => {
  it('渲染总资产、净资产与分类结构', async () => {
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="ledger-total"]').text()).toBe('30,000.00')
    expect(wrapper.text()).toContain('25,000.00')
    expect(wrapper.text()).toContain('16.67%')
    expect(wrapper.find('[data-test="structure-liquid"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="structure-fixed"]').exists()).toBe(true)
  })

  it('未同意时弹出告知并跳过数据请求', async () => {
    getConsent.mockResolvedValue({ policy_version: 'v1', required: true, consented_at: null })
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="consent-banner"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="consent-check"]').exists()).toBe(true)
    expect(getSummary).not.toHaveBeenCalled()
    // 未勾选时按钮禁用
    expect(wrapper.find('[data-test="consent-accept"]').attributes('disabled')).toBeDefined()
  })

  it('同意后记录留痕并加载总览', async () => {
    getConsent.mockResolvedValue({ policy_version: 'v1', required: true, consented_at: null })
    acceptConsent.mockResolvedValue({ policy_version: 'v1', required: false, consented_at: '2026-10-23T00:00:00Z' })
    const wrapper = await mountView()

    await wrapper.find('[data-test="consent-check"]').setValue(true)
    await wrapper.find('[data-test="consent-accept"]').trigger('click')
    await flushPromises()

    expect(acceptConsent).toHaveBeenCalledWith('v1')
    expect(getSummary).toHaveBeenCalled()
    expect(wrapper.find('[data-test="ledger-total"]').text()).toBe('30,000.00')
  })

  it('眼睛按钮隐藏全部金额', async () => {
    const wrapper = await mountView()
    await wrapper.find('[data-test="ledger-eye"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="ledger-total"]').text()).toBe(MONEY_MASK)
    expect(wrapper.find('[data-test="month-total-asset"]').text()).toBe(MONEY_MASK)
  })

  it('排除固定资产后总资产与负债率同步重算', async () => {
    const wrapper = await mountView()
    await wrapper.find('[data-test="exclude-fixed"]').trigger('click')
    await flushPromises()
    // 3,000,000 - 2,000,000 = 1,000,000 分 => 10,000.00 元
    expect(wrapper.find('[data-test="ledger-total"]').text()).toBe('10,000.00')
    expect(wrapper.text()).toContain('50.00%')
    expect(wrapper.find('[data-test="structure-fixed"]').exists()).toBe(false)
  })

  it('添加资产：填写表单后提交并刷新总览', async () => {
    const wrapper = await mountView()
    await wrapper.find('[data-test="open-add-asset"]').trigger('click')
    await flushPromises()

    await wrapper.find('input[placeholder="如：招商银行活期"]').setValue('招行活期')
    await wrapper.find('input[placeholder="点击输入金额"]').setValue('1200')
    await wrapper.find('[data-test="ledger-submit"]').trigger('click')
    await flushPromises()

    expect(createItem).toHaveBeenCalledWith(
      expect.objectContaining({ name: '招行活期', amount_cents: 120000, category: 'liquid', currency: 'CNY' }),
    )
    expect(getSummary).toHaveBeenCalledTimes(2)
  })

  it('最近更新展示条目名称与金额', async () => {
    const wrapper = await mountView()
    expect(wrapper.text()).toContain('招行活期')
    expect(wrapper.text()).toContain('10,000.00')
  })

  it('加载失败时展示错误提示', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    getSummary.mockRejectedValue({ response: { data: { message: '账本服务暂不可用' } } })
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="dashboard-error"]').text()).toContain('账本服务暂不可用')
    errorSpy.mockRestore()
  })
})
