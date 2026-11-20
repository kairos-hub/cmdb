import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '@/store/user'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { public: true },
  },
  {
    path: '/',
    component: () => import('@/layouts/MainLayout.vue'),
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('@/views/Dashboard.vue'),
        meta: { title: '首页' },
      },
      {
        path: 'overview',
        name: 'Overview',
        component: () => import('@/views/Overview.vue'),
        meta: { title: '资产总览', func: 'stats_view' },
      },
      {
        path: 'search',
        name: 'SearchView',
        component: () => import('@/views/SearchView.vue'),
        meta: { title: '全局搜索' },
      },
      {
        path: 'models',
        name: 'ModelList',
        component: () => import('@/views/model/ModelList.vue'),
        meta: { title: '模型管理', func: ['model_manage', 'model_view'] },
      },
      {
        path: 'models/:id',
        name: 'ModelDetail',
        component: () => import('@/views/model/ModelDetail.vue'),
        meta: { title: '模型详情', func: ['model_manage', 'model_view'] },
      },
      {
        path: 'instances',
        name: 'InstanceList',
        component: () => import('@/views/instance/InstanceList.vue'),
        meta: { title: '资源实例' },
      },
      {
        path: 'instances/:id',
        name: 'InstanceDetail',
        component: () => import('@/views/instance/InstanceDetail.vue'),
        meta: { title: '实例详情' },
      },
      {
        path: 'app-services',
        name: 'AppServices',
        component: () => import('@/views/app/AppServiceView.vue'),
        meta: { title: '应用服务' },
      },
      {
        path: 'topology',
        name: 'Topology',
        component: () => import('@/views/topo/TopologyView.vue'),
        meta: { title: '业务拓扑', func: ['topo_manage', 'topo_view'] },
      },
      {
        path: 'alerts',
        name: 'Alerts',
        component: () => import('@/views/Alerts.vue'),
        meta: { title: '告警事件' },
      },
      {
        path: 'integrations',
        name: 'Integrations',
        component: () => import('@/views/Integrations.vue'),
        meta: { title: '系统集成' },
      },
      {
        path: 'integrations/:id',
        name: 'IntegrationFrame',
        component: () => import('@/views/IntegrationFrame.vue'),
        meta: { title: '系统集成' },
      },
      {
        path: 'system/users',
        name: 'UserManage',
        component: () => import('@/views/system/UserManage.vue'),
        meta: { title: '用户管理', func: 'user_view' },
      },
      {
        path: 'system/roles',
        name: 'RoleManage',
        component: () => import('@/views/system/RoleManage.vue'),
        meta: { title: '角色管理', func: 'user_view' },
      },
      {
        path: 'system/audit',
        name: 'AuditLog',
        component: () => import('@/views/system/AuditLog.vue'),
        meta: { title: '审计日志', func: 'audit_view' },
      },
      {
        path: 'system/datasources',
        name: 'DataSourceManage',
        component: () => import('@/views/system/DataSourceManage.vue'),
        meta: { title: '数据源', func: 'datasource_view' },
      },
      {
        path: 'system/alert-config',
        name: 'AlertConfig',
        component: () => import('@/views/system/AlertConfig.vue'),
        meta: { title: '告警配置', func: 'datasource_view' },
      },
      {
        path: 'system/settings',
        name: 'SettingView',
        component: () => import('@/views/system/SettingView.vue'),
        meta: { title: '系统设置', func: 'user_view' },
      },
    ],
  },
  {
    // 集群拓扑大图：独立全页（新标签打开），无侧栏
    path: '/app-topo',
    name: 'AppTopology',
    component: () => import('@/views/app/AppTopoView.vue'),
    meta: { title: '集群拓扑' },
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'NotFound',
    component: () => import('@/views/NotFound.vue'),
    meta: { public: true },
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// 全局前置守卫：未登录跳转登录页
router.beforeEach((to) => {
  const userStore = useUserStore()
  if (!to.meta.public && !userStore.token) {
    return { name: 'Login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'Login' && userStore.token) {
    return { path: '/' }
  }
  // 功能权限守卫：无对应功能权限的页面，直接输 URL 也拦回首页。
  // meta.func 可为字符串或数组（数组=满足任一即可，用于「管理 或 只读查看」）。
  if (to.meta.func) {
    const need = Array.isArray(to.meta.func) ? to.meta.func : [to.meta.func]
    if (!need.some((f) => userStore.hasFunc(f))) {
      return { path: '/dashboard' }
    }
  }
  return true
})

export default router
