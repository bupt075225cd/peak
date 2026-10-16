<script setup lang="ts">
import { ref } from 'vue'
import { Sparkles, X, Send } from 'lucide-vue-next'
import { sendChat, type ChatResponse } from '../../api/poetry'

// AiChatPlaceholder AI 智能对话占位弹窗：
// 后端 /api/poetry/chat 为 RAG 二阶段预留接口，当前返回"未启用"提示。
const open = ref(false)
const question = ref('')
const response = ref<ChatResponse | null>(null)
const sending = ref(false)

async function handleSend() {
  const q = question.value.trim()
  if (!q || sending.value) return
  sending.value = true
  try {
    response.value = await sendChat(q)
  } catch (err) {
    console.error('发送对话请求失败', err)
    response.value = { enabled: false, notice: '请求失败，请稍后重试' }
  } finally {
    sending.value = false
  }
}

function show() {
  open.value = true
  response.value = null
  question.value = ''
}

defineExpose({ show })
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-50 flex items-end sm:items-center justify-center bg-ink/40 backdrop-blur-sm p-4"
      data-test="ai-chat-modal"
      @click.self="open = false"
    >
      <div
        class="w-full max-w-md rounded-2xl border border-[#E5DFD0] bg-poetry-paper shadow-xl p-5 animate-fade-up"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <div class="w-8 h-8 rounded-lg bg-poetry-cinnabar/10 flex items-center justify-center">
              <Sparkles class="w-4 h-4 text-poetry-cinnabar" />
            </div>
            <h3 class="font-serif text-base font-semibold text-poetry-inkstone">古诗文智能问答</h3>
          </div>
          <button
            class="p-1.5 rounded-lg text-ink-soft hover:bg-black/5 transition-colors cursor-pointer"
            data-test="ai-chat-close"
            aria-label="关闭"
            @click="open = false"
          >
            <X class="w-4 h-4" />
          </button>
        </div>

        <p class="mt-2 text-xs leading-relaxed text-[#8A8272]">
          基于全部 124 篇教材古诗文语料的 RAG 智能问答即将上线，现在可先提交问题体验交互。
        </p>

        <div class="mt-4 flex gap-2">
          <input
            v-model="question"
            type="text"
            maxlength="200"
            placeholder="例如：《岳阳楼记》表达了怎样的情怀？"
            class="flex-1 px-3.5 py-2.5 rounded-xl border border-[#E0D9C8] bg-white text-sm text-ink placeholder:text-ink-faint outline-none focus:border-poetry-cinnabar focus:ring-2 focus:ring-poetry-cinnabar/15 transition-shadow"
            data-test="ai-chat-input"
            @keyup.enter="handleSend"
          />
          <button
            class="inline-flex items-center gap-1.5 px-4 py-2.5 rounded-xl bg-poetry-cinnabar text-white text-sm font-semibold shadow-md shadow-poetry-cinnabar/20 hover:bg-poetry-cinnabarLight transition-colors cursor-pointer disabled:cursor-not-allowed disabled:bg-[#D8D2C4] disabled:shadow-none"
            :disabled="sending || !question.trim()"
            data-test="ai-chat-send"
            @click="handleSend"
          >
            <Send class="w-4 h-4" />
            发送
          </button>
        </div>

        <div
          v-if="response"
          class="mt-4 rounded-xl border border-[#E0D9C8] bg-white/70 p-3.5 text-sm leading-relaxed text-poetry-inkstone"
          data-test="ai-chat-response"
        >
          {{ response.notice }}
        </div>
      </div>
    </div>
  </Teleport>
</template>
