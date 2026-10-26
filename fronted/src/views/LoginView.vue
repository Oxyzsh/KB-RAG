<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { setToken, clearToken, verifySession } from '../api/client'

const router = useRouter()
const token = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  const value = token.value.trim()
  if (!value) {
    error.value = '请输入访问令牌'
    return
  }
  loading.value = true
  error.value = ''
  setToken(value)
  try {
    await verifySession()
    await router.replace('/chat')
  } catch (e) {
    clearToken()
    error.value = e instanceof Error ? e.message : '令牌校验失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <el-card class="login-card" shadow="never">
      <div class="login-brand">
        <span class="brand-mark">K</span>
        <div>
          <h1>KB-RAG</h1>
          <p>企业知识库问答</p>
        </div>
      </div>
      <el-input
        v-model="token"
        type="password"
        show-password
        size="large"
        placeholder="共享访问令牌"
        @keyup.enter="submit"
      />
      <el-alert
        v-if="error"
        class="login-error"
        :title="error"
        type="error"
        :closable="false"
        show-icon
      />
      <el-button type="primary" size="large" class="login-button" :loading="loading" @click="submit">
        进入知识库
      </el-button>
      <p class="login-hint">令牌由部署者提供，只保存在当前标签页。</p>
    </el-card>
  </div>
</template>

<style scoped>
.login-page {
  display: grid;
  min-height: 100vh;
  place-items: center;
  background:
    radial-gradient(circle at 15% 20%, rgba(46, 144, 250, 0.22), transparent 32%),
    radial-gradient(circle at 85% 80%, rgba(16, 24, 40, 0.12), transparent 30%),
    #f5f7fb;
  padding: 24px;
}

.login-card {
  width: min(400px, 100%);
  border: 1px solid #e4e7ec;
  border-radius: 16px;
  padding: 10px;
}

.login-brand {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 24px;
}

.login-brand h1 {
  margin: 0;
  font-size: 24px;
  color: #101828;
}

.login-brand p {
  margin: 4px 0 0;
  color: #667085;
  font-size: 13px;
}

.brand-mark {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  border-radius: 12px;
  background: #2e90fa;
  color: #fff;
  font-size: 20px;
  font-weight: 700;
}

.login-error {
  margin-top: 14px;
}

.login-button {
  width: 100%;
  margin-top: 18px;
}

.login-hint {
  margin: 16px 0 4px;
  color: #98a2b3;
  font-size: 12px;
  text-align: center;
}
</style>
