<script setup lang="ts">
import { RouterLink } from 'vue-router'
import type { LedgerSummaryGroup } from '../../api/ledger'
import { useLedgerPrivacy } from '../../composables/useLedgerPrivacy'
import { formatPercent, MONEY_MASK, share } from '../../utils/money'

const props = defineProps<{
  groups: LedgerSummaryGroup[]
  totalAssetCents: number
}>()

const { hidden, money } = useLedgerPrivacy()

function percentText(part: number): string {
  if (hidden.value) return MONEY_MASK
  return formatPercent(share(part, props.totalAssetCents))
}
</script>

<template>
  <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
    <RouterLink
      v-for="group in groups"
      :key="group.category"
      :to="{ path: '/ledger/items', query: { category: group.category } }"
      class="group rounded-2xl border border-slate-200/70 bg-white p-4 transition-all hover:-translate-y-0.5 hover:border-slate-300 hover:shadow-md"
      :data-test="`structure-${group.category}`"
    >
      <div class="flex items-center justify-between">
        <span class="flex items-center gap-1.5 text-xs text-ink-soft">
          <span class="h-2 w-2 rounded-full" :style="{ backgroundColor: group.color }" />
          {{ group.label }}
        </span>
        <span class="text-[11px] text-ink-faint">{{ group.count }} 项</span>
      </div>
      <p class="mt-2 text-lg font-semibold tabular-nums text-ink">{{ money(group.amount_cents) }}</p>
      <div class="mt-3 h-1.5 overflow-hidden rounded-full bg-slate-100">
        <div
          class="h-full rounded-full transition-all duration-500"
          :style="{ width: `${Math.max(2, share(group.amount_cents, totalAssetCents) * 100)}%`, backgroundColor: group.color }"
        />
      </div>
      <p class="mt-1.5 text-[11px] text-ink-faint">{{ percentText(group.amount_cents) }}</p>
    </RouterLink>
  </div>
</template>
