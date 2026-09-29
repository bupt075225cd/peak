<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { BookOpen, LogIn, ArrowLeft, Info, Eye, EyeOff } from 'lucide-vue-next'
import { useAuth } from '../composables/useAuth'

const router = useRouter()
const { login } = useAuth()

const username = ref('')
const password = ref('')
const showPassword = ref(false)

// 登录页开发前的临时引导：演示令牌，登录后可体验已登录主页形态。
function handleDemoLogin() {
  login('demo-token')
  router.push('/home')
}

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
        <p class="mt-1 text-sm text-ink-soft">登录后同步你的错题与复习进度</p>
      </div>

      <!-- 表单骨架：登录接口就绪前仅作占位展示 -->
      <form class="mt-8 space-y-4" @submit.prevent>
        <div>
          <label for="login-username" class="block text-sm font-medium text-ink mb-1.5">用户名</label>
          <input
            id="login-username"
            v-model="username"
            type="text"
            autocomplete="username"
            placeholder="请输入用户名"
            class="w-full px-3.5 py-2.5 rounded-xl border border-slate-200 bg-white text-sm text-ink placeholder:text-ink-faint outline-none focus:border-primary focus:ring-2 focus:ring-primary/15 transition-shadow"
          />
        </div>
        <div>
          <label for="login-password" class="block text-sm font-medium text-ink mb-1.5">密码</label>
          <div class="relative">
            <input
              id="login-password"
              v-model="password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="current-password"
              placeholder="请输入密码"
              class="w-full px-3.5 py-2.5 pr-10 rounded-xl border border-slate-200 bg-white text-sm text-ink placeholder:text-ink-faint outline-none focus:border-primary focus:ring-2 focus:ring-primary/15 transition-shadow"
            />
            <button
              type="button"
              class="absolute inset-y-0 right-0 px-3 flex items-center text-ink-faint hover:text-ink-soft cursor-pointer"
              :aria-label="showPassword ? '隐藏密码' : '显示密码'"
              @click="showPassword = !showPassword"
            >
              <EyeOff v-if="showPassword" class="w-4 h-4" />
              <Eye v-else class="w-4 h-4" />
            </button>
          </div>
        </div>

        <button
          type="button"
          class="w-full inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-xl bg-primary text-white text-sm font-semibold shadow-lg shadow-blue-500/25 hover:bg-primary-light transition-colors cursor-pointer"
          @click="handleDemoLogin"
        >
          <LogIn class="w-4 h-4" />
          演示登录
        </button>
      </form>

      <div class="mt-6 flex items-start gap-2 p-3 rounded-xl bg-amber-50 text-amber-700">
        <Info class="w-4 h-4 mt-0.5 shrink-0" />
        <p class="text-xs leading-relaxed">
          登录功能开发中：正式登录接口（user-service）就绪前，可点击「演示登录」体验已登录后的主页仪表盘。
        </p>
      </div>
    </div>
  </div>
</template>
