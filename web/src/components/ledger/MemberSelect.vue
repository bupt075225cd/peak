<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Check, ChevronDown } from 'lucide-vue-next'
import type { LedgerMember, MemberScope } from '../../api/ledger'

// 成员范围下拉：全部 / 我 / 各家庭成员，与总览页的切换条同源（visible members）。
const props = defineProps<{
  modelValue: MemberScope
  members: LedgerMember[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: MemberScope): void
}>()

const open = ref(false)
const root = ref<HTMLElement | null>(null)

const label = computed(() => {
  if (props.modelValue === '') return '全部'
  if (props.modelValue === 'self') return '我'
  return props.members.find((m) => String(m.id) === props.modelValue)?.name ?? '全部'
})

function choose(value: MemberScope) {
  open.value = false
  if (value !== props.modelValue) emit('update:modelValue', value)
}

function onDocumentPointerDown(event: MouseEvent) {
  if (!root.value?.contains(event.target as Node)) open.value = false
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') open.value = false
}

watch(open, (value) => {
  if (value) {
    document.addEventListener('mousedown', onDocumentPointerDown)
    document.addEventListener('keydown', onKeydown)
  } else {
    document.removeEventListener('mousedown', onDocumentPointerDown)
    document.removeEventListener('keydown', onKeydown)
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocumentPointerDown)
  document.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div ref="root" class="relative" data-test="member-select">
    <button
      type="button"
      class="inline-flex items-center gap-1 rounded-full bg-[#F2F2F4] px-3.5 py-1.5 text-sm text-ink-soft transition-colors hover:bg-slate-200/70 cursor-pointer"
      aria-haspopup="listbox"
      :aria-expanded="open"
      data-test="member-select-trigger"
      @click="open = !open"
    >
      {{ label }}
      <ChevronDown class="h-3.5 w-3.5 transition-transform" :class="open ? 'rotate-180' : ''" />
    </button>

    <div
      v-if="open"
      class="absolute left-0 top-full z-40 mt-2 max-h-72 w-44 overflow-y-auto rounded-2xl bg-white p-1.5 shadow-xl ring-1 ring-slate-200/80 animate-fade-up"
      role="listbox"
      data-test="member-select-menu"
    >
      <button
        type="button"
        class="flex w-full items-center justify-between gap-2 rounded-xl px-3 py-2 text-left text-sm transition-colors cursor-pointer"
        :class="modelValue === '' ? 'bg-ledger-mist font-medium text-ledger-ink' : 'text-ink-soft hover:bg-slate-50'"
        role="option"
        :aria-selected="modelValue === ''"
        data-test="member-option-all"
        @click="choose('')"
      >
        全部
        <Check v-if="modelValue === ''" class="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        class="flex w-full items-center justify-between gap-2 rounded-xl px-3 py-2 text-left text-sm transition-colors cursor-pointer"
        :class="modelValue === 'self' ? 'bg-ledger-mist font-medium text-ledger-ink' : 'text-ink-soft hover:bg-slate-50'"
        role="option"
        :aria-selected="modelValue === 'self'"
        data-test="member-option-self"
        @click="choose('self')"
      >
        我
        <Check v-if="modelValue === 'self'" class="h-3.5 w-3.5" />
      </button>
      <button
        v-for="member in members"
        :key="member.id"
        type="button"
        class="flex w-full items-center justify-between gap-2 rounded-xl px-3 py-2 text-left text-sm transition-colors cursor-pointer"
        :class="modelValue === String(member.id) ? 'bg-ledger-mist font-medium text-ledger-ink' : 'text-ink-soft hover:bg-slate-50'"
        role="option"
        :aria-selected="modelValue === String(member.id)"
        :data-test="`member-option-${member.id}`"
        @click="choose(String(member.id) as MemberScope)"
      >
        <span class="truncate">{{ member.name }}</span>
        <Check v-if="modelValue === String(member.id)" class="h-3.5 w-3.5 shrink-0" />
      </button>
    </div>
  </div>
</template>
