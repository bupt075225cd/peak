<script setup lang="ts">
import { ChevronRight } from 'lucide-vue-next'
import type { LedgerCategory, LedgerCategoryMeta } from '../../api/ledger'
import { formatCents } from '../../utils/money'

// 更新流程顶部步进条：逐类标出当前合计，未录入的分类显示「待填写」。
defineProps<{
  categories: LedgerCategoryMeta[]
  active: LedgerCategory
  totals: Partial<Record<LedgerCategory, number>>
  counts: Partial<Record<LedgerCategory, number>>
}>()

const emit = defineEmits<{
  (e: 'select', value: LedgerCategory): void
}>()

function tint(hex: string, alpha: number): string {
  const raw = hex.replace('#', '')
  const r = Number.parseInt(raw.slice(0, 2), 16)
  const g = Number.parseInt(raw.slice(2, 4), 16)
  const b = Number.parseInt(raw.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

function amountText(category: LedgerCategoryMeta, cents: number): string {
  // 负债在参考稿里以负号展示，提示这是需要偿还的部分。
  const value = category.kind === 'liability' ? `-${formatCents(Math.abs(cents))}` : formatCents(cents)
  return `${value} 元`
}
</script>

<template>
  <div class="-mx-1 flex items-stretch gap-1 overflow-x-auto px-1 pb-1" data-test="update-steps">
    <template v-for="(category, index) in categories" :key="category.key">
      <button
        type="button"
        class="min-w-[7.25rem] shrink-0 rounded-xl border px-3 py-2.5 text-left transition-shadow cursor-pointer"
        :style="{
          backgroundColor: tint(category.color, active === category.key ? 0.22 : 0.1),
          borderColor: active === category.key ? category.color : 'transparent',
        }"
        :aria-current="active === category.key ? 'step' : undefined"
        :data-test="`update-step-${category.key}`"
        @click="emit('select', category.key)"
      >
        <span
          class="block truncate text-xs font-medium"
          :style="{ color: active === category.key ? category.color : '#8B90A0' }"
        >
          {{ category.label }}
        </span>
        <span
          class="mt-1 block truncate text-sm font-semibold tabular-nums"
          :class="counts[category.key] ? 'text-ink' : 'text-ink-faint'"
        >
          {{ counts[category.key] ? amountText(category, totals[category.key] ?? 0) : '待填写' }}
        </span>
      </button>
      <span v-if="index < categories.length - 1" class="flex shrink-0 items-center text-slate-300">
        <ChevronRight class="h-4 w-4" />
      </span>
    </template>
  </div>
</template>
