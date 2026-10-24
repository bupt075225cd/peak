<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Download, RefreshCw } from 'lucide-vue-next'
import CompositionChart from '../components/ledger/CompositionChart.vue'
import PrivacyConsentDialog from '../components/ledger/PrivacyConsentDialog.vue'
import { COMPOSITION_LEGEND, type CompositionMode } from '../components/ledger/ledgerChartOptions'
import { buildSummaryView, errorMessage, getSummary, type LedgerSummary } from '../api/ledger'
import { useLedgerMeta } from '../composables/useLedgerMeta'
import { isLedgerHidden, useLedgerPrivacy } from '../composables/useLedgerPrivacy'
import { debtRatio, formatPercent, MONEY_MASK } from '../utils/money'
import { dataURLToBlob, downloadBlob, ledgerMark, watermarkDataURL } from '../utils/watermark'

const { meta, accepting, error: metaError, consentRequired, load, accept } = useLedgerMeta()
const { hidden, hide, show, money } = useLedgerPrivacy()

const summary = ref<LedgerSummary | null>(null)
const loading = ref(true)
const error = ref('')
const excludeFixed = ref(false)
const mode = ref<CompositionMode>(isLedgerHidden() ? 'hidden' : 'amount')
const exporting = ref(false)
const chartRef = ref<InstanceType<typeof CompositionChart> | null>(null)

const consentGate = computed(() => !!meta.value && consentRequired.value)
const view = computed(() => (summary.value ? buildSummaryView(summary.value, excludeFixed.value) : null))
const ratioText = computed(() => {
  if (!view.value) return '0.00%'
  if (hidden.value) return MONEY_MASK
  return formatPercent(debtRatio(view.value.totalLiabilityCents, view.value.totalAssetCents))
})

const MODES: { key: CompositionMode; label: string }[] = [
  { key: 'amount', label: '金额' },
  { key: 'ratio', label: '比例' },
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
    summary.value = await getSummary()
  } catch (err) {
    error.value = errorMessage(err, '加载资产组成失败')
  } finally {
    loading.value = false
  }
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
      <header class="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 class="text-2xl font-semibold tracking-tight text-ink">资产组成</h1>
          <p class="mt-1.5 text-sm text-ink-soft">从净资产与负债出发，看钱都分布在哪些资产类别里。</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <div class="flex items-center rounded-xl bg-white p-1 ring-1 ring-slate-200">
            <button
              type="button"
              class="rounded-lg px-3 py-1.5 text-xs transition-colors cursor-pointer"
              :class="!excludeFixed ? 'bg-ledger-mist text-ledger-ink' : 'text-ink-soft hover:text-ink'"
              data-test="scope-all"
              @click="excludeFixed = false"
            >
              全部资产
            </button>
            <button
              type="button"
              class="rounded-lg px-3 py-1.5 text-xs transition-colors cursor-pointer"
              :class="excludeFixed ? 'bg-ledger-mist text-ledger-ink' : 'text-ink-soft hover:text-ink'"
              data-test="scope-exclude-fixed"
              @click="excludeFixed = true"
            >
              排除固定资产
            </button>
          </div>
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
          <button
            type="button"
            class="rounded-xl border border-slate-200 bg-white p-2.5 text-ink-soft transition-colors hover:text-ledger-ink disabled:cursor-not-allowed disabled:opacity-50 cursor-pointer"
            aria-label="刷新"
            :disabled="loading"
            data-test="refresh-composition"
            @click="loadSummary"
          >
            <RefreshCw class="h-4 w-4" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-xl bg-ledger-ink px-3.5 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[#2B3A5C] disabled:cursor-not-allowed disabled:bg-slate-300 cursor-pointer"
            :disabled="exporting || !summary"
            data-test="download-composition"
            @click="handleDownload"
          >
            <Download class="h-4 w-4" />
            {{ exporting ? '生成中…' : '下载图片' }}
          </button>
        </div>
      </header>

      <p v-if="error" class="mt-4 rounded-xl border border-red-100 bg-red-50 px-4 py-3 text-sm text-ledger-up" data-test="composition-error">
        {{ error }}
      </p>

      <div class="mt-6 grid grid-cols-1 gap-4 lg:grid-cols-12">
        <section class="rounded-2xl border border-slate-200/70 bg-white p-4 shadow-[0_1px_2px_rgba(15,23,42,0.04)] lg:col-span-9">
          <div v-if="view" class="rounded-xl bg-slate-50/40">
            <CompositionChart ref="chartRef" :view="view" :mode="mode" :loading="loading" />
          </div>
          <div v-else class="h-[560px] animate-pulse rounded-xl bg-slate-100" />

          <div class="mt-3 flex flex-wrap items-center justify-between gap-3 border-t border-slate-100 pt-3">
            <div class="flex flex-wrap items-center gap-3">
              <span v-for="item in COMPOSITION_LEGEND" :key="item.key" class="flex items-center gap-1.5 text-[11px] text-ink-soft">
                <span class="h-2 w-2 rounded-full" :style="{ backgroundColor: item.color }" />
                {{ item.label }}
              </span>
            </div>
            <p class="text-[11px] text-ink-faint">金额按录入币种面值计入，不含汇率换算</p>
          </div>
        </section>

        <aside class="space-y-3 lg:col-span-3">
          <div class="rounded-2xl border border-slate-200/70 bg-white p-4">
            <p class="text-[11px] text-ink-faint">总资产</p>
            <p class="mt-1 text-xl font-semibold tabular-nums text-ink" data-test="composition-total">
              {{ view ? money(view.totalAssetCents) : MONEY_MASK }}
            </p>
          </div>
          <div class="rounded-2xl border border-slate-200/70 bg-white p-4">
            <p class="text-[11px] text-ink-faint">总负债</p>
            <p class="mt-1 text-xl font-semibold tabular-nums text-ink" data-test="composition-liability">
              {{ view ? money(view.totalLiabilityCents) : MONEY_MASK }}
            </p>
          </div>
          <div class="rounded-2xl border border-ledger-ink/12 bg-ledger-mist/70 p-4">
            <p class="text-[11px] text-ink-faint">净资产</p>
            <p class="mt-1 text-xl font-semibold tabular-nums text-ledger-ink" data-test="composition-net">
              {{ view ? money(view.netAssetCents) : MONEY_MASK }}
            </p>
            <p class="mt-2 text-[11px] text-ink-soft">负债率 {{ ratioText }}</p>
          </div>
          <p class="px-1 text-[11px] leading-relaxed text-ink-faint">
            图内明细名称已按隐私策略隐藏，仅展示分类与子类聚合；下载的图片附带会话伪名水印，便于外泄溯源。
          </p>
        </aside>
      </div>
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
