<script setup lang="ts">
import { computed } from 'vue'
import { ArrowDownRight, ArrowUpRight, Minus } from 'lucide-vue-next'
import type { LedgerMeta, LedgerSnapshot } from '../../api/ledger'
import { useLedgerPrivacy } from '../../composables/useLedgerPrivacy'
import { deltaPercent, formatMonthLabel, formatPercent, MONEY_MASK } from '../../utils/money'

const props = defineProps<{
  months: { period: string; snapshot: LedgerSnapshot | null }[]
  meta: LedgerMeta | null
  selected: string
  currentPeriod: string
}>()

const emit = defineEmits<{
  (e: 'select', period: string): void
}>()

const { hidden, money } = useLedgerPrivacy()

function colorOf(category: string): string {
  return props.meta?.categories.find((c) => c.key === category)?.color ?? '#CBD5E1'
}

// 迷你条按当月最大分类归一：只体现结构，不暴露绝对数值。
function barsOf(snapshot: LedgerSnapshot | null) {
  if (!snapshot) return []
  const rows = [...snapshot.payload.breakdown].sort((a, b) => b.amount_cents - a.amount_cents).slice(0, 4)
  const max = Math.max(...rows.map((r) => r.amount_cents), 1)
  return rows.map((row) => ({
    category: row.category,
    height: Math.max(4, Math.round((row.amount_cents / max) * 26)),
  }))
}

const cells = computed(() =>
  props.months.map((month, index) => {
    const previous = index > 0 ? props.months[index - 1]?.snapshot : null
    const delta = month.snapshot && previous ? deltaPercent(month.snapshot.payload.net_asset_cents, previous.payload.net_asset_cents) : null
    return {
      period: month.period,
      label: formatMonthLabel(month.period),
      net: month.snapshot ? month.snapshot.payload.net_asset_cents : null,
      delta,
      bars: barsOf(month.snapshot),
    }
  }),
)

function deltaText(value: number | null): string {
  if (value === null) return '—'
  if (hidden.value) return MONEY_MASK
  const sign = value > 0 ? '+' : ''
  return `${sign}${formatPercent(value)}`
}

function deltaClass(value: number | null): string {
  if (value === null || value === 0) return 'text-ink-faint'
  return value > 0 ? 'text-ledger-up' : 'text-ledger-down'
}
</script>

<template>
  <div class="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-4">
    <button
      v-for="cell in cells"
      :key="cell.period"
      type="button"
      class="rounded-2xl border bg-white p-4 text-left transition-all cursor-pointer"
      :class="[
        cell.period === selected ? 'border-ledger-ink shadow-md' : 'border-slate-200/70 hover:border-slate-300 hover:shadow-sm',
        cell.period === currentPeriod ? 'ring-1 ring-ledger-ink/15' : '',
      ]"
      :data-test="`month-${cell.period}`"
      @click="emit('select', cell.period)"
    >
      <div class="flex items-center justify-between">
        <span class="text-xs font-medium text-ink-soft">{{ cell.label }}</span>
        <span v-if="cell.period === currentPeriod" class="rounded-full bg-ledger-mist px-1.5 py-0.5 text-[10px] text-ledger-ink">当月</span>
      </div>

      <p class="mt-2 text-base font-semibold tabular-nums text-ink">
        {{ cell.net === null ? '—' : money(cell.net) }}
      </p>

      <p class="mt-1 flex items-center gap-0.5 text-[11px]" :class="deltaClass(cell.delta)">
        <template v-if="cell.delta !== null">
          <ArrowUpRight v-if="cell.delta > 0" class="h-3 w-3" />
          <ArrowDownRight v-else-if="cell.delta < 0" class="h-3 w-3" />
          <Minus v-else class="h-3 w-3" />
          {{ deltaText(cell.delta) }}
          <span class="ml-0.5 text-ink-faint">环比</span>
        </template>
        <span v-else class="text-ink-faint">{{ cell.net === null ? '未生成快照' : '无上期数据' }}</span>
      </p>

      <div class="mt-3 flex h-8 items-end gap-1">
        <span
          v-for="bar in cell.bars"
          :key="bar.category"
          class="flex-1 rounded-sm opacity-80"
          :style="{ height: `${bar.height}px`, backgroundColor: colorOf(bar.category) }"
        />
        <span v-if="!cell.bars.length" class="text-[10px] text-ink-faint">—</span>
      </div>
    </button>
  </div>
</template>
