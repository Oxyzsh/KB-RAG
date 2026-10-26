<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { clearToken } from '../api/client'

const route = useRoute()
const router = useRouter()
const active = computed(() => route.path)

function logout() {
  clearToken()
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
      <el-menu :default-active="active" router class="nav">
        <el-menu-item index="/chat">知识问答</el-menu-item>
        <el-menu-item index="/documents">知识库管理</el-menu-item>
      </el-menu>
      <div class="sidebar-footer">
        <el-button text @click="logout">退出登录</el-button>
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
  padding: 6px 10px 22px;
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
