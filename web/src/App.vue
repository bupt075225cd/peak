<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import { Camera, BookOpen, BookMarked, Search, User, LogIn, ChevronDown, Pencil } from 'lucide-vue-next'
import { ref, computed, watch } from 'vue'
import { useAuth } from './composables/useAuth'
import { getMe, updateMyName } from './api'

const route = useRoute()
const router = useRouter()
const { isLoggedIn, logout } = useAuth()
const active = computed(() => route.name)

// 应用外壳按路由前缀切换：/poetry* 为古诗文应用（独立品牌与导航），
// 其余为错题本应用；用户菜单属共享账号体系，两个应用保持一致。
const isPoetry = computed(() => route.path.startsWith('/poetry'))

function handleLogout() {
  // 退出登录并回到主页（未登录形态）。
  menuOpen.value = false
  logout()
  router.push('/home')
}

// ── 当前用户昵称（个人中心）────────────────────────────
const displayName = ref('同学')
const menuOpen = ref(false)
const editOpen = ref(false)
const editName = ref('')
const saving = ref(false)
const editError = ref('')

// 登录态变化时拉取/重置用户信息。
watch(isLoggedIn, (loggedIn) => {
  if (loggedIn) {
    loadMe()
  } else {
    displayName.value = '同学'
    menuOpen.value = false
  }
}, { immediate: true })

async function loadMe() {
  try {
    const user = await getMe()
    if (user.name?.trim()) displayName.value = user.name.trim()
  } catch (err) {
    console.error('加载用户信息失败', err)
  }
}

function openEdit() {
  menuOpen.value = false
  editName.value = displayName.value === '同学' ? '' : displayName.value
  editError.value = ''
  editOpen.value = true
}

async function handleSaveName() {
  const name = editName.value.trim()
  if (!name) {
    editError.value = '昵称不能为空'
    return
  }
  saving.value = true
  editError.value = ''
  try {
    const user = await updateMyName(name)
    displayName.value = user.name || name
    editOpen.value = false
  } catch (err) {
    const data = (err as { response?: { data?: { message?: string } } })?.response?.data
    editError.value = data?.message || '保存失败，请稍后重试'
  } finally {
    saving.value = false
  }
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
            class="w-9 h-9 rounded-xl flex items-center justify-center shadow-lg transition-colors"
            :class="isPoetry ? 'bg-poetry-cinnabar shadow-poetry-cinnabar/25' : 'bg-gradient-to-br from-primary to-primary-light shadow-blue-500/20'"
          >
            <BookMarked v-if="isPoetry" class="w-5 h-5 text-white" />
            <BookOpen v-else class="w-5 h-5 text-white" />
          </div>
          <RouterLink
            :to="isPoetry ? '/poetry' : '/home'"
            class="text-lg font-semibold tracking-tight text-ink cursor-pointer"
            data-test="app-brand"
          >
            {{ isPoetry ? 'Peak 古诗文' : 'Peak 错题本' }}
          </RouterLink>
        </div>
        <nav class="flex items-center gap-1">
          <!-- 古诗文应用导航（浏览免登录，访客可见） -->
          <template v-if="isPoetry">
            <RouterLink
              to="/poetry"
              class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm font-medium transition-colors"
              :class="active === 'poetry-dashboard' ? 'bg-poetry-cinnabar/10 text-poetry-cinnabar' : 'text-ink-soft hover:bg-slate-100'"
            >
              <BookMarked class="w-4 h-4" />
              总览
            </RouterLink>
            <RouterLink
              to="/poetry/list"
              class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm font-medium transition-colors"
              :class="active === 'poetry-list' ? 'bg-poetry-cinnabar/10 text-poetry-cinnabar' : 'text-ink-soft hover:bg-slate-100'"
            >
              <Search class="w-4 h-4" />
              篇目检索
            </RouterLink>
          </template>
          <!-- 错题本应用导航 -->
          <template v-if="isLoggedIn && !isPoetry">
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
          <!-- 古诗文为独立应用：错题本导航不放置其入口，通过独立域名/地址访问 -->
          <div class="ml-3 flex items-center pl-3 border-l border-slate-200">
            <template v-if="isLoggedIn">
              <!-- 用户菜单：展示真实昵称，点击展开个人中心入口 -->
              <div class="relative">
                <button
                  class="flex items-center gap-2 px-2 py-1.5 rounded-lg hover:bg-slate-100 transition-colors cursor-pointer"
                  data-test="user-menu-trigger"
                  @click="menuOpen = !menuOpen"
                >
                  <div class="w-8 h-8 rounded-full bg-gradient-to-br from-primary/20 to-primary-light/30 flex items-center justify-center">
                    <User class="w-4 h-4 text-primary" />
                  </div>
                  <span class="text-sm font-medium text-ink max-w-24 truncate">{{ displayName }}</span>
                  <ChevronDown class="w-3.5 h-3.5 text-ink-faint" :class="menuOpen ? 'rotate-180' : ''" />
                </button>
                <div
                  v-if="menuOpen"
                  class="absolute right-0 top-full mt-2 w-44 bg-white rounded-xl border border-slate-200/80 shadow-lg py-1.5 animate-fade-up"
                >
                  <button
                    class="w-full flex items-center gap-2 px-3.5 py-2 text-sm text-ink-soft hover:bg-surface-tint hover:text-primary transition-colors cursor-pointer"
                    data-test="edit-name-trigger"
                    @click="openEdit"
                  >
                    <Pencil class="w-4 h-4" />
                    修改昵称
                  </button>
                  <button
                    class="w-full flex items-center gap-2 px-3.5 py-2 text-sm text-ink-soft hover:bg-red-50 hover:text-red-600 transition-colors cursor-pointer"
                    @click="handleLogout"
                  >
                    <LogIn class="w-4 h-4 rotate-180" />
                    退出登录
                  </button>
                </div>
              </div>
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
    <main class="flex-1 pt-16" @click="menuOpen = false">
      <RouterView />
    </main>

    <!-- 修改昵称弹窗 -->
    <div
      v-if="editOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-ink/40 backdrop-blur-sm p-4"
      @click.self="editOpen = false"
    >
      <div class="w-full max-w-sm bg-white rounded-2xl border border-slate-200/80 shadow-xl p-6 animate-fade-up">
        <h2 class="text-lg font-semibold text-ink">修改昵称</h2>
        <p class="mt-1 text-xs text-ink-faint">昵称将显示在顶部导航栏，最长 32 个字符</p>
        <input
          v-model="editName"
          type="text"
          maxlength="32"
          placeholder="请输入新昵称"
          data-test="nickname-input"
          class="mt-4 w-full px-3.5 py-2.5 rounded-xl border border-slate-200 bg-white text-sm text-ink placeholder:text-ink-faint outline-none focus:border-primary focus:ring-2 focus:ring-primary/15 transition-shadow"
          @keyup.enter="handleSaveName"
        />
        <p v-if="editError" class="mt-1.5 text-xs text-red-600">{{ editError }}</p>
        <div class="mt-5 flex justify-end gap-2">
          <button
            class="px-4 py-2 rounded-xl text-sm font-medium text-ink-soft hover:bg-slate-100 transition-colors cursor-pointer"
            @click="editOpen = false"
          >
            取消
          </button>
          <button
            class="px-4 py-2 rounded-xl bg-primary text-white text-sm font-semibold shadow-md shadow-blue-500/20 hover:bg-primary-light transition-colors cursor-pointer disabled:cursor-not-allowed disabled:bg-slate-300 disabled:shadow-none"
            :disabled="saving"
            data-test="nickname-save"
            @click="handleSaveName"
          >
            保存
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
