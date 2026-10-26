<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { login, setToken } from '../api/client'
import { setSessionUser } from '../session'

const router = useRouter()
const tenant = ref('default')
const username = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  const tenantValue = tenant.value.trim() || 'default'
  const usernameValue = username.value.trim()
  if (!usernameValue || !password.value) {
    error.value = '请输入用户名和密码'
    return
  }
  loading.value = true
  error.value = ''
  try {
    const res = await login(tenantValue, usernameValue, password.value)
    setToken(res.token)
    setSessionUser(res.user)
    await router.replace('/chat')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '登录失败'
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
      <el-form label-position="top" @submit.prevent>
        <el-form-item label="租户">
          <el-input v-model="tenant" size="large" placeholder="default" />
        </el-form-item>
        <el-form-item label="用户名">
          <el-input v-model="username" size="large" placeholder="用户名" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input
            v-model="password"
            type="password"
            show-password
            size="large"
            placeholder="密码"
            @keyup.enter="submit"
          />
        </el-form-item>
      </el-form>
      <el-alert
        v-if="error"
        class="login-error"
        :title="error"
        type="error"
        :closable="false"
        show-icon
      />
      <el-button type="primary" size="large" class="login-button" :loading="loading" @click="submit">
        登录
      </el-button>
      <p class="login-hint">首次启动由 KB_RAG_ADMIN_USER / KB_RAG_ADMIN_PASSWORD 创建超级管理员。</p>
    </el-card>
  </div>
</template>

<style scoped>
.login-page {
  display: grid;
  min-height: 100vh;
  place-items: center;
  background: #f7f8fa;
  padding: 24px;
}

.login-card {
  width: min(420px, 100%);
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 8px;
}

.login-brand {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 20px;
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
  background: #2563eb;
  color: #fff;
  font-size: 20px;
  font-weight: 700;
}

.login-error {
  margin-top: 4px;
  margin-bottom: 14px;
}

.login-button {
  width: 100%;
  margin-top: 4px;
}

.login-hint {
  margin: 16px 0 4px;
  color: #98a2b3;
  font-size: 12px;
  text-align: center;
  line-height: 1.6;
}
</style>
