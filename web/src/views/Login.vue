<script setup lang="ts">
import { ref, computed, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { BookOpen, LogIn, ArrowLeft, Loader2, Smartphone, ShieldCheck } from 'lucide-vue-next'
import { sendSmsCode, smsLogin } from '../api'
import { useAuth } from '../composables/useAuth'

const router = useRouter()
const { login } = useAuth()

const phone = ref('')
const code = ref('')
const ticket = ref('')
const debugCode = ref('')
const sending = ref(false)
const loggingIn = ref(false)
const errorMsg = ref('')
const countdown = ref(0)
let timer: ReturnType<typeof setInterval> | undefined

// 手机号校验：1 开头，第二位 3-9，共 11 位。
const phoneValid = computed(() => /^1[3-9]\d{9}$/.test(phone.value))
const canSend = computed(() => phoneValid.value && countdown.value === 0 && !sending.value)
const canLogin = computed(() => phoneValid.value && code.value.length > 0 && ticket.value !== '' && !loggingIn.value)

// 发送验证码：成功后记录 ticket 并进入 60 秒倒计时。
async function handleSendCode() {
  if (!canSend.value) return
  sending.value = true
  errorMsg.value = ''
  try {
    const res = await sendSmsCode(phone.value.trim())
    ticket.value = res.ticket
    debugCode.value = res.debugCode ?? ''
    countdown.value = 60
    timer = setInterval(() => {
      countdown.value--
      if (countdown.value <= 0) {
        clearInterval(timer)
        timer = undefined
      }
    }, 1000)
  } catch (err) {
    errorMsg.value = extractError(err, '验证码发送失败，请稍后重试')
  } finally {
    sending.value = false
  }
}

// 验证码登录：成功后写入令牌并跳转主页。
async function handleLogin() {
  if (!canLogin.value) return
  loggingIn.value = true
  errorMsg.value = ''
  try {
    const res = await smsLogin(phone.value.trim(), code.value.trim(), ticket.value)
    login(res.token)
    router.push('/home')
  } catch (err) {
    errorMsg.value = extractError(err, '登录失败，请稍后重试')
  } finally {
    loggingIn.value = false
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

function goBack() {
  router.back()
}
</script>

<template>
  <div class="mx-auto max-w-md px-4 py-12">
    <button
      class="inline-flex items-center gap-1 text-sm text-ink-soft hover:text-ink transition-colors cursor-pointer"
      @click="goBack"
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
        <h1 class="mt-4 text-xl font-semibold text-ink">登录 Peak 错题本</h1>
        <p class="mt-1 text-sm text-ink-soft">输入手机号，验证码登录；新手机号将自动创建账号</p>
      </div>

      <form class="mt-8 space-y-4" @submit.prevent="handleLogin">
        <div>
          <label for="login-phone" class="block text-sm font-medium text-ink mb-1.5">手机号</label>
          <div class="relative">
            <Smartphone class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-faint" />
            <input
              id="login-phone"
              v-model="phone"
              type="tel"
              maxlength="11"
              autocomplete="tel"
              placeholder="请输入 11 位手机号"
              class="w-full pl-10 pr-3.5 py-2.5 rounded-xl border border-slate-200 bg-white text-sm text-ink placeholder:text-ink-faint outline-none focus:border-primary focus:ring-2 focus:ring-primary/15 transition-shadow"
              @input="errorMsg = ''"
            />
          </div>
          <p v-if="phone && !phoneValid" class="mt-1.5 text-xs text-red-600">手机号格式不正确</p>
        </div>

        <div>
          <label for="login-code" class="block text-sm font-medium text-ink mb-1.5">验证码</label>
          <div class="flex gap-2">
            <div class="relative flex-1">
              <ShieldCheck class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-faint" />
              <input
                id="login-code"
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
              <span v-else>获取验证码</span>
            </button>
          </div>
        </div>

        <p v-if="errorMsg" class="text-sm text-red-600">{{ errorMsg }}</p>

        <button
          type="submit"
          class="w-full inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-xl bg-primary text-white text-sm font-semibold shadow-lg shadow-blue-500/25 hover:bg-primary-light hover:-translate-y-0.5 transition-all duration-200 cursor-pointer disabled:cursor-not-allowed disabled:bg-slate-300 disabled:shadow-none disabled:translate-y-0"
          :disabled="!canLogin"
        >
          <Loader2 v-if="loggingIn" class="w-4 h-4 animate-spin" />
          <LogIn v-else class="w-4 h-4" />
          登录
        </button>
      </form>

      <!-- 开发模式提示：mock 短信通道不真正发短信，展示验证码便于联调 -->
      <div v-if="debugCode" class="mt-6 flex items-start gap-2 p-3 rounded-xl bg-amber-50 text-amber-700">
        <ShieldCheck class="w-4 h-4 mt-0.5 shrink-0" />
        <p class="text-xs leading-relaxed">开发模式验证码：{{ debugCode }}（mock 短信通道不发送真实短信）</p>
      </div>
    </div>
  </div>
</template>
