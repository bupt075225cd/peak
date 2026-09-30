import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { initMonitor } from './monitor'
import './style.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)

// 前端监控：Web Vitals / 错误捕获 / PV·UV·路由与停留埋点 / 批量上报。
// 采样率：错误全采，行为与性能 10%（避免上报流量放大）。
initMonitor({ router, behaviorSampleRate: 0.1 })

app.mount('#app')
