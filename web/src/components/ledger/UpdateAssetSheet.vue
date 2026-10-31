<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { Check, ChevronDown, Plus, X } from 'lucide-vue-next'
import type {
  LedgerCategoryMeta,
  LedgerCurrencyMeta,
  LedgerItem,
  LedgerItemInput,
  LedgerMember,
  LedgerRiskMeta,
  LedgerSubTypeMeta,
  RiskType,
} from '../../api/ledger'
import { formatCents, parseAmountToCents } from '../../utils/money'

// 更新流程的录入弹层：分类与子类已由上一屏确定，这里只补成员、名称、金额、币种与资产类型。
const props = defineProps<{
  open: boolean
  mode: 'create' | 'edit'
  category: LedgerCategoryMeta | null
  subType: LedgerSubTypeMeta | null
  members: LedgerMember[]
  currencies: LedgerCurrencyMeta[]
  riskTypes: LedgerRiskMeta[]
  initial: LedgerItem | null
  saving: boolean
  error: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'submit', input: LedgerItemInput): void
  (e: 'create-member', name: string): void
}>()

const memberId = ref<number | null>(null)
const name = ref('')
const amountInput = ref('')
const currency = ref('CNY')
const riskType = ref<RiskType>('none')
const memberAdding = ref(false)
const memberDraft = ref('')
const localError = ref('')

const amountCents = computed(() => parseAmountToCents(amountInput.value))
const isLiability = computed(() => props.category?.kind === 'liability')
const title = computed(() => `${props.mode === 'create' ? '添加' : '编辑'}${props.subType?.label ?? ''}`)
const canSubmit = computed(() => !props.saving && !!name.value.trim() && !!amountCents.value)
const message = computed(() => localError.value || props.error)

// 自定义子类不预填名称，其余以子类标签作为默认名称。
function defaultName(): string {
  const key = props.subType?.key ?? ''
  if (!key || key.endsWith('_custom')) return ''
  return props.subType?.label ?? ''
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    const init = props.initial
    memberId.value = init?.member_id ?? null
    name.value = init?.name ?? defaultName()
    amountInput.value = init ? formatCents(init.amount_cents) : ''
    currency.value = init?.currency ?? 'CNY'
    riskType.value = (init?.risk_type as RiskType) ?? 'none'
    localError.value = ''
    memberAdding.value = false
    memberDraft.value = ''
    await nextTick()
  },
  { immediate: true },
)

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
    localError.value = '请输入名称'
    return
  }
  if (!props.category || !props.subType) {
    localError.value = '请先选择资产分类'
    return
  }
  localError.value = ''
  emit('submit', {
    name: name.value.trim(),
    category: props.category.key,
    sub_type: props.subType.key,
    amount_cents: amountCents.value,
    currency: currency.value,
    risk_type: isLiability.value ? 'none' : riskType.value,
    member_id: memberId.value,
    note: props.initial?.note ?? '',
  })
}
</script>

<template>
  <div
    v-if="open"
    class="fixed inset-0 z-50 flex items-end justify-center bg-ink/40 backdrop-blur-sm sm:items-center"
    data-test="asset-sheet"
    @click.self="emit('close')"
  >
    <div
      class="max-h-[90vh] w-full max-w-xl overflow-y-auto rounded-t-3xl bg-white px-6 pb-8 pt-7 shadow-2xl animate-fade-up sm:rounded-3xl"
    >
      <div class="relative flex items-center justify-center">
        <button
          type="button"
          class="absolute left-0 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-slate-100 text-ink transition-colors hover:bg-slate-200 cursor-pointer"
          aria-label="关闭"
          data-test="asset-close"
          @click="emit('close')"
        >
          <X class="h-4 w-4" />
        </button>
        <h2 class="text-lg font-semibold text-ink" data-test="asset-title">{{ title }}</h2>
      </div>

      <!-- 成员归属 -->
      <div class="mt-8 flex items-start gap-5">
        <button
          type="button"
          class="flex w-14 flex-col items-center cursor-pointer"
          data-test="asset-member-self"
          @click="memberId = null"
        >
          <span
            class="relative flex h-14 w-14 items-center justify-center rounded-full text-base font-medium transition-colors"
            :class="memberId === null ? 'bg-[#6C8FD6] text-white' : 'bg-slate-100 text-ink-soft'"
          >
            我
            <Check
              v-if="memberId === null"
              class="absolute -bottom-0.5 -right-0.5 h-5 w-5 rounded-full bg-white p-0.5 text-[#6C8FD6]"
            />
          </span>
          <span class="mt-2 text-xs text-ink-soft">我</span>
        </button>

        <button
          v-for="member in members"
          :key="member.id"
          type="button"
          class="flex w-14 flex-col items-center cursor-pointer"
          :data-test="`asset-member-${member.id}`"
          @click="memberId = member.id"
        >
          <span
            class="relative flex h-14 w-14 items-center justify-center rounded-full text-base font-medium transition-colors"
            :class="memberId === member.id ? 'bg-ledger-ink text-white' : 'bg-slate-100 text-ink-soft'"
            :title="member.name"
          >
            {{ member.name.slice(0, 1) }}
            <Check
              v-if="memberId === member.id"
              class="absolute -bottom-0.5 -right-0.5 h-5 w-5 rounded-full bg-white p-0.5 text-ledger-ink"
            />
          </span>
          <span class="mt-2 max-w-14 truncate text-xs text-ink-soft">{{ member.name }}</span>
        </button>

        <div v-if="!memberAdding" class="flex w-14 flex-col items-center">
          <button
            type="button"
            class="flex h-14 w-14 items-center justify-center rounded-full border border-dashed border-slate-300 text-ink-faint transition-colors hover:border-ledger-ink hover:text-ledger-ink cursor-pointer"
            aria-label="添加成员"
            data-test="asset-add-member"
            @click="memberAdding = true"
          >
            <Plus class="h-5 w-5" />
          </button>
          <span class="mt-2 text-xs text-ink-faint">添加成员</span>
        </div>
        <div v-else class="flex flex-col gap-1">
          <input
            v-model="memberDraft"
            maxlength="32"
            placeholder="昵称，如「配偶」"
            class="w-32 rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs outline-none focus:border-ledger-ink"
            data-test="asset-member-draft"
            @keyup.enter="addMember"
          />
          <button type="button" class="rounded-lg bg-ledger-ink px-2 py-1.5 text-xs text-white cursor-pointer" @click="addMember">
            添加
          </button>
        </div>
      </div>

      <label class="mt-7 block">
        <span class="text-sm text-ink-soft">名称</span>
        <input
          v-model="name"
          maxlength="64"
          placeholder="点击输入名称"
          class="mt-2.5 w-full rounded-2xl bg-[#F5F6F8] px-5 py-4 text-base text-ink outline-none transition-shadow placeholder:text-ink-faint focus:bg-white focus:ring-1 focus:ring-ledger-ink/30"
          data-test="asset-name"
        />
      </label>

      <label class="mt-6 block">
        <span class="text-sm text-ink-soft">最新金额</span>
        <div
          class="mt-2.5 flex items-center rounded-2xl bg-[#F5F6F8] px-5 py-4 focus-within:bg-white focus-within:ring-1 focus-within:ring-ledger-ink/30"
        >
          <input
            v-model="amountInput"
            inputmode="decimal"
            placeholder="点击输入金额"
            class="min-w-0 flex-1 bg-transparent text-base text-ink outline-none placeholder:text-ink-faint"
            data-test="asset-amount"
          />
          <span class="ml-2 text-sm text-ink-faint">元</span>
        </div>
      </label>

      <label class="mt-6 block">
        <span class="text-sm text-ink-soft">币种</span>
        <div class="relative mt-2.5">
          <select
            v-model="currency"
            class="w-full appearance-none rounded-2xl bg-[#F5F6F8] px-5 py-4 text-base text-ink outline-none focus:bg-white focus:ring-1 focus:ring-ledger-ink/30 cursor-pointer"
            data-test="asset-currency"
          >
            <option v-for="item in currencies" :key="item.key" :value="item.key">{{ item.label }}</option>
          </select>
          <ChevronDown class="pointer-events-none absolute right-5 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-faint" />
        </div>
      </label>

      <!-- 负债没有「资产类型」概念，参考稿里信用卡表单也不出现该分组。 -->
      <div v-if="!isLiability" class="mt-6">
        <p class="text-sm text-ink-soft">资产类型</p>
        <div class="mt-3 flex flex-wrap gap-2">
          <button
            v-for="risk in riskTypes"
            :key="risk.key"
            type="button"
            class="rounded-full px-4 py-2 text-sm transition-colors cursor-pointer"
            :class="riskType === risk.key ? 'bg-white text-ledger-ink ring-1 ring-ledger-ink' : 'bg-[#F5F6F8] text-ink-soft hover:bg-slate-200'"
            :data-test="`asset-risk-${risk.key}`"
            @click="riskType = risk.key"
          >
            {{ risk.label }}
          </button>
        </div>
      </div>

      <p v-if="message" class="mt-4 text-xs text-ledger-up" data-test="asset-error">{{ message }}</p>

      <button
        type="button"
        class="mt-8 w-full rounded-full bg-primary py-4 text-base font-medium text-white transition-colors hover:bg-primary-light disabled:cursor-not-allowed disabled:bg-[#BFD7FB]"
        :disabled="!canSubmit"
        data-test="asset-submit"
        @click="submit"
      >
        {{ saving ? '保存中…' : mode === 'create' ? '完成添加' : '保存修改' }}
      </button>
    </div>
  </div>
</template>
