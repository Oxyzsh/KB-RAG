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
  viewer: '只读',
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
      confirmButtonText: '重置',
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
    ElMessage.success('密码已重置，该用户会话已失效')
    await loadUsers()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error(e instanceof Error ? e.message : '重置失败')
    }
  }
}

async function removeUser(row: User) {
  try {
    await ElMessageBox.confirm(`确定删除用户「${row.username}」？其会话会立即失效。`, '删除用户', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
    await deleteUser(row.id)
    ElMessage.success('用户已删除')
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
    ElMessage.success('租户已创建')
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
    ElMessage.warning('请填写管理员用户名，密码至少 12 个字符')
    return
  }
  try {
    await resetTenantAdmin(target.slug, resetForm.username.trim(), resetForm.password)
    ElMessage.success('管理员密码已重置')
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
  <div class="page">
    <div class="page-header">
      <div>
        <h2>系统管理</h2>
        <p class="muted">管理本租户用户与查看审计；超级管理员可创建租户并重置租户管理员密码。</p>
      </div>
    </div>

    <el-tabs v-model="activeTab" @tab-change="onTabChange">
      <el-tab-pane label="用户管理" name="users">
        <el-card class="panel" shadow="never">
          <template #header>
            <div class="panel-header">
              <span>用户</span>
              <el-button type="primary" size="small" @click="openCreateUser">新建用户</el-button>
            </div>
          </template>
          <el-table v-loading="usersLoading" :data="users">
            <el-table-column prop="username" label="用户名" min-width="160">
              <template #default="{ row }">
                <span>{{ row.username }}</span>
                <el-tag v-if="row.is_super" class="super-tag" type="danger" size="small">超级管理员</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="角色" width="110">
              <template #default="{ row }">
                <el-tag size="small">{{ roleLabels[row.role as Role] }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="创建时间" width="180">
              <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="240">
              <template #default="{ row }">
                <template v-if="!row.is_super">
                  <el-button size="small" @click="openRole(row as User)">角色</el-button>
                  <el-button size="small" @click="resetPassword(row as User)">重置密码</el-button>
                  <el-button size="small" type="danger" plain @click="removeUser(row as User)">删除</el-button>
                </template>
                <span v-else class="muted">受保护</span>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="审计日志" name="audit">
        <el-card class="panel" shadow="never">
          <template #header>
            <div class="panel-header audit-header">
              <div class="audit-filters">
                <el-input
                  v-model="auditAction"
                  size="small"
                  class="audit-action"
                  placeholder="按动作过滤，如 ask"
                  clearable
                  @keyup.enter="applyAuditFilter"
                />
                <el-checkbox v-if="isSuper" v-model="auditScopeAll">全局</el-checkbox>
                <el-button size="small" @click="applyAuditFilter">查询</el-button>
              </div>
              <span class="muted">共 {{ auditTotal }} 条</span>
            </div>
          </template>
          <el-table v-loading="auditLoading" :data="audit">
            <el-table-column type="expand">
              <template #default="{ row }">
                <pre class="audit-detail">{{ row.detail }}</pre>
              </template>
            </el-table-column>
            <el-table-column label="时间" width="180">
              <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column label="租户" width="100">
              <template #default="{ row }">{{ row.tenant_slug || '-' }}</template>
            </el-table-column>
            <el-table-column prop="username" label="用户" width="140" />
            <el-table-column prop="action" label="动作" width="160" />
            <el-table-column prop="target" label="目标" min-width="160" />
            <el-table-column prop="ip" label="IP" width="130" />
          </el-table>
          <div class="audit-pagination">
            <el-pagination
              layout="prev, pager, next"
              :total="auditTotal"
              :page-size="auditPageSize"
              :current-page="auditPage"
              @current-change="onAuditPageChange"
            />
          </div>
        </el-card>
      </el-tab-pane>

      <el-tab-pane v-if="isSuper" label="租户管理" name="tenants">
        <el-card class="panel" shadow="never">
          <template #header>
            <div class="panel-header">
              <span>租户</span>
              <el-button type="primary" size="small" @click="openCreateTenant">新建租户</el-button>
            </div>
          </template>
          <el-table v-loading="tenantsLoading" :data="tenants">
            <el-table-column prop="slug" label="标识" width="180" />
            <el-table-column prop="name" label="名称" min-width="180" />
            <el-table-column label="操作" width="180">
              <template #default="{ row }">
                <el-button size="small" @click="openResetTenantAdmin(row as Tenant)">重置管理员密码</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="userDialog" title="新建用户" width="420px">
      <el-form label-position="top">
        <el-form-item label="用户名">
          <el-input v-model="userForm.username" placeholder="3-64 个字符" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="userForm.password" type="password" show-password placeholder="至少 12 个字符" />
        </el-form-item>
        <el-form-item label="角色">
          <el-select v-model="userForm.role">
            <el-option label="管理员" value="admin" />
            <el-option label="编辑者" value="editor" />
            <el-option label="只读" value="viewer" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="userDialog = false">取消</el-button>
        <el-button type="primary" @click="submitUser">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="roleDialog" title="修改角色" width="360px">
      <el-form label-position="top">
        <el-form-item :label="roleTarget?.username ?? ''">
          <el-select v-model="roleValue">
            <el-option label="管理员" value="admin" />
            <el-option label="编辑者" value="editor" />
            <el-option label="只读" value="viewer" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="roleDialog = false">取消</el-button>
        <el-button type="primary" @click="submitRole">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="tenantDialog" title="新建租户" width="440px">
      <el-form label-position="top">
        <el-form-item label="租户标识">
          <el-input v-model="tenantForm.slug" placeholder="小写字母、数字、连字符" />
        </el-form-item>
        <el-form-item label="租户名称">
          <el-input v-model="tenantForm.name" />
        </el-form-item>
        <el-form-item label="首个管理员用户名">
          <el-input v-model="tenantForm.admin_username" />
        </el-form-item>
        <el-form-item label="首个管理员密码">
          <el-input v-model="tenantForm.admin_password" type="password" show-password placeholder="至少 12 个字符" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="tenantDialog = false">取消</el-button>
        <el-button type="primary" @click="submitTenant">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="resetDialog" :title="`重置 ${resetTarget?.name ?? ''} 管理员密码`" width="420px">
      <el-form label-position="top">
        <el-form-item label="管理员用户名">
          <el-input v-model="resetForm.username" />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="resetForm.password" type="password" show-password placeholder="至少 12 个字符" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="resetDialog = false">取消</el-button>
        <el-button type="primary" @click="submitResetTenantAdmin">重置</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.panel {
  border: 1px solid #e4e7ec;
  border-radius: 12px;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 14px;
  font-weight: 600;
}

.panel-header .muted {
  font-size: 12px;
  font-weight: 400;
}

.audit-header {
  flex-wrap: wrap;
}

.audit-filters {
  display: flex;
  align-items: center;
  gap: 10px;
}

.audit-action {
  width: 220px;
}

.audit-detail {
  margin: 0;
  padding: 10px 16px;
  background: #f9fafb;
  color: #475467;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}

.audit-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 14px;
}

.super-tag {
  margin-left: 8px;
}
</style>
