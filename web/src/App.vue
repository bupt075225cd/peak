<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import { Camera, BookOpen, User, LogIn } from 'lucide-vue-next'
import { computed } from 'vue'
import { useAuth } from './composables/useAuth'

const route = useRoute()
const router = useRouter()
const { isLoggedIn, logout } = useAuth()
const active = computed(() => route.name)

function handleLogout() {
  // 退出登录并回到主页（未登录形态）。
  logout()
  router.push('/home')
}
</script>

<template>
  <div class="min-h-screen flex flex-col">
    <!-- 顶部导航栏 -->
    <header
      class="fixed top-0 inset-x-0 z-40 h-16 bg-white/80 backdrop-blur-md border-b border-slate-200/60"
    >
      <div class="mx-auto max-w-5xl h-full px-4 flex items-center justify-between">
        <div class="flex items-center gap-2">
          <div
            class="w-9 h-9 rounded-xl bg-gradient-to-br from-primary to-primary-light flex items-center justify-center shadow-lg shadow-blue-500/20"
          >
            <BookOpen class="w-5 h-5 text-white" />
          </div>
          <RouterLink to="/home" class="text-lg font-semibold tracking-tight text-ink cursor-pointer">
            Peak 错题本
          </RouterLink>
        </div>
        <nav class="flex items-center gap-1">
          <template v-if="isLoggedIn">
            <RouterLink
              to="/entry"
              class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm font-medium transition-colors"
              :class="active === 'entry' ? 'bg-primary/10 text-primary' : 'text-ink-soft hover:bg-slate-100'"
            >
              <Camera class="w-4 h-4" />
              录入错题
            </RouterLink>
            <RouterLink
              to="/list"
              class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm font-medium transition-colors"
              :class="active === 'list' ? 'bg-primary/10 text-primary' : 'text-ink-soft hover:bg-slate-100'"
            >
              <BookOpen class="w-4 h-4" />
              错题本
            </RouterLink>
          </template>
          <div class="ml-3 flex items-center gap-2 pl-3 border-l border-slate-200">
            <template v-if="isLoggedIn">
              <div class="w-8 h-8 rounded-full bg-gradient-to-br from-slate-200 to-slate-300 flex items-center justify-center">
                <User class="w-4 h-4 text-ink-soft" />
              </div>
              <span class="text-sm text-ink-soft">小明</span>
              <button
                class="ml-1 text-xs text-ink-faint hover:text-primary transition-colors cursor-pointer"
                @click="handleLogout"
              >
                退出
              </button>
            </template>
            <RouterLink
              v-else
              to="/login"
              class="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg bg-primary text-white text-sm font-semibold shadow-md shadow-blue-500/20 hover:bg-primary-light transition-colors cursor-pointer"
            >
              <LogIn class="w-4 h-4" />
              登录
            </RouterLink>
          </div>
        </nav>
      </div>
    </header>

    <!-- 主内容区 -->
    <main class="flex-1 pt-16">
      <RouterView />
    </main>
  </div>
</template>
