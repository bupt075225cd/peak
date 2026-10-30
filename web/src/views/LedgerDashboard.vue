<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { ArrowRight, Plus, ShieldCheck } from 'lucide-vue-next'
import LedgerStructureCards from '../components/ledger/LedgerStructureCards.vue'
import LedgerSummaryCard from '../components/ledger/LedgerSummaryCard.vue'
import AssetFormDialog from '../components/ledger/AssetFormDialog.vue'
import FixedAssetPickerDialog from '../components/ledger/FixedAssetPickerDialog.vue'
import LedgerConfirmDialog from '../components/ledger/LedgerConfirmDialog.vue'
import MemberBar from '../components/ledger/MemberBar.vue'
import MemberFormDialog from '../components/ledger/MemberFormDialog.vue'
import MemberManagerDialog from '../components/ledger/MemberManagerDialog.vue'
import PrivacyConsentDialog from '../components/ledger/PrivacyConsentDialog.vue'
import {
  buildSummaryView,
  createItem,
  createMember,
  deleteMember,
  errorMessage,
  getSummary,
  updateMember,
  type LedgerCategory,
  type LedgerItemInput,
  type LedgerMember,
  type LedgerSummary,
  type MemberScope,
} from '../api/ledger'
import { useLedgerMeta } from '../composables/useLedgerMeta'
import { useLedgerPrivacy } from '../composables/useLedgerPrivacy'

const {
  meta,
  members,
  visibleMembers,
  accepting,
  error: metaError,
  consentRequired,
  load,
  loadMembers,
  accept,
} = useLedgerMeta()
const { hidden, toggle: toggleHidden } = useLedgerPrivacy()

const summary = ref<LedgerSummary | null>(null)
const loading = ref(true)
const error = ref('')
const excludeFixed = ref(false)
const privacyOpen = ref(false)

const addOpen = ref(false)
const pickerOpen = ref(false)
const saving = ref(false)
const formError = ref('')
const formInitial = ref<{ category?: LedgerCategory; sub_type?: string } | null>(null)

// 家庭成员：切换条（全部 / 我 / 各成员）+ 管理、新增、编辑、删除。
const memberScope = ref<MemberScope>('')
const managerOpen = ref(false)
const memberFormOpen = ref(false)
const memberRemoveOpen = ref(false)
const editingMember = ref<LedgerMember | null>(null)
const memberSaving = ref(false)
const memberDeleting = ref(false)
const memberError = ref('')

// 视图全部由聚合结果在本地推导：「排除固定资产」无需再请求一次接口。
const view = computed(() => (summary.value ? buildSummaryView(summary.value, excludeFixed.value) : null))
// 全量总资产：主卡里的净资产与负债率始终按全部资产口径展示，不随开关变化。
const fullTotalAsset = computed(() => (summary.value ? buildSummaryView(summary.value, false).totalAssetCents : 0))
const latestUpdatedAt = computed(() => summary.value?.updated_at ?? '')
// 元数据未就绪时不弹同意框，避免出现空版本号的告知书。
const consentGate = computed(() => !!meta.value && consentRequired.value)
const assetGroups = computed(() => (view.value?.groups ?? []).filter((g) => g.kind === 'asset'))
const liabilityTotal = computed(() => view.value?.totalLiabilityCents ?? 0)
const totalAsset = computed(() => view.value?.totalAssetCents ?? 0)

onMounted(bootstrap)

async function bootstrap() {
  loading.value = true
  await load()
  if (metaError.value) error.value = metaError.value
  if (consentRequired.value) {
    loading.value = false
    return
  }
  await loadData()
}

async function loadData() {
  loading.value = true
  error.value = ''
  try {
    summary.value = await getSummary(memberScope.value)
  } catch (err) {
    error.value = errorMessage(err, '加载总览失败，请稍后重试')
  } finally {
    loading.value = false
  }
}

async function handleAccept() {
  const ok = await accept()
  if (ok) await loadData()
}

function openAdd() {
  formInitial.value = { category: 'liquid' }
  formError.value = ''
  addOpen.value = true
}

function openPicker() {
  pickerOpen.value = true
}

function pickFixed(subType: string) {
  pickerOpen.value = false
  formInitial.value = { category: 'fixed', sub_type: subType }
  formError.value = ''
  addOpen.value = true
}

async function submitItem(input: LedgerItemInput) {
  saving.value = true
  formError.value = ''
  try {
    await createItem(input)
    addOpen.value = false
    await loadData()
  } catch (err) {
    formError.value = errorMessage(err, '保存失败，请稍后重试')
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

// ── 家庭成员 ──────────────────────────────────────────────

async function selectScope(scope: MemberScope) {
  if (memberScope.value === scope) return
  memberScope.value = scope
  await loadData()
}

function openManager() {
  memberError.value = ''
  managerOpen.value = true
}

function openAddMember() {
  editingMember.value = null
  memberError.value = ''
  memberFormOpen.value = true
}

function openEditMember(member: LedgerMember) {
  editingMember.value = member
  memberError.value = ''
  memberFormOpen.value = true
}

async function submitMember(payload: { name: string; visible: boolean }) {
  memberSaving.value = true
  memberError.value = ''
  const edited = editingMember.value
  try {
    if (edited) await updateMember(edited.id, { name: payload.name, visible: payload.visible })
    else await createMember(payload.name, payload.visible)
    memberFormOpen.value = false
    editingMember.value = null
    await loadMembers()
    // 被隐藏的成员不再出现在切换条：当前正筛该成员时回到「全部」。
    if (edited && !payload.visible && memberScope.value === String(edited.id)) {
      memberScope.value = ''
      await loadData()
    }
  } catch (err) {
    memberError.value = errorMessage(err, '保存成员失败，请稍后重试')
  } finally {
    memberSaving.value = false
  }
}

function askRemoveMember() {
  memberError.value = ''
  memberRemoveOpen.value = true
}

async function confirmRemoveMember() {
  const target = editingMember.value
  if (!target) return
  memberDeleting.value = true
  memberError.value = ''
  try {
    await deleteMember(target.id)
    memberRemoveOpen.value = false
    memberFormOpen.value = false
    editingMember.value = null
    await loadMembers()
    if (memberScope.value === String(target.id)) {
      memberScope.value = ''
      await loadData()
    }
  } catch (err) {
    memberError.value = errorMessage(err, '删除成员失败，请稍后重试')
  } finally {
    memberDeleting.value = false
  }
}

</script>

<template>
  <div class="min-h-screen bg-[#F7F8FA]">
    <div class="mx-auto max-w-6xl px-6 py-8">
      <header class="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 class="text-2xl font-semibold tracking-tight text-ink">家庭资产总览</h1>
          <p class="mt-1.5 text-sm text-ink-soft">每月更新一次，看清资产结构与家庭净资产的变化。</p>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-xl border border-slate-200 bg-white px-3.5 py-2.5 text-sm font-medium text-ink-soft transition-colors hover:border-ledger-fixed hover:text-ledger-ink cursor-pointer"
            data-test="open-fixed-picker"
            @click="openPicker"
          >
            <Plus class="h-4 w-4" />
            更多固定资产
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-xl bg-ledger-ink px-4 py-2.5 text-sm font-semibold text-white shadow-md shadow-slate-500/20 transition-colors hover:bg-[#2B3A5C] cursor-pointer"
            data-test="open-add-asset"
            @click="openAdd"
          >
            <Plus class="h-4 w-4" />
            添加资产
          </button>
        </div>
      </header>

      <!-- 成员切换：全部 / 我 / 各成员；右侧进入家庭成员管理 -->
      <div v-if="!consentGate" class="mt-6 animate-fade-up">
        <MemberBar
          :members="visibleMembers"
          :active="memberScope"
          @update:active="selectScope"
          @manage="openManager"
        />
      </div>

      <p v-if="error" class="mt-4 rounded-xl border border-red-100 bg-red-50 px-4 py-3 text-sm text-ledger-up" data-test="dashboard-error">
        {{ error }}
      </p>

      <!-- 首次使用：隐私告知入口 -->
      <div
        v-if="consentGate"
        class="mt-6 flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-ledger-ink/15 bg-white px-5 py-4"
        data-test="consent-banner"
      >
        <div class="flex items-start gap-3">
          <span class="flex h-9 w-9 items-center justify-center rounded-xl bg-ledger-mist text-ledger-ink">
            <ShieldCheck class="h-4 w-4" />
          </span>
          <div>
            <p class="text-sm font-medium text-ink">记账前需要你单独同意敏感个人信息处理告知</p>
            <p class="mt-0.5 text-xs text-ink-faint">金额与资产名称属敏感个人信息，将加密存储且仅你本人可见。</p>
          </div>
        </div>
        <button
          type="button"
          class="rounded-xl bg-ledger-ink px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[#2B3A5C] cursor-pointer"
          @click="handleAccept"
        >
          阅读并同意
        </button>
      </div>

      <div v-if="loading" class="mt-6 grid grid-cols-1 gap-4 lg:grid-cols-12" data-test="dashboard-skeleton">
        <div class="h-64 animate-pulse rounded-2xl bg-ledger-mist/70 lg:col-span-7" />
        <div class="h-64 animate-pulse rounded-2xl bg-white lg:col-span-5" />
      </div>

      <div v-else-if="view" class="mt-6 grid grid-cols-1 gap-4 lg:grid-cols-12">
        <!-- 总资产主卡 -->
        <div class="lg:col-span-7 animate-fade-up">
          <LedgerSummaryCard
            :view="view"
            :full-total-asset-cents="fullTotalAsset"
            :updated-at="latestUpdatedAt"
            :has-foreign="summary?.has_foreign ?? false"
            :hidden="hidden"
            @update:exclude-fixed="excludeFixed = $event"
            @toggle-hidden="toggleHidden"
          />
        </div>

        <!-- 资产结构 -->
        <section class="lg:col-span-5 animate-fade-up">
          <div class="flex items-end justify-between">
            <h2 class="text-sm font-semibold text-ink">资产结构</h2>
            <RouterLink to="/ledger/composition" class="inline-flex items-center gap-1 text-xs text-ink-soft hover:text-ledger-ink">
              查看流向图
              <ArrowRight class="h-3.5 w-3.5" />
            </RouterLink>
          </div>
          <div class="mt-3">
            <LedgerStructureCards :groups="assetGroups" :total-asset-cents="totalAsset" />
          </div>
        </section>
      </div>

      <!-- 隐私状态 -->
      <footer class="mt-6 flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-slate-200/70 bg-white px-5 py-3.5">
        <button
          type="button"
          class="inline-flex items-center gap-1.5 text-xs text-ink-soft transition-colors hover:text-ledger-ink cursor-pointer"
          data-test="privacy-badge"
          @click="privacyOpen = !privacyOpen"
        >
          <ShieldCheck class="h-3.5 w-3.5 text-ledger-down" />
          隐私保护已开启
        </button>
        <RouterLink to="/ledger/items" class="text-xs text-ink-faint hover:text-ink-soft">数据导出与注销</RouterLink>
      </footer>
      <div v-if="privacyOpen" class="mt-2 rounded-2xl border border-slate-200/70 bg-white px-5 py-4 text-xs leading-relaxed text-ink-soft animate-fade-up">
        金额、名称、备注与成员昵称均为 AES-256-GCM 密文，密钥按用户独立派生且不落库；业务表只保存不可逆伪名。
        注销账本后密钥被销毁，密文永久不可解。浏览器不会保存任何金额，切到后台会自动隐藏。
      </div>
    </div>

    <PrivacyConsentDialog
      :open="consentGate"
      :version="meta?.consent_policy_version ?? ''"
      :saving="accepting"
      :error="metaError"
      @accept="handleAccept"
    />
    <FixedAssetPickerDialog :open="pickerOpen" :meta="meta" @close="pickerOpen = false" @pick="pickFixed" />
    <AssetFormDialog
      :open="addOpen"
      mode="create"
      :meta="meta"
      :members="members"
      :initial="formInitial ?? {}"
      :saving="saving"
      :error="formError"
      @close="addOpen = false"
      @submit="submitItem"
      @create-member="handleCreateMember"
    />

    <MemberManagerDialog
      :open="managerOpen"
      :members="members"
      @close="managerOpen = false"
      @add="openAddMember"
      @edit="openEditMember"
    />
    <MemberFormDialog
      :open="memberFormOpen"
      :mode="editingMember ? 'edit' : 'create'"
      :member="editingMember"
      :saving="memberSaving"
      :deleting="memberDeleting"
      :error="memberError"
      @close="memberFormOpen = false"
      @submit="submitMember"
      @remove="askRemoveMember"
    />
    <LedgerConfirmDialog
      v-if="memberRemoveOpen && editingMember"
      title="删除该成员？"
      :message="`「${editingMember.name}」将被删除，历史记录仍保留成员昵称快照，不会丢失数据。`"
      confirm-text="确认删除"
      danger
      :busy="memberDeleting"
      :error="memberError"
      @close="memberRemoveOpen = false"
      @confirm="confirmRemoveMember"
    />
  </div>
</template>
