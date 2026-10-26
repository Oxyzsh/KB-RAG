<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import type { UploadRawFile, UploadRequestOptions } from 'element-plus'
import { uploadDocument } from '../api/client'

const emit = defineEmits<{ uploaded: [] }>()

const allowed = ['.md', '.markdown', '.txt', '.docx', '.pdf']
const maxBytes = 20 * 1024 * 1024
let queue: Promise<void> = Promise.resolve()

const uploadingFile = ref('')
const pendingCount = ref(0)

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

function uploadFile(options: UploadRequestOptions) {
  pendingCount.value += 1
  uploadingFile.value = options.file.name
  const task = queue.then(async () => {
    const res = await uploadDocument(options.file)
    options.onSuccess(res)
    if (res.outcome === 'failed') {
      ElMessage.warning(`${options.file.name} 已保存，但入库失败，可在列表中重试`)
    } else {
      ElMessage.success(`${options.file.name} 已入库`)
    }
    emit('uploaded')
  })
  queue = task.then(
    () => {
      pendingCount.value -= 1
      if (pendingCount.value === 0) {
        uploadingFile.value = ''
      }
    },
    () => {
      pendingCount.value -= 1
      if (pendingCount.value === 0) {
        uploadingFile.value = ''
      }
    },
  )
  return task.catch((err: unknown) => {
    const e = err instanceof Error ? err : new Error('上传失败')
    options.onError(Object.assign(e, { status: 0, method: 'POST', url: '/api/documents' }))
    ElMessage.error(`${options.file.name}: ${e.message}`)
  })
}
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
      :show-file-list="true"
    >
      <div class="upload-text">拖拽文件到这里，或点击选择</div>
      <div class="upload-hint">上传后同步解析与嵌入，失败文件可在列表中重试。</div>
    </el-upload>
    <div v-if="uploadingFile" class="uploading">正在处理：{{ uploadingFile }}</div>
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

.uploading {
  margin-top: 10px;
  color: #6b7280;
  font-size: 12px;
}
</style>
