<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { clearToken, logout } from '../api/client'
import { clearSession, loadSession, session } from '../session'

const route = useRoute()
const router = useRouter()
const active = computed(() => route.path)

const roleLabel = computed(() => {
  switch (session.user?.role) {
    case 'admin':
      return session.user.is_super ? '超级管理员' : '管理员'
    case 'editor':
      return '编辑者'
    case 'viewer':
      return '只读'
    default:
      return ''
  }
})

onMounted(async () => {
  if (!session.user) {
    try {
      await loadSession()
    } catch {
      // 401 已由 client 跳转登录页。
    }
  }
})

async function signOut() {
  try {
    await logout()
  } catch {
    // 会话可能已过期，本地清理后统一回登录页。
  }
  clearToken()
  clearSession()
  router.replace('/login')
}
</script>

<template>
  <el-container class="app-shell">
    <el-aside width="220px" class="sidebar">
      <div class="brand">
        <span class="brand-mark">K</span>
        <div>
          <strong>KB-RAG</strong>
          <small>企业知识库</small>
        </div>
      </div>
      <div v-if="session.user" class="user-card">
        <div class="user-name">{{ session.user.username }}</div>
        <div class="user-meta">{{ session.user.tenant_name }} · {{ roleLabel }}</div>
      </div>
      <el-menu :default-active="active" router class="nav">
        <el-menu-item index="/chat">知识问答</el-menu-item>
        <el-menu-item index="/documents">知识库管理</el-menu-item>
        <el-menu-item v-if="session.user?.role === 'admin'" index="/admin">系统管理</el-menu-item>
      </el-menu>
      <div class="sidebar-footer">
        <el-button text @click="signOut">退出登录</el-button>
      </div>
    </el-aside>
    <el-main class="main">
      <router-view />
    </el-main>
  </el-container>
</template>

<style scoped>
.app-shell {
  min-height: 100vh;
}

.sidebar {
  display: flex;
  flex-direction: column;
  background: #101828;
  color: #e4e7ec;
  padding: 18px 12px;
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px 16px;
}

.brand-mark {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  background: #2e90fa;
  color: #fff;
  font-weight: 700;
}

.brand strong {
  display: block;
  font-size: 16px;
  letter-spacing: 0.3px;
}

.brand small {
  display: block;
  margin-top: 2px;
  color: #98a2b3;
  font-size: 12px;
}

.user-card {
  margin: 0 6px 14px;
  padding: 10px 12px;
  border-radius: 10px;
  background: #1d2939;
}

.user-name {
  font-size: 13px;
  font-weight: 600;
}

.user-meta {
  margin-top: 3px;
  color: #98a2b3;
  font-size: 12px;
}

.nav {
  border-right: none;
  background: transparent;
  --el-menu-bg-color: transparent;
  --el-menu-text-color: #cbd5e1;
  --el-menu-hover-bg-color: #1d2939;
  --el-menu-active-color: #ffffff;
}

.nav :deep(.el-menu-item.is-active) {
  background: #2e90fa;
  border-radius: 8px;
}

.nav :deep(.el-menu-item:hover) {
  border-radius: 8px;
}

.sidebar-footer {
  margin-top: auto;
  padding: 12px 10px 0;
}

.sidebar-footer .el-button {
  color: #98a2b3;
}

.main {
  padding: 0;
  background: #f7f8fa;
}
</style>
