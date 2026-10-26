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
            @click="retry(row as DocumentItem)"
          >
            重试
          </el-button>
          <el-button size="small" type="danger" plain @click="remove(row as DocumentItem)">删除</el-button>
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
