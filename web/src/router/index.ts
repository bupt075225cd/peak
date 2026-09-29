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
      path: '/entry',
      name: 'entry',
      component: () => import('../views/MistakeEntry.vue'),
    },
    {
      path: '/list',
      name: 'list',
      component: () => import('../views/MistakeList.vue'),
    },
  ],
})

export default router
