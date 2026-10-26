<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { UploadRawFile, UploadRequestOptions } from 'element-plus'
import { cancelJob, getJob, uploadDocumentJob } from '../api/client'
import type { JobStatus } from '../api/types'

const emit = defineEmits<{ uploaded: [] }>()

interface UploadItem {
  key: string
  name: string
  jobId: number | null
  status: JobStatus | 'uploading'
  stage: string
  error: string
  notified: boolean
}

const allowed = ['.md', '.markdown', '.txt', '.docx', '.pdf']
const maxBytes = 20 * 1024 * 1024
const uploads = ref<UploadItem[]>([])
let pollTimer: number | undefined
let polling = false

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

function statusText(item: UploadItem): string {
  switch (item.status) {
    case 'uploading':
      return '上传中'
    case 'queued':
      return '排队中'
    case 'running':
      return item.stage || '处理中'
    case 'done':
      return '完成'
    case 'failed':
      return '失败'
    case 'canceled':
      return '已取消'
    default:
      return ''
  }
}

function canCancel(item: UploadItem): boolean {
  return item.status === 'queued' || item.status === 'running'
}

function startPolling() {
  if (pollTimer !== undefined) {
    return
  }
  pollTimer = window.setInterval(() => {
    void pollJobs()
  }, 800)
}

function stopPolling() {
  if (pollTimer !== undefined) {
    window.clearInterval(pollTimer)
    pollTimer = undefined
  }
}

async function pollJobs() {
  if (polling) {
    return
  }
  const active = uploads.value.filter(
    (item) => item.jobId !== null && (item.status === 'queued' || item.status === 'running'),
  )
  if (!active.length) {
    stopPolling()
    return
  }
  polling = true
  try {
    await Promise.all(
      active.map(async (item) => {
        try {
          const { job } = await getJob(item.jobId as number)
          item.status = job.status
          item.stage = job.stage
          item.error = job.error
          if (job.status === 'done' && !item.notified) {
            item.notified = true
            ElMessage.success(`${item.name} 已入库（${job.chunks} 个片段）`)
            emit('uploaded')
          } else if (job.status === 'failed' && !item.notified) {
            item.notified = true
            ElMessage.warning(`${item.name} 入库失败：${job.error}`)
            emit('uploaded')
          } else if (job.status === 'canceled' && !item.notified) {
            item.notified = true
            ElMessage.info(`${item.name} 已取消`)
          }
        } catch (e) {
          item.status = 'failed'
          item.error = e instanceof Error ? e.message : '读取任务状态失败'
          item.notified = true
        }
      }),
    )
  } finally {
    polling = false
  }
}

function uploadFile(options: UploadRequestOptions) {
  const item: UploadItem = {
    key: Math.random().toString(36).substring(2, 9),
    name: options.file.name,
    jobId: null,
    status: 'uploading',
    stage: '上传文件',
    error: '',
    notified: false,
  }
  uploads.value.push(item)

  return uploadDocumentJob(options.file)
    .then(({ job }) => {
      item.jobId = job.id
      item.status = job.status
      item.stage = job.stage
      options.onSuccess(job)
      startPolling()
    })
    .catch((err: unknown) => {
      const e = err instanceof Error ? err : new Error('上传失败')
      item.status = 'failed'
      item.error = e.message
      item.notified = true
      options.onError(Object.assign(e, { status: 0, method: 'POST', url: '/api/jobs/documents' }))
      ElMessage.error(`${options.file.name}: ${e.message}`)
    })
}

async function cancel(item: UploadItem) {
  if (item.jobId === null) {
    return
  }
  try {
    await cancelJob(item.jobId)
    item.stage = '取消中…'
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '取消失败')
  }
}

onBeforeUnmount(stopPolling)
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
      :show-file-list="false"
    >
      <div class="upload-text">拖拽文件到这里，或点击选择</div>
      <div class="upload-hint">上传后进入处理队列，可在此查看进度或取消。</div>
    </el-upload>

    <div v-if="uploads.length" class="job-list">
      <div v-for="item in uploads" :key="item.key" class="job-item">
        <div class="job-body">
          <div class="job-main">
            <span class="job-name">{{ item.name }}</span>
            <span class="job-status" :class="item.status">{{ statusText(item) }}</span>
          </div>
          <div v-if="item.error" class="job-error">{{ item.error }}</div>
        </div>
        <el-button v-if="canCancel(item)" size="small" link @click="cancel(item)">取消</el-button>
      </div>
    </div>
  </el-card>
</template>

<style scoped>
.upload-text {
  color: #374151;
  font-size: 14px;
}

.upload-hint {
  margin-top: 6px;
  color: #9ca3af;
  font-size: 12px;
}

.job-list {
  margin-top: 12px;
  border-top: 1px solid #f0f1f3;
}

.job-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid #f7f7f8;
}

.job-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
}

.job-body {
  flex: 1;
  min-width: 0;
}

.job-name {
  color: #374151;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.job-status {
  flex-shrink: 0;
  color: #6b7280;
  font-size: 12px;
}

.job-status.done {
  color: #15803d;
}

.job-status.failed {
  color: #b91c1c;
}

.job-error {
  margin-top: 4px;
  color: #b91c1c;
  font-size: 12px;
}
</style>
