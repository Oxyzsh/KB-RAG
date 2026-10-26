<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import type { UploadRawFile, UploadRequestOptions } from 'element-plus'
import { uploadDocument } from '../api/client'

const emit = defineEmits<{ uploaded: [] }>()

const allowed = ['.md', '.markdown', '.txt', '.docx', '.pdf']
const maxBytes = 20 * 1024 * 1024
let queue: Promise<void> = Promise.resolve()

const isUploading = ref(false)
const currentFileName = ref('')

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
  isUploading.value = true
  currentFileName.value = options.file.name

  const task = queue.then(async () => {
    const res = await uploadDocument(options.file)
    options.onSuccess(res)
    if (res.outcome === 'failed') {
      ElMessage.warning(`${options.file.name} 已保存，但入库失败，可在列表中重试`)
    } else {
      ElMessage.success(`${options.file.name} 解析并嵌入完成`)
    }
    emit('uploaded')
  })

  queue = task.then(
    () => {
      isUploading.value = false
    },
    () => {
      isUploading.value = false
    },
  )

  return task.catch((err: unknown) => {
    isUploading.value = false
    const e = err instanceof Error ? err : new Error('上传失败')
    options.onError(Object.assign(e, { status: 0, method: 'POST', url: '/api/documents' }))
    ElMessage.error(`${options.file.name}: ${e.message}`)
  })
}
</script>

<template>
  <el-card class="panel upload-card" shadow="never">
    <template #header>
      <div class="panel-header">
        <div class="header-left">
          <span>上传并入库文档</span>
          <span class="sub-tip">系统将自动切分、向量化并索引</span>
        </div>
        <span class="muted">单文件 ≤ 20 MB</span>
      </div>
    </template>

    <div class="upload-content">
      <el-upload
        drag
        multiple
        :http-request="uploadFile"
        :before-upload="beforeUpload"
        accept=".md,.markdown,.txt,.docx,.pdf"
        :show-file-list="true"
        class="custom-uploader"
      >
        <div class="upload-inner">
          <div class="upload-icon-box">📁</div>
          <div class="el-upload__text">
            点击或将文件拖拽到此区域上传
          </div>
          <div class="format-tags">
            <span class="format-tag">Markdown (.md)</span>
            <span class="format-tag">Text (.txt)</span>
            <span class="format-tag">Word (.docx)</span>
            <span class="format-tag">PDF (.pdf)</span>
          </div>
        </div>
      </el-upload>

      <div v-if="isUploading" class="uploading-indicator">
        <div class="indicator-spinner">⏳</div>
        <div class="indicator-text">
          正在处理: <strong>{{ currentFileName }}</strong> (切分与文本嵌入中…)
        </div>
      </div>
    </div>
  </el-card>
</template>

<style scoped>
.upload-card {
  border-radius: 12px;
}

.header-left {
  display: flex;
  align-items: baseline;
  gap: 10px;
}

.sub-tip {
  font-size: 12.5px;
  color: #64748b;
  font-weight: 400;
}

.custom-uploader :deep(.el-upload-dragger) {
  padding: 24px 16px;
  border: 1.5px dashed #cbd5e1;
  border-radius: 10px;
  background: #f8fafc;
  transition: all 0.2s ease;
}

.custom-uploader :deep(.el-upload-dragger:hover) {
  border-color: #2563eb;
  background: #eff6ff;
}

.upload-inner {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}

.upload-icon-box {
  font-size: 32px;
  margin-bottom: 2px;
}

.el-upload__text {
  font-size: 14px;
  color: #334155;
}

.format-tags {
  display: flex;
  gap: 6px;
  margin-top: 6px;
  flex-wrap: wrap;
  justify-content: center;
}

.format-tag {
  font-size: 11px;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  color: #64748b;
  padding: 2px 8px;
  border-radius: 4px;
}

.uploading-indicator {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  padding: 10px 14px;
  background: #eff6ff;
  border: 1px solid #bfdbfe;
  border-radius: 8px;
  font-size: 13px;
  color: #1d4ed8;
}

.indicator-spinner {
  animation: spin 2s infinite linear;
}

@keyframes spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}
</style>
