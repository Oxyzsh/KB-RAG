<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { deleteDocument, getJob, reingestDocumentJob } from '../api/client'
import type { DocumentItem, Job } from '../api/types'

const props = defineProps<{ documents: DocumentItem[]; canEdit: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const retrying = ref<number | null>(null)
const retryStage = ref('')

const searchQuery = ref('')
const statusFilter = ref('all')

const filteredDocuments = computed(() => {
  const keyword = searchQuery.value.trim().toLowerCase()
  return props.documents.filter((doc) => {
    const matchesSearch =
      !keyword ||
      doc.name.toLowerCase().includes(keyword) ||
      doc.title.toLowerCase().includes(keyword)
    const matchesStatus = statusFilter.value === 'all' || doc.status === statusFilter.value
    return matchesSearch && matchesStatus
  })
})

function formatTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

async function waitJob(id: number): Promise<Job> {
  for (let i = 0; i < 600; i += 1) {
    const { job } = await getJob(id)
    retryStage.value = job.stage
    if (job.status === 'done' || job.status === 'failed' || job.status === 'canceled') {
      return job
    }
    await new Promise((resolve) => setTimeout(resolve, 500))
  }
  throw new Error('任务处理超时')
}

async function retry(row: DocumentItem) {
  retrying.value = row.id
  retryStage.value = '排队中'
  try {
    const { job } = await reingestDocumentJob(row.id)
    const done = await waitJob(job.id)
    if (done.status === 'done') {
      ElMessage.success(`${row.title || row.name} 已重新入库`)
    } else if (done.status === 'canceled') {
      ElMessage.info(`${row.title || row.name} 已取消`)
    } else {
      ElMessage.warning(`${row.title || row.name} 重试失败：${done.error || '未知错误'}`)
    }
    emit('changed')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '重试失败')
  } finally {
    retrying.value = null
    retryStage.value = ''
  }
}

async function remove(row: DocumentItem) {
  try {
    await ElMessageBox.confirm(
      `确定删除「${row.title || row.name}」的索引？只删除索引，源文件保留。`,
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
        <span>文档列表<span class="count">共 {{ filteredDocuments.length }} 个</span></span>
        <div class="filters">
          <el-input
            v-model="searchQuery"
            size="small"
            clearable
            placeholder="搜索名称或标题"
            class="search-input"
          />
          <el-select v-model="statusFilter" size="small" class="status-select">
            <el-option label="全部状态" value="all" />
            <el-option label="正常" value="ok" />
            <el-option label="失败" value="failed" />
          </el-select>
        </div>
      </div>
    </template>

    <el-table :data="filteredDocuments" stripe>
      <el-table-column label="文档" min-width="240">
        <template #default="{ row }">
          <div class="doc-title">{{ row.title || row.name }}</div>
          <div class="doc-name">{{ row.name }}</div>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.status === 'ok' ? 'success' : 'danger'" size="small" effect="plain">
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
      <el-table-column v-if="canEdit" label="操作" width="140" fixed="right">
        <template #default="{ row }">
          <el-button
            v-if="row.status === 'failed'"
            size="small"
            link
            type="primary"
            :loading="retrying === row.id"
            @click="retry(row as DocumentItem)"
          >
            {{ retrying === row.id ? retryStage || '处理中' : '重试' }}
          </el-button>
          <el-button size="small" link type="danger" @click="remove(row as DocumentItem)">
            删除
          </el-button>
        </template>
      </el-table-column>
      <template #empty>
        <div class="table-empty">{{ searchQuery || statusFilter !== 'all' ? '无匹配文档' : '暂无文档' }}</div>
      </template>
    </el-table>

    <el-alert
      v-if="documents.some((d) => d.status === 'failed')"
      class="table-hint"
      type="warning"
      :closable="false"
      show-icon
      title="存在失败文档，可点击“重试”重新入库。"
    />
  </el-card>
</template>

<style scoped>
.count {
  margin-left: 8px;
  color: #9ca3af;
  font-size: 12px;
  font-weight: 400;
}

.filters {
  display: flex;
  align-items: center;
  gap: 8px;
}

.search-input {
  width: 180px;
}

.status-select {
  width: 104px;
}

.doc-title {
  color: #111827;
  font-weight: 500;
}

.doc-name {
  margin-top: 2px;
  color: #9ca3af;
  font-size: 12px;
}

.table-empty {
  padding: 24px 0;
  color: #9ca3af;
  font-size: 13px;
}

.table-hint {
  margin-top: 12px;
}
</style>
