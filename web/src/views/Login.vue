<script setup lang="ts">
import { ref, computed, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  BookOpen, LogIn, ArrowLeft, Loader2, Smartphone, ShieldCheck,
  KeyRound, Mail, Lock, Eye, EyeOff,
} from 'lucide-vue-next'
import { sendSmsCode, smsLogin, passwordLogin } from '../api'
import { useAuth } from '../composables/useAuth'
import { SMS_LOGIN_ENABLED } from '../config/features'

const router = useRouter()
const { login } = useAuth()

// 登录方式：sms（验证码）/ password（账号密码），Tab 切换。
// 短信服务未接入时（SMS_LOGIN_ENABLED=false）隐藏验证码登录，默认密码登录。
const mode = ref<'sms' | 'password'>(SMS_LOGIN_ENABLED ? 'sms' : 'password')

// ===== 验证码登录状态 =====
const phone = ref('')
const code = ref('')
const ticket = ref('')
const debugCode = ref('')
const sending = ref(false)
const countdown = ref(0)
let timer: ReturnType<typeof setInterval> | undefined

// ===== 密码登录状态 =====
const account = ref('')
const password = ref('')
const showPassword = ref(false)
const loggingIn = ref(false)
const errorMsg = ref('')

// 手机号校验：1 开头，第二位 3-9，共 11 位。
const phoneValid = computed(() => /^1[3-9]\d{9}$/.test(phone.value))
const canSend = computed(() => phoneValid.value && countdown.value === 0 && !sending.value)
const canSmsLogin = computed(() => phoneValid.value && code.value.length > 0 && ticket.value !== '' && !loggingIn.value)
// 密码登录：账号为手机号或邮箱，密码非空。
const accountValid = computed(() =>
  /^1[3-9]\d{9}$/.test(account.value.trim()) || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(account.value.trim()))
const canPasswordLogin = computed(() => accountValid.value && password.value.length > 0 && !loggingIn.value)

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
async function handleSmsLogin() {
  if (!canSmsLogin.value) return
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

// 密码登录：账号（手机号或邮箱）+ 密码。
async function handlePasswordLogin() {
  if (!canPasswordLogin.value) return
  loggingIn.value = true
  errorMsg.value = ''
  try {
    const res = await passwordLogin(account.value.trim(), password.value)
    login(res.token)
    router.push('/home')
  } catch (err) {
    errorMsg.value = extractError(err, '登录失败，请稍后重试')
  } finally {
    loggingIn.value = false
  }
}

function switchMode(m: 'sms' | 'password') {
  mode.value = m
  errorMsg.value = ''
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
        <p class="mt-1 text-sm text-ink-soft">
          {{ mode === 'sms' ? '输入手机号，验证码登录；新手机号将自动创建账号' : '使用手机号或邮箱 + 密码登录' }}
        </p>
      </div>

      <!-- 登录方式切换 Tab（短信服务接入后才展示） -->
      <div v-if="SMS_LOGIN_ENABLED" class="mt-6 grid grid-cols-2 gap-1 p-1 rounded-xl bg-slate-100" role="tablist">
        <button
          type="button"
          role="tab"
          :aria-selected="mode === 'sms'"
          class="flex items-center justify-center gap-1.5 py-2 rounded-lg text-sm font-medium transition-all duration-200 cursor-pointer"
          :class="mode === 'sms' ? 'bg-white text-ink shadow-sm' : 'text-ink-soft hover:text-ink'"
          @click="switchMode('sms')"
        >
          <ShieldCheck class="w-4 h-4" />
          验证码登录
        </button>
        <button
          type="button"
          role="tab"
          :aria-selected="mode === 'password'"
          class="flex items-center justify-center gap-1.5 py-2 rounded-lg text-sm font-medium transition-all duration-200 cursor-pointer"
          :class="mode === 'password' ? 'bg-white text-ink shadow-sm' : 'text-ink-soft hover:text-ink'"
          @click="switchMode('password')"
        >
          <KeyRound class="w-4 h-4" />
          密码登录
        </button>
      </div>

      <!-- 验证码登录表单 -->
      <form v-if="mode === 'sms'" class="mt-6 space-y-4 animate-fade-up" @submit.prevent="handleSmsLogin">
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
          :disabled="!canSmsLogin"
        >
          <Loader2 v-if="loggingIn" class="w-4 h-4 animate-spin" />
          <LogIn v-else class="w-4 h-4" />
          登录
        </button>
      </form>

      <!-- 密码登录表单 -->
      <form v-else class="mt-6 space-y-4 animate-fade-up" @submit.prevent="handlePasswordLogin">
        <div>
          <label for="login-account" class="block text-sm font-medium text-ink mb-1.5">手机号或邮箱</label>
          <div class="relative">
            <Mail class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-faint" />
            <input
              id="login-account"
              v-model="account"
              type="text"
              autocomplete="username"
              placeholder="请输入手机号或邮箱"
              class="w-full pl-10 pr-3.5 py-2.5 rounded-xl border border-slate-200 bg-white text-sm text-ink placeholder:text-ink-faint outline-none focus:border-primary focus:ring-2 focus:ring-primary/15 transition-shadow"
              @input="errorMsg = ''"
            />
          </div>
          <p v-if="account && !accountValid" class="mt-1.5 text-xs text-red-600">请输入正确的手机号或邮箱</p>
        </div>

        <div>
          <div class="flex items-center justify-between mb-1.5">
            <label for="login-password" class="block text-sm font-medium text-ink">密码</label>
            <router-link to="/forgot-password" class="text-xs text-primary hover:text-primary-light transition-colors">
              忘记密码？
            </router-link>
          </div>
          <div class="relative">
            <Lock class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-faint" />
            <input
              id="login-password"
              v-model="password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="current-password"
              placeholder="请输入密码"
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
        </div>

        <p v-if="errorMsg" class="text-sm text-red-600">{{ errorMsg }}</p>

        <button
          type="submit"
          class="w-full inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-xl bg-primary text-white text-sm font-semibold shadow-lg shadow-blue-500/25 hover:bg-primary-light hover:-translate-y-0.5 transition-all duration-200 cursor-pointer disabled:cursor-not-allowed disabled:bg-slate-300 disabled:shadow-none disabled:translate-y-0"
          :disabled="!canPasswordLogin"
        >
          <Loader2 v-if="loggingIn" class="w-4 h-4 animate-spin" />
          <LogIn v-else class="w-4 h-4" />
          登录
        </button>

        <p class="text-center text-sm text-ink-soft">
          还没有账号？
          <router-link to="/register" class="text-primary hover:text-primary-light font-medium transition-colors">
            邮箱注册
          </router-link>
        </p>
      </form>

      <!-- 开发模式提示：mock 短信通道不真正发短信，展示验证码便于联调 -->
      <div v-if="mode === 'sms' && debugCode" class="mt-6 flex items-start gap-2 p-3 rounded-xl bg-amber-50 text-amber-700">
        <ShieldCheck class="w-4 h-4 mt-0.5 shrink-0" />
        <p class="text-xs leading-relaxed">开发模式验证码：{{ debugCode }}（mock 短信通道不发送真实短信）</p>
      </div>
    </div>
  </div>
</template>
