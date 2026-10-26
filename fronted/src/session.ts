import { reactive } from 'vue'
import { clearTenantOverride, getSession, listTenants, setTenantOverride } from './api/client'
import type { Tenant, User } from './api/types'

export const session = reactive<{ user: User | null }>({ user: null })
export const tenantState = reactive<{ list: Tenant[] }>({ list: [] })

export async function refreshTenants(): Promise<void> {
  if (!session.user?.can_switch) {
    tenantState.list = []
    return
  }
  tenantState.list = (await listTenants()).tenants
}

export async function loadSession(force = false): Promise<User | null> {
  if (session.user && !force) {
    return session.user
  }
  const res = await getSession()
  session.user = res.user
  return res.user
}

export function setSessionUser(user: User): void {
  session.user = user
}

export function clearSession(): void {
  session.user = null
  clearTenantOverride()
}

export async function switchTenant(slug: string): Promise<User | null> {
  const current = session.user
  if (!current?.can_switch) {
    return current
  }
  const previousSlug = current.tenant_slug
  setTenantOverride(slug === current.home_tenant_slug ? '' : slug)
  try {
    return await loadSession(true)
  } catch (e) {
    setTenantOverride(previousSlug === current.home_tenant_slug ? '' : previousSlug)
    throw e
  }
}

export function isAdmin(): boolean {
  return session.user?.role === 'admin'
}

export function canEditDocuments(): boolean {
  return session.user?.role === 'admin' || session.user?.role === 'editor'
}
