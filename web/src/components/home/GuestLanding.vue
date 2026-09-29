<script setup lang="ts">
import { Camera, BookOpen, Sparkles, BarChart3, Download, Shapes, LogIn, ArrowRight } from 'lucide-vue-next'

// 功能亮点：对应产品学习闭环的真实能力（README 产品定位）。
const features = [
  {
    icon: Camera,
    title: '拍照录入',
    desc: '手机拍下错题，自动裁切识别题干，手写笔记也能一键擦除还原干净卷面。',
    tint: 'bg-blue-50 text-primary',
  },
  {
    icon: Sparkles,
    title: 'AI 识别整理',
    desc: '题干、答案、知识点自动识别归档，几何图形还能 AI 重绘成清晰矢量图。',
    tint: 'bg-violet-50 text-violet-600',
  },
  {
    icon: BookOpen,
    title: '分类管理',
    desc: '按学科、年级、来源多维筛选，快速定位薄弱知识点，复习不再大海捞针。',
    tint: 'bg-emerald-50 text-emerald-600',
  },
  {
    icon: BarChart3,
    title: '组卷练习',
    desc: '勾选错题一键导出 PDF / Word 试卷，重做检验掌握程度，记录每一步进步。',
    tint: 'bg-amber-50 text-amber-600',
  },
]
</script>

<template>
  <div class="pb-16">
    <!-- Hero 区 -->
    <section class="relative overflow-hidden bg-gradient-to-b from-surface-tint via-surface-muted to-surface-muted">
      <div class="mx-auto max-w-5xl px-4 pt-16 pb-20 lg:pt-24 lg:pb-28">
        <div class="flex flex-col lg:flex-row items-center gap-12">
          <div class="flex-1 animate-fade-up">
            <div
              class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-white/80 border border-primary/20 text-xs font-medium text-primary shadow-sm"
            >
              <Sparkles class="w-3.5 h-3.5" />
              面向中学生的智能错题本
            </div>
            <h1 class="mt-5 text-4xl lg:text-5xl font-semibold leading-tight tracking-tight text-ink">
              让每一道错题
              <br />
              都成为<span class="text-transparent bg-clip-text bg-gradient-to-r from-primary to-primary-light">进步的阶梯</span>
            </h1>
            <p class="mt-5 text-base lg:text-lg text-ink-soft leading-relaxed max-w-xl">
              拍照收录错题，AI 自动识别整理；按学科分类管理，薄弱点一目了然；
              随时组卷重练，记录你的每一步成长。
            </p>
            <div class="mt-8 flex flex-wrap items-center gap-3">
              <RouterLink
                to="/login"
                class="inline-flex items-center gap-2 px-6 py-3 rounded-xl bg-primary text-white text-sm font-semibold shadow-lg shadow-blue-500/25 hover:bg-primary-light hover:shadow-blue-500/40 hover:-translate-y-0.5 transition-all duration-200 cursor-pointer"
              >
                <LogIn class="w-4 h-4" />
                立即登录
              </RouterLink>
              <RouterLink
                to="/list"
                class="inline-flex items-center gap-2 px-6 py-3 rounded-xl bg-white text-ink text-sm font-semibold border border-slate-200 shadow-sm hover:border-primary/40 hover:text-primary hover:-translate-y-0.5 transition-all duration-200 cursor-pointer"
              >
                先随便看看
                <ArrowRight class="w-4 h-4" />
              </RouterLink>
            </div>
          </div>

          <!-- 装饰性插画：错题卡片浮动 -->
          <div class="flex-1 w-full max-w-sm animate-fade-up [animation-delay:100ms]">
            <div class="relative">
              <div
                class="absolute -inset-6 bg-gradient-to-br from-primary/10 via-primary-light/10 to-transparent rounded-[2rem] blur-xl"
              />
              <div
                class="relative bg-white rounded-2xl border border-slate-200/80 shadow-xl shadow-blue-500/5 p-6 animate-float"
              >
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-xl bg-primary/10 flex items-center justify-center">
                    <Shapes class="w-5 h-5 text-primary" />
                  </div>
                  <div>
                    <div class="text-sm font-semibold text-ink">数学 · 二次函数</div>
                    <div class="text-xs text-ink-faint mt-0.5">来源：期中考试</div>
                  </div>
                </div>
                <div class="mt-4 space-y-2">
                  <div class="h-2.5 rounded-full bg-slate-100 w-full" />
                  <div class="h-2.5 rounded-full bg-slate-100 w-4/5" />
                  <div class="h-2.5 rounded-full bg-surface-tint w-3/5" />
                </div>
                <div class="mt-5 flex items-center gap-2">
                  <span class="px-2 py-1 rounded-md bg-primary/10 text-primary text-xs font-medium">函数与方程</span>
                  <span class="px-2 py-1 rounded-md bg-violet-50 text-violet-600 text-xs font-medium">待复习</span>
                </div>
              </div>
              <div
                class="absolute -bottom-5 -left-5 bg-white rounded-xl border border-slate-200/80 shadow-lg px-4 py-3 flex items-center gap-2.5 animate-float [animation-delay:400ms]"
              >
                <Download class="w-4 h-4 text-emerald-600" />
                <span class="text-xs font-medium text-ink">已导出「期中错题卷.pdf」</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 功能亮点 -->
    <section class="mx-auto max-w-5xl px-4 mt-16">
      <h2 class="text-2xl font-semibold tracking-tight text-ink text-center">学习闭环，一站搞定</h2>
      <p class="mt-2 text-sm text-ink-soft text-center">收集错题 → 分类管理 → 组卷练习 → 记录练习数据</p>
      <div class="mt-10 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-5">
        <div
          v-for="(f, i) in features"
          :key="f.title"
          class="group bg-white rounded-2xl border border-slate-200/80 p-6 shadow-sm hover:shadow-xl hover:shadow-blue-500/5 hover:-translate-y-1 transition-all duration-300 animate-fade-up"
          :style="{ animationDelay: `${i * 80}ms` }"
        >
          <div :class="[f.tint, 'w-11 h-11 rounded-xl flex items-center justify-center']">
            <component :is="f.icon" class="w-5 h-5" />
          </div>
          <h3 class="mt-4 text-base font-semibold text-ink">{{ f.title }}</h3>
          <p class="mt-2 text-sm text-ink-soft leading-relaxed">{{ f.desc }}</p>
        </div>
      </div>
    </section>

    <!-- 底部引导 -->
    <section class="mx-auto max-w-5xl px-4 mt-16">
      <div
        class="relative overflow-hidden rounded-2xl bg-gradient-to-r from-primary to-primary-light px-8 py-10 text-center shadow-xl shadow-blue-500/20"
      >
        <h2 class="text-2xl font-semibold text-white">现在开始，建立你的错题本</h2>
        <p class="mt-2 text-sm text-blue-100">登录后即可收录第一道错题，让复习有据可依。</p>
        <RouterLink
          to="/login"
          class="mt-6 inline-flex items-center gap-2 px-6 py-3 rounded-xl bg-white text-primary text-sm font-semibold shadow-lg hover:-translate-y-0.5 hover:shadow-xl transition-all duration-200 cursor-pointer"
        >
          <LogIn class="w-4 h-4" />
          免费登录使用
        </RouterLink>
      </div>
    </section>
  </div>
</template>
