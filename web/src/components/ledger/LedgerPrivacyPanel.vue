<script setup lang="ts">
import { Download, ShieldCheck, Trash2 } from 'lucide-vue-next'
import type { LedgerDeletion } from '../../api/ledger'
import { formatDate } from '../../utils/money'

defineProps<{
  deletion: LedgerDeletion | null
  exporting: boolean
}>()

const emit = defineEmits<{
  (e: 'export'): void
  (e: 'open-deletion'): void
  (e: 'cancel-deletion'): void
}>()
</script>

<template>
  <section class="mt-6 rounded-2xl border border-slate-200/70 bg-white p-5" data-test="privacy-panel">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="flex items-start gap-3">
        <span class="flex h-9 w-9 items-center justify-center rounded-xl bg-ledger-mist text-ledger-ink">
          <ShieldCheck class="h-4 w-4" />
        </span>
        <div>
          <h2 class="text-sm font-semibold text-ink">数据与隐私</h2>
          <p class="mt-0.5 max-w-xl text-xs leading-relaxed text-ink-faint">
            你可以随时导出全部账本数据（可携带权），或注销账本。导出与注销都会写入不可篡改的审计日志，且需要重新验证身份。
          </p>
        </div>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-xl border border-slate-200 px-3.5 py-2 text-sm text-ink-soft transition-colors hover:border-ledger-ink/30 hover:text-ledger-ink disabled:cursor-not-allowed disabled:opacity-50 cursor-pointer"
          :disabled="exporting"
          data-test="export-data"
          @click="emit('export')"
        >
          <Download class="h-4 w-4" />
          {{ exporting ? '导出中…' : '导出全部数据' }}
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-xl border border-ledger-up/35 px-3.5 py-2 text-sm font-medium text-ledger-up transition-colors hover:bg-red-50 cursor-pointer"
          data-test="open-deletion"
          @click="emit('open-deletion')"
        >
          <Trash2 class="h-4 w-4" />
          注销账本
        </button>
      </div>
    </div>

    <div
      v-if="deletion?.status === 'pending'"
      class="mt-3 flex flex-wrap items-center justify-between gap-2 rounded-xl bg-amber-50 px-3.5 py-2.5 text-xs text-amber-700"
      data-test="deletion-hint"
    >
      <span>
        已申请注销，剩余 {{ deletion.days_left }} 天（{{ formatDate(deletion.execute_after) }} 执行删除），期间可随时撤销。
      </span>
      <button
        type="button"
        class="rounded-lg border border-amber-300 bg-white px-2.5 py-1 text-[11px] font-medium text-amber-800 transition-colors hover:bg-amber-100 cursor-pointer"
        data-test="cancel-deletion"
        @click="emit('cancel-deletion')"
      >
        撤销注销
      </button>
    </div>
  </section>
</template>
