<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Eye, EyeOff, Plus } from 'lucide-vue-next'
import AssetFormDialog from '../components/ledger/AssetFormDialog.vue'
import DeletionDialog from '../components/ledger/DeletionDialog.vue'
import FixedAssetPickerDialog from '../components/ledger/FixedAssetPickerDialog.vue'
import LedgerConfirmDialog from '../components/ledger/LedgerConfirmDialog.vue'
import LedgerFilterBar from '../components/ledger/LedgerFilterBar.vue'
import LedgerItemTable from '../components/ledger/LedgerItemTable.vue'
import LedgerPager from '../components/ledger/LedgerPager.vue'
import LedgerPrivacyPanel from '../components/ledger/LedgerPrivacyPanel.vue'
import PrivacyConsentDialog from '../components/ledger/PrivacyConsentDialog.vue'
import ReauthDialog from '../components/ledger/ReauthDialog.vue'
import {
  createItem,
  createMember,
  deleteItem,
  errorMessage,
  updateItem,
  type LedgerCategory,
  type LedgerItem,
  type LedgerItemInput,
} from '../api/ledger'
import { useLedgerItems } from '../composables/useLedgerItems'
import { useLedgerMeta } from '../composables/useLedgerMeta'
import { useLedgerPrivacy } from '../composables/useLedgerPrivacy'
import { useLedgerPrivacyActions } from '../composables/useLedgerPrivacyActions'

const route = useRoute()
const { meta, members, accepting, error: metaError, consentRequired, load, loadMembers, accept } = useLedgerMeta()
const { hidden, toggle: toggleHidden, money } = useLedgerPrivacy()
const { category, keyword, memberFilter, sort, page, items, total, sumCents, counts, loading, error, pageCount, start, reload, reloadAfterRemove } =
  useLedgerItems((route.query.category as string) ?? '')

const consentGate = computed(() => !!meta.value && consentRequired.value)

const formOpen = ref(false)
const editing = ref<LedgerItem | null>(null)
const formInitial = ref<{ category?: LedgerCategory; sub_type?: string } | null>(null)

const saving = ref(false)
const formError = ref('')
const pickerOpen = ref(false)

const removeTarget = ref<LedgerItem | null>(null)
const removing = ref(false)
const removeError = ref('')

const {
  exporting,
  deletion,
  deletionOpen,
  deletionBusy,
  deletionError,
  error: privacyError,
  reauthOpen,
  reauthAction,
  loadDeletion,
  handleExport,
  handleDeletion,
  handleCancelDeletion,
  onReauthSuccess,
} = useLedgerPrivacyActions()

onMounted(bootstrap)

async function bootstrap() {
  await load()
  if (consentRequired.value) return
  await Promise.all([start(), loadDeletion()])
}

async function handleAccept() {
  const ok = await accept()
  if (ok) await Promise.all([start(), loadDeletion()])
}

function openCreate() {
  editing.value = null
  formInitial.value = { category: (category.value || 'liquid') as LedgerCategory }
  formError.value = ''
  formOpen.value = true
}

function openEdit(item: LedgerItem) {
  editing.value = item
  formInitial.value = { category: item.category, sub_type: item.sub_type }
  formError.value = ''
  formOpen.value = true
}

function pickFixed(subType: string) {
  pickerOpen.value = false
  editing.value = null
  formInitial.value = { category: 'fixed', sub_type: subType }
  formError.value = ''
  formOpen.value = true
}

async function submitItem(input: LedgerItemInput) {
  saving.value = true
  formError.value = ''
  try {
    if (editing.value) await updateItem(editing.value.id, input, editing.value.version)
    else await createItem(input)
    formOpen.value = false
    editing.value = null
    await reload()
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

async function confirmRemove() {
  if (!removeTarget.value) return
  const removedId = removeTarget.value.id
  removing.value = true
  removeError.value = ''
  try {
    await deleteItem(removedId)
    removeTarget.value = null
    await reloadAfterRemove(removedId)
  } catch (err) {
    removeError.value = errorMessage(err, '删除失败，请稍后重试')
  } finally {
    removing.value = false
  }
}

</script>

<template>
  <div class="min-h-screen bg-[#F7F8FA]">
    <div class="mx-auto max-w-6xl px-6 py-8">
      <header class="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 class="text-2xl font-semibold tracking-tight text-ink">资产明细</h1>
          <p class="mt-1.5 text-sm text-ink-soft">逐笔登记家庭资产与负债，金额仅以密文形式落库。</p>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="rounded-xl p-2.5 text-ink-soft transition-colors hover:bg-white hover:text-ledger-ink cursor-pointer"
            :aria-label="hidden ? '显示金额' : '隐藏金额'"
            data-test="items-eye"
            @click="toggleHidden"
          >
            <EyeOff v-if="hidden" class="h-4 w-4" />
            <Eye v-else class="h-4 w-4" />
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-xl border border-slate-200 bg-white px-3.5 py-2.5 text-sm font-medium text-ink-soft transition-colors hover:border-ledger-fixed hover:text-ledger-ink cursor-pointer"
            data-test="items-fixed-picker"
            @click="pickerOpen = true"
          >
            <Plus class="h-4 w-4" />
            固定资产
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-xl bg-ledger-ink px-4 py-2.5 text-sm font-semibold text-white shadow-md shadow-slate-500/20 transition-colors hover:bg-[#2B3A5C] cursor-pointer"
            data-test="add-item"
            @click="openCreate"
          >
            <Plus class="h-4 w-4" />
            添加资产
          </button>
        </div>
      </header>

      <div class="mt-6">
        <LedgerFilterBar
          :meta="meta"
          :members="members"
          :category="category"
          :keyword="keyword"
          :member-filter="memberFilter"
          :sort="sort"
          :counts="counts"
          @update:category="category = $event"
          @update:keyword="keyword = $event"
          @update:memberFilter="memberFilter = $event"
          @update:sort="sort = $event"
        />
      </div>

      <p
        v-if="error || privacyError"
        class="mt-4 rounded-xl border border-red-100 bg-red-50 px-4 py-3 text-sm text-ledger-up"
        data-test="items-error"
      >
        {{ error || privacyError }}
      </p>

      <div v-if="loading" class="mt-4 space-y-2" data-test="items-skeleton">
        <div v-for="i in 5" :key="i" class="h-14 animate-pulse rounded-xl bg-white" />
      </div>

      <div v-else-if="!items.length" class="mt-4 rounded-2xl border border-dashed border-slate-200 bg-white px-6 py-14 text-center">
        <p class="text-sm text-ink-soft">还没有记录，先添加第一笔资产</p>
        <button
          type="button"
          class="mt-3 inline-flex items-center gap-1.5 rounded-xl bg-ledger-ink px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[#2B3A5C] cursor-pointer"
          @click="openCreate"
        >
          <Plus class="h-4 w-4" />
          添加资产
        </button>
      </div>

      <template v-else>
        <div class="mt-4">
          <LedgerItemTable
            :items="items"
            :meta="meta"
            :hidden="hidden"
            :busy-id="removing ? (removeTarget?.id ?? null) : null"
            @edit="openEdit"
            @remove="removeTarget = $event"
          />
        </div>

        <LedgerPager
          :total="total"
          :sum-cents="sumCents"
          :page="page"
          :page-count="pageCount"
          @prev="page -= 1"
          @next="page += 1"
        />
      </template>

      <LedgerPrivacyPanel
        :deletion="deletion"
        :exporting="exporting"
        @export="handleExport"
        @open-deletion="deletionOpen = true"
        @cancel-deletion="handleCancelDeletion"
      />

      <p class="mt-6 text-center text-[11px] text-ink-faint">
        金额不进浏览器存储，页面切到后台会自动隐藏；删除与导出都会写入审计日志。
      </p>
    </div>

    <LedgerConfirmDialog
      v-if="removeTarget"
      title="删除这条记录？"
      :message="`「${removeTarget.name}」将被删除，当月资产快照会同步重算。该操作不可撤销。`"
      confirm-text="确认删除"
      danger
      :busy="removing"
      :error="removeError"
      @close="removeTarget = null"
      @confirm="confirmRemove"
    />

    <PrivacyConsentDialog
      :open="consentGate"
      :version="meta?.consent_policy_version ?? ''"
      :saving="accepting"
      :error="metaError"
      @accept="handleAccept"
    />
    <FixedAssetPickerDialog :open="pickerOpen" :meta="meta" @close="pickerOpen = false" @pick="pickFixed" />
    <AssetFormDialog
      :open="formOpen"
      :mode="editing ? 'edit' : 'create'"
      :meta="meta"
      :members="members"
      :initial="formInitial ?? {}"
      :saving="saving"
      :error="formError"
      @close="formOpen = false"
      @submit="submitItem"
      @create-member="handleCreateMember"
    />
    <DeletionDialog
      :open="deletionOpen"
      :deletion="deletion"
      :busy="deletionBusy"
      :error="deletionError"
      @close="deletionOpen = false"
      @request="handleDeletion"
      @cancel="handleCancelDeletion"
    />
    <ReauthDialog :open="reauthOpen" :action="reauthAction" @close="reauthOpen = false" @success="onReauthSuccess" />
  </div>
</template>
