<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Download } from 'lucide-vue-next'
import CompositionChart from '../components/ledger/CompositionChart.vue'
import MemberSelect from '../components/ledger/MemberSelect.vue'
import PrivacyConsentDialog from '../components/ledger/PrivacyConsentDialog.vue'
import type { CompositionMode } from '../components/ledger/ledgerChartOptions'
import { buildSummaryView, errorMessage, getSummary, type LedgerSummary, type MemberScope } from '../api/ledger'
import { useLedgerMeta } from '../composables/useLedgerMeta'
import { isLedgerHidden, useLedgerPrivacy } from '../composables/useLedgerPrivacy'
import { dataURLToBlob, downloadBlob, ledgerMark, watermarkDataURL } from '../utils/watermark'

const { meta, visibleMembers, accepting, error: metaError, consentRequired, load, accept } = useLedgerMeta()
const { hidden, hide, show } = useLedgerPrivacy()

// 成员范围：全部 / 我 / 各成员；切换后按同一口径重新聚合再渲染桑基图。
const memberScope = ref<MemberScope>('')

const summary = ref<LedgerSummary | null>(null)
const loading = ref(true)
const error = ref('')
const mode = ref<CompositionMode>(isLedgerHidden() ? 'hidden' : 'ratio')
const exporting = ref(false)
const chartRef = ref<InstanceType<typeof CompositionChart> | null>(null)

const consentGate = computed(() => !!meta.value && consentRequired.value)
const view = computed(() => (summary.value ? buildSummaryView(summary.value, false) : null))

// 顺序与默认态对齐参考稿：比例在前并默认选中。
const MODES: { key: CompositionMode; label: string }[] = [
  { key: 'ratio', label: '比例' },
  { key: 'amount', label: '金额' },
  { key: 'hidden', label: '隐藏金额' },
]

onMounted(bootstrap)

async function bootstrap() {
  loading.value = true
  await load()
  if (metaError.value) error.value = metaError.value
  if (consentRequired.value) {
    loading.value = false
    return
  }
  await loadSummary()
}

async function loadSummary() {
  loading.value = true
  error.value = ''
  try {
    summary.value = await getSummary(memberScope.value)
  } catch (err) {
    error.value = errorMessage(err, '加载资产组成失败')
  } finally {
    loading.value = false
  }
}

async function selectScope(scope: MemberScope) {
  if (memberScope.value === scope) return
  memberScope.value = scope
  await loadSummary()
}

async function handleAccept() {
  const ok = await accept()
  if (ok) await loadSummary()
}

// 隐藏金额与其它页面共享同一开关：选中隐藏态时同步隐藏表格与总览里的金额。
function setMode(next: CompositionMode) {
  mode.value = next
  if (next === 'hidden') hide()
  else if (hidden.value) show()
}

async function handleDownload() {
  if (!chartRef.value || exporting.value) return
  exporting.value = true
  try {
    const dataUrl = chartRef.value.toDataURL()
    if (!dataUrl) {
      error.value = '图表尚未绘制完成，请稍后再试'
      return
    }
    const marked = await watermarkDataURL(dataUrl, ledgerMark())
    downloadBlob(dataURLToBlob(marked), `资产组成-${new Date().toISOString().slice(0, 7)}.png`)
  } catch (err) {
    console.error('导出图片失败', err)
    error.value = '导出图片失败，请稍后重试'
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-[#F7F8FA]">
    <div class="mx-auto max-w-6xl px-6 py-8">
      <header class="flex flex-wrap items-center justify-between gap-4">
        <div class="flex items-center gap-3">
          <h1 class="text-2xl font-semibold tracking-tight text-ink">家庭资产组成</h1>
          <MemberSelect
            v-if="!consentGate"
            :model-value="memberScope"
            :members="visibleMembers"
            @update:model-value="selectScope"
          />
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <div class="flex items-center rounded-xl bg-white p-1 ring-1 ring-slate-200">
            <button
              v-for="item in MODES"
              :key="item.key"
              type="button"
              class="rounded-lg px-3 py-1.5 text-xs transition-colors cursor-pointer"
              :class="mode === item.key ? 'bg-ledger-ink text-white' : 'text-ink-soft hover:text-ink'"
              :data-test="`mode-${item.key}`"
              @click="setMode(item.key)"
            >
              {{ item.label }}
            </button>
          </div>
          <!-- 只保留图标（对齐参考稿），文案通过 aria-label / title 提供给读屏与悬停。 -->
          <button
            type="button"
            class="inline-flex h-9 w-9 items-center justify-center rounded-xl bg-ledger-ink text-white transition-colors hover:bg-[#2B3A5C] disabled:cursor-not-allowed disabled:bg-slate-300 cursor-pointer"
            :disabled="exporting || !summary"
            :aria-label="exporting ? '正在生成图片' : '下载图片'"
            :title="exporting ? '正在生成图片' : '下载图片'"
            :aria-busy="exporting"
            data-test="download-composition"
            @click="handleDownload"
          >
            <Download class="h-4 w-4" />
          </button>
        </div>
      </header>

      <p v-if="error" class="mt-4 rounded-xl border border-red-100 bg-red-50 px-4 py-3 text-sm text-ledger-up" data-test="composition-error">
        {{ error }}
      </p>

      <section class="mt-6 rounded-2xl border border-slate-200/70 bg-white p-4 shadow-[0_1px_2px_rgba(15,23,42,0.04)]">
        <!-- 负债/总资产方框由 CompositionChart 渲染，与图内竖条垂直对齐 -->
        <div v-if="view">
          <CompositionChart ref="chartRef" :view="view" :mode="mode" :loading="loading" />
        </div>
        <div v-else class="h-[620px] animate-pulse rounded-xl bg-slate-100" />
      </section>
    </div>

    <PrivacyConsentDialog
      :open="consentGate"
      :version="meta?.consent_policy_version ?? ''"
      :saving="accepting"
      :error="metaError"
      @accept="handleAccept"
    />
  </div>
</template>
