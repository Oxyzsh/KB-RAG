import type {
  AskResponse,
  AuditListResponse,
  CreateTenantResponse,
  DocumentListResponse,
  ErrorBody,
  LoginResponse,
  Role,
  SessionResponse,
  Stats,
  TenantListResponse,
  UploadResponse,
  UserListResponse,
} from './types'

const TOKEN_KEY = 'kbrag_token'
const TENANT_KEY = 'kbrag_tenant'

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export function getToken(): string {
  return sessionStorage.getItem(TOKEN_KEY) ?? ''
}

export function setToken(token: string): void {
  sessionStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  sessionStorage.removeItem(TOKEN_KEY)
}

export function getTenantOverride(): string {
  return sessionStorage.getItem(TENANT_KEY) ?? ''
}

export function setTenantOverride(slug: string): void {
  if (slug) {
    sessionStorage.setItem(TENANT_KEY, slug)
  } else {
    sessionStorage.removeItem(TENANT_KEY)
  }
}

export function clearTenantOverride(): void {
  sessionStorage.removeItem(TENANT_KEY)
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
    const tenant = getTenantOverride()
    if (tenant) {
      headers.set('X-Tenant', tenant)
    }
  }
  if (init.body && !(init.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const res = await fetch(`/api${path}`, { ...init, headers })
  const text = await res.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = null
    }
  }

  if (!res.ok) {
    if (res.status === 401) {
      clearToken()
      clearTenantOverride()
      if (!window.location.hash.startsWith('#/login')) {
        window.location.hash = '#/login'
      }
    }
    const message = (data as ErrorBody | null)?.error ?? `请求失败（HTTP ${res.status}）`
    throw new ApiError(res.status, message)
  }
  return data as T
}

export function login(tenant: string, username: string, password: string): Promise<LoginResponse> {
  return request<LoginResponse>('/login', {
    method: 'POST',
    body: JSON.stringify({ tenant, username, password }),
  })
}

export function logout(): Promise<void> {
  return request<void>('/logout', { method: 'POST' })
}

export function getSession(): Promise<SessionResponse> {
  return request<SessionResponse>('/session')
}

export function getStats(): Promise<Stats> {
  return request<Stats>('/stats')
}

export function listDocuments(): Promise<DocumentListResponse> {
  return request<DocumentListResponse>('/documents')
}

export function uploadDocument(file: File): Promise<UploadResponse> {
  const form = new FormData()
  form.append('file', file)
  return request<UploadResponse>('/documents', { method: 'POST', body: form })
}

export function deleteDocument(id: number): Promise<void> {
  return request<void>(`/documents/${id}`, { method: 'DELETE' })
}

export function reingestDocument(id: number): Promise<UploadResponse> {
  return request<UploadResponse>(`/documents/${id}/reingest`, { method: 'POST' })
}

export function ask(question: string, topK: number): Promise<AskResponse> {
  return request<AskResponse>('/ask', {
    method: 'POST',
    body: JSON.stringify({ question, top_k: topK }),
  })
}

export function listUsers(): Promise<UserListResponse> {
  return request<UserListResponse>('/users')
}

export function createUser(username: string, password: string, role: Role): Promise<void> {
  return request<void>('/users', {
    method: 'POST',
    body: JSON.stringify({ username, password, role }),
  })
}

export function updateUser(id: number, data: { role?: Role; password?: string }): Promise<void> {
  return request<void>(`/users/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(data),
  })
}

export function deleteUser(id: number): Promise<void> {
  return request<void>(`/users/${id}`, { method: 'DELETE' })
}

export function listTenants(): Promise<TenantListResponse> {
  return request<TenantListResponse>('/tenants')
}

export function createTenant(data: {
  slug: string
  name: string
  admin_username: string
  admin_password: string
}): Promise<CreateTenantResponse> {
  return request<CreateTenantResponse>('/tenants', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export function resetTenantAdmin(slug: string, username: string, password: string): Promise<void> {
  return request<void>(`/tenants/${encodeURIComponent(slug)}/admin/reset`, {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

export interface AuditQuery {
  page?: number
  page_size?: number
  action?: string
  scope?: 'all' | ''
  from?: string
  to?: string
}

export function listAudit(query: AuditQuery = {}): Promise<AuditListResponse> {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== '' && value !== null) {
      params.set(key, String(value))
    }
  }
  const queryString = params.toString()
  return request<AuditListResponse>(`/audit${queryString ? `?${queryString}` : ''}`)
}
