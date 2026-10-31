<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft, ArrowRight, ChevronDown, ChevronRight, Plus } from 'lucide-vue-next'
import PrivacyConsentDialog from '../components/ledger/PrivacyConsentDialog.vue'
import UpdateAssetSheet from '../components/ledger/UpdateAssetSheet.vue'
import UpdateCategorySteps from '../components/ledger/UpdateCategorySteps.vue'
import UpdateSubtypeSheet from '../components/ledger/UpdateSubtypeSheet.vue'
import { ledgerIcon } from '../components/ledger/ledgerIcons'
import {
  createItem,
  createMember,
  errorMessage,
  updateItem,
  type LedgerCategory,
  type LedgerItem,
  type LedgerItemInput,
  type LedgerSubTypeMeta,
} from '../api/ledger'
import { useLedgerMeta } from '../composables/useLedgerMeta'
import { useLedgerUpdate } from '../composables/useLedgerUpdate'
import { formatCents, formatMonthDay } from '../utils/money'

// 更新家庭资产：按 流动资金 → 固定资产 → 投资理财 → 应收款 → 负债 逐类过一遍，
// 每类下按成员分组展示既有条目，可继续添加或点击条目修正金额。
const router = useRouter()
const { meta, members, accepting, error: metaError, consentRequired, load, loadMembers, accept } = useLedgerMeta()
const { totals, counts, loading, error, load: loadData, groupsOf } = useLedgerUpdate()

const categories = computed(() => meta.value?.categories ?? [])
const activeKey = ref<LedgerCategory>('liquid')
const activeCategory = computed(
  () => categories.value.find((item) => item.key === activeKey.value) ?? categories.value[0] ?? null,
)
const activeIndex = computed(() => categories.value.findIndex((item) => item.key === activeKey.value))
const isLast = computed(() => activeIndex.value >= 0 && activeIndex.value === categories.value.length - 1)
const groups = computed(() => (activeCategory.value ? groupsOf(activeCategory.value.key) : []))
const consentGate = computed(() => !!meta.value && consentRequired.value)
const quickSubType = computed(() => activeCategory.value?.sub_types[0] ?? null)

const hintOpen = ref(false)
const subtypeOpen = ref(false)
const assetOpen = ref(false)
const draftSubType = ref<LedgerSubTypeMeta | null>(null)
const editing = ref<LedgerItem | null>(null)
const saving = ref(false)
const formError = ref('')

onMounted(bootstrap)

async function bootstrap() {
  await load()
  if (metaError.value) error.value = metaError.value
  if (consentRequired.value) return
  await loadData()
}

async function handleAccept() {
  const ok = await accept()
  if (ok) await loadData()
}

function selectCategory(key: LedgerCategory) {
  activeKey.value = key
  hintOpen.value = false
}

function openSubtypes() {
  subtypeOpen.value = true
}

function pickSubtype(key: string) {
  subtypeOpen.value = false
  const sub = activeCategory.value?.sub_types.find((item) => item.key === key)
  if (!sub) return
  editing.value = null
  draftSubType.value = sub
  formError.value = ''
  assetOpen.value = true
}

function openEdit(item: LedgerItem) {
  const sub =
    activeCategory.value?.sub_types.find((s) => s.key === item.sub_type) ??
    ({ key: item.sub_type, label: item.name, icon: 'Package', is_fixed: false } as LedgerSubTypeMeta)
  editing.value = item
  draftSubType.value = sub
  formError.value = ''
  assetOpen.value = true
}

async function submitAsset(input: LedgerItemInput) {
  saving.value = true
  formError.value = ''
  try {
    if (editing.value) await updateItem(editing.value.id, input, editing.value.version)
    else await createItem(input)
    assetOpen.value = false
    editing.value = null
    await loadData()
  } catch (err) {
    const status = (err as { response?: { status?: number } })?.response?.status
    formError.value =
      status === 409 ? '该条目已在其他窗口被修改，请关闭后重新打开再编辑' : errorMessage(err, '保存失败，请稍后重试')
  } finally {
    saving.value = false
  }
}

async function handleCreateMember(name: string) {
  try {
    await createMember(name)
    await loadMembers()
  } catch (err) {
    formError.value = errorMessage(err, '添加成员失败')
  }
}

function next() {
  if (isLast.value) {
    router.push('/ledger')
    return
  }
  const nextCategory = categories.value[activeIndex.value + 1]
  if (nextCategory) selectCategory(nextCategory.key)
}

function amountText(item: LedgerItem): string {
  const sign = item.category === 'liability' ? '-' : ''
  return `¥ ${sign}${formatCents(Math.abs(item.amount_cents))}`
}
</script>

<template>
  <div class="min-h-screen bg-[#F7F8FA]">
    <div class="mx-auto max-w-xl px-6 pb-28 pt-6">
      <div class="flex items-center gap-3">
        <button
          type="button"
          class="flex h-10 w-10 items-center justify-center rounded-full bg-white text-ink shadow-sm ring-1 ring-slate-200/70 transition-colors hover:bg-slate-50 cursor-pointer"
          aria-label="返回"
          data-test="update-back"
          @click="router.push('/ledger')"
        >
          <ArrowLeft class="h-4 w-4" />
        </button>
      </div>

      <h1 class="mt-4 text-2xl font-semibold tracking-tight text-ink">更新家庭资产</h1>

      <p
        v-if="error"
        class="mt-4 rounded-xl border border-red-100 bg-red-50 px-4 py-3 text-sm text-ledger-up"
        data-test="update-error"
      >
        {{ error }}
      </p>

      <div v-if="loading && !categories.length" class="mt-6 space-y-3" data-test="update-skeleton">
        <div class="h-16 animate-pulse rounded-xl bg-white" />
        <div class="h-20 animate-pulse rounded-2xl bg-white" />
      </div>

      <template v-else>
        <div class="mt-5">
          <UpdateCategorySteps
            :categories="categories"
            :active="activeKey"
            :totals="totals"
            :counts="counts"
            @select="selectCategory"
          />
        </div>

        <!-- 什么是{分类} -->
        <div class="mt-5 overflow-hidden rounded-xl bg-white" v-if="activeCategory?.description">
          <button
            type="button"
            class="flex w-full items-center justify-between px-4 py-3.5 text-left cursor-pointer"
            data-test="update-hint-toggle"
            :aria-expanded="hintOpen"
            @click="hintOpen = !hintOpen"
          >
            <span class="text-sm text-ink-soft">什么是{{ activeCategory.label }}</span>
            <ChevronDown
              class="h-4 w-4 text-ink-faint transition-transform"
              :class="hintOpen ? 'rotate-180' : ''"
            />
          </button>
          <p
            v-if="hintOpen"
            class="border-t border-slate-100 px-4 py-3 text-xs leading-relaxed text-ink-soft"
            data-test="update-hint"
          >
            {{ activeCategory.description }}
          </p>
        </div>

        <!-- 按成员分组的既有条目 -->
        <section
          v-for="group in groups"
          :key="group.key"
          class="mt-4 rounded-2xl bg-white px-4 py-4"
          :data-test="`update-group-${group.key}`"
        >
          <p class="text-sm font-semibold" :class="group.key === 'self' ? 'text-ledger-invest' : 'text-ink-soft'">
            {{ group.name }}
          </p>
          <ul class="mt-1 divide-y divide-slate-100">
            <li v-for="item in group.items" :key="item.id">
              <button
                type="button"
                class="flex w-full items-center justify-between gap-3 py-3.5 text-left transition-colors hover:text-ledger-ink cursor-pointer"
                :data-test="`update-item-${item.id}`"
                @click="openEdit(item)"
              >
                <span class="min-w-0 flex-1 truncate text-sm text-ink">{{ item.name }}</span>
                <span class="shrink-0 text-right">
                  <span class="block text-sm font-medium tabular-nums text-ink">{{ amountText(item) }}</span>
                  <span class="block text-xs text-ink-faint">{{ formatMonthDay(item.updated_at) }}</span>
                </span>
              </button>
            </li>
          </ul>
        </section>

        <!-- 继续添加 -->
        <div class="mt-6">
          <p v-if="groups.length" class="text-xs text-ink-faint">继续添加</p>
          <div class="mt-2 space-y-2">
            <button
              v-if="!groups.length && quickSubType"
              type="button"
              class="flex w-full items-center gap-3 rounded-xl border border-dashed border-slate-300 bg-white px-4 py-3.5 text-left transition-colors hover:border-ledger-ink cursor-pointer"
              data-test="update-quick-add"
              @click="pickSubtype(quickSubType.key)"
            >
              <component :is="ledgerIcon(quickSubType.icon)" class="h-4 w-4 text-ink-soft" />
              <span class="flex-1 text-sm text-ink">{{ quickSubType.label }}</span>
              <Plus class="h-4 w-4 text-ink-faint" />
            </button>
            <button
              type="button"
              class="flex w-full items-center gap-3 rounded-xl border border-dashed border-slate-300 bg-white px-4 py-3.5 text-left transition-colors hover:border-ledger-ink cursor-pointer"
              data-test="update-more"
              @click="openSubtypes"
            >
              <span class="flex h-5 w-5 items-center justify-center rounded-full bg-slate-200 text-white">
                <Plus class="h-3 w-3" />
              </span>
              <span class="flex-1 text-sm text-ink">更多{{ activeCategory?.label }}</span>
              <Plus class="h-4 w-4 text-ink-faint" />
            </button>
          </div>
        </div>
      </template>
    </div>

    <!-- 下一步：最后一类跳回总览 -->
    <div v-if="!consentGate" class="pointer-events-none fixed inset-x-0 bottom-8 flex justify-center">
      <button
        type="button"
        class="pointer-events-auto flex h-14 w-14 items-center justify-center rounded-full bg-primary text-white shadow-lg shadow-blue-500/30 transition-colors hover:bg-primary-light cursor-pointer"
        :aria-label="isLast ? '完成更新，返回总览' : '下一类'"
        data-test="update-next"
        @click="next"
      >
        <ChevronRight v-if="!isLast" class="h-6 w-6" />
        <ArrowRight v-else class="h-6 w-6" />
      </button>
    </div>

    <UpdateSubtypeSheet
      :open="subtypeOpen"
      :title="`更多${activeCategory?.label ?? ''}`"
      :description="activeCategory?.description ?? ''"
      :subtypes="activeCategory?.sub_types ?? []"
      @close="subtypeOpen = false"
      @pick="pickSubtype"
    />

    <UpdateAssetSheet
      :open="assetOpen"
      :mode="editing ? 'edit' : 'create'"
      :category="activeCategory"
      :sub-type="draftSubType"
      :members="members"
      :currencies="meta?.currencies ?? []"
      :risk-types="meta?.risk_types ?? []"
      :initial="editing"
      :saving="saving"
      :error="formError"
      @close="assetOpen = false"
      @submit="submitAsset"
      @create-member="handleCreateMember"
    />

    <PrivacyConsentDialog
      :open="consentGate"
      :version="meta?.consent_policy_version ?? ''"
      :saving="accepting"
      :error="metaError"
      @accept="handleAccept"
    />
  </div>
</template>
