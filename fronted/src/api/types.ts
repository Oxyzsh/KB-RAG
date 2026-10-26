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

export interface DocumentListResponse {
  documents: DocumentItem[]
}

export interface ErrorBody {
  error: string
}
