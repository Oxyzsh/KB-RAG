export interface EmbeddingInfo {
  model: string
  dim: number
  chunk_size: number
}

export interface Stats {
  documents: number
  documents_failed: number
  chunks: number
  size_bytes: number
  tenant_size_bytes: number
  embedding: EmbeddingInfo
}

export type DocumentStatus = 'ok' | 'failed'
export type DocumentSource = 'upload' | 'external'

export interface DocumentItem {
  id: number
  title: string
  name: string
  status: DocumentStatus
  chunks: number
  source: DocumentSource
  error: string
  updated_at: string
}

export interface Citation {
  n: number
  source: string
  heading: string
  text: string
  score: number
}

export interface AskResponse {
  answer: string
  generated: boolean
  warning: string
  citations: Citation[]
}

export interface UploadResponse {
  document: DocumentItem
  outcome: 'ok' | 'failed' | 'skipped'
}

export type JobStatus = 'queued' | 'running' | 'done' | 'failed' | 'canceled'

export interface Job {
  id: number
  name: string
  action: string
  status: JobStatus
  stage: string
  chunks: number
  error: string
  created_at: string
  updated_at: string
}

export interface JobResponse {
  job: Job
}

export interface DocumentListResponse {
  documents: DocumentItem[]
}

export interface ErrorBody {
  error: string
}

export type Role = 'admin' | 'editor' | 'viewer'

export interface User {
  id: number
  username: string
  role: Role
  is_super: boolean
  tenant_id: number
  tenant_slug: string
  tenant_name: string
  home_tenant_id: number
  home_tenant_slug: string
  home_tenant_name: string
  can_switch: boolean
  created_at: string
  updated_at: string
}

export interface Tenant {
  id: number
  slug: string
  name: string
}

export interface LoginResponse {
  token: string
  expires_at: string
  user: User
}

export interface SessionResponse {
  user: User
}

export interface UserListResponse {
  users: User[]
}

export interface TenantListResponse {
  tenants: Tenant[]
}

export interface CreateTenantResponse {
  tenant: Tenant
  user: User
}

export interface AuditItem {
  id: number
  tenant_id: number | null
  tenant_slug: string
  user_id: number | null
  username: string
  action: string
  target: string
  detail: string
  ip: string
  created_at: string
}

export interface AuditListResponse {
  items: AuditItem[]
  total: number
  page: number
  page_size: number
}
