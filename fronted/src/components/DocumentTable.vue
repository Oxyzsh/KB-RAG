<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { deleteDocument, reingestDocument } from '../api/client'
import type { DocumentItem } from '../api/types'

const props = defineProps<{ documents: DocumentItem[]; canEdit: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const retrying = ref<number | null>(null)

const searchQuery = ref('')
const statusFilter = ref<string>('all')

const filteredDocuments = computed(() => {
  return props.documents.filter((doc) => {
    const matchesSearch =
      doc.name.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
      doc.title.toLowerCase().includes(searchQuery.value.toLowerCase())
    const matchesStatus =
      statusFilter.value === 'all' || doc.status === statusFilter.value
    return matchesSearch && matchesStatus
  })
})

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
      `确定删除「${row.title || row.name}」的索引？只删除索引，服务器上的源文件将保留。`,
      '删除确认',
      {
        type: 'warning',
        confirmButtonText: '删除索引',
        cancelButtonText: '取消',
        confirmButtonClass: 'el-button--danger',
      },
    )
    await deleteDocument(row.id)
    ElMessage.success('已安全删除知识库索引')
    emit('changed')
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error(e instanceof Error ? e.message : '删除失败')
    }
  }
}
</script>

<template>
  <el-card class="panel table-card" shadow="never">
    <template #header>
      <div class="panel-header">
        <div class="header-left">
          <span class="title">已索引文档</span>
          <span class="counter-badge">{{ filteredDocuments.length }} / {{ documents.length }}</span>
        </div>
        <div class="header-actions">
          <el-input
            v-model="searchQuery"
            placeholder="搜索文档名称/标题…"
            size="small"
            clearable
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

    <el-table :data="filteredDocuments" stripe style="width: 100%" class="custom-table">
      <el-table-column label="文档信息" min-width="260">
        <template #default="{ row }">
          <div class="doc-info-cell">
            <span class="doc-icon">📄</span>
            <div class="doc-text">
              <div class="doc-title" :title="row.title || row.name">
                {{ row.title || row.name }}
              </div>
              <div class="doc-filename" :title="row.name">{{ row.name }}</div>
            </div>
          </div>
        </template>
      </el-table-column>

      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag
            :type="row.status === 'ok' ? 'success' : 'danger'"
            size="small"
            class="status-tag"
            effect="light"
          >
            {{ row.status === 'ok' ? '已入库' : '失败' }}
          </el-tag>
        </template>
      </el-table-column>

      <el-table-column label="切分片段" width="100" align="center">
        <template #default="{ row }">
          <span class="chunks-badge">{{ row.chunks }}</span>
        </template>
      </el-table-column>

      <el-table-column label="来源" width="100">
        <template #default="{ row }">
          <span class="source-tag" :class="row.source">
            {{ row.source === 'upload' ? 'Web 上传' : 'CLI 扫描' }}
          </span>
        </template>
      </el-table-column>

      <el-table-column label="更新时间" width="170">
        <template #default="{ row }">
          <span class="time-text">{{ formatTime(row.updated_at) }}</span>
        </template>
      </el-table-column>

      <el-table-column v-if="canEdit" label="操作" width="140" fixed="right">
        <template #default="{ row }">
          <div class="op-cell">
            <el-button
              v-if="row.status === 'failed'"
              size="small"
              type="primary"
              link
              :loading="retrying === row.id"
              @click="retry(row as DocumentItem)"
            >
              重试
            </el-button>
            <el-button
              size="small"
              type="danger"
              link
              @click="remove(row as DocumentItem)"
            >
              删除索引
            </el-button>
          </div>
        </template>
      </el-table-column>

      <template #empty>
        <div class="table-empty">
          <div class="empty-icon">🗂️</div>
          <p v-if="searchQuery || statusFilter !== 'all'">无符合条件的筛选文档</p>
          <p v-else>暂无入库文档，请在上方上传或通过命令行添加</p>
        </div>
      </template>
    </el-table>

    <div v-if="documents.some((d) => d.status === 'failed')" class="table-alert-footer">
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        title="存在解析或嵌入失败的文档，可点击「重试」重新触发处理。"
      />
    </div>
  </el-card>
</template>

<style scoped>
.table-card {
  overflow: hidden;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.title {
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
}

.counter-badge {
  font-size: 12px;
  color: #64748b;
  background: #f1f5f9;
  padding: 2px 8px;
  border-radius: 12px;
  font-weight: 500;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.search-input {
  width: 200px;
}

.status-select {
  width: 100px;
}

.doc-info-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}

.doc-icon {
  font-size: 20px;
  flex-shrink: 0;
}

.doc-text {
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.doc-title {
  color: #0f172a;
  font-weight: 600;
  font-size: 13.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.doc-filename {
  color: #64748b;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.status-tag {
  font-weight: 500;
  border-radius: 4px;
}

.chunks-badge {
  display: inline-block;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 2px 8px;
  font-size: 12px;
  font-weight: 600;
  color: #334155;
}

.source-tag {
  font-size: 12px;
  color: #475569;
}

.source-tag.upload {
  color: #2563eb;
}

.time-text {
  font-size: 12.5px;
  color: #64748b;
}

.op-cell {
  display: flex;
  align-items: center;
  gap: 4px;
}

.table-empty {
  padding: 32px 0;
  text-align: center;
}

.empty-icon {
  font-size: 32px;
  margin-bottom: 8px;
}

.table-empty p {
  color: #64748b;
  font-size: 13.5px;
  margin: 0;
}

.table-alert-footer {
  padding: 12px 16px;
  background: #fffbeb;
  border-top: 1px solid #fef3c7;
}

.table-alert-footer :deep(.el-alert) {
  padding: 0;
  background: transparent;
}
</style>
