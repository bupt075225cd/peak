<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { ChevronRight, TrendingDown, TrendingUp } from 'lucide-vue-next'
import LedgerCategoryBars from './LedgerCategoryBars.vue'
import type { LedgerSummaryView } from '../../api/ledger'
import { useLedgerPrivacy } from '../../composables/useLedgerPrivacy'

defineProps<{
  view: LedgerSummaryView
}>()

const { hidden, money } = useLedgerPrivacy()
</script>

<template>
  <section class="rounded-2xl border border-slate-200/70 bg-white p-6 shadow-[0_1px_2px_rgba(15,23,42,0.04)] lg:col-span-5 animate-fade-up">
    <div class="flex items-start justify-between gap-3">
      <div>
        <h2 class="text-sm font-semibold text-ink">每月更新，掌握家庭资产变化</h2>
        <p class="mt-1 text-xs text-ink-faint">条目变更后当月快照自动刷新</p>
      </div>
      <RouterLink
        to="/ledger/calendar"
        class="inline-flex items-center gap-1 whitespace-nowrap text-xs font-medium text-ledger-ink hover:underline"
      >
        资产月历
        <ChevronRight class="h-3.5 w-3.5" />
      </RouterLink>
    </div>

    <div class="mt-5 grid grid-cols-2 gap-3">
      <div class="rounded-xl bg-slate-50/80 px-3.5 py-3">
        <p class="flex items-center gap-1 text-[11px] text-ink-faint">
          <TrendingUp class="h-3.5 w-3.5 text-ledger-up" />
          总资产
        </p>
        <p class="mt-1 text-base font-semibold tabular-nums text-ink" data-test="month-total-asset">
          {{ money(view.totalAssetCents) }}
        </p>
      </div>
      <div class="rounded-xl bg-slate-50/80 px-3.5 py-3">
        <p class="flex items-center gap-1 text-[11px] text-ink-faint">
          <TrendingDown class="h-3.5 w-3.5 text-ledger-down" />
          总负债
        </p>
        <p class="mt-1 text-base font-semibold tabular-nums text-ink" data-test="month-total-liability">
          {{ money(view.totalLiabilityCents) }}
        </p>
      </div>
    </div>

    <div class="mt-5">
      <LedgerCategoryBars :groups="view.groups" :hidden="hidden" />
    </div>
  </section>
</template>
