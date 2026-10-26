import { reactive } from 'vue'
import { getSession } from './api/client'
import type { User } from './api/types'

export const session = reactive<{ user: User | null }>({ user: null })

export async function loadSession(): Promise<User | null> {
  if (session.user) {
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
}

export function isAdmin(): boolean {
  return session.user?.role === 'admin'
}

export function canEditDocuments(): boolean {
  return session.user?.role === 'admin' || session.user?.role === 'editor'
}
