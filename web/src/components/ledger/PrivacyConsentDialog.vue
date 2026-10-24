<script setup lang="ts">
import { ref, watch } from 'vue'
import { Lock, ShieldCheck } from 'lucide-vue-next'

const props = defineProps<{
  open: boolean
  version: string
  saving: boolean
  error: string
}>()

const emit = defineEmits<{
  (e: 'accept'): void
}>()

const agreed = ref(false)

watch(
  () => props.open,
  (open) => {
    if (open) agreed.value = false
  },
)

const points = [
  '金额、资产名称、备注与成员昵称均以 AES-256-GCM 密文存储，数据库中没有明文。',
  '业务表只保存不可逆伪名，不保存手机号、邮箱或任何可直接识别你的标识。',
  '只收集记账必需的信息；请勿在名称或备注中填写身份证号、银行卡号。',
  '你可以随时导出全部数据，或注销账本；注销冷静期结束后密文将永久不可解密。',
]
</script>

<template>
  <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center bg-ink/40 p-4 backdrop-blur-sm">
    <div class="w-full max-w-lg rounded-2xl border border-slate-200/80 bg-white p-6 shadow-2xl animate-fade-up">
      <div class="flex items-center gap-2">
        <span class="flex h-9 w-9 items-center justify-center rounded-xl bg-ledger-mist text-ledger-ink">
          <ShieldCheck class="h-4 w-4" />
        </span>
        <div>
          <h2 class="text-lg font-semibold text-ink">敏感个人信息处理告知</h2>
          <p class="text-[11px] text-ink-faint">政策版本 {{ version }}</p>
        </div>
      </div>

      <p class="mt-4 text-xs leading-relaxed text-ink-soft">
        家庭资产与负债属于《个人信息保护法》定义的敏感个人信息（金融账户）。为向你提供记账、
        资产组成分析与月度回顾功能，我们需要处理你主动填写的金额、资产名称与成员标签。
      </p>

      <ul class="mt-3 space-y-2">
        <li v-for="point in points" :key="point" class="flex gap-2 text-xs leading-relaxed text-ink-soft">
          <Lock class="mt-0.5 h-3.5 w-3.5 shrink-0 text-ledger-down" />
          <span>{{ point }}</span>
        </li>
      </ul>

      <label class="mt-4 flex cursor-pointer items-start gap-2 rounded-xl bg-slate-50 p-3">
        <input v-model="agreed" type="checkbox" class="mt-0.5 h-4 w-4 cursor-pointer accent-ledger-ink" data-test="consent-check" />
        <span class="text-xs leading-relaxed text-ink-soft">
          我已阅读并单独同意上述处理规则，同意你按此方式处理我的敏感个人信息。
        </span>
      </label>

      <p v-if="error" class="mt-2 text-xs text-ledger-up">{{ error }}</p>

      <button
        type="button"
        class="mt-4 w-full rounded-xl bg-ledger-ink py-3 text-sm font-semibold text-white transition-colors hover:bg-[#2B3A5C] disabled:cursor-not-allowed disabled:bg-slate-300"
        :disabled="!agreed || saving"
        data-test="consent-accept"
        @click="emit('accept')"
      >
        {{ saving ? '记录中…' : '同意并开始记账' }}
      </button>
      <p class="mt-2 text-center text-[11px] text-ink-faint">不同意将无法保存或查看账本数据</p>
    </div>
  </div>
</template>
