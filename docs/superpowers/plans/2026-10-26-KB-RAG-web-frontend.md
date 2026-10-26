# KB-RAG Web 前端实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `fronted/` 下实现 Vue 3 单页应用：登录、问答与引用、知识库统计、文档上传/列表/重试/删除。

**Architecture:** Vue 3 + TypeScript + Vite + Element Plus + Vue Router + markdown-it；前端通过 `/api` 调用后端计划实现的 API；生产构建到 `fronted/dist`，由 Go `serve` 从磁盘托管；开发时 Vite 代理 `/api` 到 `127.0.0.1:8080`。

**Tech Stack:** Vue 3、TypeScript、Vite、Element Plus、Vue Router、markdown-it、npm。

**设计依据：** [`docs/superpowers/specs/2026-10-26-KB-RAG-web-design.md`](../specs/2026-10-26-KB-RAG-web-design.md)
**后端计划：** [`docs/superpowers/plans/2026-10-26-KB-RAG-web-backend.md`](2026-10-26-KB-RAG-web-backend.md)

## Global Constraints

- 目录固定为 `fronted/`（用户指定名称，不改为 `frontend`）。
- 前端只保存 Web Token（`sessionStorage`）；永不保存 `KB_RAG_LLM_API_KEY`。
- API 契约以后端计划的 DTO 为准：`documents`、`documents_failed`、`chunks`、`size_bytes`、`embedding`、`answer`、`generated`、`warning`、`citations` 等字段名不得改动。
- 路由使用 hash 模式，Go 静态服务不需要 SPA fallback。
- 答案使用 markdown-it，`html:false`；引用片段使用文本插值，禁止原始 HTML 渲染。
- 删除确认文案必须包含“只删除索引，源文件保留”。
- 所有界面文案使用中文；代码标识符使用英文。
- `fronted/node_modules/`、`fronted/dist/` 不提交；`package-lock.json` 提交。
- 构建门槛：`npm run type-check` 与 `npm run build` 必须通过。

## File Structure

```text
fronted/
  package.json
  package-lock.json            npm install 生成并提交
  vite.config.ts
  tsconfig.json
  index.html
  README.md
  src/
    env.d.ts
    main.ts
    App.vue
    styles.css
    api/
      types.ts
      client.ts
    router/index.ts
    markdown.ts
    views/
      LoginView.vue
      ChatView.vue             Task 2 替换占位
      DocumentsView.vue        Task 3 替换占位
    components/
      AppShell.vue
      CitationList.vue         Task 2
      UploadPanel.vue          Task 3
      DocumentTable.vue        Task 3
```

---

### Task 1: Vue 脚手架、登录、API 客户端与应用外壳

**Files:**
- Create: `fronted/package.json`、`fronted/vite.config.ts`、`fronted/tsconfig.json`、`fronted/index.html`
- Create: `fronted/src/env.d.ts`、`fronted/src/main.ts`、`fronted/src/App.vue`、`fronted/src/styles.css`
- Create: `fronted/src/api/types.ts`、`fronted/src/api/client.ts`
- Create: `fronted/src/router/index.ts`
- Create: `fronted/src/views/LoginView.vue`
- Create: `fronted/src/components/AppShell.vue`
- Create: `fronted/src/views/ChatView.vue`（占位）
- Create: `fronted/src/views/DocumentsView.vue`（占位）
- Create: `fronted/README.md`
- Modify: `.gitignore`

**Interfaces:**
- Produces: 可构建的 Vue 应用、路由、`api/client.ts` 的请求方法、登录页、应用外壳。
- `api/client.ts` 提供的函数在后续任务中使用：
  - `verifySession(): Promise<{ok:boolean}>`
  - `getStats(): Promise<Stats>`
  - `listDocuments(): Promise<{documents: DocumentItem[]}>`
  - `uploadDocument(file: File): Promise<UploadResponse>`
  - `deleteDocument(id: number): Promise<void>`
  - `reingestDocument(id: number): Promise<UploadResponse>`
  - `ask(question: string, topK: number): Promise<AskResponse>`

- [ ] **Step 1: 创建 `fronted/package.json`**

```json
{
  "name": "kb-rag-fronted",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vue-tsc --noEmit && vite build",
    "type-check": "vue-tsc --noEmit",
    "preview": "vite preview"
  },
  "dependencies": {
    "element-plus": "^2.9.3",
    "markdown-it": "^14.1.0",
    "vue": "^3.5.13",
    "vue-router": "^4.5.0"
  },
  "devDependencies": {
    "@types/markdown-it": "^14.1.2",
    "@vitejs/plugin-vue": "^5.2.1",
    "typescript": "^5.7.2",
    "vite": "^6.0.7",
    "vue-tsc": "^2.2.0"
  }
}
```

- [ ] **Step 2: 创建 Vite 与 TypeScript 配置**

`fronted/vite.config.ts`：

```ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
})
```

`fronted/tsconfig.json`：

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "strict": true,
    "jsx": "preserve",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "esModuleInterop": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "skipLibCheck": true,
    "noEmit": true,
    "types": ["vite/client", "element-plus/global"]
  },
  "include": ["src/**/*.ts", "src/**/*.d.ts", "src/**/*.vue", "vite.config.ts"]
}
```

`fronted/index.html`：

```html
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>KB-RAG 知识库</title>
  </head>
  <body>
    <div id="app"></div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
```

`fronted/src/env.d.ts`：

```ts
/// <reference types="vite/client" />
```

- [ ] **Step 3: 创建 API 类型与客户端**

`fronted/src/api/types.ts`：

```ts
export interface EmbeddingInfo {
  model: string
  dim: number
  chunk_size: number
}

export interface Stats {
  documents: number
  documents_failed: number
  chunks: number
  size_bytes: number
  embedding: EmbeddingInfo
}

export type DocumentStatus = 'ok' | 'failed'
export type DocumentSource = 'upload' | 'external'

export interface DocumentItem {
  id: number
  title: string
  name: string
  status: DocumentStatus
  chunks: number
  source: DocumentSource
  error: string
  updated_at: string
}

export interface Citation {
  n: number
  source: string
  heading: string
  text: string
  score: number
}

export interface AskResponse {
  answer: string
  generated: boolean
  warning: string
  citations: Citation[]
}

export interface UploadResponse {
  document: DocumentItem
  outcome: 'ok' | 'failed' | 'skipped'
}

export interface DocumentListResponse {
  documents: DocumentItem[]
}

export interface ErrorBody {
  error: string
}
```

`fronted/src/api/client.ts`：

```ts
import type {
  AskResponse,
  DocumentListResponse,
  ErrorBody,
  Stats,
  UploadResponse,
} from './types'

const TOKEN_KEY = 'kbrag_token'

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export function getToken(): string {
  return sessionStorage.getItem(TOKEN_KEY) ?? ''
}

export function setToken(token: string): void {
  sessionStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  sessionStorage.removeItem(TOKEN_KEY)
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  if (init.body && !(init.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const res = await fetch(`/api${path}`, { ...init, headers })
  const text = await res.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = null
    }
  }

  if (!res.ok) {
    if (res.status === 401) {
      clearToken()
      if (!window.location.hash.startsWith('#/login')) {
        window.location.hash = '#/login'
      }
    }
    const message = (data as ErrorBody | null)?.error ?? `请求失败（HTTP ${res.status}）`
    throw new ApiError(res.status, message)
  }
  return data as T
}

export function verifySession(): Promise<{ ok: boolean }> {
  return request<{ ok: boolean }>('/session')
}

export function getStats(): Promise<Stats> {
  return request<Stats>('/stats')
}

export function listDocuments(): Promise<DocumentListResponse> {
  return request<DocumentListResponse>('/documents')
}

export function uploadDocument(file: File): Promise<UploadResponse> {
  const form = new FormData()
  form.append('file', file)
  return request<UploadResponse>('/documents', { method: 'POST', body: form })
}

export function deleteDocument(id: number): Promise<void> {
  return request<void>(`/documents/${id}`, { method: 'DELETE' })
}

export function reingestDocument(id: number): Promise<UploadResponse> {
  return request<UploadResponse>(`/documents/${id}/reingest`, { method: 'POST' })
}

export function ask(question: string, topK: number): Promise<AskResponse> {
  return request<AskResponse>('/ask', {
    method: 'POST',
    body: JSON.stringify({ question, top_k: topK }),
  })
}
```

- [ ] **Step 4: 创建路由、入口与应用外壳**

`fronted/src/router/index.ts`：

```ts
import { createRouter, createWebHashHistory } from 'vue-router'
import AppShell from '../components/AppShell.vue'
import ChatView from '../views/ChatView.vue'
import DocumentsView from '../views/DocumentsView.vue'
import LoginView from '../views/LoginView.vue'
import { getToken } from '../api/client'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/login', component: LoginView },
    {
      path: '/',
      component: AppShell,
      children: [
        { path: '', redirect: '/chat' },
        { path: 'chat', component: ChatView },
        { path: 'documents', component: DocumentsView },
      ],
    },
  ],
})

router.beforeEach((to) => {
  const token = getToken()
  if (to.path !== '/login' && !token) {
    return '/login'
  }
  if (to.path === '/login' && token) {
    return '/chat'
  }
  return true
})

export default router
```

`fronted/src/main.ts`：

```ts
import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import router from './router'
import './styles.css'

createApp(App).use(router).use(ElementPlus).mount('#app')
```

`fronted/src/App.vue`：

```vue
<template>
  <router-view />
</template>
```

`fronted/src/components/AppShell.vue`：

```vue
<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { clearToken } from '../api/client'

const route = useRoute()
const router = useRouter()
const active = computed(() => route.path)

function logout() {
  clearToken()
  router.replace('/login')
}
</script>

<template>
  <el-container class="app-shell">
    <el-aside width="220px" class="sidebar">
      <div class="brand">
        <span class="brand-mark">K</span>
        <div>
          <strong>KB-RAG</strong>
          <small>企业知识库</small>
        </div>
      </div>
      <el-menu :default-active="active" router class="nav">
        <el-menu-item index="/chat">知识问答</el-menu-item>
        <el-menu-item index="/documents">知识库管理</el-menu-item>
      </el-menu>
      <div class="sidebar-footer">
        <el-button text @click="logout">退出登录</el-button>
      </div>
    </el-aside>
    <el-main class="main">
      <router-view />
    </el-main>
  </el-container>
</template>

<style scoped>
.app-shell {
  min-height: 100vh;
}

.sidebar {
  display: flex;
  flex-direction: column;
  background: #101828;
  color: #e4e7ec;
  padding: 18px 12px;
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px 22px;
}

.brand-mark {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  background: #2e90fa;
  color: #fff;
  font-weight: 700;
}

.brand strong {
  display: block;
  font-size: 16px;
  letter-spacing: 0.3px;
}

.brand small {
  display: block;
  margin-top: 2px;
  color: #98a2b3;
  font-size: 12px;
}

.nav {
  border-right: none;
  background: transparent;
  --el-menu-bg-color: transparent;
  --el-menu-text-color: #cbd5e1;
  --el-menu-hover-bg-color: #1d2939;
  --el-menu-active-color: #ffffff;
}

.nav :deep(.el-menu-item.is-active) {
  background: #2e90fa;
  border-radius: 8px;
}

.nav :deep(.el-menu-item:hover) {
  border-radius: 8px;
}

.sidebar-footer {
  margin-top: auto;
  padding: 12px 10px 0;
}

.sidebar-footer .el-button {
  color: #98a2b3;
}

.main {
  padding: 0;
  background: #f7f8fa;
}
</style>
```

`fronted/src/views/LoginView.vue`：

```vue
<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { setToken, clearToken, verifySession } from '../api/client'

const router = useRouter()
const token = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  const value = token.value.trim()
  if (!value) {
    error.value = '请输入访问令牌'
    return
  }
  loading.value = true
  error.value = ''
  setToken(value)
  try {
    await verifySession()
    await router.replace('/chat')
  } catch (e) {
    clearToken()
    error.value = e instanceof Error ? e.message : '令牌校验失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <el-card class="login-card" shadow="never">
      <div class="login-brand">
        <span class="brand-mark">K</span>
        <div>
          <h1>KB-RAG</h1>
          <p>企业知识库问答</p>
        </div>
      </div>
      <el-input
        v-model="token"
        type="password"
        show-password
        size="large"
        placeholder="共享访问令牌"
        @keyup.enter="submit"
      />
      <el-alert
        v-if="error"
        class="login-error"
        :title="error"
        type="error"
        :closable="false"
        show-icon
      />
      <el-button type="primary" size="large" class="login-button" :loading="loading" @click="submit">
        进入知识库
      </el-button>
      <p class="login-hint">令牌由部署者提供，只保存在当前标签页。</p>
    </el-card>
  </div>
</template>

<style scoped>
.login-page {
  display: grid;
  min-height: 100vh;
  place-items: center;
  background:
    radial-gradient(circle at 15% 20%, rgba(46, 144, 250, 0.22), transparent 32%),
    radial-gradient(circle at 85% 80%, rgba(16, 24, 40, 0.12), transparent 30%),
    #f5f7fb;
  padding: 24px;
}

.login-card {
  width: min(400px, 100%);
  border: 1px solid #e4e7ec;
  border-radius: 16px;
  padding: 10px;
}

.login-brand {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 24px;
}

.login-brand h1 {
  margin: 0;
  font-size: 24px;
  color: #101828;
}

.login-brand p {
  margin: 4px 0 0;
  color: #667085;
  font-size: 13px;
}

.brand-mark {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  border-radius: 12px;
  background: #2e90fa;
  color: #fff;
  font-size: 20px;
  font-weight: 700;
}

.login-error {
  margin-top: 14px;
}

.login-button {
  width: 100%;
  margin-top: 18px;
}

.login-hint {
  margin: 16px 0 4px;
  color: #98a2b3;
  font-size: 12px;
  text-align: center;
}
</style>
```

`fronted/src/views/ChatView.vue`（占位）：

```vue
<template>
  <div class="page">
    <h2>知识问答</h2>
    <el-empty description="问答页面将在下一步实现" />
  </div>
</template>
```

`fronted/src/views/DocumentsView.vue`（占位）：

```vue
<template>
  <div class="page">
    <h2>知识库管理</h2>
    <el-empty description="知识库页面将在下一步实现" />
  </div>
</template>
```

- [ ] **Step 5: 创建全局样式**

`fronted/src/styles.css`：

```css
:root {
  font-family: "Segoe UI", "PingFang SC", "Microsoft YaHei", system-ui, sans-serif;
  color: #101828;
  background: #f7f8fa;
}

* {
  box-sizing: border-box;
}

body {
  margin: 0;
  min-width: 320px;
  min-height: 100vh;
}

.page {
  padding: 28px 32px 40px;
}

.muted {
  color: #667085;
}

@media (max-width: 760px) {
  .page {
    padding: 18px;
  }
}
```

- [ ] **Step 6: 更新 `.gitignore`**

在根 `.gitignore` 末尾增加：

```gitignore
/fronted/node_modules/
/fronted/dist/
```

- [ ] **Step 7: 安装依赖并验证构建**

```bash
cd fronted
npm install
npm run type-check
npm run build
```

Expected:
- `npm install` 成功并生成 `fronted/package-lock.json`；
- `type-check` 无错误；
- `build` 生成 `fronted/dist/index.html`。

- [ ] **Step 8: 写前端 README**

创建 `fronted/README.md`：

````markdown
# fronted

KB-RAG 的 Vue 3 + TypeScript 前端。

## 开发

先启动 Go API（默认 `127.0.0.1:8080`），再运行：

```bash
npm install
npm run dev
```

Vite 会把 `/api` 代理到 `http://127.0.0.1:8080`。

## 构建

```bash
npm run type-check
npm run build
```

产物写入 `fronted/dist`，由 `KB-RAG serve` 从磁盘托管。`dist` 与 `node_modules` 不提交。

## 说明

- 令牌保存在 `sessionStorage`；
- 答案使用 markdown-it，关闭原始 HTML；
- 只读引用片段，不渲染片段内的 HTML。
````

- [ ] **Step 9: 提交**

```bash
git add .gitignore fronted
git commit -m "feat: 初始化 Vue 3 前端与登录外壳"
```

---

### Task 2: 问答页与引用片段

**Files:**
- Create: `fronted/src/markdown.ts`
- Create: `fronted/src/components/CitationList.vue`
- Modify: `fronted/src/views/ChatView.vue`（替换占位）
- Modify: `fronted/src/styles.css`（追加聊天与 Markdown 样式）

**Interfaces:**
- Consumes: `ask(question, topK)`、`AskResponse`、`Citation`。
- Produces: 可用的问答页与引用折叠列表。

- [ ] **Step 1: 创建 Markdown 渲染器**

`fronted/src/markdown.ts`：

```ts
import MarkdownIt from 'markdown-it'

const md = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
})

export function renderMarkdown(text: string): string {
  return md.render(text)
}
```

- [ ] **Step 2: 创建引用片段组件**

`fronted/src/components/CitationList.vue`：

```vue
<script setup lang="ts">
import type { Citation } from '../api/types'

defineProps<{ citations: Citation[] }>()
</script>

<template>
  <div class="citations">
    <div class="citations-label">引用来源</div>
    <el-collapse>
      <el-collapse-item v-for="c in citations" :key="c.n" :name="c.n">
        <template #title>
          <span class="citation-title">
            [{{ c.n }}] {{ c.source }}<span v-if="c.heading"> · {{ c.heading }}</span>
          </span>
          <span class="citation-score">{{ c.score.toFixed(3) }}</span>
        </template>
        <pre class="citation-text">{{ c.text }}</pre>
      </el-collapse-item>
    </el-collapse>
  </div>
</template>

<style scoped>
.citations {
  margin-top: 14px;
  padding-top: 10px;
  border-top: 1px dashed #d0d5dd;
}

.citations-label {
  margin-bottom: 6px;
  color: #667085;
  font-size: 12px;
}

.citation-title {
  color: #344054;
  font-size: 13px;
}

.citation-score {
  margin-left: auto;
  color: #667085;
  font-size: 12px;
}

.citation-text {
  margin: 0;
  padding: 10px;
  border-radius: 8px;
  background: #f9fafb;
  color: #475467;
  font-family: inherit;
  font-size: 12px;
  line-height: 1.65;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
```

- [ ] **Step 3: 替换问答页**

`fronted/src/views/ChatView.vue`：

```vue
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
```

- [ ] **Step 4: 追加聊天与 Markdown 全局样式**

在 `fronted/src/styles.css` 末尾追加：

```css
.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 20px;
}

.page-header h2 {
  margin: 0 0 6px;
  color: #101828;
  font-size: 22px;
}

.page-header p {
  margin: 0;
  font-size: 13px;
}

.markdown {
  font-size: 14px;
  line-height: 1.75;
}

.markdown > :first-child {
  margin-top: 0;
}

.markdown > :last-child {
  margin-bottom: 0;
}

.markdown p {
  margin: 8px 0;
}

.markdown h1,
.markdown h2,
.markdown h3,
.markdown h4 {
  margin: 16px 0 8px;
  color: #101828;
  line-height: 1.4;
}

.markdown ul,
.markdown ol {
  margin: 8px 0;
  padding-left: 22px;
}

.markdown li {
  margin: 3px 0;
}

.markdown code {
  padding: 1px 5px;
  border-radius: 5px;
  background: #f2f4f7;
  color: #344054;
  font-size: 0.92em;
}

.markdown pre {
  overflow-x: auto;
  padding: 12px;
  border-radius: 8px;
  background: #101828;
  color: #f2f4f7;
  font-size: 12px;
}

.markdown pre code {
  padding: 0;
  background: transparent;
  color: inherit;
}

.markdown table {
  width: 100%;
  margin: 10px 0;
  border-collapse: collapse;
  font-size: 13px;
}

.markdown th,
.markdown td {
  padding: 7px 9px;
  border: 1px solid #eaecf0;
  text-align: left;
  vertical-align: top;
}

.markdown th {
  background: #f9fafb;
}

.markdown blockquote {
  margin: 10px 0;
  padding: 6px 12px;
  border-left: 3px solid #d0d5dd;
  color: #667085;
}

.markdown a {
  color: #1570ef;
}

@media (max-width: 760px) {
  .page-header {
    flex-direction: column;
    align-items: stretch;
  }
}
```

- [ ] **Step 5: 验证类型与构建**

```bash
cd fronted
npm run type-check
npm run build
```

Expected: 两项均通过。

- [ ] **Step 6: 提交**

```bash
git add fronted
git commit -m "feat: 实现问答页与引用片段"
```

---

### Task 3: 知识库页、上传与文档表格

**Files:**
- Modify: `fronted/src/views/DocumentsView.vue`（替换占位）
- Create: `fronted/src/components/UploadPanel.vue`
- Create: `fronted/src/components/DocumentTable.vue`
- Modify: `fronted/src/styles.css`（追加统计/上传/表格样式）

**Interfaces:**
- Consumes: `getStats`、`listDocuments`、`uploadDocument`、`deleteDocument`、`reingestDocument`、`Stats`、`DocumentItem`。
- Produces: 知识库管理页。

- [ ] **Step 1: 替换知识库页**

`fronted/src/views/DocumentsView.vue`：

```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getStats, listDocuments } from '../api/client'
import type { DocumentItem, Stats } from '../api/types'
import DocumentTable from '../components/DocumentTable.vue'
import UploadPanel from '../components/UploadPanel.vue'

const stats = ref<Stats | null>(null)
const documents = ref<DocumentItem[]>([])
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [nextStats, nextDocuments] = await Promise.all([getStats(), listDocuments()])
    stats.value = nextStats
    documents.value = nextDocuments.documents
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载知识库失败'
  } finally {
    loading.value = false
  }
}

function formatSize(bytes: number): string {
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div>
        <h2>知识库管理</h2>
        <p class="muted">上传文档、查看入库状态；删除只移除索引，源文件保留。</p>
      </div>
      <el-button :loading="loading" @click="load">刷新</el-button>
    </div>

    <el-alert v-if="error" class="page-error" type="error" :title="error" :closable="false" show-icon />

    <div v-loading="loading" class="documents-body">
      <div v-if="stats" class="stat-grid">
        <el-card shadow="never" class="stat-card">
          <div class="stat-label">文档</div>
          <div class="stat-value">{{ stats.documents }}</div>
          <div class="stat-sub">失败 {{ stats.documents_failed }}</div>
        </el-card>
        <el-card shadow="never" class="stat-card">
          <div class="stat-label">片段</div>
          <div class="stat-value">{{ stats.chunks }}</div>
          <div class="stat-sub">可检索片段数</div>
        </el-card>
        <el-card shadow="never" class="stat-card">
          <div class="stat-label">嵌入模型</div>
          <div class="stat-value stat-value-small">{{ stats.embedding.model || '未初始化' }}</div>
          <div class="stat-sub">{{ stats.embedding.dim }} 维 · {{ stats.embedding.chunk_size }} 字符</div>
        </el-card>
        <el-card shadow="never" class="stat-card">
          <div class="stat-label">库大小</div>
          <div class="stat-value stat-value-small">{{ formatSize(stats.size_bytes) }}</div>
          <div class="stat-sub">SQLite 数据文件</div>
        </el-card>
      </div>

      <UploadPanel @uploaded="load" />
      <DocumentTable :documents="documents" @changed="load" />
    </div>
  </div>
</template>
```

- [ ] **Step 2: 创建上传面板**

`fronted/src/components/UploadPanel.vue`：

```vue
<script setup lang="ts">
import { ElMessage } from 'element-plus'
import type { UploadRawFile, UploadRequestOptions } from 'element-plus'
import { uploadDocument } from '../api/client'

const emit = defineEmits<{ uploaded: [] }>()

const allowed = ['.md', '.markdown', '.txt', '.docx', '.pdf']
const maxBytes = 20 * 1024 * 1024
let queue: Promise<void> = Promise.resolve()

function beforeUpload(file: UploadRawFile): boolean {
  const name = file.name.toLowerCase()
  if (!allowed.some((ext) => name.endsWith(ext))) {
    ElMessage.error(`不支持的文件类型：${file.name}`)
    return false
  }
  if (file.size > maxBytes) {
    ElMessage.error(`${file.name} 超过 20 MB`)
    return false
  }
  return true
}

function uploadFile(options: UploadRequestOptions) {
  const task = queue.then(async () => {
    const res = await uploadDocument(options.file)
    options.onSuccess(res)
    if (res.outcome === 'failed') {
      ElMessage.warning(`${options.file.name} 已保存，但入库失败，可在列表中重试`)
    } else {
      ElMessage.success(`${options.file.name} 已入库`)
    }
    emit('uploaded')
  })
  queue = task.then(
    () => undefined,
    () => undefined,
  )
  return task.catch((err: unknown) => {
    const e = err instanceof Error ? err : new Error('上传失败')
    options.onError(e)
    ElMessage.error(`${options.file.name}: ${e.message}`)
  })
}
</script>

<template>
  <el-card class="panel" shadow="never">
    <template #header>
      <div class="panel-header">
        <span>上传文档</span>
        <span class="muted">Markdown / TXT / DOCX / PDF，单文件 ≤ 20 MB</span>
      </div>
    </template>
    <el-upload
      drag
      multiple
      :http-request="uploadFile"
      :before-upload="beforeUpload"
      accept=".md,.markdown,.txt,.docx,.pdf"
      :show-file-list="true"
    >
      <div class="upload-icon">＋</div>
      <div class="el-upload__text">拖拽文件到这里，或<em>点击选择</em></div>
      <template #tip>
        <div class="el-upload__tip">上传后同步解析与嵌入；失败文件可在文档列表中重试。</div>
      </template>
    </el-upload>
  </el-card>
</template>
```

- [ ] **Step 3: 创建文档表格**

`fronted/src/components/DocumentTable.vue`：

```vue
<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { deleteDocument, reingestDocument } from '../api/client'
import type { DocumentItem } from '../api/types'

defineProps<{ documents: DocumentItem[] }>()
const emit = defineEmits<{ changed: [] }>()
const retrying = ref<number | null>(null)

function formatTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

async function retry(row: DocumentItem) {
  retrying.value = row.id
  try {
    const res = await reingestDocument(row.id)
    if (res.outcome === 'failed') {
      ElMessage.warning(`${row.title || row.name} 重试后仍失败`)
    } else {
      ElMessage.success(`${row.title || row.name} 已重新入库`)
    }
    emit('changed')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '重试失败')
  } finally {
    retrying.value = null
  }
}

async function remove(row: DocumentItem) {
  try {
    await ElMessageBox.confirm(
      `将删除「${row.title || row.name}」的知识库索引。只删除索引，源文件保留。`,
      '删除文档',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
    await deleteDocument(row.id)
    ElMessage.success('已删除索引')
    emit('changed')
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error(e instanceof Error ? e.message : '删除失败')
    }
  }
}
</script>

<template>
  <el-card class="panel" shadow="never">
    <template #header>
      <div class="panel-header">
        <span>文档列表</span>
        <span class="muted">共 {{ documents.length }} 个</span>
      </div>
    </template>
    <el-table :data="documents" stripe>
      <el-table-column label="文档" min-width="220">
        <template #default="{ row }">
          <div class="doc-title">{{ row.title || row.name }}</div>
          <div class="doc-name">{{ row.name }}</div>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.status === 'ok' ? 'success' : 'danger'" size="small">
            {{ row.status === 'ok' ? '正常' : '失败' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="chunks" label="片段" width="80" />
      <el-table-column label="来源" width="90">
        <template #default="{ row }">{{ row.source === 'upload' ? '上传' : 'CLI' }}</template>
      </el-table-column>
      <el-table-column label="更新时间" width="170">
        <template #default="{ row }">{{ formatTime(row.updated_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button
            v-if="row.status === 'failed'"
            size="small"
            :loading="retrying === row.id"
            @click="retry(row)"
          >
            重试
          </el-button>
          <el-button size="small" type="danger" plain @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
      <template #empty>
        <el-empty description="还没有文档，先上传一份" />
      </template>
    </el-table>
    <el-alert
      v-if="documents.some((d) => d.status === 'failed')"
      class="table-hint"
      type="warning"
      :closable="false"
      show-icon
      title="存在失败文档：可点击“重试”重新入库。"
    />
  </el-card>
</template>
```

- [ ] **Step 4: 追加知识库样式**

在 `fronted/src/styles.css` 末尾追加：

```css
.page-error {
  margin-bottom: 16px;
}

.documents-body {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.stat-card {
  border: 1px solid #e4e7ec;
  border-radius: 12px;
}

.stat-label {
  color: #667085;
  font-size: 12px;
}

.stat-value {
  margin-top: 8px;
  color: #101828;
  font-size: 26px;
  font-weight: 650;
  line-height: 1.2;
}

.stat-value-small {
  overflow: hidden;
  font-size: 18px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stat-sub {
  margin-top: 6px;
  color: #98a2b3;
  font-size: 12px;
}

.panel {
  border: 1px solid #e4e7ec;
  border-radius: 12px;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 14px;
  font-weight: 600;
}

.panel-header .muted {
  font-size: 12px;
  font-weight: 400;
}

.upload-icon {
  color: #2e90fa;
  font-size: 34px;
  line-height: 1;
}

.doc-title {
  color: #101828;
  font-weight: 500;
}

.doc-name {
  margin-top: 2px;
  color: #98a2b3;
  font-size: 12px;
}

.table-hint {
  margin-top: 12px;
}

@media (max-width: 980px) {
  .stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 560px) {
  .stat-grid {
    grid-template-columns: 1fr;
  }

  .panel-header {
    flex-direction: column;
    align-items: flex-start;
  }
}
```

- [ ] **Step 5: 验证类型与构建**

```bash
cd fronted
npm run type-check
npm run build
```

Expected: 两项均通过。

- [ ] **Step 6: 提交**

```bash
git add fronted
git commit -m "feat: 实现知识库上传与文档管理"
```

---

### Task 4: 集成文档、构建与手工冒烟

**Files:**
- Modify: `README.md`（增加 Web 使用说明）
- 无新增代码文件

**Interfaces:**
- Consumes: 后端 `serve` 与前端构建产物。
- Produces: 可部署的本地/局域网 Web 使用说明与最终验证。

- [ ] **Step 1: 更新根 README**

在“构建与使用”与“已知限制”之间增加：

````markdown
## Web 界面

Web 界面由 Go `serve` 命令托管，前端源码在 `fronted/`。

```powershell
# 构建前端
cd fronted
npm install
npm run build
cd ..

# 构建 Go
go build -o KB-RAG.exe ./cmd/KB-RAG

# 设置两个密钥（PowerShell 示例）
$env:KB_RAG_WEB_TOKEN = "至少16个字符的共享令牌"
$env:KB_RAG_LLM_API_KEY = "你的生成服务Key"

# 启动 Web 服务（默认 http://127.0.0.1:8080）
./KB-RAG.exe serve --config config.yaml
```

浏览器打开 `http://127.0.0.1:8080`，输入共享令牌登录。

局域网访问：把 `config.yaml` 的 `web.addr` 改为 `0.0.0.0:8080`（或指定网卡 IP），并从其他设备访问 `http://<主机IP>:8080`。令牌使用明文 HTTP 传输，请只在可信局域网使用；需要更强安全时放在 HTTPS 反向代理后。

前端开发模式：

```bash
cd fronted
npm run dev
```

Vite 会把 `/api` 代理到 `127.0.0.1:8080`，需要同时运行 `KB-RAG serve`。

删除文档只删除知识库索引，任何源文件都会保留；上传目录不会自动清理。

详见 [Web 设计文档](docs/superpowers/specs/2026-10-26-KB-RAG-web-design.md)。
````

- [ ] **Step 2: 全量验证**

```bash
cd fronted
npm run type-check
npm run build
cd ..

go test ./...
go vet ./...
go build -o KB-RAG.exe ./cmd/KB-RAG
```

Expected:
- 前端类型检查与构建通过，生成 `fronted/dist/index.html`；
- Go 测试全部 PASS；
- Go 构建成功。

- [ ] **Step 3: 手工冒烟**

在真实 Ollama 与云端生成服务可用、且已设置 `KB_RAG_WEB_TOKEN` 和 `KB_RAG_LLM_API_KEY` 的 PowerShell 中：

```powershell
.\KB-RAG.exe serve --config config.yaml
```

浏览器验证：

1. 未登录访问 `http://127.0.0.1:8080/#/chat` 会被重定向到登录页；
2. 输入错误令牌显示错误，输入正确令牌进入问答页；
3. 打开“知识库管理”，上传 `test-ingest/ARCHITECTURE.md`，状态为“正常”，片段数大于 0；
4. 上传一个不支持的 `.png`，前端提示不支持；
5. 回到问答页，提问“系统的统一入口是什么？”，显示答案与引用卡片；
6. 删除该文档，确认弹窗包含“源文件保留”，列表移除文档，`test-ingest/ARCHITECTURE.md` 仍在；
7. 用 `stats` 或页面统计确认片段数下降；
8. 局域网设备访问：未带令牌 401，登录后可正常使用；
9. 运行 `go test ./...`，确认 CLI 无回归。

- [ ] **Step 4: 提交**

```bash
git add README.md
git commit -m "docs: 增加 Web 界面使用说明"
```

---

## 自检

**Spec coverage**

| 设计文档要求 | 覆盖任务 |
|--------------|----------|
| 登录与共享令牌 | Task 1 |
| 问答、回答 Markdown、引用片段 | Task 2 |
| 统计、上传、文档列表、重试、删除 | Task 3 |
| 删除只删索引 | Task 3（调用后端；确认文案） |
| 前端构建与 Go 托管 | Task 1、Task 4 |
| Vite 开发代理 | Task 1、Task 4 |
| 安全渲染（html:false、片段文本） | Task 2 |
| 局域网说明与构建文档 | Task 4 |
| 手工冒烟 | Task 4 |

**Placeholder scan:** 无 TBD/TODO；两处占位页面在 Task 2/3 被完整文件替换。

**Type consistency:** `Stats`、`DocumentItem`、`Citation`、`AskResponse`、`UploadResponse` 在 `api/types.ts` 定义，Task 2/3 按相同字段使用；API 路径与后端计划一致：`/session`、`/stats`、`/documents`、`/documents/{id}`、`/documents/{id}/reingest`、`/ask`。

