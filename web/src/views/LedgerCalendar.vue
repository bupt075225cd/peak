<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import VChart from 'vue-echarts'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import PrivacyConsentDialog from '../components/ledger/PrivacyConsentDialog.vue'
import SnapshotCalendar from '../components/ledger/SnapshotCalendar.vue'
import { buildTrendOption, ensureLedgerECharts } from '../components/ledger/ledgerChartOptions'
import { errorMessage, listSnapshots, type LedgerSnapshot } from '../api/ledger'
import { useLedgerMeta } from '../composables/useLedgerMeta'
import { useLedgerPrivacy } from '../composables/useLedgerPrivacy'
import { formatPercent, MONEY_MASK, share } from '../utils/money'

ensureLedgerECharts()

const { meta, accepting, error: metaError, consentRequired, load, accept } = useLedgerMeta()
const { hidden, money } = useLedgerPrivacy()

const now = new Date()
const currentPeriod = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
const year = ref(now.getFullYear())
const snapshots = ref<LedgerSnapshot[]>([])
const selected = ref(currentPeriod)
const loading = ref(true)
const error = ref('')

const consentGate = computed(() => !!meta.value && consentRequired.value)
const isCurrentYear = computed(() => year.value === now.getFullYear())

// 补齐 12 个月：没有快照的月份保留占位，避免网格随数据缺月而变形。
const months = computed(() => {
  const byPeriod = new Map(snapshots.value.map((item) => [item.period, item]))
  return Array.from({ length: 12 }, (_, index) => {
    const period = `${year.value}-${String(index + 1).padStart(2, '0')}`
    return { period, snapshot: byPeriod.get(period) ?? null }
  })
})

const selectedSnapshot = computed(() => months.value.find((m) => m.period === selected.value)?.snapshot ?? null)
const latest = computed(() => {
  const filled = months.value.filter((m) => m.snapshot)
  return filled.length ? filled[filled.length - 1].snapshot : null
})
const yearTotalAsset = computed(() => latest.value?.payload.total_asset_cents ?? 0)
const yearTotalLiability = computed(() => latest.value?.payload.total_liability_cents ?? 0)
const yearNet = computed(() => latest.value?.payload.net_asset_cents ?? 0)
const trendOption = computed(() => buildTrendOption(snapshots.value, hidden.value))

const breakdown = computed(() => {
  const rows = selectedSnapshot.value?.payload.breakdown ?? []
  const total = rows.filter((r) => r.category !== 'liability').reduce((sum, r) => sum + r.amount_cents, 0)
  return [...rows]
    .sort((a, b) => b.amount_cents - a.amount_cents)
    .map((row) => ({
      ...row,
      color: meta.value?.categories.find((c) => c.key === row.category)?.color ?? '#CBD5E1',
      ratio: row.category === 'liability' ? 0 : share(row.amount_cents, total),
    }))
})

onMounted(bootstrap)

async function bootstrap() {
  loading.value = true
  await load()
  if (metaError.value) error.value = metaError.value
  if (consentRequired.value) {
    loading.value = false
    return
  }
  await loadYear()
}

async function loadYear() {
  loading.value = true
  error.value = ''
  try {
    snapshots.value = await listSnapshots(String(year.value))
    const available = snapshots.value.map((s) => s.period)
    if (available.length && !available.includes(selected.value)) selected.value = available[available.length - 1]
  } catch (err) {
    error.value = errorMessage(err, '加载资产月历失败')
  } finally {
    loading.value = false
  }
}

async function handleAccept() {
  const ok = await accept()
  if (ok) await loadYear()
}

async function shiftYear(step: number) {
  const next = year.value + step
  if (next > now.getFullYear() || next < 2000) return
  year.value = next
  selected.value = next === now.getFullYear() ? currentPeriod : `${next}-01`
  await loadYear()
}
</script>

<template>
  <div class="min-h-screen bg-[#F7F8FA]">
    <div class="mx-auto max-w-6xl px-6 py-8">
      <header class="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 class="text-2xl font-semibold tracking-tight text-ink">资产月历</h1>
          <p class="mt-1.5 text-sm text-ink-soft">每月一张快照，回看净资产与分类构成的推移。</p>
        </div>
        <div class="flex items-center gap-2 rounded-xl bg-white p-1 ring-1 ring-slate-200">
          <button
            type="button"
            class="rounded-lg p-1.5 text-ink-soft transition-colors hover:bg-slate-50 hover:text-ledger-ink cursor-pointer"
            aria-label="上一年"
            data-test="prev-year"
            @click="shiftYear(-1)"
          >
            <ChevronLeft class="h-4 w-4" />
          </button>
          <span class="min-w-16 text-center text-sm font-medium tabular-nums text-ink" data-test="current-year">{{ year }} 年</span>
          <button
            type="button"
            class="rounded-lg p-1.5 text-ink-soft transition-colors hover:bg-slate-50 hover:text-ledger-ink disabled:cursor-not-allowed disabled:opacity-30 cursor-pointer"
            aria-label="下一年"
            :disabled="isCurrentYear"
            data-test="next-year"
            @click="shiftYear(1)"
          >
            <ChevronRight class="h-4 w-4" />
          </button>
        </div>
      </header>

      <p v-if="error" class="mt-4 rounded-xl border border-red-100 bg-red-50 px-4 py-3 text-sm text-ledger-up" data-test="calendar-error">
        {{ error }}
      </p>

      <section class="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-3">
        <div class="rounded-2xl border border-slate-200/70 bg-white p-5">
          <p class="text-[11px] text-ink-faint">{{ year }} 年净资产（最新月）</p>
          <p class="mt-1.5 text-xl font-semibold tabular-nums text-ledger-ink" data-test="year-net">{{ money(yearNet) }}</p>
        </div>
        <div class="rounded-2xl border border-slate-200/70 bg-white p-5">
          <p class="text-[11px] text-ink-faint">总资产</p>
          <p class="mt-1.5 text-xl font-semibold tabular-nums text-ink" data-test="year-total">{{ money(yearTotalAsset) }}</p>
        </div>
        <div class="rounded-2xl border border-slate-200/70 bg-white p-5">
          <p class="text-[11px] text-ink-faint">总负债</p>
          <p class="mt-1.5 text-xl font-semibold tabular-nums text-ink" data-test="year-liability">{{ money(yearTotalLiability) }}</p>
        </div>
      </section>

      <div v-if="loading" class="mt-4 grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-4" data-test="calendar-skeleton">
        <div v-for="i in 8" :key="i" class="h-32 animate-pulse rounded-2xl bg-white" />
      </div>

      <div v-else class="mt-4">
        <SnapshotCalendar
          :months="months"
          :meta="meta"
          :selected="selected"
          :current-period="currentPeriod"
          @select="selected = $event"
        />
      </div>

      <div class="mt-6 grid grid-cols-1 gap-4 lg:grid-cols-12">
        <section class="rounded-2xl border border-slate-200/70 bg-white p-5 lg:col-span-5">
          <h2 class="text-sm font-semibold text-ink">
            {{ selected }} 分类构成
          </h2>
          <p v-if="!selectedSnapshot" class="mt-6 text-xs text-ink-faint">该月还没有快照，添加或修改条目后会自动生成。</p>
          <ul v-else class="mt-4 space-y-3">
            <li v-for="row in breakdown" :key="row.category">
              <div class="flex items-center justify-between text-xs">
                <span class="flex items-center gap-1.5 text-ink-soft">
                  <span class="h-2 w-2 rounded-full" :style="{ backgroundColor: row.color }" />
                  {{ row.label }}
                  <span class="text-ink-faint">{{ row.count }} 项</span>
                </span>
                <span class="tabular-nums text-ink">{{ money(row.amount_cents) }}</span>
              </div>
              <div class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-slate-100">
                <div class="h-full rounded-full" :style="{ width: `${Math.min(100, row.ratio * 100)}%`, backgroundColor: row.color }" />
              </div>
              <p class="mt-1 text-[11px] text-ink-faint">
                {{ row.category === 'liability' ? '负债项，不计入资产占比' : hidden ? MONEY_MASK : formatPercent(row.ratio) }}
              </p>
            </li>
          </ul>
        </section>

        <section class="rounded-2xl border border-slate-200/70 bg-white p-5 lg:col-span-7">
          <div class="flex items-baseline justify-between">
            <h2 class="text-sm font-semibold text-ink">近 12 个月净资产趋势</h2>
            <span class="text-[11px] text-ink-faint">{{ snapshots.length ? `共 ${snapshots.length} 个月快照` : '暂无数据' }}</span>
          </div>
          <div v-if="snapshots.length" class="mt-3 h-64">
            <VChart class="h-full w-full" :option="trendOption" autoresize />
          </div>
          <div v-else class="mt-3 flex h-64 items-center justify-center rounded-xl bg-slate-50 text-xs text-ink-faint">
            还没有月度快照，先在资产明细中登记一笔资产
          </div>
          <p class="mt-2 text-[11px] leading-relaxed text-ink-faint">
            快照在条目变更时自动重算；历史月份的快照只读，不会被后续编辑改写。
          </p>
        </section>
      </div>

      <p class="mt-6 text-center text-[11px] text-ink-faint">
        月历中的金额与分类构成均为密文解密后的即时结果，不会写入浏览器存储。
      </p>
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
