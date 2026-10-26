import { createRouter, createWebHashHistory } from 'vue-router'
import { getToken } from '../api/client'
import { loadSession } from '../session'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/login', component: () => import('../views/LoginView.vue') },
    {
      path: '/',
      component: () => import('../components/AppShell.vue'),
      children: [
        { path: '', redirect: '/chat' },
        { path: 'chat', component: () => import('../views/ChatView.vue') },
        { path: 'documents', component: () => import('../views/DocumentsView.vue') },
        {
          path: 'admin',
          component: () => import('../views/AdminView.vue'),
          meta: { requiresAdmin: true },
        },
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
