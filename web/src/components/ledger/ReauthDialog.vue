<script setup lang="ts">
import { ref, watch } from 'vue'
import { KeyRound, X } from 'lucide-vue-next'
import { getMe, passwordLogin } from '../../api'
import { login } from '../../composables/useAuth'
import { errorMessage } from '../../api/ledger'

const props = defineProps<{
  open: boolean
  action: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'success'): void
}>()

const account = ref('')
const password = ref('')
const busy = ref(false)
const error = ref('')

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    password.value = ''
    error.value = ''
    // 账号用于密码登录，仅在本机内存中使用。
    const me = await getMe().catch(() => null)
    account.value = me?.phone || me?.email || me?.account || ''
  },
)

async function submit() {
  if (!account.value || !password.value) {
    error.value = '请输入账号密码重新验证'
    return
  }
  busy.value = true
  error.value = ''
  try {
    const result = await passwordLogin(account.value.trim(), password.value)
    login(result.token)
    password.value = ''
    emit('success')
  } catch (err) {
    error.value = errorMessage(err, '验证失败，请检查账号密码')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center bg-ink/40 p-4 backdrop-blur-sm" @click.self="emit('close')">
    <div class="w-full max-w-md rounded-2xl border border-slate-200/80 bg-white p-6 shadow-2xl animate-fade-up">
      <div class="flex items-start justify-between">
        <div class="flex items-center gap-2">
          <span class="flex h-9 w-9 items-center justify-center rounded-xl bg-ledger-mist text-ledger-ink">
            <KeyRound class="h-4 w-4" />
          </span>
          <h2 class="text-base font-semibold text-ink">重新验证身份</h2>
        </div>
        <button type="button" class="rounded-lg p-1.5 text-ink-faint hover:bg-slate-100 cursor-pointer" @click="emit('close')">
          <X class="h-4 w-4" />
        </button>
      </div>

      <p class="mt-3 rounded-xl bg-slate-50 px-3.5 py-2.5 text-xs leading-relaxed text-ink-soft">
        「{{ action }}」会导出或销毁敏感数据，需要 5 分钟内重新登录的凭证。验证成功后操作会自动重试。
      </p>

      <label class="mt-4 block">
        <span class="text-xs font-medium text-ink-soft">账号（手机号或邮箱）</span>
        <input
          v-model="account"
          autocomplete="username"
          class="mt-1.5 w-full rounded-xl bg-slate-50 px-3.5 py-2.5 text-sm text-ink outline-none focus:bg-white focus:ring-1 focus:ring-ledger-ink/30"
        />
      </label>
      <label class="mt-3 block">
        <span class="text-xs font-medium text-ink-soft">密码</span>
        <input
          v-model="password"
          type="password"
          autocomplete="current-password"
          placeholder="登录密码"
          class="mt-1.5 w-full rounded-xl bg-slate-50 px-3.5 py-2.5 text-sm text-ink outline-none focus:bg-white focus:ring-1 focus:ring-ledger-ink/30"
          @keyup.enter="submit"
        />
      </label>
      <p class="mt-2 text-[11px] text-ink-faint">仅支持密码账号；验证码注册的账号请先在账号设置中设置密码。</p>
      <p v-if="error" class="mt-2 text-xs text-ledger-up">{{ error }}</p>

      <button
        type="button"
        class="mt-4 w-full rounded-xl bg-primary py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-light disabled:cursor-not-allowed disabled:bg-slate-300"
        :disabled="busy"
        @click="submit"
      >
        {{ busy ? '验证中…' : '验证并继续' }}
      </button>
    </div>
  </div>
</template>
