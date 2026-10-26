<script setup lang="ts">
import type { Citation } from '../api/types'

defineProps<{ citations: Citation[] }>()
</script>

<template>
  <div class="citations">
    <div class="citations-label">引用来源</div>
    <details v-for="c in citations" :key="c.n" class="citation">
      <summary class="citation-summary">
        <span class="citation-source">
          [{{ c.n }}] {{ c.source }}<span v-if="c.heading"> · {{ c.heading }}</span>
        </span>
        <span class="citation-score">相似度 {{ c.score.toFixed(3) }}</span>
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

.citation-score {
  flex-shrink: 0;
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
