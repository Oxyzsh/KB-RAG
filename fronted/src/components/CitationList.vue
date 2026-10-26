<script setup lang="ts">
import { ElMessage } from 'element-plus/es/components/message/index'
import { fetchDocumentRaw, getDocumentPreview } from '../api/client'
import type { Citation, DocumentPreview } from '../api/types'
import { renderMarkdown } from '../markdown'

defineProps<{ citations: Citation[] }>()

function escapeHTML(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function kindLabel(kind: DocumentPreview['kind']): string {
  switch (kind) {
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
}

function pageShell(title: string, body: string): string {
  return `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${escapeHTML(title)}</title>
<style>
  :root { color-scheme: light; }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    padding: 24px;
    background: #f7f8fa;
    color: #1f2937;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
    line-height: 1.7;
  }
  .wrap { max-width: 960px; margin: 0 auto; }
  .header { margin-bottom: 16px; }
  h1 { margin: 0 0 6px; font-size: 18px; font-weight: 600; }
  .meta { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; color: #6b7280; font-size: 12px; }
  .tag { padding: 1px 6px; border: 1px solid #e5e7eb; border-radius: 4px; background: #fff; }
  .snippet {
    margin-bottom: 16px; padding: 10px 12px; background: #fff;
    border: 1px solid #e5e7eb; border-left: 3px solid #2563eb; border-radius: 6px;
  }
  .label { margin-bottom: 6px; color: #6b7280; font-size: 12px; }
  .snippet pre { margin: 0; font-family: inherit; font-size: 12.5px; white-space: pre-wrap; word-break: break-word; }
  .content { padding: 20px; background: #fff; border: 1px solid #e5e7eb; border-radius: 6px; }
  .plain { margin: 0; font-family: inherit; font-size: 13px; white-space: pre-wrap; word-break: break-word; }
  .muted { color: #6b7280; font-size: 13px; }
  .actions { margin-top: 16px; display: flex; gap: 12px; font-size: 13px; }
  .actions a { color: #2563eb; text-decoration: none; }
  .actions a:hover { text-decoration: underline; }
  iframe.pdf { width: 100%; height: calc(100vh - 240px); border: 1px solid #e5e7eb; border-radius: 6px; background: #fff; }
  .md > :first-child { margin-top: 0; }
  .md > :last-child { margin-bottom: 0; }
  .md p { margin: 8px 0; }
  .md h1, .md h2, .md h3, .md h4 { margin: 16px 0 8px; line-height: 1.4; }
  .md ul, .md ol { margin: 8px 0; padding-left: 22px; }
  .md code { padding: 1px 5px; border-radius: 4px; background: #f3f4f6; font-size: 0.9em; }
  .md pre { overflow-x: auto; padding: 12px; border-radius: 6px; background: #1f2937; color: #f3f4f6; font-size: 12.5px; }
  .md pre code { padding: 0; background: transparent; color: inherit; }
  .md table { width: 100%; border-collapse: collapse; font-size: 13px; margin: 12px 0; }
  .md th, .md td { padding: 7px 10px; border: 1px solid #e5e7eb; text-align: left; vertical-align: top; }
  .md th { background: #f9fafb; }
  .md blockquote { margin: 10px 0; padding: 6px 12px; border-left: 3px solid #d1d5db; color: #6b7280; }
  .md a { color: #2563eb; }
  .error { padding: 16px; background: #fef2f2; border: 1px solid #fecaca; border-radius: 6px; color: #b91c1c; }
</style>
</head>
<body>
<div class="wrap">${body}</div>
</body>
</html>`
}

function buildPage(citation: Citation, preview: DocumentPreview, rawUrl: string): string {
  const title = preview.title || preview.name
  const heading = citation.heading ? ` · ${escapeHTML(citation.heading)}` : ''
  const snippet = `<section class="snippet">
  <div class="label">当前引用 [${citation.n}]${heading}</div>
  <pre>${escapeHTML(citation.text)}</pre>
</section>`

  let content = ''
  if (preview.kind === 'pdf' && rawUrl) {
    content = `<iframe class="pdf" src="${rawUrl}" title="PDF 预览"></iframe>`
  } else if (preview.text) {
    content =
      preview.text_format === 'markdown'
        ? `<article class="content md">${renderMarkdown(preview.text)}</article>`
        : `<div class="content"><pre class="plain">${escapeHTML(preview.text)}</pre></div>`
  } else {
    content = `<div class="content"><p class="muted">暂无可预览的文本内容。</p></div>`
  }

  let actions = ''
  if (rawUrl) {
    actions =
      preview.kind === 'pdf'
        ? `<div class="actions"><a href="${rawUrl}" target="_blank" rel="noopener">在新窗口打开 PDF</a></div>`
        : `<div class="actions"><a href="${rawUrl}" download="${escapeHTML(preview.name)}">下载原文件</a></div>`
  }

  return pageShell(`${title} - 引用来源`, `<header class="header">
  <h1>${escapeHTML(title)}</h1>
  <div class="meta">
    <span>${escapeHTML(preview.name)}</span>
    <span class="tag">${kindLabel(preview.kind)}</span>
    <span class="tag">${preview.source === 'upload' ? '上传文件' : 'CLI 导入'}</span>
  </div>
</header>
${snippet}
${content}
${actions}`)
}

function writePage(win: Window, html: string) {
  win.document.open()
  win.document.write(html)
  win.document.close()
}

async function open(citation: Citation) {
  const win = window.open('', '_blank')
  if (!win) {
    ElMessage.warning('浏览器拦截了弹出窗口，请允许后重试')
    return
  }
  writePage(win, pageShell('正在加载…', '<p class="muted">正在加载引用来源…</p>'))
  try {
    const preview = await getDocumentPreview(citation.document_id)
    let rawUrl = ''
    if (preview.raw_available) {
      try {
        const blob = await fetchDocumentRaw(preview.id)
        rawUrl = URL.createObjectURL(blob)
      } catch {
        // 原文件不可用时回退到文本预览。
      }
    }
    writePage(win, buildPage(citation, preview, rawUrl))
  } catch (e) {
    const message = e instanceof Error ? e.message : '加载来源失败'
    writePage(win, pageShell('加载失败', `<div class="error">${escapeHTML(message)}</div>`))
  }
}
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
</style>
