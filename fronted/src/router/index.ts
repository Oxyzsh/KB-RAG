import { createRouter, createWebHashHistory } from 'vue-router'
import AppShell from '../components/AppShell.vue'
import AdminView from '../views/AdminView.vue'
import ChatView from '../views/ChatView.vue'
import DocumentsView from '../views/DocumentsView.vue'
import LoginView from '../views/LoginView.vue'
import { getToken } from '../api/client'
import { loadSession } from '../session'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/login', component: LoginView },
    {
      path: '/',
      component: AppShell,
      children: [
        { path: '', redirect: '/chat' },
        { path: 'chat', component: ChatView },
        { path: 'documents', component: DocumentsView },
        { path: 'admin', component: AdminView, meta: { requiresAdmin: true } },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const token = getToken()
  if (to.path !== '/login' && !token) {
    return '/login'
  }
  if (to.path === '/login' && token) {
    return '/chat'
  }
  if (to.meta.requiresAdmin) {
    try {
      const user = await loadSession()
      if (user?.role !== 'admin') {
        return '/chat'
      }
    } catch {
      return '/login'
    }
  }
  return true
})

export default router
