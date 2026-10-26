<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import CitationList from '../components/CitationList.vue'
import { ask } from '../api/client'
import type { Citation } from '../api/types'
import { renderMarkdown } from '../markdown'
import { session } from '../session'

interface ChatItem {
  id: string
  role: 'user' | 'assistant'
  text: string
  citations?: Citation[]
  warning?: string
  error?: boolean
}

const input = ref('')
const topK = ref(8)
const loading = ref(false)
const messages = ref<ChatItem[]>([])
const listRef = ref<HTMLElement | null>(null)

async function scrollToBottom() {
  await nextTick()
  if (listRef.value) {
    listRef.value.scrollTop = listRef.value.scrollHeight
  }
}

async function send() {
  const question = input.value.trim()
  if (!question || loading.value) {
    return
  }
  messages.value.push({
    id: Math.random().toString(36).substring(2, 9),
    role: 'user',
    text: question,
  })
  input.value = ''
  loading.value = true
  await scrollToBottom()
  try {
    const res = await ask(question, topK.value)
    messages.value.push({
      id: Math.random().toString(36).substring(2, 9),
      role: 'assistant',
      text: res.answer,
      citations: res.citations,
      warning: res.warning,
    })
  } catch (e) {
    messages.value.push({
      id: Math.random().toString(36).substring(2, 9),
      role: 'assistant',
      text: e instanceof Error ? e.message : '请求失败',
      error: true,
    })
  } finally {
    loading.value = false
    await scrollToBottom()
  }
}

function clearMessages() {
  messages.value = []
}

watch(
  () => session.user?.tenant_slug,
  () => {
    messages.value = []
  },
)

async function copyAnswer(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制')
  } catch {
    ElMessage.error('复制失败，请手动选择复制')
  }
}
</script>

<template>
  <div class="chat-page">
    <div class="chat-header">
      <div>
        <h2>知识问答</h2>
        <p class="muted">基于已入库文档回答，并列出引用片段。</p>
      </div>
      <div class="header-actions">
        <el-select v-model="topK" size="small" class="topk-select">
          <el-option label="Top 4" :value="4" />
          <el-option label="Top 8" :value="8" />
          <el-option label="Top 12" :value="12" />
        </el-select>
        <el-button text :disabled="loading || messages.length === 0" @click="clearMessages">
          清空对话
        </el-button>
      </div>
    </div>

    <div ref="listRef" class="chat-list">
      <div v-if="messages.length === 0 && !loading" class="chat-empty">
        <p class="empty-title">暂无对话</p>
        <p class="muted">上传文档后，在下方输入问题开始检索。</p>
      </div>

      <div
        v-for="message in messages"
        :key="message.id"
        class="message"
        :class="[message.role, { 'has-error': message.error }]"
      >
        <div v-if="message.role === 'assistant'" class="message-label">助手</div>
        <div class="bubble">
          <template v-if="message.role === 'assistant'">
            <div v-if="message.text" class="markdown" v-html="renderMarkdown(message.text)" />
            <el-alert
              v-if="message.warning"
              :title="message.warning"
              type="warning"
              :closable="false"
              show-icon
              class="message-alert"
            />
            <CitationList
              v-if="message.citations && message.citations.length"
              :citations="message.citations"
            />
          </template>
          <template v-else>{{ message.text }}</template>
        </div>
        <button
          v-if="message.role === 'assistant' && message.text && !message.error"
          class="copy-btn"
          @click="copyAnswer(message.text)"
        >
          复制
        </button>
      </div>

      <div v-if="loading" class="message assistant">
        <div class="message-label">助手</div>
        <div class="bubble loading-bubble">正在检索并生成…</div>
      </div>
    </div>

    <div class="composer">
      <el-input
        v-model="input"
        type="textarea"
        :autosize="{ minRows: 2, maxRows: 6 }"
        resize="none"
        placeholder="输入问题，Enter 发送，Shift+Enter 换行"
        @keydown.enter.exact.prevent="send"
      />
      <div class="composer-actions">
        <span class="shortcut-hint">Enter 发送 / Shift+Enter 换行</span>
        <el-button type="primary" :loading="loading" :disabled="!input.trim()" @click="send">
          发送
        </el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
}

.chat-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 22px 32px 16px;
  border-bottom: 1px solid #e5e7eb;
  background: #ffffff;
}

.chat-header h2 {
  margin: 0 0 4px;
  font-size: 18px;
  font-weight: 600;
  color: #111827;
}

.chat-header p {
  margin: 0;
  font-size: 13px;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.topk-select {
  width: 96px;
}

.chat-list {
  flex: 1;
  overflow-y: auto;
  padding: 24px 32px;
  background: #f7f8fa;
}

.chat-empty {
  max-width: 760px;
  margin: 96px auto 0;
  text-align: center;
}

.empty-title {
  margin: 0 0 6px;
  font-size: 15px;
  font-weight: 500;
  color: #374151;
}

.chat-empty .muted {
  margin: 0;
  font-size: 13px;
}

.message {
  max-width: 760px;
  margin: 0 auto 20px;
}

.message.user {
  display: flex;
  justify-content: flex-end;
}

.message-label {
  margin-bottom: 6px;
  font-size: 12px;
  color: #6b7280;
}

.bubble {
  padding: 12px 16px;
  border-radius: 8px;
  line-height: 1.7;
  word-break: break-word;
}

.message.assistant .bubble {
  border: 1px solid #e5e7eb;
  background: #ffffff;
  color: #1f2937;
}

.message.user .bubble {
  max-width: 82%;
  background: #f3f4f6;
  color: #111827;
  white-space: pre-wrap;
}

.message.has-error .bubble {
  border-color: #fecaca;
  background: #fef2f2;
  color: #b91c1c;
}

.message-alert {
  margin-top: 10px;
}

.copy-btn {
  margin-top: 6px;
  padding: 0;
  border: none;
  background: transparent;
  color: #9ca3af;
  font-size: 12px;
  cursor: pointer;
}

.copy-btn:hover {
  color: #2563eb;
}

.loading-bubble {
  border: 1px solid #e5e7eb;
  background: #ffffff;
  color: #6b7280;
  font-size: 13px;
}

.composer {
  padding: 14px 32px 20px;
  border-top: 1px solid #e5e7eb;
  background: #ffffff;
}

.composer :deep(.el-textarea__inner) {
  font-size: 14px;
  line-height: 1.6;
}

.composer-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 10px;
}

.shortcut-hint {
  font-size: 12px;
  color: #9ca3af;
}

@media (max-width: 760px) {
  .chat-header,
  .chat-list,
  .composer {
    padding-left: 16px;
    padding-right: 16px;
  }
}
</style>
