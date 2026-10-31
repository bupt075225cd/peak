import { ref } from 'vue'
import { errorMessage, getSummary, listItems, type LedgerCategory, type LedgerItem } from '../api/ledger'

// 更新流程需要一次性看到某一分类下的全部条目（按成员分组展示），
// 服务端单页上限 100，这里翻页取全量并设上限兜底，避免异常数据拖垮页面。
const PAGE_SIZE = 100
const MAX_ITEMS = 2000

/** 同一分类下按成员聚合的一组条目。 */
export interface UpdateMemberGroup {
  /** 'self' 或成员 ID 字符串。 */
  key: string
  name: string
  items: LedgerItem[]
}

/**
 * 更新资产流程的数据层：分类合计 + 全量条目。
 *
 * 合计取自服务端聚合（与总览口径一致），条目用于按成员分组展示与逐条修改。
 */
export function useLedgerUpdate() {
  const items = ref<LedgerItem[]>([])
  const totals = ref<Partial<Record<LedgerCategory, number>>>({})
  const counts = ref<Partial<Record<LedgerCategory, number>>>({})
  const loading = ref(false)
  const error = ref('')

  async function loadAllItems(): Promise<LedgerItem[]> {
    const collected: LedgerItem[] = []
    let offset = 0
    while (collected.length < MAX_ITEMS) {
      const page = await listItems({ offset, limit: PAGE_SIZE })
      collected.push(...page.items)
      offset += page.items.length
      // 空页或已取满总数即停止，避免服务端 total 与实际不一致时死循环。
      if (!page.items.length || offset >= page.total) break
    }
    return collected
  }

  async function load() {
    loading.value = true
    error.value = ''
    try {
      const [summary, list] = await Promise.all([getSummary(), loadAllItems()])
      items.value = list
      totals.value = Object.fromEntries(summary.groups.map((g) => [g.category, g.amount_cents]))
      counts.value = Object.fromEntries(summary.groups.map((g) => [g.category, g.count]))
    } catch (err) {
      error.value = errorMessage(err, '加载资产失败，请稍后重试')
    } finally {
      loading.value = false
    }
  }

  function itemsOf(category: LedgerCategory): LedgerItem[] {
    return items.value.filter((item) => item.category === category)
  }

  /** 按成员分组：未归属的条目归到「我」，其余按成员 ID 升序。 */
  function groupsOf(category: LedgerCategory): UpdateMemberGroup[] {
    const map = new Map<string, UpdateMemberGroup>()
    for (const item of itemsOf(category)) {
      const key = item.member_id == null ? 'self' : String(item.member_id)
      let group = map.get(key)
      if (!group) {
        group = { key, name: item.member_id == null ? '我' : item.member_name || '成员', items: [] }
        map.set(key, group)
      }
      group.items.push(item)
    }
    const groups = [...map.values()]
    groups.sort((a, b) => {
      if (a.key === 'self') return -1
      if (b.key === 'self') return 1
      return Number(a.key) - Number(b.key)
    })
    for (const group of groups) {
      group.items.sort((a, b) => (a.updated_at < b.updated_at ? 1 : -1))
    }
    return groups
  }

  return { items, totals, counts, loading, error, load, itemsOf, groupsOf }
}
