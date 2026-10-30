<script setup lang="ts">
import { ChevronRight } from 'lucide-vue-next'
import type { LedgerMember, MemberScope } from '../../api/ledger'

// 总览页的成员切换条：全部 / 我 / 各成员，右侧进入家庭成员管理。
// 「我」指未归属任何成员的条目，与服务端 member_id=self 对应。
defineProps<{
  members: LedgerMember[]
  active: MemberScope
}>()

const emit = defineEmits<{
  (e: 'update:active', value: MemberScope): void
  (e: 'manage'): void
}>()

function scopeOf(member: LedgerMember): MemberScope {
  return String(member.id) as MemberScope
}
</script>

<template>
  <div class="flex items-center justify-between gap-3" data-test="member-bar">
    <div class="flex min-w-0 flex-1 items-center gap-2 overflow-x-auto pb-1">
      <button
        type="button"
        class="shrink-0 whitespace-nowrap rounded-full px-5 py-2 text-sm transition-colors cursor-pointer"
        :class="active === '' ? 'bg-ledger-invest font-medium text-white' : 'bg-white text-ink-soft ring-1 ring-slate-200/80 hover:text-ledger-ink'"
        :aria-pressed="active === ''"
        data-test="member-scope-all"
        @click="emit('update:active', '')"
      >
        全部
      </button>
      <button
        type="button"
        class="shrink-0 whitespace-nowrap rounded-full px-5 py-2 text-sm transition-colors cursor-pointer"
        :class="active === 'self' ? 'bg-ledger-invest font-medium text-white' : 'bg-white text-ink-soft ring-1 ring-slate-200/80 hover:text-ledger-ink'"
        :aria-pressed="active === 'self'"
        data-test="member-scope-self"
        @click="emit('update:active', 'self')"
      >
        我
      </button>
      <button
        v-for="member in members"
        :key="member.id"
        type="button"
        class="shrink-0 whitespace-nowrap rounded-full px-5 py-2 text-sm transition-colors cursor-pointer"
        :class="active === scopeOf(member) ? 'bg-ledger-invest font-medium text-white' : 'bg-white text-ink-soft ring-1 ring-slate-200/80 hover:text-ledger-ink'"
        :aria-pressed="active === scopeOf(member)"
        :data-test="`member-scope-${member.id}`"
        @click="emit('update:active', scopeOf(member))"
      >
        {{ member.name }}
      </button>
    </div>

    <div class="flex shrink-0 items-center border-l border-slate-200 pl-3">
      <button
        type="button"
        class="inline-flex items-center gap-0.5 text-sm text-ink-soft transition-colors hover:text-ledger-ink cursor-pointer"
        data-test="manage-members"
        @click="emit('manage')"
      >
        管理
        <ChevronRight class="h-4 w-4" />
      </button>
    </div>
  </div>
</template>
