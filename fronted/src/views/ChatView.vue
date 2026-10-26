<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { ElMessage } from 'element-plus'
import CitationList from '../components/CitationList.vue'
import { ask } from '../api/client'
import type { Citation } from '../api/types'
import { renderMarkdown } from '../markdown'

interface ChatItem {
  id: string
  role: 'user' | 'assistant'
  text: string
  citations?: Citation[]
  warning?: string
  error?: boolean
  copied?: boolean
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

async function copyAnswer(msg: ChatItem) {
  try {
    await navigator.clipboard.writeText(msg.text)
    msg.copied = true
    ElMessage.success('已复制答案')
    setTimeout(() => {
      msg.copied = false
    }, 2000)
  } catch {
    ElMessage.error('复制失败，请手动选择复制')
  }
}
</script>

<template>
  <div class="chat-container">
    <header class="chat-header">
      <div class="header-info">
        <h2 class="header-title">知识问答</h2>
        <span class="header-desc">基于知识库文档精准检索与生成，溯源原文</span>
      </div>
      <div class="header-tools">
        <div class="topk-wrapper">
          <span class="tool-label">检索条数</span>
          <el-select v-model="topK" size="default" class="topk-select">
            <el-option label="Top 4" :value="4" />
            <el-option label="Top 8" :value="8" />
            <el-option label="Top 12" :value="12" />
          </el-select>
        </div>
        <el-button
          class="clear-btn"
          text
          :disabled="loading || messages.length === 0"
          @click="clearMessages"
        >
          清空对话
        </el-button>
      </div>
    </header>

    <main ref="listRef" class="chat-main">
      <div v-if="messages.length === 0 && !loading" class="empty-state">
        <div class="empty-icon">💡</div>
        <h3>开启智能知识检索</h3>
        <p>在知识库上传文档后，提出任何业务或技术问题，获取精准答案和原文引用引用。</p>
      </div>

      <div class="message-feed">
        <div
          v-for="message in messages"
          :key="message.id"
          class="message-row"
          :class="[message.role, { 'has-error': message.error }]"
        >
          <div class="avatar">
            <span v-if="message.role === 'user'">👤</span>
            <span v-else>🤖</span>
          </div>
          <div class="message-content">
            <div class="bubble">
              <template v-if="message.role === 'assistant'">
                <div v-if="message.text" class="markdown" v-html="renderMarkdown(message.text)" />
                <el-alert
                  v-if="message.warning"
                  :title="message.warning"
                  type="warning"
                  :closable="false"
                  show-icon
                  class="msg-alert"
                />
                <CitationList
                  v-if="message.citations && message.citations.length"
                  :citations="message.citations"
                />
              </template>
              <template v-else>
                <div class="user-text">{{ message.text }}</div>
              </template>
            </div>

            <div v-if="message.role === 'assistant' && message.text" class="message-actions">
              <button class="action-btn" @click="copyAnswer(message)">
                {{ message.copied ? '✓ 已复制' : '复制内容' }}
              </button>
            </div>
          </div>
        </div>

        <div v-if="loading" class="message-row assistant">
          <div class="avatar">🤖</div>
          <div class="message-content">
            <div class="bubble loading-bubble">
              <div class="loading-dots">
                <span></span><span></span><span></span>
              </div>
              <span class="loading-text">正在检索相关文档并生成解答…</span>
            </div>
          </div>
        </div>
      </div>
    </main>

    <footer class="chat-footer">
      <div class="input-card">
        <el-input
          v-model="input"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 6 }"
          resize="none"
          placeholder="提出您的问题… (Enter 发送，Shift+Enter 换行)"
          class="main-input"
          @keydown.enter.exact.prevent="send"
        />
        <div class="input-toolbar">
          <span class="shortcut-tip">按 <strong>Enter</strong> 发送，<strong>Shift+Enter</strong> 换行</span>
          <el-button
            type="primary"
            class="send-btn"
            :loading="loading"
            :disabled="!input.trim()"
            @click="send"
          >
            发送
          </el-button>
        </div>
      </div>
    </footer>
  </div>
</template>

<style scoped>
.chat-container {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: #f8fafc;
}

.chat-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 36px;
  background: #ffffff;
  border-bottom: 1px solid #e2e8f0;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.02);
}

.header-title {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
  color: #0f172a;
}

.header-desc {
  margin-top: 2px;
  font-size: 13px;
  color: #64748b;
  display: block;
}

.header-tools {
  display: flex;
  align-items: center;
  gap: 16px;
}

.topk-wrapper {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tool-label {
  font-size: 13px;
  color: #64748b;
}

.topk-select {
  width: 96px;
}

.chat-main {
  flex: 1;
  overflow-y: auto;
  padding: 24px 36px;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 80%;
  text-align: center;
}

.empty-icon {
  font-size: 40px;
  margin-bottom: 12px;
}

.empty-state h3 {
  margin: 0 0 8px;
  font-size: 18px;
  color: #0f172a;
  font-weight: 600;
}

.empty-state p {
  margin: 0;
  color: #64748b;
  font-size: 14px;
  max-width: 440px;
  line-height: 1.6;
}

.message-feed {
  max-width: 900px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.message-row {
  display: flex;
  gap: 14px;
  align-items: flex-start;
}

.message-row.user {
  flex-direction: row-reverse;
}

.avatar {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: #e2e8f0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  flex-shrink: 0;
}

.message-row.user .avatar {
  background: #2563eb;
}

.message-content {
  max-width: calc(100% - 50px);
  display: flex;
  flex-direction: column;
}

.bubble {
  padding: 14px 18px;
  border-radius: 14px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  line-height: 1.6;
  position: relative;
  word-break: break-word;
}

.message-row.assistant .bubble {
  background: #ffffff;
  border: 1px solid #e2e8f0;
  color: #1e293b;
  border-top-left-radius: 4px;
}

.message-row.user .bubble {
  background: #2563eb;
  color: #ffffff;
  border-top-right-radius: 4px;
}

.user-text {
  font-size: 14px;
  line-height: 1.6;
  white-space: pre-wrap;
}

.message-row.has-error .bubble {
  background: #fef2f2;
  border: 1px solid #fecaca;
  color: #dc2626;
}

.message-actions {
  display: flex;
  gap: 8px;
  margin-top: 6px;
  padding-left: 2px;
}

.action-btn {
  background: transparent;
  border: none;
  font-size: 12px;
  color: #94a3b8;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 4px;
  transition: all 0.15s ease;
}

.action-btn:hover {
  color: #2563eb;
  background: #eff6ff;
}

.msg-alert {
  margin-top: 10px;
}

.loading-bubble {
  display: flex;
  align-items: center;
  gap: 10px;
}

.loading-dots {
  display: flex;
  gap: 4px;
}

.loading-dots span {
  width: 6px;
  height: 6px;
  background: #3b82f6;
  border-radius: 50%;
  animation: pulse 1.4s infinite ease-in-out both;
}

.loading-dots span:nth-child(1) { animation-delay: -0.32s; }
.loading-dots span:nth-child(2) { animation-delay: -0.16s; }

@keyframes pulse {
  0%, 80%, 100% { transform: scale(0); }
  40% { transform: scale(1); }
}

.loading-text {
  font-size: 13px;
  color: #64748b;
}

.chat-footer {
  padding: 16px 36px 24px;
  background: #ffffff;
  border-top: 1px solid #e2e8f0;
}

.input-card {
  max-width: 900px;
  margin: 0 auto;
  border: 1px solid #cbd5e1;
  border-radius: 12px;
  padding: 10px 14px;
  background: #ffffff;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.03);
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

.input-card:focus-within {
  border-color: #2563eb;
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
}

.main-input :deep(.el-textarea__inner) {
  box-shadow: none !important;
  padding: 0;
  font-size: 14px;
  line-height: 1.6;
  border: none !important;
  color: #0f172a;
}

.input-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 8px;
  padding-top: 6px;
  border-top: 1px solid #f1f5f9;
}

.shortcut-tip {
  font-size: 12px;
  color: #94a3b8;
}

.shortcut-tip strong {
  color: #64748b;
}

.send-btn {
  padding: 8px 18px;
  border-radius: 8px;
}

@media (max-width: 768px) {
  .chat-header, .chat-main, .chat-footer {
    padding-left: 16px;
    padding-right: 16px;
  }
}
</style>
