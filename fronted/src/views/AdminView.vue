<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  clearToken,
  createTenant,
  createUser,
  deleteUser,
  listAudit,
  listTenants,
  listUsers,
  resetTenantAdmin,
  updateUser,
} from '../api/client'
import type { AuditItem, Role, Tenant, User } from '../api/types'
import { clearSession, session } from '../session'

const router = useRouter()
const activeTab = ref('users')
const isSuper = computed(() => session.user?.is_super === true)

const users = ref<User[]>([])
const usersLoading = ref(false)
const userDialog = ref(false)
const userForm = reactive({ username: '', password: '', role: 'viewer' as Role })

const roleDialog = ref(false)
const roleTarget = ref<User | null>(null)
const roleValue = ref<Role>('viewer')

const audit = ref<AuditItem[]>([])
const auditTotal = ref(0)
const auditPage = ref(1)
const auditPageSize = 20
const auditAction = ref('')
const auditScopeAll = ref(false)
const auditLoading = ref(false)

const tenants = ref<Tenant[]>([])
const tenantsLoading = ref(false)
const tenantDialog = ref(false)
const tenantForm = reactive({ slug: '', name: '', admin_username: '', admin_password: '' })

const resetDialog = ref(false)
const resetTarget = ref<Tenant | null>(null)
const resetForm = reactive({ username: '', password: '' })

const roleLabels: Record<Role, string> = {
  admin: '管理员',
  editor: '编辑者',
  viewer: '只读访客',
}

function formatTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

async function loadUsers() {
  usersLoading.value = true
  try {
    const res = await listUsers()
    users.value = res.users
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '加载用户失败')
  } finally {
    usersLoading.value = false
  }
}

async function loadAudit() {
  auditLoading.value = true
  try {
    const res = await listAudit({
      page: auditPage.value,
      page_size: auditPageSize,
      action: auditAction.value.trim(),
      scope: auditScopeAll.value && isSuper.value ? 'all' : '',
    })
    audit.value = res.items
    auditTotal.value = res.total
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '加载审计失败')
  } finally {
    auditLoading.value = false
  }
}

async function loadTenants() {
  if (!isSuper.value) {
    return
  }
  tenantsLoading.value = true
  try {
    const res = await listTenants()
    tenants.value = res.tenants
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '加载租户失败')
  } finally {
    tenantsLoading.value = false
  }
}

function openCreateUser() {
  userForm.username = ''
  userForm.password = ''
  userForm.role = 'viewer'
  userDialog.value = true
}

async function submitUser() {
  if (!userForm.username.trim()) {
    ElMessage.warning('请输入用户名')
    return
  }
  if (userForm.password.length < 12) {
    ElMessage.warning('密码至少 12 个字符')
    return
  }
  try {
    await createUser(userForm.username.trim(), userForm.password, userForm.role)
    ElMessage.success('用户已创建')
    userDialog.value = false
    await loadUsers()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '创建失败')
  }
}

function openRole(row: User) {
  roleTarget.value = row
  roleValue.value = row.role
  roleDialog.value = true
}

async function submitRole() {
  const target = roleTarget.value
  if (!target) {
    return
  }
  try {
    await updateUser(target.id, { role: roleValue.value })
    ElMessage.success('角色已更新')
    roleDialog.value = false
    await loadUsers()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '更新失败')
  }
}

async function resetPassword(row: User) {
  try {
    const { value } = await ElMessageBox.prompt('输入新密码（至少 12 个字符）', `重置 ${row.username} 的密码`, {
      inputType: 'password',
      confirmButtonText: '重置密码',
      cancelButtonText: '取消',
    })
    if (!value || value.length < 12) {
      ElMessage.warning('密码至少 12 个字符')
      return
    }
    await updateUser(row.id, { password: value })
    if (row.id === session.user?.id) {
      ElMessage.success('密码已重置，请重新登录')
      clearToken()
      clearSession()
      await router.replace('/login')
      return
    }
    ElMessage.success('密码已重置，该用户会话已立即失效')
    await loadUsers()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error(e instanceof Error ? e.message : '重置失败')
    }
  }
}

async function removeUser(row: User) {
  try {
    await ElMessageBox.confirm(`确定删除用户「${row.username}」？删除后该用户的所有会话将立即失效。`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      confirmButtonClass: 'el-button--danger',
    })
    await deleteUser(row.id)
    ElMessage.success('用户已成功删除')
    await loadUsers()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error(e instanceof Error ? e.message : '删除失败')
    }
  }
}

function openCreateTenant() {
  tenantForm.slug = ''
  tenantForm.name = ''
  tenantForm.admin_username = ''
  tenantForm.admin_password = ''
  tenantDialog.value = true
}

async function submitTenant() {
  if (!/^[a-z0-9][a-z0-9-]{1,31}$/.test(tenantForm.slug.trim())) {
    ElMessage.warning('租户标识需为 2-32 位小写字母、数字或连字符')
    return
  }
  if (tenantForm.admin_password.length < 12) {
    ElMessage.warning('管理员密码至少 12 个字符')
    return
  }
  try {
    await createTenant({
      slug: tenantForm.slug.trim(),
      name: tenantForm.name.trim(),
      admin_username: tenantForm.admin_username.trim(),
      admin_password: tenantForm.admin_password,
    })
    ElMessage.success('租户创建成功')
    tenantDialog.value = false
    await loadTenants()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '创建失败')
  }
}

function openResetTenantAdmin(row: Tenant) {
  resetTarget.value = row
  resetForm.username = ''
  resetForm.password = ''
  resetDialog.value = true
}

async function submitResetTenantAdmin() {
  const target = resetTarget.value
  if (!target) {
    return
  }
  if (!resetForm.username.trim() || resetForm.password.length < 12) {
    ElMessage.warning('请填写管理员用户名，且密码至少 12 个字符')
    return
  }
  try {
    await resetTenantAdmin(target.slug, resetForm.username.trim(), resetForm.password)
    ElMessage.success('管理员密码已成功重置')
    resetDialog.value = false
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '重置失败')
  }
}

function onTabChange(name: string | number) {
  if (name === 'users') {
    void loadUsers()
  } else if (name === 'audit') {
    void loadAudit()
  } else if (name === 'tenants') {
    void loadTenants()
  }
}

function onAuditPageChange(page: number) {
  auditPage.value = page
  void loadAudit()
}

function applyAuditFilter() {
  auditPage.value = 1
  void loadAudit()
}

onMounted(() => {
  void loadUsers()
  void loadAudit()
  void loadTenants()
})
</script>

<template>
  <div class="page admin-page">
    <div class="page-header">
      <div>
        <h2>系统控制台</h2>
        <p class="muted">用户授权、审计日志追溯与多租户管理</p>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="custom-tabs" @tab-change="onTabChange">
      <el-tab-pane label="用户成员管理" name="users">
        <el-card class="panel" shadow="never">
          <template #header>
            <div class="panel-header">
              <div class="header-left">
                <span>租户用户列表</span>
                <span class="count-tag">{{ users.length }} 名成员</span>
              </div>
              <el-button type="primary" size="small" @click="openCreateUser">添加成员</el-button>
            </div>
          </template>
          <el-table v-loading="usersLoading" :data="users" stripe>
            <el-table-column prop="username" label="用户名" min-width="160">
              <template #default="{ row }">
                <div class="user-cell">
                  <span class="user-avatar">👤</span>
                  <span class="user-name">{{ row.username }}</span>
                  <el-tag v-if="row.is_super" class="super-tag" type="danger" size="small" effect="dark">
                    超级管理员
                  </el-tag>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="角色权限" width="130">
              <template #default="{ row }">
                <el-tag
                  :type="row.role === 'admin' ? 'danger' : row.role === 'editor' ? 'primary' : 'info'"
                  size="small"
                  effect="light"
                >
                  {{ roleLabels[row.role as Role] }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="加入时间" width="180">
              <template #default="{ row }">
                <span class="time-text">{{ formatTime(row.created_at) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="220" fixed="right">
              <template #default="{ row }">
                <template v-if="!row.is_super">
                  <el-button size="small" link type="primary" @click="openRole(row as User)">修改角色</el-button>
                  <el-button size="small" link type="warning" @click="resetPassword(row as User)">重置密码</el-button>
                  <el-button size="small" link type="danger" @click="removeUser(row as User)">删除</el-button>
                </template>
                <span v-else class="muted-tag">内置超管保护</span>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="安全审计追踪" name="audit">
        <el-card class="panel" shadow="never">
          <template #header>
            <div class="panel-header audit-header">
              <div class="audit-filters">
                <el-input
                  v-model="auditAction"
                  size="small"
                  class="audit-action"
                  placeholder="按操作行为过滤 (如 ask, document.upload)"
                  clearable
                  @keyup.enter="applyAuditFilter"
                />
                <el-checkbox v-if="isSuper" v-model="auditScopeAll" border size="small">查看全局租户</el-checkbox>
                <el-button size="small" type="primary" plain @click="applyAuditFilter">检索</el-button>
              </div>
              <span class="muted">共查询到 {{ auditTotal }} 条审计记录</span>
            </div>
          </template>
          <el-table v-loading="auditLoading" :data="audit" stripe>
            <el-table-column type="expand">
              <template #default="{ row }">
                <div class="detail-wrapper">
                  <div class="detail-title">操作元数据 (Detail Payload)</div>
                  <pre class="audit-detail">{{ row.detail }}</pre>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="时间" width="170">
              <template #default="{ row }">
                <span class="time-text">{{ formatTime(row.created_at) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="租户" width="110">
              <template #default="{ row }">
                <el-tag size="small" type="info" effect="plain">{{ row.tenant_slug || '系统未归属' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="username" label="操作人" width="130">
              <template #default="{ row }">
                <strong>{{ row.username || '-' }}</strong>
              </template>
            </el-table-column>
            <el-table-column prop="action" label="动作类型" width="160">
              <template #default="{ row }">
                <span class="action-code">{{ row.action }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="target" label="操作目标" min-width="150" />
            <el-table-column prop="ip" label="来源 IP" width="130" />
          </el-table>
          <div class="audit-pagination">
            <el-pagination
              layout="prev, pager, next, total"
              :total="auditTotal"
              :page-size="auditPageSize"
              :current-page="auditPage"
              background
              @current-change="onAuditPageChange"
            />
          </div>
        </el-card>
      </el-tab-pane>

      <el-tab-pane v-if="isSuper" label="租户配置中心" name="tenants">
        <el-card class="panel" shadow="never">
          <template #header>
            <div class="panel-header">
              <div class="header-left">
                <span>系统租户列表</span>
                <span class="count-tag">{{ tenants.length }} 个租户</span>
              </div>
              <el-button type="primary" size="small" @click="openCreateTenant">创建新租户</el-button>
            </div>
          </template>
          <el-table v-loading="tenantsLoading" :data="tenants" stripe>
            <el-table-column prop="slug" label="租户唯一标识 (Slug)" width="200">
              <template #default="{ row }">
                <code class="slug-code">{{ row.slug }}</code>
              </template>
            </el-table-column>
            <el-table-column prop="name" label="租户显示名称" min-width="200" />
            <el-table-column label="管理员运维" width="180">
              <template #default="{ row }">
                <el-button size="small" link type="warning" @click="openResetTenantAdmin(row as Tenant)">
                  重置租户管理员密码
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <!-- 对话框 -->
    <el-dialog v-model="userDialog" title="新建用户成员" width="400px">
      <el-form label-position="top">
        <el-form-item label="用户名">
          <el-input v-model="userForm.username" placeholder="3-64 个字符，不含空格" />
        </el-form-item>
        <el-form-item label="初始密码">
          <el-input v-model="userForm.password" type="password" show-password placeholder="至少 12 个字符" />
        </el-form-item>
        <el-form-item label="分配角色">
          <el-select v-model="userForm.role" style="width: 100%">
            <el-option label="管理员 (完全权限)" value="admin" />
            <el-option label="编辑者 (可上传与删除文档)" value="editor" />
            <el-option label="只读访客 (仅支持问答检索)" value="viewer" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="userDialog = false">取消</el-button>
        <el-button type="primary" @click="submitUser">确认创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="roleDialog" title="修改用户角色" width="360px">
      <el-form label-position="top">
        <el-form-item :label="'当前用户：' + (roleTarget?.username ?? '')">
          <el-select v-model="roleValue" style="width: 100%">
            <el-option label="管理员" value="admin" />
            <el-option label="编辑者" value="editor" />
            <el-option label="只读访客" value="viewer" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="roleDialog = false">取消</el-button>
        <el-button type="primary" @click="submitRole">保存变更</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="tenantDialog" title="创建新租户空间" width="440px">
      <el-form label-position="top">
        <el-form-item label="租户标识 (Slug)">
          <el-input v-model="tenantForm.slug" placeholder="如 team-finance，限小写字母与连字符" />
        </el-form-item>
        <el-form-item label="租户显示名称">
          <el-input v-model="tenantForm.name" placeholder="如 财务管理部" />
        </el-form-item>
        <el-form-item label="首个租户管理员用户名">
          <el-input v-model="tenantForm.admin_username" placeholder="如 dept-admin" />
        </el-form-item>
        <el-form-item label="管理员初始密码">
          <el-input v-model="tenantForm.admin_password" type="password" show-password placeholder="至少 12 个字符" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="tenantDialog = false">取消</el-button>
        <el-button type="primary" @click="submitTenant">立即创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="resetDialog" :title="`重置 ${resetTarget?.name ?? ''} 管理员密码`" width="420px">
      <el-form label-position="top">
        <el-form-item label="租户管理员用户名">
          <el-input v-model="resetForm.username" placeholder="请输入待重置的用户名" />
        </el-form-item>
        <el-form-item label="新设密码">
          <el-input v-model="resetForm.password" type="password" show-password placeholder="至少 12 个字符" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="resetDialog = false">取消</el-button>
        <el-button type="primary" @click="submitResetTenantAdmin">确认重置</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.admin-page {
  max-width: 1200px;
}

.custom-tabs :deep(.el-tabs__nav-wrap::after) {
  height: 1px;
  background-color: #e2e8f0;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.count-tag {
  font-size: 12px;
  color: #64748b;
  background: #f1f5f9;
  padding: 2px 8px;
  border-radius: 10px;
}

.user-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.user-avatar {
  font-size: 16px;
}

.user-name {
  font-weight: 600;
  color: #0f172a;
}

.super-tag {
  font-weight: 500;
  border-radius: 4px;
}

.muted-tag {
  color: #94a3b8;
  font-size: 12px;
}

.time-text {
  font-size: 12.5px;
  color: #64748b;
}

.audit-header {
  flex-wrap: wrap;
  gap: 12px;
}

.audit-filters {
  display: flex;
  align-items: center;
  gap: 12px;
}

.audit-action {
  width: 260px;
}

.action-code {
  font-family: ui-monospace, SFMono-Regular, monospace;
  font-size: 12px;
  background: #f1f5f9;
  padding: 2px 6px;
  border-radius: 4px;
  color: #0f172a;
}

.slug-code {
  font-family: ui-monospace, SFMono-Regular, monospace;
  font-size: 12px;
  color: #2563eb;
  background: #eff6ff;
  padding: 2px 6px;
  border-radius: 4px;
}

.detail-wrapper {
  padding: 12px 16px;
  background: #f8fafc;
  border-radius: 8px;
  border: 1px solid #f1f5f9;
}

.detail-title {
  font-size: 12px;
  font-weight: 600;
  color: #475569;
  margin-bottom: 6px;
}

.audit-detail {
  margin: 0;
  font-size: 12px;
  color: #334155;
  white-space: pre-wrap;
  word-break: break-word;
}

.audit-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid #f1f5f9;
}
</style>
