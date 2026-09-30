<script setup lang="ts">
import { ref, computed, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  BookOpen, ArrowLeft, Loader2, ShieldCheck, Mail, Lock, UserPlus, Eye, EyeOff,
} from 'lucide-vue-next'
import { sendEmailCode, emailRegister } from '../api'
import { useAuth } from '../composables/useAuth'

const router = useRouter()
const { login } = useAuth()

const email = ref('')
const password = ref('')
const confirm = ref('')
const code = ref('')
const debugCode = ref('')
const showPassword = ref(false)
const sending = ref(false)
const registering = ref(false)
const errorMsg = ref('')
const countdown = ref(0)
let timer: ReturnType<typeof setInterval> | undefined

// 邮箱与密码校验（与后端规则一致：密码 8-64 位）。
const emailValid = computed(() => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value.trim()))
const passwordValid = computed(() => password.value.length >= 8)
const confirmValid = computed(() => confirm.value === password.value)
const canSend = computed(() => emailValid.value && countdown.value === 0 && !sending.value)
const canRegister = computed(() =>
  emailValid.value && passwordValid.value && confirmValid.value && code.value.length > 0 && !registering.value)

// 发送注册验证码：成功后展示 debug_code（开发模式）并进入 60 秒倒计时。
async function handleSendCode() {
  if (!canSend.value) return
  sending.value = true
  errorMsg.value = ''
  try {
    const res = await sendEmailCode(email.value.trim(), 'register')
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

// 提交注册：成功后写入令牌并进入主页（注册即登录态）。
async function handleRegister() {
  if (!canRegister.value) return
  registering.value = true
  errorMsg.value = ''
  try {
    const res = await emailRegister(email.value.trim(), password.value, code.value.trim())
    login(res.token)
    router.push('/home')
  } catch (err) {
    errorMsg.value = extractError(err, '注册失败，请稍后重试')
  } finally {
    registering.value = false
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
        <h1 class="mt-4 text-xl font-semibold text-ink">注册 Peak 错题本</h1>
        <p class="mt-1 text-sm text-ink-soft">使用邮箱创建账号，验证邮件已为你准备好</p>
      </div>

      <form class="mt-8 space-y-4" @submit.prevent="handleRegister">
        <div>
          <label for="reg-email" class="block text-sm font-medium text-ink mb-1.5">邮箱</label>
          <div class="relative">
            <Mail class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-faint" />
            <input
              id="reg-email"
              v-model="email"
              type="email"
              autocomplete="email"
              placeholder="请输入邮箱地址"
              class="w-full pl-10 pr-3.5 py-2.5 rounded-xl border border-slate-200 bg-white text-sm text-ink placeholder:text-ink-faint outline-none focus:border-primary focus:ring-2 focus:ring-primary/15 transition-shadow"
              @input="errorMsg = ''"
            />
          </div>
          <p v-if="email && !emailValid" class="mt-1.5 text-xs text-red-600">邮箱格式不正确</p>
        </div>

        <div>
          <label for="reg-password" class="block text-sm font-medium text-ink mb-1.5">密码</label>
          <div class="relative">
            <Lock class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-faint" />
            <input
              id="reg-password"
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
        </div>

        <div>
          <label for="reg-confirm" class="block text-sm font-medium text-ink mb-1.5">确认密码</label>
          <div class="relative">
            <Lock class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-faint" />
            <input
              id="reg-confirm"
              v-model="confirm"
              type="password"
              autocomplete="new-password"
              placeholder="请再次输入密码"
              class="w-full pl-10 pr-3.5 py-2.5 rounded-xl border border-slate-200 bg-white text-sm text-ink placeholder:text-ink-faint outline-none focus:border-primary focus:ring-2 focus:ring-primary/15 transition-shadow"
              @input="errorMsg = ''"
            />
          </div>
          <p v-if="confirm && !confirmValid" class="mt-1.5 text-xs text-red-600">两次输入的密码不一致</p>
        </div>

        <div>
          <label for="reg-code" class="block text-sm font-medium text-ink mb-1.5">邮箱验证码</label>
          <div class="flex gap-2">
            <div class="relative flex-1">
              <ShieldCheck class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-faint" />
              <input
                id="reg-code"
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
          :disabled="!canRegister"
        >
          <Loader2 v-if="registering" class="w-4 h-4 animate-spin" />
          <UserPlus v-else class="w-4 h-4" />
          注册
        </button>

        <p class="text-center text-sm text-ink-soft">
          已有账号？
          <router-link to="/login" class="text-primary hover:text-primary-light font-medium transition-colors">
            直接登录
          </router-link>
        </p>
      </form>

      <!-- 开发模式提示：mock 邮件通道不真正发信，展示验证码便于联调 -->
      <div v-if="debugCode" class="mt-6 flex items-start gap-2 p-3 rounded-xl bg-amber-50 text-amber-700">
        <ShieldCheck class="w-4 h-4 mt-0.5 shrink-0" />
        <p class="text-xs leading-relaxed">开发模式验证码：{{ debugCode }}（mock 邮件通道不发送真实邮件）</p>
      </div>
    </div>
  </div>
</template>
