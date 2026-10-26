<script setup lang="ts">
import type { Citation } from '../api/types'

defineProps<{ citations: Citation[] }>()
</script>

<template>
  <div class="citations-container">
    <div class="citations-header">
      <span class="header-icon">📑</span>
      <span class="header-title">引用来源 ({{ citations.length }})</span>
    </div>
    <div class="citation-cards">
      <details v-for="c in citations" :key="c.n" class="citation-card">
        <summary class="citation-summary">
          <div class="summary-left">
            <span class="citation-badge">[{{ c.n }}]</span>
            <span class="source-name" :title="c.source">{{ c.source }}</span>
            <span v-if="c.heading" class="source-heading" :title="c.heading">· {{ c.heading }}</span>
          </div>
          <div class="summary-right">
            <span class="similarity-score" :title="'相似度匹配分: ' + c.score.toFixed(4)">
              匹配度: {{ (c.score * 100).toFixed(0) }}%
            </span>
            <span class="toggle-icon">▾</span>
          </div>
        </summary>
        <div class="citation-body">
          <div class="text-preview">{{ c.text }}</div>
        </div>
      </details>
    </div>
  </div>
</template>

<style scoped>
.citations-container {
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid #f1f5f9;
}

.citations-header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 600;
  color: #475569;
}

.citation-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.citation-card {
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #f8fafc;
  overflow: hidden;
  transition: all 0.15s ease;
}

.citation-card[open] {
  background: #ffffff;
  border-color: #cbd5e1;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.03);
}

.citation-summary {
  list-style: none;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  cursor: pointer;
  user-select: none;
  font-size: 13px;
}

.citation-summary::-webkit-details-marker {
  display: none;
}

.summary-left {
  display: flex;
  align-items: center;
  gap: 8px;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.citation-badge {
  color: #2563eb;
  font-weight: 700;
  font-size: 12px;
}

.source-name {
  font-weight: 500;
  color: #1e293b;
  overflow: hidden;
  text-overflow: ellipsis;
}

.source-heading {
  color: #64748b;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.summary-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.similarity-score {
  font-size: 11px;
  color: #059669;
  background: #ecfdf5;
  padding: 2px 6px;
  border-radius: 4px;
  border: 1px solid #a7f3d0;
}

.toggle-icon {
  font-size: 12px;
  color: #94a3b8;
  transition: transform 0.2s ease;
}

.citation-card[open] .toggle-icon {
  transform: rotate(180deg);
}

.citation-body {
  padding: 10px 14px 12px;
  border-top: 1px dashed #e2e8f0;
  background: #ffffff;
}

.text-preview {
  font-size: 12.5px;
  line-height: 1.6;
  color: #334155;
  white-space: pre-wrap;
  word-break: break-word;
  background: #f8fafc;
  padding: 10px 12px;
  border-radius: 6px;
  border: 1px solid #f1f5f9;
}
</style>
