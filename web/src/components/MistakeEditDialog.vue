<script setup lang="ts">
// 错题编辑弹窗：修正年级、错误原因、来源，并维护重做（复习）记录。
//
// 服务端 PUT /mistakes/:id 会整条覆盖保存，因此提交时需带上 user_id /
// question_id / recorded_at 等不可丢字段（否则会被写成零值），这里基于原记录构造。
// 年级挂在关联的 Question 上，走 PUT /questions/:id（同样整条覆盖，需带全题目字段）。
import { computed, ref, watch } from 'vue'
import { X, Plus, Trash2, Loader2, Check, ClipboardList } from 'lucide-vue-next'
import { updateMistake, updateQuestion, GRADE_OPTIONS, type Mistake, type ReviewRecord } from '../api'
import StemText from './StemText.vue'

const props = defineProps<{
  open: boolean
  mistake: Mistake | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved', mistake: Mistake): void
}>()

// 表单状态：编辑期间不改动原记录，保存成功后再回写列表。
const grade = ref('')
const wrongReason = ref('')
const source = ref('')
const records = ref<{ reviewed_at: string; result: string; wrong_reason: string }[]>([])
const saving = ref(false)
const errorMsg = ref('')

// 打开时的原始年级：年级只在用户改动后才提交，避免历史无年级数据被误清。
const initialGrade = ref('')

// 年级下拉选项：历史数据可能存有枚举外的自由文本（早期无校验），
// 把它作为额外选项保留，避免编辑时被覆盖丢失。
const gradeOptions = computed(() => {
  const g = initialGrade.value
  return g && !GRADE_OPTIONS.includes(g) ? [g, ...GRADE_OPTIONS] : [...GRADE_OPTIONS]
})

// 重做结果固定选项：统一口径便于统计，避免自由输入出现同义不同写法。
const RESULT_OPTIONS = ['正确', '部分正确', '错误']

// 结果为「正确」时不记录原因；其余结果（含历史自由文本）可补充本次仍出错的原因。
function showRecordReason(result: string): boolean {
  return !!result && result !== '正确'
}

// 历史数据可能存有旧的自由文本结果，把它作为额外选项保留，避免编辑时被覆盖丢失。
function resultOptions(current: string): string[] {
  return current && !RESULT_OPTIONS.includes(current) ? [current, ...RESULT_OPTIONS] : RESULT_OPTIONS
}

// 打开（或切换到另一道错题）时用原记录初始化表单。
watch(
  [() => props.open, () => props.mistake],
  ([open, m]) => {
    if (!open || !m) return
    grade.value = m.question?.grade ?? ''
    initialGrade.value = m.question?.grade ?? ''
    wrongReason.value = m.wrong_reason ?? ''
    source.value = m.source ?? ''
    records.value = (m.review_records ?? []).map((r) => ({
      reviewed_at: toDateInput(r.reviewed_at),
      result: r.result ?? '',
      wrong_reason: r.wrong_reason ?? '',
    }))
    saving.value = false
    errorMsg.value = ''
  },
  { immediate: true },
)

// 后端时间为 RFC3339，"date" 输入框只接受 yyyy-MM-dd。
function toDateInput(value: string): string {
  return (value ?? '').slice(0, 10)
}

// yyyy-MM-dd → RFC3339（UTC 零点），后端 time.Time 才能解析。
function toRFC3339(date: string): string {
  return date ? `${date}T00:00:00Z` : ''
}

function todayInput(): string {
  return new Date().toISOString().slice(0, 10)
}

function addRecord() {
  records.value.push({ reviewed_at: todayInput(), result: '', wrong_reason: '' })
}

function removeRecord(index: number) {
  records.value.splice(index, 1)
}

async function handleSave() {
  const current = props.mistake
  if (!current) return
  errorMsg.value = ''
  if (!source.value.trim()) {
    errorMsg.value = '请填写来源'
    return
  }
  if (records.value.some((r) => r.reviewed_at && !r.result.trim())) {
    errorMsg.value = '请选择每次重做的结果'
    return
  }

  // 未填日期的空行不提交（可能只是点了「添加记录」还没填）。
  // 结果为「正确」的记录原因置空；其余结果保留填写的原因（选填）。
  const reviewRecords: ReviewRecord[] = records.value
    .filter((r) => r.reviewed_at)
    .map((r) => ({
      reviewed_at: toRFC3339(r.reviewed_at),
      result: r.result.trim(),
      wrong_reason: r.result === '正确' ? '' : r.wrong_reason.trim(),
    }))

  saving.value = true
  try {
    const updated = await updateMistake(current.id, {
      user_id: current.user_id,
      question_id: current.question_id,
      wrong_reason: wrongReason.value.trim(),
      source: source.value.trim(),
      review_records: reviewRecords,
      recorded_at: current.recorded_at,
    })

    // 年级在 Question 上：仅当用户改动过才提交（后端 PUT /questions/:id 整条覆盖，
    // 必须带上原题目全部字段，避免未传字段被写成零值）。
    let question = current.question
    const gradeChanged = question && grade.value !== initialGrade.value
    if (question && gradeChanged) {
      question = await updateQuestion(question.id, {
        subject: question.subject,
        grade: grade.value,
        stem_text: question.stem_text,
        answer: question.answer,
        analysis: question.analysis,
        question_type: question.question_type,
        image: question.image,
        knowledge_points: question.knowledge_points,
        source: question.source,
      })
    }

    emit('saved', { ...updated, question })
  } catch (err) {
    console.error('更新错题失败', err)
    errorMsg.value = '保存失败，请重试'
  } finally {
    saving.value = false
  }
}

function onKey(e: KeyboardEvent) {
  if (props.open && e.key === 'Escape') emit('close')
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 p-4"
      role="dialog"
      aria-modal="true"
      aria-label="编辑错题"
      tabindex="0"
      @click.self="emit('close')"
      @keydown="onKey"
    >
      <div class="w-full max-w-lg max-h-[90vh] overflow-y-auto rounded-2xl bg-white p-6 shadow-xl">
        <div class="flex items-start justify-between mb-4">
          <div>
            <h2 class="text-lg font-semibold text-ink">编辑错题</h2>
            <p class="text-xs text-ink-faint mt-1">更新年级、错误原因、来源与重做记录</p>
          </div>
          <button
            type="button"
            class="inline-flex items-center justify-center w-8 h-8 rounded-lg text-ink-faint hover:bg-slate-100 transition-colors"
            title="关闭 (Esc)"
            @click="emit('close')"
          >
            <X class="w-4 h-4" />
          </button>
        </div>

        <!-- 题干只读，帮助确认编辑的是哪道题（表格同样渲染成表格，与列表页一致） -->
        <div v-if="mistake?.question?.stem_text" class="rounded-xl bg-surface-muted/50 p-3 mb-4">
          <StemText
            :text="mistake.question.stem_text"
            :clamp="3"
            class="text-sm text-ink-soft"
          />
        </div>

        <div class="space-y-4">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="block text-sm font-medium text-ink-soft mb-1.5">来源 <span class="text-red-500">*</span></label>
              <input
                v-model="source"
                type="text"
                maxlength="128"
                autofocus
                data-testid="edit-source-input"
                class="w-full rounded-xl border border-slate-200 bg-surface-muted/50 px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 focus:border-primary transition-shadow"
                placeholder="如：期中考试 / 练习册 P32"
              />
            </div>

            <div>
              <label class="block text-sm font-medium text-ink-soft mb-1.5">年级</label>
              <select
                v-model="grade"
                data-testid="edit-grade-select"
                class="w-full rounded-xl border border-slate-200 bg-surface-muted/50 px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 focus:border-primary transition-shadow"
              >
                <option disabled value="">未设置</option>
                <option v-for="opt in gradeOptions" :key="opt" :value="opt">{{ opt }}</option>
              </select>
            </div>
          </div>

          <div>
            <label class="block text-sm font-medium text-ink-soft mb-1.5">错误原因</label>
            <textarea
              v-model="wrongReason"
              rows="3"
              maxlength="255"
              data-testid="edit-wrong-reason"
              class="w-full rounded-xl border border-slate-200 bg-surface-muted/50 px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 focus:border-primary transition-shadow resize-none"
              placeholder="如：概念不清 / 计算失误"
            />
          </div>

          <!-- 重做记录：可增删，记录每次重做的时间与结果 -->
          <div>
            <div class="flex items-center justify-between mb-1.5">
              <label class="inline-flex items-center gap-1.5 text-sm font-medium text-ink-soft">
                <ClipboardList class="w-4 h-4 text-primary" /> 重做记录
              </label>
              <button
                type="button"
                class="inline-flex items-center gap-1 rounded-lg text-primary text-xs font-medium hover:bg-blue-50 px-2 py-1 transition-colors"
                @click="addRecord"
              >
                <Plus class="w-3.5 h-3.5" /> 添加记录
              </button>
            </div>

            <p v-if="!records.length" class="text-xs text-ink-faint py-2">
              还没有重做记录，点击「添加记录」记录一次重做
            </p>

            <div v-else class="space-y-2">
              <div
                v-for="(rec, i) in records"
                :key="i"
                class="space-y-1.5"
              >
                <div class="flex items-center gap-2">
                  <input
                    v-model="rec.reviewed_at"
                    type="date"
                    class="w-36 shrink-0 rounded-xl border border-slate-200 bg-surface-muted/50 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
                  />
                  <select
                    v-model="rec.result"
                    class="flex-1 min-w-0 rounded-xl border border-slate-200 bg-surface-muted/50 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 focus:border-primary transition-shadow"
                    :aria-label="`第 ${i + 1} 条重做结果`"
                  >
                    <option disabled value="">请选择结果</option>
                    <option v-for="opt in resultOptions(rec.result)" :key="opt" :value="opt">{{ opt }}</option>
                  </select>
                  <button
                    type="button"
                    class="shrink-0 inline-flex items-center justify-center w-9 h-9 rounded-lg text-ink-faint hover:bg-red-50 hover:text-red-500 transition-colors"
                    :title="`删除第 ${i + 1} 条重做记录`"
                    @click="removeRecord(i)"
                  >
                    <Trash2 class="w-4 h-4" />
                  </button>
                </div>
                <!-- 结果不是「正确」时，可补充本次仍出错的原因（选填）。 -->
                <input
                  v-if="showRecordReason(rec.result)"
                  v-model="rec.wrong_reason"
                  type="text"
                  maxlength="255"
                  class="w-full rounded-xl border border-slate-200 bg-surface-muted/50 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 focus:border-primary transition-shadow"
                  :aria-label="`第 ${i + 1} 条重做错误原因`"
                  placeholder="本次重做仍出错的原因（选填），如：仍混淆公式"
                />
              </div>
            </div>
          </div>
        </div>

        <p v-if="errorMsg" class="mt-4 text-sm text-red-500 text-right">{{ errorMsg }}</p>

        <div class="flex items-center justify-end gap-3 mt-6">
          <button
            type="button"
            class="rounded-xl px-5 py-2.5 text-sm font-medium text-ink-soft border border-slate-200 hover:bg-slate-50 transition-colors"
            @click="emit('close')"
          >
            取消
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-xl bg-primary text-white px-6 py-2.5 text-sm font-medium shadow-lg shadow-blue-500/25 hover:bg-primary-light transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            :disabled="saving || !source.trim()"
            @click="handleSave"
          >
            <Loader2 v-if="saving" class="w-4 h-4 animate-spin" />
            <Check v-else class="w-4 h-4" />
            保存
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
