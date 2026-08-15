<script setup lang="ts">
import { useRoute } from 'vue-router'
import { Camera, BookOpen, User } from 'lucide-vue-next'
import { computed } from 'vue'

const route = useRoute()
const active = computed(() => route.name)
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
          <span class="text-lg font-semibold tracking-tight text-ink">Peak 错题本</span>
        </div>
        <nav class="flex items-center gap-1">
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
          <div class="ml-3 flex items-center gap-2 pl-3 border-l border-slate-200">
            <div class="w-8 h-8 rounded-full bg-gradient-to-br from-slate-200 to-slate-300 flex items-center justify-center">
              <User class="w-4 h-4 text-ink-soft" />
            </div>
            <span class="text-sm text-ink-soft">小明</span>
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
