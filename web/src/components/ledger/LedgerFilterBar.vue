<script setup lang="ts">
import { Search } from 'lucide-vue-next'
import type { LedgerCategory, LedgerMember, LedgerMeta } from '../../api/ledger'
import type { LedgerSort } from '../../composables/useLedgerItems'

defineProps<{
  meta: LedgerMeta | null
  members: LedgerMember[]
  category: string
  keyword: string
  memberFilter: string
  sort: LedgerSort
  counts: Partial<Record<LedgerCategory, number>>
}>()

const emit = defineEmits<{
  (e: 'update:category', value: string): void
  (e: 'update:keyword', value: string): void
  (e: 'update:memberFilter', value: string): void
  (e: 'update:sort', value: LedgerSort): void
}>()

const SORTS: { key: LedgerSort; label: string }[] = [
  { key: 'default', label: '按更新时间' },
  { key: 'amount_desc', label: '金额从高到低' },
  { key: 'amount_asc', label: '金额从低到高' },
]
</script>

<template>
  <div>
    <div class="flex flex-wrap items-center gap-3 rounded-2xl border border-slate-200/70 bg-white px-4 py-3">
      <div class="flex min-w-56 flex-1 items-center gap-2 rounded-xl bg-slate-50 px-3 py-2">
        <Search class="h-4 w-4 shrink-0 text-ink-faint" />
        <input
          :value="keyword"
          type="search"
          placeholder="搜索资产名称或备注"
          class="w-full bg-transparent text-sm text-ink outline-none placeholder:text-ink-faint"
          data-test="keyword"
          @input="emit('update:keyword', ($event.target as HTMLInputElement).value)"
        />
      </div>
      <select
        :value="memberFilter"
        class="rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm text-ink-soft outline-none cursor-pointer"
        data-test="member-filter"
        @change="emit('update:memberFilter', ($event.target as HTMLSelectElement).value)"
      >
        <option value="">全部成员</option>
        <option value="self">我</option>
        <option v-for="member in members" :key="member.id" :value="String(member.id)">{{ member.name }}</option>
      </select>
      <select
        :value="sort"
        class="rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm text-ink-soft outline-none cursor-pointer"
        data-test="sort"
        @change="emit('update:sort', ($event.target as HTMLSelectElement).value as LedgerSort)"
      >
        <option v-for="item in SORTS" :key="item.key" :value="item.key">{{ item.label }}</option>
      </select>
    </div>

    <div class="mt-3 flex flex-wrap items-center gap-2">
      <button
        type="button"
        class="rounded-full px-3 py-1.5 text-xs transition-colors cursor-pointer"
        :class="category === '' ? 'bg-ledger-ink text-white' : 'bg-white text-ink-soft ring-1 ring-slate-200 hover:text-ledger-ink'"
        data-test="filter-all"
        @click="emit('update:category', '')"
      >
        全部
      </button>
      <button
        v-for="group in meta?.categories ?? []"
        :key="group.key"
        type="button"
        class="inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs transition-colors cursor-pointer"
        :class="category === group.key ? 'bg-ledger-ink text-white' : 'bg-white text-ink-soft ring-1 ring-slate-200 hover:text-ledger-ink'"
        :data-test="`filter-${group.key}`"
        @click="emit('update:category', group.key)"
      >
        <span class="h-1.5 w-1.5 rounded-full" :style="{ backgroundColor: group.color }" />
        {{ group.label }}
        <span v-if="counts[group.key]" class="tabular-nums opacity-70">{{ counts[group.key] }}</span>
      </button>
    </div>
  </div>
</template>
