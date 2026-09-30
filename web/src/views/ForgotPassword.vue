<script setup lang="ts">
import { ref, computed, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  BookOpen, ArrowLeft, Loader2, ShieldCheck, Mail, Lock, CheckCircle2, Eye, EyeOff,
} from 'lucide-vue-next'
import { sendEmailCode, resetPassword } from '../api'
import { usePasswordStrength } from '../composables/usePasswordStrength'

const router = useRouter()

// 分步流程：1 输入邮箱发验证码 → 2 输入验证码与新密码 → 3 完成。
const step = ref<1 | 2 | 3>(1)
const email = ref('')
const code = ref('')
const password = ref('')
const confirm = ref('')
const debugCode = ref('')
const showPassword = ref(false)
const sending = ref(false)
const resetting = ref(false)
const errorMsg = ref('')
const countdown = ref(0)
let timer: ReturnType<typeof setInterval> | undefined

// 邮箱与密码校验（与后端规则一致：密码 8-64 位）。
const emailValid = computed(() => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value.trim()))
const passwordValid = computed(() => password.value.length >= 8)
const confirmValid = computed(() => confirm.value === password.value)
// 新密码强度实时指示（弱/中/强）。
const { strength, barClass } = usePasswordStrength(password)
const canSend = computed(() => emailValid.value && countdown.value === 0 && !sending.value)
const canReset = computed(() => code.value.length > 0 && passwordValid.value && confirmValid.value && !resetting.value)

// 发送重置验证码，成功后进入第二步。
async function handleSendCode() {
  if (!canSend.value) return
  sending.value = true
  errorMsg.value = ''
  try {
    const res = await sendEmailCode(email.value.trim(), 'reset')
    debugCode.value = res.debugCode ?? ''
    countdown.value = 60
    timer = setInterval(() => {
      countdown.value--
      if (countdown.value <= 0) {
        clearInterval(timer)
        timer = undefined
      }
    }, 1000)
    step.value = 2
  } catch (err) {
    errorMsg.value = extractError(err, '验证码发送失败，请稍后重试')
  } finally {
    sending.value = false
  }
}

// 提交重置：成功后进入完成页。
async function handleReset() {
  if (!canReset.value) return
  resetting.value = true
  errorMsg.value = ''
  try {
    await resetPassword(email.value.trim(), code.value.trim(), password.value)
    step.value = 3
  } catch (err) {
    errorMsg.value = extractError(err, '重置失败，请稍后重试')
  } finally {
    resetting.value = false
  }
}

// 从统一响应结构中提取错误信息。
function extractError(err: unknown, fallback: string): string {
  const data = (err as { response?: { data?: { message?: string } } })?.response?.data
  return data?.message || fallback
}

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="mx-auto max-w-md px-4 py-12">
    <button
      class="inline-flex items-center gap-1 text-sm text-ink-soft hover:text-ink transition-colors cursor-pointer"
      @click="router.back()"
    >
      <ArrowLeft class="w-4 h-4" />
      返回
    </button>

    <div class="mt-6 bg-white rounded-2xl border border-slate-200/80 p-8 shadow-sm animate-fade-up">
      <div class="flex flex-col items-center text-center">
        <div
          class="w-12 h-12 rounded-2xl bg-gradient-to-br from-primary to-primary-light flex items-center justify-center shadow-lg shadow-blue-500/20"
        >
          <BookOpen class="w-6 h-6 text-white" />
        </div>
        <h1 class="mt-4 text-xl font-semibold text-ink">找回密码</h1>
        <p class="mt-1 text-sm text-ink-soft">
          {{ step === 1 ? '输入注册时绑定的邮箱，我们将发送验证码' : step === 2 ? `验证码已发送至 ${email}` : '' }}
        </p>
      </div>

      <!-- 第一步：输入邮箱发送验证码 -->
      <form v-if="step === 1" class="mt-8 space-y-4 animate-fade-up" @submit.prevent="handleSendCode">
        <div>
          <label for="fp-email" class="block text-sm font-medium text-ink mb-1.5">邮箱</label>
          <div class="relative">
            <Mail class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-faint" />
            <input
              id="fp-email"
              v-model="email"
              type="email"
              autocomplete="email"
              placeholder="请输入绑定的邮箱地址"
              class="w-full pl-10 pr-3.5 py-2.5 rounded-xl border border-slate-200 bg-white text-sm text-ink placeholder:text-ink-faint outline-none focus:border-primary focus:ring-2 focus:ring-primary/15 transition-shadow"
              @input="errorMsg = ''"
            />
          </div>
          <p v-if="email && !emailValid" class="mt-1.5 text-xs text-red-600">邮箱格式不正确</p>
        </div>

        <p v-if="errorMsg" class="text-sm text-red-600">{{ errorMsg }}</p>

        <button
          type="submit"
          class="w-full inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-xl bg-primary text-white text-sm font-semibold shadow-lg shadow-blue-500/25 hover:bg-primary-light hover:-translate-y-0.5 transition-all duration-200 cursor-pointer disabled:cursor-not-allowed disabled:bg-slate-300 disabled:shadow-none disabled:translate-y-0"
          :disabled="!emailValid || sending"
        >
          <Loader2 v-if="sending" class="w-4 h-4 animate-spin" />
          <Mail v-else class="w-4 h-4" />
          发送验证码
        </button>
      </form>

      <!-- 第二步：验证码 + 新密码 -->
      <form v-else-if="step === 2" class="mt-8 space-y-4 animate-fade-up" @submit.prevent="handleReset">
        <div>
          <label for="fp-code" class="block text-sm font-medium text-ink mb-1.5">邮箱验证码</label>
          <div class="flex gap-2">
            <div class="relative flex-1">
              <ShieldCheck class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-faint" />
              <input
                id="fp-code"
                v-model="code"
                type="text"
                inputmode="numeric"
                maxlength="6"
                autocomplete="one-time-code"
                placeholder="6 位验证码"
                class="w-full pl-10 pr-3.5 py-2.5 rounded-xl border border-slate-200 bg-white text-sm text-ink placeholder:text-ink-faint outline-none focus:border-primary focus:ring-2 focus:ring-primary/15 transition-shadow"
                @input="errorMsg = ''"
              />
            </div>
            <button
              type="button"
              class="shrink-0 px-4 py-2.5 rounded-xl text-sm font-medium transition-colors cursor-pointer disabled:cursor-not-allowed"
              :class="canSend ? 'bg-primary/10 text-primary hover:bg-primary/20' : 'bg-slate-100 text-ink-faint cursor-not-allowed'"
              :disabled="!canSend"
              @click="handleSendCode"
            >
              <Loader2 v-if="sending" class="w-4 h-4 animate-spin" />
              <span v-else-if="countdown > 0">重新发送({{ countdown }}s)</span>
              <span v-else>重新发送</span>
            </button>
          </div>
        </div>

        <div>
          <label for="fp-password" class="block text-sm font-medium text-ink mb-1.5">新密码</label>
          <div class="relative">
            <Lock class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-faint" />
            <input
              id="fp-password"
              v-model="password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="new-password"
              placeholder="至少 8 位字符"
              class="w-full pl-10 pr-10 py-2.5 rounded-xl border border-slate-200 bg-white text-sm text-ink placeholder:text-ink-faint outline-none focus:border-primary focus:ring-2 focus:ring-primary/15 transition-shadow"
              @input="errorMsg = ''"
            />
            <button
              type="button"
              class="absolute right-3 top-1/2 -translate-y-1/2 text-ink-faint hover:text-ink-soft transition-colors cursor-pointer"
              :aria-label="showPassword ? '隐藏密码' : '显示密码'"
              @click="showPassword = !showPassword"
            >
              <EyeOff v-if="showPassword" class="w-4 h-4" />
              <Eye v-else class="w-4 h-4" />
            </button>
          </div>
          <p v-if="password && !passwordValid" class="mt-1.5 text-xs text-red-600">密码至少 8 位</p>
          <div v-else-if="password" class="mt-2">
            <div class="flex gap-1.5" aria-hidden="true">
              <div
                v-for="i in 3"
                :key="i"
                class="h-1 flex-1 rounded-full transition-colors duration-300"
                :class="i <= strength.score ? barClass.active : 'bg-slate-200'"
              />
            </div>
            <p class="mt-1 text-xs" :class="barClass.text">密码强度：{{ strength.label }}</p>
          </div>
        </div>

        <div>
          <label for="fp-confirm" class="block text-sm font-medium text-ink mb-1.5">确认新密码</label>
          <div class="relative">
            <Lock class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-faint" />
            <input
              id="fp-confirm"
              v-model="confirm"
              type="password"
              autocomplete="new-password"
              placeholder="请再次输入新密码"
              class="w-full pl-10 pr-3.5 py-2.5 rounded-xl border border-slate-200 bg-white text-sm text-ink placeholder:text-ink-faint outline-none focus:border-primary focus:ring-2 focus:ring-primary/15 transition-shadow"
              @input="errorMsg = ''"
            />
          </div>
          <p v-if="confirm && !confirmValid" class="mt-1.5 text-xs text-red-600">两次输入的密码不一致</p>
        </div>

        <p v-if="errorMsg" class="text-sm text-red-600">{{ errorMsg }}</p>

        <button
          type="submit"
          class="w-full inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-xl bg-primary text-white text-sm font-semibold shadow-lg shadow-blue-500/25 hover:bg-primary-light hover:-translate-y-0.5 transition-all duration-200 cursor-pointer disabled:cursor-not-allowed disabled:bg-slate-300 disabled:shadow-none disabled:translate-y-0"
          :disabled="!canReset"
        >
          <Loader2 v-if="resetting" class="w-4 h-4 animate-spin" />
          <Lock v-else class="w-4 h-4" />
          重置密码
        </button>
      </form>

      <!-- 第三步：完成 -->
      <div v-else class="mt-8 flex flex-col items-center text-center animate-fade-up">
        <div class="w-14 h-14 rounded-full bg-emerald-50 flex items-center justify-center">
          <CheckCircle2 class="w-8 h-8 text-emerald-600" />
        </div>
        <p class="mt-4 text-sm text-ink">密码已重置，请使用新密码登录</p>
        <button
          type="button"
          class="mt-6 w-full inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-xl bg-primary text-white text-sm font-semibold shadow-lg shadow-blue-500/25 hover:bg-primary-light hover:-translate-y-0.5 transition-all duration-200 cursor-pointer"
          @click="router.push('/login')"
        >
          去登录
        </button>
      </div>

      <!-- 开发模式提示：mock 邮件通道不真正发信，展示验证码便于联调 -->
      <div v-if="debugCode && step === 2" class="mt-6 flex items-start gap-2 p-3 rounded-xl bg-amber-50 text-amber-700">
        <ShieldCheck class="w-4 h-4 mt-0.5 shrink-0" />
        <p class="text-xs leading-relaxed">开发模式验证码：{{ debugCode }}（mock 邮件通道不发送真实邮件）</p>
      </div>
    </div>
  </div>
</template>
