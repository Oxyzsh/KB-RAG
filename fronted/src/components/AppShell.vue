<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus/es/components/message/index'
import { clearToken, logout } from '../api/client'
import { clearSession, loadSession, refreshTenants, session, switchTenant, tenantState } from '../session'

const route = useRoute()
const router = useRouter()
const active = computed(() => route.path)

const switching = ref(false)
const activeTenantSlug = computed(() => session.user?.tenant_slug ?? '')
const canSwitchTenant = computed(() => session.user?.can_switch === true)

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
  if (session.user?.can_switch) {
    try {
      await refreshTenants()
    } catch {
      // 租户列表加载失败不影响其他功能。
    }
  }
})

async function onTenantChange(slug: string) {
  if (!slug || switching.value) {
    return
  }
  switching.value = true
  try {
    const user = await switchTenant(slug)
    ElMessage.success(`已切换到「${user?.tenant_name ?? slug}」`)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '切换租户失败')
  } finally {
    switching.value = false
  }
}

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
      <div v-if="canSwitchTenant" class="tenant-switcher">
        <div class="tenant-label">切换租户</div>
        <el-select
          :model-value="activeTenantSlug"
          size="small"
          :loading="switching"
          @change="onTenantChange"
        >
          <el-option v-for="t in tenantState.list" :key="t.id" :label="t.name" :value="t.slug" />
        </el-select>
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
  background: #ffffff;
  color: #374151;
  border-right: 1px solid #e5e7eb;
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
  background: #2563eb;
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
  color: #9ca3af;
  font-size: 12px;
}

.user-card {
  margin: 0 6px 14px;
  padding: 10px 12px;
  border: 1px solid #f0f1f3;
  border-radius: 6px;
  background: #f9fafb;
}

.user-name {
  font-size: 13px;
  font-weight: 600;
  color: #111827;
}

.user-meta {
  margin-top: 3px;
  color: #6b7280;
  font-size: 12px;
}

.tenant-switcher {
  margin: 0 6px 14px;
}

.tenant-label {
  margin-bottom: 6px;
  color: #9ca3af;
  font-size: 12px;
}

.tenant-switcher :deep(.el-select) {
  width: 100%;
}

.nav {
  border-right: none;
  background: transparent;
  --el-menu-bg-color: transparent;
  --el-menu-text-color: #4b5563;
  --el-menu-hover-bg-color: #f3f4f6;
  --el-menu-active-color: #2563eb;
}

.nav :deep(.el-menu-item.is-active) {
  background: #eff6ff;
  border-radius: 6px;
}

.nav :deep(.el-menu-item:hover) {
  border-radius: 6px;
}

.sidebar-footer {
  margin-top: auto;
  padding: 12px 10px 0;
}

.sidebar-footer .el-button {
  color: #6b7280;
}

.main {
  padding: 0;
  background: #f7f8fa;
}
</style>
