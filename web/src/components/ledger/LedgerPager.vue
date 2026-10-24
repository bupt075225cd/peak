<script setup lang="ts">
import { useLedgerPrivacy } from '../../composables/useLedgerPrivacy'

defineProps<{
  total: number
  sumCents: number
  page: number
  pageCount: number
}>()

const emit = defineEmits<{
  (e: 'prev'): void
  (e: 'next'): void
}>()

const { money } = useLedgerPrivacy()
</script>

<template>
  <div class="mt-3 flex flex-wrap items-center justify-between gap-3 text-xs text-ink-soft">
    <p>
      共 <b class="tabular-nums text-ink">{{ total }}</b> 项，金额合计（含负债）
      <b class="tabular-nums text-ink" data-test="items-sum">{{ money(sumCents) }}</b>
    </p>
    <div class="flex items-center gap-2">
      <button
        type="button"
        class="rounded-lg border border-slate-200 bg-white px-3 py-1.5 transition-colors hover:text-ledger-ink disabled:cursor-not-allowed disabled:opacity-40 cursor-pointer"
        :disabled="page <= 1"
        data-test="prev-page"
        @click="emit('prev')"
      >
        上一页
      </button>
      <span class="tabular-nums">{{ page }} / {{ pageCount }}</span>
      <button
        type="button"
        class="rounded-lg border border-slate-200 bg-white px-3 py-1.5 transition-colors hover:text-ledger-ink disabled:cursor-not-allowed disabled:opacity-40 cursor-pointer"
        :disabled="page >= pageCount"
        data-test="next-page"
        @click="emit('next')"
      >
        下一页
      </button>
    </div>
  </div>
</template>
