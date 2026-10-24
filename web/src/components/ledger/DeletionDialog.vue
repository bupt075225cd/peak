<script setup lang="ts">
import { computed } from 'vue'
import { AlertTriangle, CalendarClock, X } from 'lucide-vue-next'
import type { LedgerDeletion } from '../../api/ledger'
import { formatDate } from '../../utils/money'

const props = defineProps<{
  open: boolean
  deletion: LedgerDeletion | null
  busy: boolean
  error: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'request'): void
  (e: 'cancel'): void
}>()

const pending = computed(() => props.deletion?.status === 'pending')
const daysLeft = computed(() => props.deletion?.days_left ?? 0)
</script>

<template>
  <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center bg-ink/40 p-4 backdrop-blur-sm" @click.self="emit('close')">
    <div class="w-full max-w-lg rounded-2xl border border-slate-200/80 bg-white p-6 shadow-2xl animate-fade-up">
      <div class="flex items-start justify-between">
        <div class="flex items-center gap-2">
          <span class="flex h-9 w-9 items-center justify-center rounded-xl bg-red-50 text-ledger-up">
            <AlertTriangle class="h-4 w-4" />
          </span>
          <div>
            <h2 class="text-base font-semibold text-ink">注销家庭资产账本</h2>
            <p class="text-[11px] text-ink-faint">注销后不可恢复，请谨慎操作</p>
          </div>
        </div>
        <button type="button" class="rounded-lg p-1.5 text-ink-faint hover:bg-slate-100 cursor-pointer" @click="emit('close')">
          <X class="h-4 w-4" />
        </button>
      </div>

      <ul class="mt-4 space-y-2 rounded-xl bg-slate-50 p-3.5 text-xs leading-relaxed text-ink-soft">
        <li>· 提交后进入 30 天冷静期，期间可随时撤销，账本照常使用。</li>
        <li>· 冷静期结束后删除你的专属密钥，所有密文永久不可解密，等同于删除。</li>
        <li>· 审计日志只保留动作与哈希，不保留金额、名称等任何内容。</li>
      </ul>

      <div v-if="pending" class="mt-4 rounded-xl border border-amber-200 bg-amber-50/70 p-4">
        <p class="flex items-center gap-1.5 text-sm font-medium text-amber-700">
          <CalendarClock class="h-4 w-4" />
          注销冷静期中
        </p>
        <p class="mt-1 text-xs text-amber-700/90">
          已提交于 {{ formatDate(deletion?.requested_at) }}，剩余
          <b class="tabular-nums" data-test="deletion-days">{{ daysLeft }}</b>
          天，将于 {{ formatDate(deletion?.execute_after) }} 执行删除。
        </p>
      </div>

      <p v-if="error" class="mt-3 text-xs text-ledger-up">{{ error }}</p>

      <div class="mt-5 flex justify-end gap-2">
        <button
          type="button"
          class="rounded-xl px-4 py-2.5 text-sm font-medium text-ink-soft transition-colors hover:bg-slate-100 cursor-pointer"
          @click="emit('close')"
        >
          关闭
        </button>
        <button
          v-if="pending"
          type="button"
          class="rounded-xl bg-ledger-ink px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-[#2B3A5C] disabled:cursor-not-allowed disabled:bg-slate-300 cursor-pointer"
          :disabled="busy"
          data-test="cancel-deletion"
          @click="emit('cancel')"
        >
          {{ busy ? '处理中…' : '撤销注销' }}
        </button>
        <button
          v-else
          type="button"
          class="rounded-xl border border-ledger-up/40 px-4 py-2.5 text-sm font-semibold text-ledger-up transition-colors hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50 cursor-pointer"
          :disabled="busy"
          data-test="request-deletion"
          @click="emit('request')"
        >
          {{ busy ? '提交中…' : '确认申请注销' }}
        </button>
      </div>
    </div>
  </div>
</template>
