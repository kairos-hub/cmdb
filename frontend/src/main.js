import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus, { ElMessage } from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn' // 中文语言包（分页"共X条/条页/前往"等文案）
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css' // EP 深色模式变量（配合 html.dark）
import './styles/tokens.css' // 全局设计 token（在 EP 样式后引入以覆盖主色）
import './styles/global.css' // 全站通用观感（卡片/页头/表格/按钮精修）
import * as ElementPlusIconsVue from '@element-plus/icons-vue'

import App from './App.vue'
import router from './router'

// 统一全局提示样式：出现在顶栏下方（避免遮挡菜单）、停留更短、合并重复
const MSG_DEFAULTS = { offset: 64, duration: 2000, grouping: true }
for (const type of ['success', 'warning', 'error', 'info']) {
  const orig = ElMessage[type]
  ElMessage[type] = (opts) =>
    orig(typeof opts === 'string' ? { message: opts, ...MSG_DEFAULTS } : { ...MSG_DEFAULTS, ...opts })
}

// 启动时应用已保存的主题（深色/浅色）
try {
  if (localStorage.getItem('cmdb_theme') === 'dark') {
    document.documentElement.classList.add('dark')
  }
} catch { /* ignore */ }

const app = createApp(App)

// 全量注册 Element Plus 图标
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(createPinia())
app.use(router)
app.use(ElementPlus, { locale: zhCn })
app.mount('#app')
