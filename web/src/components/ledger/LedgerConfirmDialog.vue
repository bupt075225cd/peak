<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string
    message: string
    confirmText?: string
    busy?: boolean
    error?: string
    danger?: boolean
  }>(),
  { confirmText: '确认', busy: false, error: '', danger: false },
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'confirm'): void
}>()
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-ink/40 p-4 backdrop-blur-sm" @click.self="emit('close')">
    <div class="w-full max-w-sm rounded-2xl border border-slate-200/80 bg-white p-6 shadow-2xl animate-fade-up">
      <h2 class="text-base font-semibold text-ink">{{ title }}</h2>
      <p class="mt-2 text-xs leading-relaxed text-ink-soft">{{ message }}</p>
      <p v-if="error" class="mt-2 text-xs text-ledger-up">{{ error }}</p>
      <div class="mt-5 flex justify-end gap-2">
        <button
          type="button"
          class="rounded-xl px-4 py-2 text-sm text-ink-soft transition-colors hover:bg-slate-100 cursor-pointer"
          @click="emit('close')"
        >
          取消
        </button>
        <button
          type="button"
          class="rounded-xl px-4 py-2 text-sm font-semibold text-white transition-colors disabled:cursor-not-allowed disabled:bg-slate-300 cursor-pointer"
          :class="danger ? 'bg-ledger-up hover:bg-[#C24E3D]' : 'bg-ledger-ink hover:bg-[#2B3A5C]'"
          :disabled="busy"
          data-test="confirm-action"
          @click="emit('confirm')"
        >
          {{ busy ? '处理中…' : confirmText }}
        </button>
      </div>
    </div>
  </div>
</template>
