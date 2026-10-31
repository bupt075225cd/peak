<script setup lang="ts">
import { Plus, X } from 'lucide-vue-next'
import type { LedgerSubTypeMeta } from '../../api/ledger'
import { ledgerIcon } from './ledgerIcons'

// 「更多{分类}」弹层：列出该分类下的全部子类，点 + 进入录入。
defineProps<{
  open: boolean
  title: string
  description: string
  subtypes: LedgerSubTypeMeta[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'pick', key: string): void
}>()
</script>

<template>
  <div
    v-if="open"
    class="fixed inset-0 z-50 flex items-end justify-center bg-ink/40 backdrop-blur-sm sm:items-center"
    data-test="subtype-sheet"
    @click.self="emit('close')"
  >
    <div
      class="max-h-[85vh] w-full max-w-xl overflow-y-auto rounded-t-3xl bg-white px-6 pb-8 pt-7 shadow-2xl animate-fade-up sm:rounded-3xl"
    >
      <div class="relative flex items-center justify-center">
        <button
          type="button"
          class="absolute left-0 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-slate-100 text-ink transition-colors hover:bg-slate-200 cursor-pointer"
          aria-label="关闭"
          data-test="subtype-close"
          @click="emit('close')"
        >
          <X class="h-4 w-4" />
        </button>
        <h2 class="text-lg font-semibold text-ink" data-test="subtype-title">{{ title }}</h2>
      </div>

      <p class="mt-6 rounded-xl bg-ledger-mist px-4 py-3 text-xs leading-relaxed text-ink-soft">
        {{ description }}
      </p>

      <ul class="mt-2">
        <li v-for="sub in subtypes" :key="sub.key">
          <button
            type="button"
            class="flex w-full items-center gap-3 rounded-xl px-1 py-4 text-left transition-colors hover:bg-slate-50 cursor-pointer"
            :data-test="`subtype-${sub.key}`"
            @click="emit('pick', sub.key)"
          >
            <span class="flex h-6 w-6 shrink-0 items-center justify-center text-ink-soft">
              <component :is="ledgerIcon(sub.icon)" class="h-5 w-5" />
            </span>
            <span class="flex-1 text-sm text-ink">{{ sub.label }}</span>
            <Plus class="h-5 w-5 shrink-0 text-ink-faint" />
          </button>
        </li>
      </ul>
    </div>
  </div>
</template>
