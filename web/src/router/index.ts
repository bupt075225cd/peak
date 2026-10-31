import { createRouter, createWebHistory } from 'vue-router'
import { TOKEN_KEY } from '../composables/useAuth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      redirect: '/home',
    },
    {
      path: '/home',
      name: 'home',
      component: () => import('../views/Home.vue'),
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/Login.vue'),
    },
    {
      path: '/register',
      name: 'register',
      component: () => import('../views/Register.vue'),
    },
    {
      path: '/forgot-password',
      name: 'forgot-password',
      component: () => import('../views/ForgotPassword.vue'),
    },
    {
      path: '/entry',
      name: 'entry',
      component: () => import('../views/MistakeEntry.vue'),
    },
    {
      path: '/list',
      name: 'list',
      component: () => import('../views/MistakeList.vue'),
    },
    {
      path: '/poetry',
      name: 'poetry-dashboard',
      component: () => import('../views/PoetryDashboard.vue'),
    },
    {
      path: '/poetry/list',
      name: 'poetry-list',
      component: () => import('../views/PoetryList.vue'),
    },
    {
      path: '/poetry/detail/:id',
      name: 'poetry-detail',
      component: () => import('../views/PoetryDetail.vue'),
    },
    // 家庭资产账本：全部为私有数据，未登录统一引导登录。
    {
      path: '/ledger',
      name: 'ledger-dashboard',
      component: () => import('../views/LedgerDashboard.vue'),
      meta: { requiresAuth: true },
    },
    {
      path: '/ledger/items',
      name: 'ledger-items',
      component: () => import('../views/LedgerItems.vue'),
      meta: { requiresAuth: true },
    },
    {
      path: '/ledger/composition',
      name: 'ledger-composition',
      component: () => import('../views/LedgerComposition.vue'),
      meta: { requiresAuth: true },
    },
    {
      path: '/ledger/update',
      name: 'ledger-update',
      component: () => import('../views/LedgerUpdate.vue'),
      meta: { requiresAuth: true },
    },
  ],
})

// 仅拦 requiresAuth 路由，不改动错题本/古诗文现有导航行为。
router.beforeEach((to) => {
  if (to.meta.requiresAuth && !localStorage.getItem(TOKEN_KEY)) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  return true
})

export default router
