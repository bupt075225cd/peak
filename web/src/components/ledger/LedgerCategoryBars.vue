<script setup lang="ts">
import { computed } from 'vue'
import { TrendingDown, TrendingUp } from 'lucide-vue-next'
import type { LedgerSummaryGroup } from '../../api/ledger'
import { formatCents, MONEY_MASK } from '../../utils/money'

const props = defineProps<{
  groups: LedgerSummaryGroup[]
  hidden: boolean
}>()

// 柱高按金额归一：最高柱固定 132px，其余等比缩放（最小 6px 保证可见）。
const CHART_HEIGHT = 132

const bars = computed(() => {
  const max = Math.max(...props.groups.map((g) => g.amount_cents), 1)
  return props.groups.map((g) => ({
    ...g,
    height: g.amount_cents > 0 ? Math.max(6, Math.round((g.amount_cents / max) * CHART_HEIGHT)) : 4,
  }))
})
</script>

<template>
  <div class="flex items-end justify-between gap-3" :style="{ height: `${CHART_HEIGHT + 64}px` }">
    <div v-for="bar in bars" :key="bar.category" class="flex flex-1 flex-col items-center justify-end gap-1.5">
      <div class="flex h-11 flex-col items-center justify-end gap-0.5">
        <component
          :is="bar.kind === 'liability' ? TrendingDown : TrendingUp"
          class="h-3 w-3"
          :class="bar.kind === 'liability' ? 'text-ledger-down' : 'text-ledger-up'"
        />
        <span class="whitespace-nowrap text-[11px] font-medium text-ink-soft tabular-nums">
          {{ hidden ? MONEY_MASK : formatCents(bar.amount_cents) }}
        </span>
      </div>
      <div
        class="w-7 rounded-t-md transition-all duration-500 ease-out"
        :class="bar.amount_cents > 0 ? '' : 'opacity-30'"
        :style="{ height: `${bar.height}px`, backgroundColor: bar.color }"
        :title="`${bar.label} ${bar.count} 项`"
      />
      <span class="whitespace-nowrap text-[11px] text-ink-faint">{{ bar.label }}</span>
    </div>
  </div>
</template>
