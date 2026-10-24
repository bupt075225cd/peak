<script setup lang="ts">
import { computed } from 'vue'
import { Plus, X } from 'lucide-vue-next'
import type { LedgerMeta } from '../../api/ledger'
import { ledgerIcon } from './ledgerIcons'

const props = defineProps<{
  open: boolean
  meta: LedgerMeta | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'pick', subType: string): void
}>()

const fixedGroup = computed(() => props.meta?.categories.find((c) => c.key === 'fixed') ?? null)
</script>

<template>
  <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center bg-ink/40 p-4 backdrop-blur-sm" @click.self="emit('close')">
    <div class="w-full max-w-2xl rounded-2xl border border-slate-200/80 bg-white p-6 shadow-2xl animate-fade-up">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-semibold text-ink">更多固定资产</h2>
        <button type="button" class="rounded-lg p-1.5 text-ink-faint hover:bg-slate-100 cursor-pointer" @click="emit('close')">
          <X class="h-4 w-4" />
        </button>
      </div>
      <p class="mt-2 rounded-xl bg-slate-50 px-3.5 py-2.5 text-xs text-ink-soft">
        用于投资或自用的、流动性低的实物类资产；金额可按购入价或当前估值填写。
      </p>

      <div class="mt-4 grid grid-cols-2 gap-2">
        <button
          v-for="sub in fixedGroup?.sub_types ?? []"
          :key="sub.key"
          type="button"
          class="group flex items-center gap-3 rounded-xl border border-slate-100 bg-white px-3.5 py-3 text-left transition-colors hover:border-ledger-fixed hover:bg-slate-50 cursor-pointer"
          :data-test="`pick-${sub.key}`"
          @click="emit('pick', sub.key)"
        >
          <span class="flex h-9 w-9 items-center justify-center rounded-lg bg-ledger-mist text-ledger-ink">
            <component :is="ledgerIcon(sub.icon)" class="h-4 w-4" />
          </span>
          <span class="flex-1 text-sm text-ink">{{ sub.label }}</span>
          <Plus class="h-4 w-4 text-ink-faint transition-colors group-hover:text-ledger-ink" />
        </button>
      </div>
    </div>
  </div>
</template>
