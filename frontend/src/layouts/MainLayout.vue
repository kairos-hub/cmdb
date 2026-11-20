<template>
  <div class="app-shell" :class="{ collapsed }">
    <!-- 左侧深色导航栏 -->
    <aside class="sidebar">
      <div class="brand" @click="router.push('/')">
        <span class="brand-badge">C</span>
        <span class="brand-text">
          <b>天与 CMDB</b>
          <i>配置管理平台</i>
        </span>
      </div>

      <nav class="menu">
        <div v-for="group in navGroups" :key="group.label" class="menu-group">
          <div class="menu-group-label">{{ group.label }}</div>
          <el-tooltip
            v-for="item in group.items"
            :key="item.path"
            :content="item.title"
            placement="right"
            :disabled="!collapsed"
          >
            <RouterLink
              :to="item.path"
              class="menu-item"
              :class="{ active: activeItem && activeItem.path === item.path }"
            >
              <el-icon class="menu-icon"><component :is="item.icon || 'Menu'" /></el-icon>
              <span class="menu-text">{{ item.title }}</span>
            </RouterLink>
          </el-tooltip>
        </div>
      </nav>

      <div class="sidebar-foot">天与 CMDB</div>
    </aside>

    <!-- 右侧主区 -->
    <div class="main-area">
      <header class="topbar">
        <button class="collapse-btn" :title="collapsed ? '展开侧栏' : '收起侧栏'" @click="toggleSidebar">
          <el-icon><component :is="collapsed ? 'Expand' : 'Fold'" /></el-icon>
        </button>
        <div class="crumbs">
          <span class="crumb-group">{{ activeGroupLabel }}</span>
          <span class="crumb-sep">/</span>
          <span class="crumb-current">{{ currentTitle }}</span>
        </div>
        <div class="topbar-right">
          <div class="top-search">
            <el-icon class="ts-icon"><Search /></el-icon>
            <input
              ref="searchInput"
              v-model="kw"
              placeholder="请输入IP、实例、固资编号进行搜索"
              @keyup.enter="goSearch"
              @keyup.esc="onSearchEsc"
            />
            <el-icon v-if="kw" class="ts-clear" title="清除" @click="clearSearch"><Close /></el-icon>
            <span v-else class="ts-kbd">{{ kbdHint }}</span>
          </div>
          <el-tooltip :content="isDark ? '切换到浅色' : '切换到深色'" placement="bottom">
            <span class="theme-toggle" @click="toggleTheme">
              <el-icon><component :is="isDark ? 'Sunny' : 'Moon'" /></el-icon>
            </span>
          </el-tooltip>
          <el-dropdown @command="onCommand">
            <span class="user-trigger">
              <span class="avatar"><el-icon><User /></el-icon></span>
              <span class="user-name">{{ displayName }}</span>
              <el-icon class="user-caret"><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item disabled>{{ displayName }}</el-dropdown-item>
                <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>

      <main class="content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Search, Close, ArrowDown } from '@element-plus/icons-vue'
import { useUserStore } from '@/store/user'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

// 侧栏折叠（记忆到本机 localStorage）
const collapsed = ref(false)
try {
  collapsed.value = localStorage.getItem('cmdb_sidebar_collapsed') === '1'
} catch { /* 忽略 */ }
function toggleSidebar() {
  collapsed.value = !collapsed.value
  try { localStorage.setItem('cmdb_sidebar_collapsed', collapsed.value ? '1' : '0') } catch { /* 忽略 */ }
}

const kw = ref('')
const searchInput = ref(null)
const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || '')
const kbdHint = computed(() => (isMac ? '⌘K' : 'Ctrl K'))

function goSearch() {
  const keyword = kw.value.trim()
  if (!keyword) {
    router.push('/search')
    return
  }
  router.push({ path: '/search', query: { keyword, mode: 'fuzzy' } })
}
function clearSearch() {
  kw.value = ''
  searchInput.value?.focus()
}
function onSearchEsc() {
  kw.value = ''
  searchInput.value?.blur()
}
// 全局快捷键：⌘K / Ctrl+K 聚焦搜索框
function onGlobalKey(e) {
  if ((e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K')) {
    e.preventDefault()
    searchInput.value?.focus()
    searchInput.value?.select()
  }
}
onMounted(() => window.addEventListener('keydown', onGlobalKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onGlobalKey))

const hasFunc = (f) => userStore.hasFunc(f)

// 分组导航（按功能权限过滤；空组自动隐藏）
const navGroups = computed(() => {
  const groups = [
    {
      label: '概览',
      items: [
        { title: '全局搜索', path: '/dashboard', icon: 'Search', show: true },
        { title: '资产总览', path: '/overview', icon: 'DataAnalysis', show: hasFunc('stats_view') },
      ],
    },
    {
      label: '资源目录',
      items: [
        { title: '资源实例', path: '/instances', icon: 'Coin', show: true },
        { title: '应用服务', path: '/app-services', icon: 'Menu', show: true },
        { title: '业务拓扑', path: '/topology', icon: 'Share', show: hasFunc('topo_manage') || hasFunc('topo_view') },
      ],
    },
    {
      label: '模型与字段',
      items: [
        { title: '模型管理', path: '/models', icon: 'Grid', show: hasFunc('model_manage') || hasFunc('model_view') },
      ],
    },
    {
      label: '集成中心',
      items: [
        { title: '数据源', path: '/system/datasources', icon: 'Connection', show: hasFunc('datasource_view') },
        { title: '告警配置', path: '/system/alert-config', icon: 'AlarmClock', show: hasFunc('datasource_view') },
        { title: '告警事件', path: '/alerts', icon: 'Bell', show: true },
        { title: '系统集成', path: '/integrations', icon: 'Link', show: true },
      ],
    },
    {
      label: '系统管理',
      items: [
        { title: '用户管理', path: '/system/users', icon: 'User', show: hasFunc('user_view') },
        { title: '角色管理', path: '/system/roles', icon: 'Avatar', show: hasFunc('user_view') },
        { title: '审计日志', path: '/system/audit', icon: 'Document', show: hasFunc('audit_view') },
        { title: '系统设置', path: '/system/settings', icon: 'Setting', show: hasFunc('user_view') },
      ],
    },
  ]
  return groups
    .map((g) => ({ ...g, items: g.items.filter((i) => i.show) }))
    .filter((g) => g.items.length)
})

// 当前激活项（按路径前缀匹配，详情页归属其列表项）
const prefixOf = (path) => {
  const p = route.path
  if (path === '/dashboard') return p === '/' || p.startsWith('/dashboard') || p.startsWith('/search')
  if (path === '/instances') return p.startsWith('/instances')
  if (path === '/models') return p.startsWith('/models')
  if (path === '/topology') return p.startsWith('/topology')
  return p === path || p.startsWith(path + '/')
}
const activeItem = computed(() => {
  for (const g of navGroups.value) {
    for (const i of g.items) {
      if (prefixOf(i.path)) return i
    }
  }
  return null
})
const activeGroupLabel = computed(() => {
  for (const g of navGroups.value) {
    if (g.items.some((i) => activeItem.value && i.path === activeItem.value.path)) return g.label
  }
  return '概览'
})
const currentTitle = computed(() => route.meta?.title || activeItem.value?.title || '')

const displayName = computed(
  () => userStore.user?.display_name || userStore.user?.username || '用户',
)
const avatarChar = computed(() => (displayName.value || '用')[0])

// 深色/浅色切换
const isDark = ref(document.documentElement.classList.contains('dark'))
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  try { localStorage.setItem('cmdb_theme', isDark.value ? 'dark' : 'light') } catch { /* ignore */ }
}

function onCommand(cmd) {
  if (cmd === 'logout') {
    userStore.logout()
    router.push('/login')
  }
}
</script>

<style scoped>
.app-shell {
  display: flex;
  height: 100vh;
  overflow: hidden;
  background: var(--page-bg);
}

/* —— 侧栏 —— */
.sidebar {
  width: 192px;
  flex-shrink: 0;
  background: var(--nav-bg);
  box-shadow: inset -1px 0 var(--nav-border);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  transition: width 0.18s ease;
}

/* —— 折叠态 —— */
.collapsed .sidebar { width: 72px; }
.collapsed .brand { justify-content: center; padding: 20px 0 18px; }
.collapsed .brand-text { display: none; }
.collapsed .menu { padding-left: 10px; padding-right: 10px; }
.collapsed .menu-group { margin-top: 12px; }
.collapsed .menu-group-label { display: none; }
.collapsed .menu-item { justify-content: center; gap: 0; padding: 11px 0; }
.collapsed .menu-text { display: none; }
.collapsed .sidebar-foot { display: none; }
.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 20px 22px 18px;
  cursor: pointer;
}
.brand-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  background: linear-gradient(135deg, var(--brand), #4f7bf5);
  color: #fff;
  font-size: 20px;
  font-weight: 700;
  flex-shrink: 0;
}
.brand-text {
  display: flex;
  flex-direction: column;
  line-height: 1.25;
}
.brand-text b {
  color: var(--nav-heading);
  font-size: 17px;
  font-weight: 700;
  letter-spacing: 0.2px;
}
.brand-text i {
  color: var(--nav-text-dim);
  font-size: 12px;
  font-style: normal;
}

.menu {
  flex: 1;
  overflow-y: auto;
  padding: 6px 14px 20px;
}
.menu::-webkit-scrollbar { width: 6px; }
.menu::-webkit-scrollbar-thumb { background: var(--nav-scroll); }
.menu-group {
  margin-top: 18px;
}
.menu-group:first-child { margin-top: 6px; }
.menu-group-label {
  padding: 0 10px 8px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.5px;
  color: var(--nav-text-dim);
}
.menu-item {
  display: flex;
  align-items: center;
  gap: 11px;
  padding: 9px 12px;
  margin-bottom: 2px;
  border-radius: 8px;
  color: var(--nav-text);
  font-size: 14px;
  text-decoration: none;
  white-space: nowrap;
  overflow: hidden;
  transition: background 0.15s, color 0.15s;
}
.menu-icon {
  font-size: 17px;
  flex-shrink: 0;
}
.menu-text {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
}
.menu-item:hover {
  background: var(--nav-bg-soft);
  color: var(--nav-text-active);
}
.menu-item.active {
  background: var(--nav-active-bg);
  color: var(--nav-text-active);
  font-weight: 600;
  box-shadow: inset 3px 0 0 var(--nav-active-indicator);
}
.sidebar-foot {
  padding: 14px 24px;
  font-size: 12px;
  text-align: center;
  color: var(--nav-text-dim);
  border-top: 1px solid var(--nav-border);
}

/* —— 主区 —— */
.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.topbar {
  height: 60px;
  flex-shrink: 0;
  background: var(--card-bg);
  border-bottom: 1px solid var(--card-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 28px;
  gap: 14px;
}
.collapse-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--text-secondary);
  font-size: 18px;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}
.collapse-btn:hover {
  background: var(--fill-bg);
  color: var(--brand);
}
.crumbs {
  margin-right: auto;
}
.crumbs {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
}
.crumb-group { color: var(--text-secondary); }
.crumb-sep { color: var(--text-muted); }
.crumb-current { color: var(--text-primary); font-weight: 600; }

.topbar-right {
  display: flex;
  align-items: center;
  gap: 16px;
}
.top-search {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 340px;
  height: 38px;
  padding: 0 12px;
  border: 1px solid var(--border-base);
  border-radius: 10px;
  background: var(--fill-bg-soft);
  transition: border-color 0.15s, background 0.15s;
}
.top-search:focus-within {
  border-color: var(--brand);
  background: var(--card-bg);
}
.ts-icon {
  color: var(--text-muted);
  font-size: 16px;
}
.top-search input {
  flex: 1;
  border: none;
  outline: none;
  background: transparent;
  font-size: 13.5px;
  color: var(--text-primary);
}
.top-search input::placeholder { color: var(--text-muted); }
.ts-kbd {
  font-size: 12px;
  color: var(--text-muted);
  border: 1px solid var(--border-base);
  border-radius: 5px;
  padding: 1px 6px;
  background: var(--card-bg);
  white-space: nowrap;
}
.ts-clear {
  color: var(--text-muted);
  cursor: pointer;
  font-size: 15px;
}
.ts-clear:hover {
  color: var(--text-regular);
}
.user-trigger {
  display: flex;
  align-items: center;
  gap: 9px;
  cursor: pointer;
  outline: none;
  padding: 3px 10px 3px 3px;
  border-radius: 10px;
  transition: background 0.15s;
}
.user-trigger:hover {
  background: var(--fill-bg);
}
.theme-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: 9px;
  cursor: pointer;
  color: var(--text-secondary);
  font-size: 17px;
  transition: background 0.15s, color 0.15s;
}
.theme-toggle:hover { background: var(--fill-bg); color: var(--brand); }
.avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--brand), #5b86f5);
  color: #fff;
  font-size: 17px;
  flex-shrink: 0;
}
.user-name {
  font-size: 14px;
  color: var(--text-primary);
  font-weight: 500;
  white-space: nowrap;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.user-caret {
  font-size: 13px;
  color: var(--text-muted);
  flex-shrink: 0;
}

.content {
  flex: 1;
  overflow-y: auto;
  padding: 24px 28px;
  scrollbar-gutter: stable;
}

html:not(.dark) .topbar {
  background:
    linear-gradient(90deg, #b8d2f0, #b0dfe9 34%, var(--card-border) 75%) left bottom / 100% 1px no-repeat,
    linear-gradient(110deg, #f3f8ff, var(--card-bg) 52%);
  border-bottom-color: transparent;
  background-origin: border-box;
}
html:not(.dark) .content {
  background:
    radial-gradient(ellipse 720px 360px at 96% 0, #dfeefa 0, transparent 100%),
    var(--page-bg);
}

html.dark .sidebar {
  background:
    radial-gradient(190px 135px at 30% 0, #1a304c 0, transparent 100%),
    var(--nav-bg);
}
html.dark .brand-badge {
  background: linear-gradient(145deg, #46c3e8, #3769d0);
  color: #071a30;
  box-shadow: inset 0 1px 0 #d5f8ff70, 0 3px 14px #36aaf04a;
}
html.dark .menu-item.active {
  background: linear-gradient(105deg, var(--nav-active-bg), var(--nav-bg-soft));
  box-shadow: inset 2px 0 var(--tech-accent), inset 0 0 0 1px #2e537666;
}
html.dark .menu-item.active .menu-icon { color: var(--tech-accent); }
html.dark .topbar {
  background:
    linear-gradient(90deg, #2668a8, #60c9ee 28%, #29435e 62%) left bottom / 100% 1px no-repeat,
    linear-gradient(110deg, #192a42, var(--card-bg) 46%);
  border-bottom-color: transparent;
  background-origin: border-box;
}
html.dark .content {
  background:
    radial-gradient(ellipse 560px 280px at 96% 0, #142b46 0, transparent 100%),
    var(--page-bg);
}

@media (max-width: 900px) {
  .sidebar { width: 72px; }
  .brand-text, .menu-group-label, .sidebar-foot { display: none; }
  .menu-item { text-align: center; font-size: 12px; padding: 10px 4px; }
  .top-search { width: 180px; }
}
</style>
