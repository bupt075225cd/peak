<script setup lang="ts">
import { computed } from 'vue'
import { Eye, EyeOff, ShieldCheck } from 'lucide-vue-next'
import type { LedgerSummaryView } from '../../api/ledger'
import { debtRatio, formatCents, formatDate, formatPercent, MONEY_MASK } from '../../utils/money'

const props = defineProps<{
  view: LedgerSummaryView
  updatedAt: string
  hasForeign: boolean
  hidden: boolean
}>()

const emit = defineEmits<{
  (e: 'update:excludeFixed', value: boolean): void
  (e: 'toggle-hidden'): void
}>()

const totalText = computed(() => (props.hidden ? MONEY_MASK : formatCents(props.view.totalAssetCents)))
const netText = computed(() => (props.hidden ? MONEY_MASK : formatCents(props.view.netAssetCents)))
const ratio = computed(() => debtRatio(props.view.totalLiabilityCents, props.view.totalAssetCents))
const ratioText = computed(() => (props.hidden ? MONEY_MASK : formatPercent(ratio.value)))
</script>

<template>
  <section
    class="relative overflow-hidden rounded-2xl border border-white/70 bg-gradient-to-br from-ledger-mist to-ledger-haze p-6 shadow-[0_10px_30px_-18px_rgba(30,42,68,0.45)]"
  >
    <div class="pointer-events-none absolute -right-16 -top-24 h-56 w-56 rounded-full bg-white/45 blur-2xl" />

    <div class="relative flex items-start justify-between gap-4">
      <div class="flex items-center gap-2">
        <h2 class="text-sm font-medium text-ink-soft">总资产（元）</h2>
        <button
          type="button"
          class="rounded-lg p-1.5 text-ink-faint transition-colors hover:bg-white/70 hover:text-ink-soft cursor-pointer"
          :aria-label="hidden ? '显示金额' : '隐藏金额'"
          data-test="ledger-eye"
          @click="emit('toggle-hidden')"
        >
          <EyeOff v-if="hidden" class="h-4 w-4" />
          <Eye v-else class="h-4 w-4" />
        </button>
      </div>

      <div class="flex items-center gap-2">
        <span class="text-xs text-ink-soft">排除固定资产</span>
        <button
          type="button"
          role="switch"
          :aria-checked="view.excludeFixed"
          class="relative h-6 w-11 rounded-full transition-colors duration-200 cursor-pointer"
          :class="view.excludeFixed ? 'bg-ledger-ink' : 'bg-white/90 ring-1 ring-slate-300/70'"
          data-test="exclude-fixed"
          @click="emit('update:excludeFixed', !view.excludeFixed)"
        >
          <span
            class="absolute top-0.5 h-5 w-5 rounded-full bg-white shadow-sm transition-all duration-200"
            :class="view.excludeFixed ? 'left-[22px]' : 'left-0.5'"
          />
        </button>
      </div>
    </div>

    <p
      class="relative mt-4 text-[42px] font-semibold leading-none tracking-tight text-ledger-ink tabular-nums transition-opacity"
      data-test="ledger-total"
    >
      {{ totalText }}
    </p>
    <p class="relative mt-3 flex items-center gap-2 text-xs text-ink-faint">
      <span>{{ formatDate(updatedAt) }} 更新</span>
      <span v-if="hasForeign" class="rounded-full bg-white/80 px-2 py-0.5 text-[11px] text-ink-soft">
        含外币，按面值计入
      </span>
    </p>

    <div class="relative mt-5 grid grid-cols-2 gap-4 border-t border-white/80 pt-4">
      <div>
        <p class="text-xs text-ink-faint">净资产</p>
        <p
          class="mt-1 text-lg font-semibold tabular-nums"
          :class="view.netAssetCents < 0 && !hidden ? 'text-ledger-up' : 'text-ledger-ink'"
          data-test="ledger-net"
        >
          {{ netText }}
        </p>
      </div>
      <div>
        <p class="text-xs text-ink-faint">负债率</p>
        <p class="mt-1 text-lg font-semibold tabular-nums" :class="ratio > 0.6 ? 'text-ledger-up' : 'text-ledger-down'">
          {{ ratioText }}
        </p>
      </div>
    </div>

    <p class="relative mt-4 inline-flex items-center gap-1.5 rounded-full bg-white/70 px-2.5 py-1 text-[11px] text-ink-soft">
      <ShieldCheck class="h-3.5 w-3.5 text-ledger-down" />
      金额已加密存储，仅你本人可见
    </p>
  </section>
</template>
