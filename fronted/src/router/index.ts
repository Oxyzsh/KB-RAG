import { createRouter, createWebHashHistory } from 'vue-router'
import AppShell from '../components/AppShell.vue'
import ChatView from '../views/ChatView.vue'
import DocumentsView from '../views/DocumentsView.vue'
import LoginView from '../views/LoginView.vue'
import { getToken } from '../api/client'

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
      ],
    },
  ],
})

router.beforeEach((to) => {
  const token = getToken()
  if (to.path !== '/login' && !token) {
    return '/login'
  }
  if (to.path === '/login' && token) {
    return '/chat'
  }
  return true
})

export default router
