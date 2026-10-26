<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { fetchDocumentRaw, getDocumentPreview } from '../api/client'
import type { Citation, DocumentPreview } from '../api/types'
import { renderMarkdown } from '../markdown'

defineProps<{ citations: Citation[] }>()

const visible = ref(false)
const active = ref<Citation | null>(null)
const preview = ref<DocumentPreview | null>(null)
const loading = ref(false)
const pdfLoading = ref(false)
const error = ref('')
const blobUrl = ref('')

const kindLabel = computed(() => {
  switch (preview.value?.kind) {
    case 'markdown':
      return 'Markdown'
    case 'text':
      return '纯文本'
    case 'pdf':
      return 'PDF'
    case 'docx':
      return 'Word'
    default:
      return '文档'
  }
})

const drawerTitle = computed(() => {
  if (!active.value) {
    return '引用来源'
  }
  return `引用 [${active.value.n}] · ${active.value.source}`
})

function revokeBlob() {
  if (blobUrl.value) {
    URL.revokeObjectURL(blobUrl.value)
    blobUrl.value = ''
  }
}

async function open(citation: Citation) {
  active.value = citation
  preview.value = null
  error.value = ''
  revokeBlob()
  visible.value = true
  loading.value = true
  try {
    const data = await getDocumentPreview(citation.document_id)
    preview.value = data
    if (data.kind === 'pdf' && data.raw_available) {
      await loadPDF(data.id)
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载来源失败'
  } finally {
    loading.value = false
  }
}

async function loadPDF(id: number) {
  pdfLoading.value = true
  try {
    const blob = await fetchDocumentRaw(id)
    blobUrl.value = URL.createObjectURL(blob)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载 PDF 失败'
  } finally {
    pdfLoading.value = false
  }
}

function openNewTab() {
  if (blobUrl.value) {
    window.open(blobUrl.value, '_blank', 'noopener')
  }
}

async function downloadOriginal() {
  if (!preview.value) {
    return
  }
  try {
    const blob = await fetchDocumentRaw(preview.value.id)
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = preview.value.name
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '下载失败'
  }
}

function onClosed() {
  visible.value = false
  active.value = null
  preview.value = null
  error.value = ''
  revokeBlob()
}

onBeforeUnmount(revokeBlob)
</script>

<template>
  <div class="citations">
    <div class="citations-label">引用来源</div>
    <details v-for="c in citations" :key="c.n" class="citation">
      <summary class="citation-summary">
        <span class="citation-source">
          [{{ c.n }}] {{ c.source }}<span v-if="c.heading"> · {{ c.heading }}</span>
        </span>
        <span class="citation-actions">
          <button class="view-btn" @click.stop.prevent="open(c)">查看来源</button>
          <span class="citation-score">相似度 {{ c.score.toFixed(3) }}</span>
        </span>
      </summary>
      <pre class="citation-text">{{ c.text }}</pre>
    </details>

    <el-drawer
      v-model="visible"
      :title="drawerTitle"
      size="60%"
      class="citation-drawer"
      @closed="onClosed"
    >
      <div v-loading="loading" class="preview-body">
        <el-alert v-if="error" class="preview-error" type="error" :title="error" :closable="false" show-icon />
        <template v-if="preview">
          <div class="preview-meta">
            <span class="preview-title">{{ preview.title || preview.name }}</span>
            <span class="preview-tag">{{ kindLabel }}</span>
            <span v-if="preview.source === 'external'" class="preview-tag">CLI 导入</span>
          </div>

          <div v-if="active" class="cited-snippet">
            <div class="cited-label">
              当前引用<span v-if="active.heading"> · {{ active.heading }}</span>
            </div>
            <pre>{{ active.text }}</pre>
          </div>

          <div v-if="preview.kind === 'pdf' && preview.raw_available && blobUrl" class="pdf-box">
            <iframe :src="blobUrl" class="pdf-frame" title="PDF 预览"></iframe>
          </div>
          <div v-else-if="preview.kind === 'pdf' && preview.raw_available && pdfLoading" class="muted">
            正在加载 PDF…
          </div>
          <div v-else-if="preview.text" class="preview-content">
            <div
              v-if="preview.text_format === 'markdown'"
              class="markdown"
              v-html="renderMarkdown(preview.text)"
            />
            <pre v-else class="plain-text">{{ preview.text }}</pre>
          </div>
          <div v-else class="muted">暂无可预览的文本内容。</div>

          <div class="preview-actions">
            <el-button
              v-if="preview.kind === 'pdf' && preview.raw_available && blobUrl"
              size="small"
              @click="openNewTab"
            >
              新窗口打开
            </el-button>
            <el-button
              v-if="preview.raw_available && preview.kind !== 'pdf'"
              size="small"
              @click="downloadOriginal"
            >
              下载原文件
            </el-button>
          </div>
        </template>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.citations {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid #f0f1f3;
}

.citations-label {
  margin-bottom: 6px;
  color: #6b7280;
  font-size: 12px;
}

.citation {
  border-bottom: 1px solid #f0f1f3;
}

.citation:last-child {
  border-bottom: none;
}

.citation-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 2px;
  cursor: pointer;
  list-style: none;
  font-size: 13px;
}

.citation-summary::-webkit-details-marker {
  display: none;
}

.citation-source {
  color: #374151;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.citation-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.view-btn {
  padding: 0;
  border: none;
  background: transparent;
  color: #2563eb;
  font-size: 12px;
  cursor: pointer;
}

.view-btn:hover {
  text-decoration: underline;
}

.citation-score {
  color: #9ca3af;
  font-size: 12px;
}

.citation-text {
  margin: 0 0 10px;
  padding: 10px 12px;
  border: 1px solid #f0f1f3;
  border-radius: 6px;
  background: #f9fafb;
  color: #4b5563;
  font-family: inherit;
  font-size: 12px;
  line-height: 1.65;
  white-space: pre-wrap;
  word-break: break-word;
}

.preview-body {
  min-height: 200px;
}

.preview-error {
  margin-bottom: 12px;
}

.preview-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 14px;
}

.preview-title {
  color: #111827;
  font-size: 15px;
  font-weight: 600;
}

.preview-tag {
  padding: 1px 6px;
  border: 1px solid #e5e7eb;
  border-radius: 4px;
  color: #6b7280;
  font-size: 12px;
}

.cited-snippet {
  margin-bottom: 16px;
  padding: 10px 12px;
  border: 1px solid #e5e7eb;
  border-left: 3px solid #2563eb;
  border-radius: 6px;
  background: #f9fafb;
}

.cited-label {
  margin-bottom: 6px;
  color: #6b7280;
  font-size: 12px;
}

.cited-snippet pre {
  margin: 0;
  color: #374151;
  font-family: inherit;
  font-size: 12.5px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
}

.pdf-frame {
  width: 100%;
  height: calc(100vh - 260px);
  border: 1px solid #e5e7eb;
  border-radius: 6px;
}

.plain-text {
  margin: 0;
  color: #374151;
  font-family: inherit;
  font-size: 13px;
  line-height: 1.75;
  white-space: pre-wrap;
  word-break: break-word;
}

.preview-actions {
  margin-top: 16px;
  display: flex;
  gap: 8px;
}
</style>
