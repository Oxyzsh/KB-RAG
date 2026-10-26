<script setup lang="ts">
import { nextTick, ref } from 'vue'
import CitationList from '../components/CitationList.vue'
import { ask } from '../api/client'
import type { Citation } from '../api/types'
import { renderMarkdown } from '../markdown'

interface ChatItem {
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
  messages.value.push({ role: 'user', text: question })
  input.value = ''
  loading.value = true
  await scrollToBottom()
  try {
    const res = await ask(question, topK.value)
    messages.value.push({
      role: 'assistant',
      text: res.answer,
      citations: res.citations,
      warning: res.warning,
    })
  } catch (e) {
    messages.value.push({
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
</script>

<template>
  <div class="page chat-page">
    <div class="page-header chat-header">
      <div>
        <h2>知识问答</h2>
        <p class="muted">基于已入库文档回答，并在下方列出引用片段。</p>
      </div>
      <div class="header-actions">
        <el-select v-model="topK" size="small" class="topk-select">
          <el-option label="top 4" :value="4" />
          <el-option label="top 8" :value="8" />
          <el-option label="top 12" :value="12" />
        </el-select>
        <el-button text :disabled="loading || messages.length === 0" @click="clearMessages">清空对话</el-button>
      </div>
    </div>

    <div ref="listRef" class="chat-list">
      <el-empty v-if="messages.length === 0 && !loading" description="上传文档后，在这里提问" />
      <div
        v-for="(message, index) in messages"
        :key="index"
        class="message"
        :class="[message.role, { error: message.error }]"
      >
        <div class="bubble">
          <template v-if="message.role === 'assistant'">
            <div v-if="message.text" class="markdown" v-html="renderMarkdown(message.text)" />
            <el-alert
              v-if="message.warning"
              :title="message.warning"
              type="warning"
              :closable="false"
              show-icon
            />
            <CitationList v-if="message.citations && message.citations.length" :citations="message.citations" />
          </template>
          <template v-else>{{ message.text }}</template>
        </div>
      </div>
      <div v-if="loading" class="message assistant">
        <div class="bubble typing">正在检索并生成…</div>
      </div>
    </div>

    <div class="composer" @keydown.ctrl.enter.prevent="send">
      <el-input
        v-model="input"
        type="textarea"
        :rows="2"
        resize="none"
        placeholder="输入问题，Ctrl+Enter 发送"
      />
      <el-button type="primary" :loading="loading" @click="send">发送</el-button>
    </div>
  </div>
</template>

<style scoped>
.chat-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  padding: 0;
}

.chat-header {
  padding: 24px 32px 12px;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.topk-select {
  width: 96px;
}

.chat-list {
  flex: 1;
  overflow-y: auto;
  padding: 12px 32px 20px;
}

.message {
  display: flex;
  margin: 14px 0;
}

.message.user {
  justify-content: flex-end;
}

.bubble {
  max-width: min(760px, 92%);
  padding: 12px 16px;
  border: 1px solid #e4e7ec;
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 1px 2px rgba(16, 24, 40, 0.04);
  line-height: 1.7;
  word-break: break-word;
}

.message.assistant .bubble {
  width: min(880px, 100%);
}

.message.user .bubble {
  border-color: #2e90fa;
  background: #2e90fa;
  color: #fff;
}

.message.error .bubble {
  border-color: #fda29b;
  background: #fef3f2;
  color: #b42318;
}

.typing {
  color: #667085;
}

.composer {
  display: flex;
  gap: 12px;
  padding: 16px 32px 24px;
  border-top: 1px solid #eaecf0;
  background: #f7f8fa;
}

.composer .el-textarea {
  flex: 1;
}

@media (max-width: 760px) {
  .chat-header,
  .chat-list,
  .composer {
    padding-left: 18px;
    padding-right: 18px;
  }
}
</style>
