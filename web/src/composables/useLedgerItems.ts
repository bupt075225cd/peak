import { computed, ref, watch } from 'vue'
import { errorMessage, listItems, type LedgerCategory, type LedgerItem } from '../api/ledger'

const PAGE_SIZE = 10
// 成员筛选与排序不在服务端接口内，此时一次取回较大结果集在内存里处理。
const CLIENT_LIMIT = 500

export type LedgerSort = 'default' | 'amount_desc' | 'amount_asc'

/**
 * 资产明细列表状态：分类与关键词由服务端筛选分页；
 * 选择成员或切换排序时改为取回结果集后在内存中过滤、排序与分页。
 */
export function useLedgerItems(initialCategory = '') {
  const category = ref(initialCategory)
  const keyword = ref('')
  const memberFilter = ref('')
  const sort = ref<LedgerSort>('default')
  const page = ref(1)
  const items = ref<LedgerItem[]>([])
  const total = ref(0)
  const sumCents = ref(0)
  const counts = ref<Partial<Record<LedgerCategory, number>>>({})
  const loading = ref(false)
  const error = ref('')
  const enabled = ref(false)

  const clientMode = computed(() => memberFilter.value !== '' || sort.value !== 'default')
  const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))

  async function load() {
    if (!enabled.value) return
    loading.value = true
    error.value = ''
    try {
      if (clientMode.value) await loadClientSide()
      else await loadServerSide()
    } catch (err) {
      error.value = errorMessage(err, '加载资产明细失败')
    } finally {
      loading.value = false
    }
  }

  async function loadServerSide() {
    const result = await listItems({
      category: category.value || undefined,
      keyword: keyword.value,
      offset: (page.value - 1) * PAGE_SIZE,
      limit: PAGE_SIZE,
    })
    items.value = result.items
    total.value = result.total
    sumCents.value = result.sum_cents
    counts.value = result.category_counts
  }

  async function loadClientSide() {
    const result = await listItems({
      category: category.value || undefined,
      keyword: keyword.value,
      offset: 0,
      limit: CLIENT_LIMIT,
    })
    let list = result.items
    if (memberFilter.value === 'self') list = list.filter((item) => !item.member_id)
    else if (memberFilter.value) list = list.filter((item) => String(item.member_id) === memberFilter.value)
    if (sort.value !== 'default') {
      const sorted = [...list].sort((a, b) => b.amount_cents - a.amount_cents)
      list = sort.value === 'amount_desc' ? sorted : sorted.reverse()
    }
    total.value = list.length
    counts.value = result.category_counts
    sumCents.value = list.reduce((sum, item) => sum + item.amount_cents, 0)
    const start = (page.value - 1) * PAGE_SIZE
    items.value = list.slice(start, start + PAGE_SIZE)
  }

  /** 同意后首次加载：开启后筛选变化才会自动请求。 */
  async function start() {
    enabled.value = true
    await load()
  }

  async function reload() {
    page.value = 1
    await load()
  }

  watch([category, memberFilter, sort], async () => {
    page.value = 1
    await load()
  })

  watch(page, async () => {
    await load()
  })

  let keywordTimer: ReturnType<typeof setTimeout> | undefined
  watch(keyword, () => {
    if (keywordTimer) clearTimeout(keywordTimer)
    keywordTimer = setTimeout(() => {
      page.value = 1
      load()
    }, 300)
  })

  /** 删除最后一页的唯一一条时回退一页，避免停留在空页。 */
  async function reloadAfterRemove(removedId: number) {
    if (items.value.length === 1 && page.value > 1) {
      page.value -= 1
      return
    }
    items.value = items.value.filter((item) => item.id !== removedId)
    await load()
  }

  return {
    pageSize: PAGE_SIZE,
    category,
    keyword,
    memberFilter,
    sort,
    page,
    items,
    total,
    sumCents,
    counts,
    loading,
    error,
    clientMode,
    pageCount,
    start,
    load,
    reload,
    reloadAfterRemove,
  }
}
