import type {
  AskResponse,
  AuditListResponse,
  Citation,
  CreateTenantResponse,
  DocumentListResponse,
  DocumentPreview,
  ErrorBody,
  JobResponse,
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

export function getDocumentPreview(id: number): Promise<DocumentPreview> {
  return request<DocumentPreview>(`/documents/${id}/preview`)
}

export async function fetchDocumentRaw(id: number): Promise<Blob> {
  const headers = new Headers()
  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
    const tenant = getTenantOverride()
    if (tenant) {
      headers.set('X-Tenant', tenant)
    }
  }
  const res = await fetch(`/api/documents/${id}/raw`, { headers })
  if (!res.ok) {
    let message = `请求失败（HTTP ${res.status}）`
    try {
      const data = (await res.json()) as ErrorBody
      if (data?.error) {
        message = data.error
      }
    } catch {
      // 保持默认错误信息
    }
    if (res.status === 401) {
      clearToken()
      clearTenantOverride()
      if (!window.location.hash.startsWith('#/login')) {
        window.location.hash = '#/login'
      }
    }
    throw new ApiError(res.status, message)
  }
  return res.blob()
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

export function uploadDocumentJob(file: File): Promise<JobResponse> {
  const form = new FormData()
  form.append('file', file)
  return request<JobResponse>('/jobs/documents', { method: 'POST', body: form })
}

export function reingestDocumentJob(id: number): Promise<JobResponse> {
  return request<JobResponse>(`/jobs/documents/${id}/reingest`, { method: 'POST' })
}

export function getJob(id: number): Promise<JobResponse> {
  return request<JobResponse>(`/jobs/${id}`)
}

export function cancelJob(id: number): Promise<void> {
  return request<void>(`/jobs/${id}`, { method: 'DELETE' })
}

export function ask(question: string, topK: number): Promise<AskResponse> {
  return request<AskResponse>('/ask', {
    method: 'POST',
    body: JSON.stringify({ question, top_k: topK }),
  })
}

export interface AskStreamHandlers {
  onCitations?: (citations: Citation[]) => void
  onDelta?: (text: string) => void
  onDone?: (answer: string) => void
  onError?: (message: string) => void
}

export async function askStream(
  question: string,
  topK: number,
  handlers: AskStreamHandlers,
  signal?: AbortSignal,
): Promise<void> {
  const headers = new Headers()
  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
    const tenant = getTenantOverride()
    if (tenant) {
      headers.set('X-Tenant', tenant)
    }
  }
  headers.set('Content-Type', 'application/json')

  const res = await fetch('/api/ask/stream', {
    method: 'POST',
    headers,
    body: JSON.stringify({ question, top_k: topK }),
    signal,
  })
  if (!res.ok) {
    const text = await res.text()
    let data: unknown = null
    try {
      data = JSON.parse(text)
    } catch {
      data = null
    }
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
  if (!res.body) {
    throw new ApiError(502, '流式响应不可用')
  }

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  const dispatch = (raw: string) => {
    let event = 'message'
    const dataLines: string[] = []
    for (const line of raw.split('\n')) {
      if (line.startsWith('event:')) {
        event = line.slice(6).trim()
      } else if (line.startsWith('data:')) {
        dataLines.push(line.slice(5).trim())
      }
    }
    if (!dataLines.length) {
      return
    }
    let payload: unknown
    try {
      payload = JSON.parse(dataLines.join('\n'))
    } catch {
      return
    }
    switch (event) {
      case 'citations':
        handlers.onCitations?.(payload as Citation[])
        break
      case 'delta':
        handlers.onDelta?.((payload as { text: string }).text)
        break
      case 'done':
        handlers.onDone?.((payload as { answer: string }).answer)
        break
      case 'error':
        handlers.onError?.((payload as { error: string }).error)
        break
    }
  }

  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) {
        break
      }
      buffer += decoder.decode(value, { stream: true })
      let index = buffer.indexOf('\n\n')
      while (index >= 0) {
        const raw = buffer.slice(0, index)
        buffer = buffer.slice(index + 2)
        dispatch(raw)
        index = buffer.indexOf('\n\n')
      }
    }
    buffer += decoder.decode()
    if (buffer.trim()) {
      dispatch(buffer)
    }
  } finally {
    reader.releaseLock()
  }
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
