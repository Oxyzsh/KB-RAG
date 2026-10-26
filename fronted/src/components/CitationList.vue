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
