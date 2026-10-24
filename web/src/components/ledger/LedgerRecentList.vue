<script setup lang="ts">
import { RouterLink } from 'vue-router'
import type { LedgerItem } from '../../api/ledger'
import { useLedgerPrivacy } from '../../composables/useLedgerPrivacy'
import { formatDate } from '../../utils/money'

defineProps<{
  items: LedgerItem[]
}>()

const { money } = useLedgerPrivacy()
</script>

<template>
  <div class="rounded-2xl border border-slate-200/70 bg-white p-2">
    <p v-if="!items.length" class="px-3 py-8 text-center text-xs text-ink-faint">还没有记录，先添加第一笔资产</p>
    <RouterLink
      v-for="item in items"
      :key="item.id"
      :to="`/ledger/items?category=${item.category}`"
      class="flex items-center justify-between gap-2 rounded-xl px-3 py-2.5 transition-colors hover:bg-slate-50"
    >
      <div class="min-w-0">
        <p class="truncate text-sm text-ink">{{ item.name }}</p>
        <p class="text-[11px] text-ink-faint">{{ formatDate(item.updated_at) }}</p>
      </div>
      <span class="whitespace-nowrap text-sm font-medium tabular-nums text-ink">{{ money(item.amount_cents) }}</span>
    </RouterLink>
  </div>
</template>
