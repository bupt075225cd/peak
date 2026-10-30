<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { X } from 'lucide-vue-next'
import type { LedgerMember } from '../../api/ledger'

// 添加 / 编辑成员：昵称 + 「在家庭资产记账中显示该成员」开关。
const props = defineProps<{
  open: boolean
  mode: 'create' | 'edit'
  member: LedgerMember | null
  saving: boolean
  deleting: boolean
  error: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'submit', payload: { name: string; visible: boolean }): void
  (e: 'remove'): void
}>()

const name = ref('')
const visible = ref(true)
const localError = ref('')
const nameInput = ref<HTMLInputElement | null>(null)

const title = computed(() => (props.mode === 'create' ? '添加成员' : '编辑成员'))
const busy = computed(() => props.saving || props.deleting)
const canSubmit = computed(() => !busy.value && name.value.trim().length > 0)
const message = computed(() => localError.value || props.error)

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    name.value = props.member?.name ?? ''
    visible.value = props.member?.visible ?? true
    localError.value = ''
    await nextTick()
    nameInput.value?.focus()
  },
  { immediate: true },
)

function submit() {
  const value = name.value.trim()
  if (!value) {
    localError.value = '请输入成员昵称'
    return
  }
  localError.value = ''
  emit('submit', { name: value, visible: visible.value })
}
</script>

<template>
  <div
    v-if="open"
    class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-ink/40 p-4 backdrop-blur-sm sm:items-center"
    data-test="member-form"
    @click.self="emit('close')"
  >
    <div class="my-6 w-full max-w-lg rounded-3xl bg-white p-6 shadow-2xl animate-fade-up sm:p-8">
      <div class="relative flex items-center justify-center">
        <button
          type="button"
          class="absolute left-0 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-slate-50 text-ink transition-colors hover:bg-slate-100 cursor-pointer"
          aria-label="关闭"
          data-test="member-form-close"
          @click="emit('close')"
        >
          <X class="h-4 w-4" />
        </button>
        <h2 class="text-lg font-semibold text-ink" data-test="member-form-title">{{ title }}</h2>
      </div>

      <label class="mt-8 block">
        <span class="text-sm text-ink-soft">昵称</span>
        <input
          ref="nameInput"
          v-model="name"
          maxlength="32"
          placeholder="为家人取个名字吧"
          class="mt-3 w-full rounded-2xl bg-[#F5F6F8] px-5 py-4 text-base text-ink outline-none transition-shadow placeholder:text-ink-faint focus:bg-white focus:ring-1 focus:ring-ledger-ink/30"
          data-test="member-name"
          @keyup.enter="submit"
        />
      </label>

      <div class="mt-8 flex items-center justify-between gap-4">
        <span class="text-sm text-ink">在家庭资产记账中显示该成员</span>
        <button
          type="button"
          role="switch"
          :aria-checked="visible"
          class="relative h-8 w-14 shrink-0 rounded-full transition-colors cursor-pointer"
          :class="visible ? 'bg-primary' : 'bg-slate-300'"
          data-test="member-visible"
          @click="visible = !visible"
        >
          <span
            class="absolute top-1 h-6 w-6 rounded-full bg-white shadow-sm transition-all"
            :class="visible ? 'left-7' : 'left-1'"
          />
        </button>
      </div>

      <p v-if="message" class="mt-4 text-xs text-ledger-up" data-test="member-form-error">{{ message }}</p>

      <button
        type="button"
        class="mt-8 w-full rounded-full bg-primary py-4 text-base font-medium text-white transition-colors hover:bg-primary-light disabled:cursor-not-allowed disabled:bg-[#BFD7FB]"
        :disabled="!canSubmit"
        data-test="member-submit"
        @click="submit"
      >
        {{ saving ? '保存中…' : '完成' }}
      </button>

      <button
        v-if="mode === 'edit'"
        type="button"
        class="mt-3 w-full rounded-full py-2.5 text-sm text-ledger-up transition-colors hover:bg-red-50 cursor-pointer disabled:cursor-not-allowed disabled:text-ink-faint"
        :disabled="busy"
        data-test="member-remove"
        @click="emit('remove')"
      >
        删除成员
      </button>
    </div>
  </div>
</template>
