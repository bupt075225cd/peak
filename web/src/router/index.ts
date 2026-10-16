import { createRouter, createWebHistory } from 'vue-router'

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
  ],
})

export default router
