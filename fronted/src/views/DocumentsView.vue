<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { getStats, listDocuments } from '../api/client'
import type { DocumentItem, Stats } from '../api/types'
import DocumentTable from '../components/DocumentTable.vue'
import UploadPanel from '../components/UploadPanel.vue'
import { canEditDocuments, session } from '../session'

const stats = ref<Stats | null>(null)
const documents = ref<DocumentItem[]>([])
const loading = ref(false)
const error = ref('')
const canEdit = computed(() => canEditDocuments())

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
watch(
  () => session.user?.tenant_slug,
  () => {
    void load()
  },
)
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
          <div class="stat-label">本租户数据量</div>
          <div class="stat-value stat-value-small">{{ formatSize(stats.tenant_size_bytes) }}</div>
          <div class="stat-sub">全库文件 {{ formatSize(stats.size_bytes) }}</div>
        </el-card>
      </div>

      <UploadPanel v-if="canEdit" @uploaded="load" />
      <DocumentTable :documents="documents" :can-edit="canEdit" @changed="load" />
    </div>
  </div>
</template>
