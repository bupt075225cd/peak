<script setup lang="ts">
import { Plus, X } from 'lucide-vue-next'
import type { LedgerMember } from '../../api/ledger'

// 家庭成员管理：只做展示与入口，新增/编辑交给 MemberFormDialog。
defineProps<{
  open: boolean
  members: LedgerMember[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'add'): void
  (e: 'edit', member: LedgerMember): void
}>()

const AVATAR_COLORS = ['#6C8FD6', '#E9A66B', '#63C6A5', '#A9A3D4', '#9DBCC4', '#D98C8C', '#7FA8A0']

function avatarColor(id: number): string {
  return AVATAR_COLORS[id % AVATAR_COLORS.length]
}
</script>

<template>
  <div
    v-if="open"
    class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-ink/40 p-4 backdrop-blur-sm sm:items-center"
    data-test="member-manager"
    @click.self="emit('close')"
  >
    <div class="my-6 w-full max-w-2xl rounded-3xl bg-white p-6 shadow-2xl animate-fade-up sm:p-8">
      <div class="flex items-start justify-between gap-4">
        <h2 class="text-xl font-semibold tracking-tight text-ink">家庭成员</h2>
        <button
          type="button"
          class="-mr-1 -mt-1 rounded-full p-2 text-ink-faint transition-colors hover:bg-slate-100 hover:text-ink cursor-pointer"
          aria-label="关闭"
          data-test="manager-close"
          @click="emit('close')"
        >
          <X class="h-4 w-4" />
        </button>
      </div>

      <div class="mt-4 space-y-3 text-sm leading-7 text-ink-soft">
        <p>记录多人家庭资产时，建议添加家庭成员，录入后分别查看每位成员的资产。</p>
        <p>如有需要，你可以找一个方便的时间，和家人面对面坐下来一起讨论、交流和管理家庭资产。</p>
      </div>

      <div class="mt-10 grid grid-cols-3 gap-x-4 gap-y-8 sm:grid-cols-4">
        <!-- 「我」：未归属成员的条目，不可编辑 -->
        <div class="flex flex-col items-center" data-test="manager-self">
          <span class="flex h-16 w-16 items-center justify-center rounded-full bg-[#6C8FD6] text-lg font-medium text-white">
            我
          </span>
          <span class="mt-3 text-sm text-ink">我</span>
        </div>

        <button
          v-for="member in members"
          :key="member.id"
          type="button"
          class="group flex flex-col items-center cursor-pointer"
          :data-test="`manager-member-${member.id}`"
          :title="`管理「${member.name}」`"
          @click="emit('edit', member)"
        >
          <span
            class="flex h-16 w-16 items-center justify-center rounded-full text-lg font-medium text-white ring-offset-2 transition-shadow group-hover:ring-2 group-hover:ring-slate-300"
            :style="{ backgroundColor: avatarColor(member.id) }"
          >
            {{ member.name.slice(0, 1) }}
          </span>
          <span class="mt-3 max-w-[5.5rem] truncate text-sm text-ink">{{ member.name }}</span>
          <span v-if="!member.visible" class="mt-0.5 text-[11px] text-ink-faint">已隐藏</span>
        </button>

        <button
          type="button"
          class="group flex flex-col items-center cursor-pointer"
          data-test="manager-add"
          @click="emit('add')"
        >
          <span
            class="flex h-16 w-16 items-center justify-center rounded-full border border-dashed border-slate-300 text-ink-faint transition-colors group-hover:border-ledger-ink group-hover:text-ledger-ink"
          >
            <Plus class="h-5 w-5" />
          </span>
          <span class="mt-3 text-sm text-ink-faint transition-colors group-hover:text-ink-soft">添加成员</span>
        </button>
      </div>

      <p class="mt-10 text-[11px] text-ink-faint">
        成员只保存昵称或角色，请勿填写真实姓名与证件信息；点击成员可重命名、调整显示或删除。
      </p>
    </div>
  </div>
</template>
