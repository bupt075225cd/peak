<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Check, Plus, X } from 'lucide-vue-next'
import type { LedgerCategory, LedgerItem, LedgerItemInput, LedgerMember, LedgerMeta, RiskType } from '../../api/ledger'
import { formatCents, parseAmountToCents } from '../../utils/money'

const props = defineProps<{
  open: boolean
  mode: 'create' | 'edit'
  meta: LedgerMeta | null
  members: LedgerMember[]
  initial: Partial<LedgerItem> & { category?: LedgerCategory; sub_type?: string }
  saving: boolean
  error: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'submit', input: LedgerItemInput): void
  (e: 'create-member', name: string): void
}>()

const name = ref('')
const amountInput = ref('')
const currency = ref('CNY')
const riskType = ref<RiskType>('none')
const category = ref<LedgerCategory>('liquid')
const subType = ref('')
const memberId = ref<number | null>(null)
const note = ref('')
const memberAdding = ref(false)
const memberDraft = ref('')
const localError = ref('')

const currentCategory = computed(() => props.meta?.categories.find((c) => c.key === category.value) ?? null)
const amountCents = computed(() => parseAmountToCents(amountInput.value))
const amountText = computed(() => (amountCents.value ? formatCents(amountCents.value) : ''))
const title = computed(() => {
  const label = currentCategory.value?.sub_types.find((s) => s.key === subType.value)?.label
  const action = props.mode === 'create' ? '添加资产' : '编辑资产'
  return label ? `${action} · ${label}` : action
})
const canSubmit = computed(() => !props.saving && !!name.value.trim() && !!amountCents.value && !!subType.value)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    const init = props.initial
    name.value = init.name ?? labelOf(init.sub_type ?? '') ?? ''
    amountInput.value = init.amount_cents ? formatCents(init.amount_cents) : ''
    currency.value = init.currency ?? 'CNY'
    riskType.value = (init.risk_type as RiskType) ?? 'none'
    category.value = init.category ?? 'liquid'
    subType.value = init.sub_type ?? props.meta?.categories.find((c) => c.key === category.value)?.sub_types[0]?.key ?? ''
    memberId.value = init.member_id ?? null
    note.value = init.note ?? ''
    localError.value = ''
    memberAdding.value = false
  },
  { immediate: true },
)

// 切换分类时把子类重置为该分类下的默认项。
watch(category, (next) => {
  const group = props.meta?.categories.find((c) => c.key === next)
  if (group && !group.sub_types.some((s) => s.key === subType.value)) {
    subType.value = group.sub_types[0]?.key ?? ''
  }
})

function labelOf(key: string): string {
  for (const group of props.meta?.categories ?? []) {
    const hit = group.sub_types.find((s) => s.key === key)
    if (hit) return hit.label
  }
  return ''
}

function addMember() {
  const draft = memberDraft.value.trim()
  if (!draft) return
  emit('create-member', draft)
  memberDraft.value = ''
  memberAdding.value = false
}

function submit() {
  if (!amountCents.value) {
    localError.value = '请输入大于 0 且最多两位小数的金额'
    return
  }
  if (!name.value.trim()) {
    localError.value = '请输入资产名称'
    return
  }
  localError.value = ''
  emit('submit', {
    name: name.value.trim(),
    category: category.value,
    sub_type: subType.value,
    amount_cents: amountCents.value,
    currency: currency.value,
    risk_type: riskType.value,
    member_id: memberId.value,
    note: note.value.trim(),
  })
}
</script>

<template>
  <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center bg-ink/40 p-4 backdrop-blur-sm" @click.self="emit('close')">
    <div class="max-h-[88vh] w-full max-w-xl overflow-y-auto rounded-2xl border border-slate-200/80 bg-white p-6 shadow-2xl animate-fade-up">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-semibold text-ink">{{ title }}</h2>
        <button type="button" class="rounded-lg p-1.5 text-ink-faint hover:bg-slate-100 cursor-pointer" @click="emit('close')">
          <X class="h-4 w-4" />
        </button>
      </div>

      <!-- 成员归属 -->
      <div class="mt-5">
        <p class="text-xs font-medium text-ink-soft">成员</p>
        <div class="mt-2 flex flex-wrap items-center gap-3">
          <button
            type="button"
            class="relative flex h-10 w-10 items-center justify-center rounded-full text-sm font-medium transition-colors cursor-pointer"
            :class="memberId === null ? 'bg-ledger-ink text-white' : 'bg-slate-100 text-ink-soft hover:bg-slate-200'"
            @click="memberId = null"
          >
            我
            <Check v-if="memberId === null" class="absolute -bottom-1 -right-1 h-4 w-4 rounded-full bg-white p-0.5 text-ledger-ink" />
          </button>
          <button
            v-for="member in members"
            :key="member.id"
            type="button"
            class="relative flex h-10 w-10 items-center justify-center rounded-full text-sm font-medium transition-colors cursor-pointer"
            :class="memberId === member.id ? 'bg-ledger-ink text-white' : 'bg-slate-100 text-ink-soft hover:bg-slate-200'"
            :title="member.name"
            @click="memberId = member.id"
          >
            {{ member.name.slice(0, 1) }}
            <Check v-if="memberId === member.id" class="absolute -bottom-1 -right-1 h-4 w-4 rounded-full bg-white p-0.5 text-ledger-ink" />
          </button>
          <button
            v-if="!memberAdding"
            type="button"
            class="flex h-10 w-10 items-center justify-center rounded-full border border-dashed border-slate-300 text-ink-faint transition-colors hover:border-ledger-ink hover:text-ledger-ink cursor-pointer"
            aria-label="添加成员"
            @click="memberAdding = true"
          >
            <Plus class="h-4 w-4" />
          </button>
          <div v-else class="flex items-center gap-1">
            <input
              v-model="memberDraft"
              maxlength="32"
              placeholder="昵称，如「配偶」"
              class="w-32 rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs outline-none focus:border-ledger-ink"
              @keyup.enter="addMember"
            />
            <button type="button" class="rounded-lg bg-ledger-ink px-2 py-1.5 text-xs text-white cursor-pointer" @click="addMember">添加</button>
          </div>
        </div>
      </div>

      <!-- 名称 -->
      <label class="mt-5 block">
        <span class="text-xs font-medium text-ink-soft">名称</span>
        <input
          v-model="name"
          maxlength="64"
          placeholder="如：招商银行活期"
          class="mt-1.5 w-full rounded-xl bg-slate-50 px-3.5 py-3 text-sm text-ink outline-none ring-1 ring-transparent transition-shadow focus:bg-white focus:ring-ledger-ink/30"
        />
      </label>

      <!-- 金额 -->
      <label class="mt-4 block">
        <span class="text-xs font-medium text-ink-soft">最新金额</span>
        <div class="mt-1.5 flex items-center rounded-xl bg-slate-50 px-3.5 py-3 focus-within:bg-white focus-within:ring-1 focus-within:ring-ledger-ink/30">
          <input
            v-model="amountInput"
            inputmode="decimal"
            placeholder="点击输入金额"
            class="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-ink-faint"
          />
          <span class="ml-2 text-sm text-ink-faint">元</span>
        </div>
        <p v-if="amountText" class="mt-1 text-[11px] text-ink-faint">= {{ amountText }} 元</p>
      </label>

      <div class="mt-4 grid grid-cols-2 gap-3">
        <label class="block">
          <span class="text-xs font-medium text-ink-soft">币种</span>
          <select
            v-model="currency"
            class="mt-1.5 w-full rounded-xl bg-slate-50 px-3 py-3 text-sm text-ink outline-none focus:bg-white focus:ring-1 focus:ring-ledger-ink/30"
          >
            <option v-for="item in meta?.currencies ?? []" :key="item.key" :value="item.key">
              {{ item.label }}
            </option>
          </select>
        </label>
        <label class="block">
          <span class="text-xs font-medium text-ink-soft">资产分类</span>
          <select
            v-model="category"
            class="mt-1.5 w-full rounded-xl bg-slate-50 px-3 py-3 text-sm text-ink outline-none focus:bg-white focus:ring-1 focus:ring-ledger-ink/30"
          >
            <option v-for="item in meta?.categories ?? []" :key="item.key" :value="item.key">{{ item.label }}</option>
          </select>
        </label>
      </div>

      <!-- 子类 -->
      <div class="mt-4">
        <p class="text-xs font-medium text-ink-soft">子类</p>
        <div class="mt-2 flex flex-wrap gap-2">
          <button
            v-for="sub in currentCategory?.sub_types ?? []"
            :key="sub.key"
            type="button"
            class="rounded-full px-3 py-1.5 text-xs transition-colors cursor-pointer"
            :class="subType === sub.key ? 'bg-white text-ledger-ink ring-1 ring-ledger-ink' : 'bg-slate-100 text-ink-soft hover:bg-slate-200'"
            @click="subType = sub.key"
          >
            {{ sub.label }}
          </button>
        </div>
      </div>

      <!-- 资产类型 -->
      <div class="mt-4">
        <p class="text-xs font-medium text-ink-soft">资产类型</p>
        <div class="mt-2 flex flex-wrap gap-2">
          <button
            v-for="risk in meta?.risk_types ?? []"
            :key="risk.key"
            type="button"
            class="rounded-full px-3.5 py-1.5 text-xs transition-colors cursor-pointer"
            :class="riskType === risk.key ? 'bg-white text-ledger-ink ring-1 ring-ledger-ink' : 'bg-slate-100 text-ink-soft hover:bg-slate-200'"
            @click="riskType = risk.key"
          >
            {{ risk.label }}
          </button>
        </div>
      </div>

      <!-- 备注 -->
      <label class="mt-4 block">
        <span class="text-xs font-medium text-ink-soft">备注（可选，请勿填写证件号/卡号）</span>
        <input
          v-model="note"
          maxlength="200"
          placeholder="如：尾号 6688 的储蓄卡"
          class="mt-1.5 w-full rounded-xl bg-slate-50 px-3.5 py-3 text-sm text-ink outline-none focus:bg-white focus:ring-1 focus:ring-ledger-ink/30"
        />
      </label>

      <p v-if="localError || error" class="mt-3 text-xs text-ledger-up">{{ localError || error }}</p>

      <button
        type="button"
        class="mt-5 w-full rounded-xl bg-primary py-3 text-sm font-semibold text-white shadow-md shadow-blue-500/20 transition-colors hover:bg-primary-light disabled:cursor-not-allowed disabled:bg-slate-300 disabled:shadow-none"
        :disabled="!canSubmit"
        data-test="ledger-submit"
        @click="submit"
      >
        {{ saving ? '保存中…' : mode === 'create' ? '完成添加' : '保存修改' }}
      </button>
    </div>
  </div>
</template>
