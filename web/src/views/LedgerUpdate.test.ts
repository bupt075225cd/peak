import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import LedgerUpdate from './LedgerUpdate.vue'
import { resetLedgerMetaCache } from '../composables/useLedgerMeta'

const { getMeta, getConsent, acceptConsent, getSummary, listItems, listMembers, createItem, updateItem, createMember } =
  vi.hoisted(() => ({
    getMeta: vi.fn(),
    getConsent: vi.fn(),
    acceptConsent: vi.fn(),
    getSummary: vi.fn(),
    listItems: vi.fn(),
    listMembers: vi.fn(),
    createItem: vi.fn(),
    updateItem: vi.fn(),
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
    updateItem,
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
      description: '可随用随取、即时变现的钱。',
      sub_types: [
        { key: 'bank_current', label: '银行活期', icon: 'Landmark', is_fixed: false },
        { key: 'liquid_custom', label: '自定义流动资金', icon: 'CircleEllipsis', is_fixed: false },
      ],
    },
    {
      key: 'fixed',
      label: '固定资产',
      kind: 'asset',
      color: '#9DBCC4',
      description: '用于投资或自用的、流动性低的实物类资产。',
      sub_types: [{ key: 'house_self', label: '房产（自住）', icon: 'House', is_fixed: true }],
    },
    {
      key: 'investment',
      label: '投资理财',
      kind: 'asset',
      color: '#A9A3D4',
      description: '投资于金融产品、追求保值增值的钱。',
      sub_types: [{ key: 'fund', label: '基金', icon: 'LineChart', is_fixed: false }],
    },
    {
      key: 'receivable',
      label: '应收款',
      kind: 'asset',
      color: '#63C6A5',
      description: '家庭资产中应收未收的款项。',
      sub_types: [{ key: 'loan_out', label: '借出款', icon: 'HandCoins', is_fixed: false }],
    },
    {
      key: 'liability',
      label: '负债',
      kind: 'liability',
      color: '#B9B4C7',
      description: '家庭需要偿还的债务与欠款。',
      sub_types: [{ key: 'credit_card', label: '信用卡', icon: 'CreditCard', is_fixed: false }],
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
    { category: 'liquid', label: '流动资金', kind: 'asset', color: '#E9A66B', amount_cents: 220_000_00, count: 1, children: [] },
    { category: 'fixed', label: '固定资产', kind: 'asset', color: '#9DBCC4', amount_cents: 0, count: 0, children: [] },
    { category: 'investment', label: '投资理财', kind: 'asset', color: '#A9A3D4', amount_cents: 0, count: 0, children: [] },
    { category: 'receivable', label: '应收款', kind: 'asset', color: '#63C6A5', amount_cents: 0, count: 0, children: [] },
    { category: 'liability', label: '负债', kind: 'liability', color: '#B9B4C7', amount_cents: 0, count: 0, children: [] },
  ],
  total_asset_cents: 220_000_00,
  total_liability_cents: 0,
  net_asset_cents: 220_000_00,
  updated_at: '2026-02-28T10:00:00Z',
}

const bankItem = {
  id: 7,
  name: '银行活期',
  category: 'liquid',
  sub_type: 'bank_current',
  amount_cents: 220_000_00,
  currency: 'CNY',
  risk_type: 'cash',
  member_id: null,
  member_name: null,
  note: '',
  version: 1,
  updated_at: '2026-02-28T10:00:00Z',
}

const selfItems = {
  items: [bankItem],
  total: 1,
  sum_cents: 220_000_00,
  category_counts: { liquid: 1 },
}

function buildRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/ledger', name: 'ledger-dashboard', component: { template: '<div />' } },
      { path: '/ledger/update', name: 'ledger-update', component: LedgerUpdate },
    ],
  })
}

async function mountView() {
  const router = buildRouter()
  router.push('/ledger/update')
  await router.isReady()
  const wrapper = mount(LedgerUpdate, { global: { plugins: [router] } })
  await flushPromises()
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  resetLedgerMetaCache()
  getMeta.mockReset()
  getConsent.mockReset()
  acceptConsent.mockReset()
  getSummary.mockReset()
  listItems.mockReset()
  listMembers.mockReset()
  createItem.mockReset()
  updateItem.mockReset()
  createMember.mockReset()

  getMeta.mockResolvedValue(meta)
  getConsent.mockResolvedValue({ policy_version: 'v1', required: false, consented_at: '2026-02-01T00:00:00Z' })
  getSummary.mockResolvedValue(summary)
  listItems.mockResolvedValue(selfItems)
  listMembers.mockResolvedValue([])
})

describe('LedgerUpdate.vue 更新资产', () => {
  it('按分类渲染步进卡，空分类显示「待填写」', async () => {
    const { wrapper } = await mountView()

    expect(wrapper.text()).toContain('更新家庭资产')
    expect(wrapper.find('[data-test="update-step-liquid"]').text()).toContain('220,000.00 元')
    expect(wrapper.find('[data-test="update-step-fixed"]').text()).toContain('待填写')
    expect(wrapper.find('[data-test="update-step-liability"]').exists()).toBe(true)
  })

  it('分类说明可展开，切换分类会收起', async () => {
    const { wrapper } = await mountView()

    expect(wrapper.find('[data-test="update-hint"]').exists()).toBe(false)
    await wrapper.find('[data-test="update-hint-toggle"]').trigger('click')
    expect(wrapper.find('[data-test="update-hint"]').text()).toContain('可随用随取')

    await wrapper.find('[data-test="update-step-fixed"]').trigger('click')
    expect(wrapper.find('[data-test="update-hint"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="update-hint-toggle"]').text()).toContain('什么是固定资产')
  })

  it('按成员分组展示条目，负债显示负号', async () => {
    listItems.mockResolvedValue({
      items: [
        bankItem,
        { ...bankItem, id: 8, name: '配偶活期', member_id: 3, member_name: '配偶', amount_cents: 50_000_00 },
        {
          ...bankItem,
          id: 9,
          name: '信用卡',
          category: 'liability',
          sub_type: 'credit_card',
          amount_cents: 12_000_00,
        },
      ],
      total: 3,
      sum_cents: 0,
      category_counts: {},
    })
    const { wrapper } = await mountView()

    expect(wrapper.find('[data-test="update-group-self"]').text()).toContain('我')
    expect(wrapper.find('[data-test="update-group-self"]').text()).toContain('¥ 220,000.00')
    expect(wrapper.find('[data-test="update-group-self"]').text()).toContain('02月28日')
    expect(wrapper.find('[data-test="update-group-3"]').text()).toContain('配偶')

    await wrapper.find('[data-test="update-step-liability"]').trigger('click')
    expect(wrapper.find('[data-test="update-item-9"]').text()).toContain('¥ -12,000.00')
  })

  it('更多{分类}打开子类弹层，选子类后录入并提交', async () => {
    createItem.mockResolvedValue({ ...bankItem, id: 10 })
    const { wrapper } = await mountView()

    await wrapper.find('[data-test="update-more"]').trigger('click')
    expect(wrapper.find('[data-test="subtype-sheet"]').exists()).toBe(true)
    await wrapper.find('[data-test="subtype-bank_current"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="asset-sheet"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="asset-title"]').text()).toBe('添加银行活期')
    // 非自定义子类预填名称。
    expect((wrapper.find('[data-test="asset-name"]').element as HTMLInputElement).value).toBe('银行活期')

    await wrapper.find('[data-test="asset-amount"]').setValue('1200.50')
    await wrapper.find('[data-test="asset-risk-cash"]').trigger('click')
    await wrapper.find('[data-test="asset-submit"]').trigger('click')
    await flushPromises()

    expect(createItem).toHaveBeenCalledWith(
      expect.objectContaining({
        name: '银行活期',
        category: 'liquid',
        sub_type: 'bank_current',
        amount_cents: 120050,
        currency: 'CNY',
        risk_type: 'cash',
        member_id: null,
      }),
    )
    expect(wrapper.find('[data-test="asset-sheet"]').exists()).toBe(false)
  })

  it('点击条目进入编辑并按 version 提交更新', async () => {
    updateItem.mockResolvedValue({ ...bankItem, amount_cents: 300_000_00 })
    const { wrapper } = await mountView()

    await wrapper.find('[data-test="update-item-7"]').trigger('click')
    expect(wrapper.find('[data-test="asset-title"]').text()).toBe('编辑银行活期')

    await wrapper.find('[data-test="asset-amount"]').setValue('300000')
    await wrapper.find('[data-test="asset-submit"]').trigger('click')
    await flushPromises()

    expect(updateItem).toHaveBeenCalledWith(7, expect.objectContaining({ amount_cents: 300_000_00 }), 1)
  })

  it('下一步切到下一分类，最后一类回到总览', async () => {
    const { wrapper, router } = await mountView()

    await wrapper.find('[data-test="update-next"]').trigger('click')
    expect(wrapper.find('[data-test="update-step-fixed"]').attributes('aria-current')).toBe('step')

    for (let i = 0; i < 3; i += 1) {
      await wrapper.find('[data-test="update-next"]').trigger('click')
    }
    expect(wrapper.find('[data-test="update-step-liability"]').attributes('aria-current')).toBe('step')

    await wrapper.find('[data-test="update-next"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/ledger')
  })
})
