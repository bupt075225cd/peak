import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import LedgerDashboard from './LedgerDashboard.vue'
import { resetLedgerMetaCache } from '../composables/useLedgerMeta'
import { useLedgerPrivacy } from '../composables/useLedgerPrivacy'
import { MONEY_MASK } from '../utils/money'

const { getMeta, getConsent, acceptConsent, getSummary, listItems, listMembers, createItem, createMember, updateMember, deleteMember } =
  vi.hoisted(() => ({
    getMeta: vi.fn(),
    getConsent: vi.fn(),
    acceptConsent: vi.fn(),
    getSummary: vi.fn(),
    listItems: vi.fn(),
    listMembers: vi.fn(),
    createItem: vi.fn(),
    createMember: vi.fn(),
    updateMember: vi.fn(),
    deleteMember: vi.fn(),
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
    updateMember,
    deleteMember,
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
  updateMember.mockReset()
  deleteMember.mockReset()

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
    expect(wrapper.find('[data-test="ledger-net"]').text()).toBe(MONEY_MASK)
  })

  it('排除固定资产后标题改为资产汇总、金额扣除固定资产，净资产与负债率不变', async () => {
    const wrapper = await mountView()
    const title = () => wrapper.find('[data-test="ledger-total-title"]').text()
    expect(title()).toBe('总资产（元）')

    await wrapper.find('[data-test="exclude-fixed"]').trigger('click')
    await flushPromises()

    // 标题切换为「资产汇总」，金额 = 3,000,000 - 2,000,000 = 1,000,000 分 => 10,000.00 元
    expect(title()).toBe('资产汇总')
    expect(wrapper.find('[data-test="ledger-total"]').text()).toBe('10,000.00')
    // 净资产（2,500,000 分）与负债率（500,000 / 3,000,000）仍按全部资产口径，不随开关变化。
    expect(wrapper.find('[data-test="ledger-net"]').text()).toBe('25,000.00')
    expect(wrapper.text()).toContain('16.67%')
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

  it('不再展示资产月历与最近更新', async () => {
    const wrapper = await mountView()
    expect(wrapper.text()).not.toContain('资产月历')
    expect(wrapper.text()).not.toContain('最近更新')
    expect(wrapper.find('[data-test="month-total-asset"]').exists()).toBe(false)
    // 总览只拉取一次汇总，不再为最近列表多发一次条目请求。
    expect(listItems).not.toHaveBeenCalled()
  })

  it('成员切换条展示可见成员，点击成员按 member_id 重新聚合', async () => {
    listMembers.mockResolvedValue([
      { id: 3, name: '配偶', sort_order: 1, visible: true },
      { id: 4, name: '隐藏成员', sort_order: 2, visible: false },
    ])
    const wrapper = await mountView()

    expect(wrapper.find('[data-test="member-scope-all"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="member-scope-self"]').text()).toBe('我')
    expect(wrapper.find('[data-test="member-scope-3"]').text()).toBe('配偶')
    // 关闭「在记账中显示」的成员不出现在切换条。
    expect(wrapper.find('[data-test="member-scope-4"]').exists()).toBe(false)

    await wrapper.find('[data-test="member-scope-3"]').trigger('click')
    await flushPromises()
    expect(getSummary).toHaveBeenLastCalledWith('3')

    await wrapper.find('[data-test="member-scope-self"]').trigger('click')
    await flushPromises()
    expect(getSummary).toHaveBeenLastCalledWith('self')
  })

  it('管理入口打开家庭成员对话框，可添加成员', async () => {
    listMembers.mockResolvedValue([{ id: 3, name: '配偶', sort_order: 1, visible: true }])
    createMember.mockResolvedValue({ id: 5, name: '孩子', sort_order: 2, visible: true })
    const wrapper = await mountView()

    await wrapper.find('[data-test="manage-members"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="member-manager"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="manager-member-3"]').text()).toContain('配偶')

    await wrapper.find('[data-test="manager-add"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="member-form-title"]').text()).toBe('添加成员')

    await wrapper.find('[data-test="member-name"]').setValue('孩子')
    await wrapper.find('[data-test="member-submit"]').trigger('click')
    await flushPromises()
    expect(createMember).toHaveBeenCalledWith('孩子', true)
  })

  it('关闭成员显示开关后该成员从切换条移除并回落「全部」', async () => {
    listMembers.mockResolvedValue([{ id: 3, name: '配偶', sort_order: 1, visible: true }])
    updateMember.mockResolvedValue({ id: 3, name: '配偶', sort_order: 1, visible: false })
    const wrapper = await mountView()

    await wrapper.find('[data-test="member-scope-3"]').trigger('click')
    await flushPromises()

    await wrapper.find('[data-test="manage-members"]').trigger('click')
    await wrapper.find('[data-test="manager-member-3"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-test="member-visible"]').trigger('click')

    listMembers.mockResolvedValue([{ id: 3, name: '配偶', sort_order: 1, visible: false }])
    await wrapper.find('[data-test="member-submit"]').trigger('click')
    await flushPromises()

    expect(updateMember).toHaveBeenCalledWith(3, { name: '配偶', visible: false })
    expect(wrapper.find('[data-test="member-scope-3"]').exists()).toBe(false)
    expect(getSummary).toHaveBeenLastCalledWith('')
  })

  it('加载失败时展示错误提示', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    getSummary.mockRejectedValue({ response: { data: { message: '账本服务暂不可用' } } })
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="dashboard-error"]').text()).toContain('账本服务暂不可用')
    errorSpy.mockRestore()
  })
})
