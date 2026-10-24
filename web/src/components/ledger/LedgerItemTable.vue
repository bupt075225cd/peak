<script setup lang="ts">
import { Pencil, Trash2 } from 'lucide-vue-next'
import type { LedgerItem, LedgerMeta } from '../../api/ledger'
import { formatDate, formatCents, MONEY_MASK } from '../../utils/money'
import { ledgerIcon } from './ledgerIcons'

const props = defineProps<{
  items: LedgerItem[]
  meta: LedgerMeta | null
  hidden: boolean
  busyId: number | null
}>()

const emit = defineEmits<{
  (e: 'edit', item: LedgerItem): void
  (e: 'remove', item: LedgerItem): void
}>()

function subTypeLabel(item: LedgerItem): string {
  for (const group of props.meta?.categories ?? []) {
    const hit = group.sub_types.find((s) => s.key === item.sub_type)
    if (hit) return hit.label
  }
  return item.sub_type
}

function subTypeIcon(item: LedgerItem): string {
  for (const group of props.meta?.categories ?? []) {
    const hit = group.sub_types.find((s) => s.key === item.sub_type)
    if (hit) return hit.icon
  }
  return 'Package'
}

function categoryOf(item: LedgerItem) {
  return props.meta?.categories.find((c) => c.key === item.category) ?? null
}

// 副标题：子类与名称重复时只保留一次，备注追加在后。
function subtitle(item: LedgerItem): string {
  const parts: string[] = []
  const label = subTypeLabel(item)
  if (label && label !== item.name) parts.push(label)
  if (item.note) parts.push(item.note)
  return parts.join(' · ')
}

function riskLabel(item: LedgerItem): string {
  return props.meta?.risk_types.find((r) => r.key === item.risk_type)?.label ?? '不标记'
}

function currencySymbol(code: string): string {
  return props.meta?.currencies.find((c) => c.key === code)?.symbol ?? '¥'
}

function amountText(item: LedgerItem): string {
  if (props.hidden) return MONEY_MASK
  return `${currencySymbol(item.currency)} ${formatCents(item.amount_cents)}`
}

// 成员用低饱和圆点区分，避免首字头像与昵称重复出现。
const MEMBER_COLORS = ['#9DBCC4', '#A9A3D4', '#E9A66B', '#63C6A5', '#B9B4C7']

function memberColor(item: LedgerItem): string {
  if (!item.member_id) return '#94A3B8'
  return MEMBER_COLORS[item.member_id % MEMBER_COLORS.length]
}

function isLiability(item: LedgerItem): boolean {
  return categoryOf(item)?.kind === 'liability'
}
</script>

<template>
  <div class="overflow-hidden rounded-2xl border border-slate-200/70 bg-white shadow-[0_1px_2px_rgba(15,23,42,0.04)]">
    <table class="w-full text-sm">
      <thead>
        <tr class="border-b border-slate-100 text-left text-xs font-medium text-ink-faint">
          <th class="px-5 py-3">资产名称</th>
          <th class="px-3 py-3">分类</th>
          <th class="px-3 py-3">成员</th>
          <th class="px-3 py-3">资产类型</th>
          <th class="px-3 py-3 text-right">金额</th>
          <th class="px-3 py-3">更新时间</th>
          <th class="px-5 py-3 text-right">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="item in items"
          :key="item.id"
          class="border-b border-slate-50 transition-colors last:border-0 hover:bg-slate-50/70"
          :data-test="`item-row-${item.id}`"
        >
          <td class="px-5 py-3.5">
            <div class="flex items-center gap-2.5">
              <span
                class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
                :style="{ backgroundColor: `${categoryOf(item)?.color ?? '#E6EAF3'}22`, color: categoryOf(item)?.color ?? '#1E2A44' }"
              >
                <component :is="ledgerIcon(subTypeIcon(item))" class="h-4 w-4" />
              </span>
              <div class="min-w-0">
                <p class="truncate font-medium text-ink">{{ item.name }}</p>
                <p v-if="subtitle(item)" class="truncate text-[11px] text-ink-faint">{{ subtitle(item) }}</p>
              </div>
            </div>
          </td>
          <td class="px-3 py-3.5">
            <span class="inline-flex items-center gap-1.5 whitespace-nowrap rounded-full bg-slate-50 px-2 py-1 text-[11px] text-ink-soft">
              <span class="h-1.5 w-1.5 rounded-full" :style="{ backgroundColor: categoryOf(item)?.color ?? '#94A3B8' }" />
              {{ categoryOf(item)?.label ?? item.category }}
            </span>
          </td>
          <td class="px-3 py-3.5">
            <span class="flex items-center gap-1.5 whitespace-nowrap text-xs text-ink-soft">
              <span class="h-1.5 w-1.5 rounded-full" :style="{ backgroundColor: memberColor(item) }" />
              {{ item.member_name || '我' }}
            </span>
          </td>
          <td class="px-3 py-3.5 text-xs text-ink-soft">{{ riskLabel(item) }}</td>
          <td
            class="whitespace-nowrap px-3 py-3.5 text-right font-medium tabular-nums"
            :class="isLiability(item) ? 'text-ledger-down' : 'text-ink'"
          >
            {{ isLiability(item) && !hidden ? '-' : '' }}{{ amountText(item) }}
          </td>
          <td class="whitespace-nowrap px-3 py-3.5 text-xs text-ink-faint">{{ formatDate(item.updated_at) }}</td>
          <td class="px-5 py-3.5">
            <div class="flex items-center justify-end gap-1">
              <button
                type="button"
                class="rounded-lg p-1.5 text-ink-faint transition-colors hover:bg-slate-100 hover:text-ledger-ink cursor-pointer"
                aria-label="编辑"
                :data-test="`edit-${item.id}`"
                @click="emit('edit', item)"
              >
                <Pencil class="h-3.5 w-3.5" />
              </button>
              <button
                type="button"
                class="rounded-lg p-1.5 text-ink-faint transition-colors hover:bg-red-50 hover:text-ledger-up cursor-pointer disabled:cursor-not-allowed disabled:opacity-40"
                aria-label="删除"
                :disabled="busyId === item.id"
                :data-test="`remove-${item.id}`"
                @click="emit('remove', item)"
              >
                <Trash2 class="h-3.5 w-3.5" />
              </button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
