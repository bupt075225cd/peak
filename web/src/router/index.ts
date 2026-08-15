import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      redirect: '/entry',
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
