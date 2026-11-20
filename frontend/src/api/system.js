import request from './request'

// 当前用户权限
export const getMyPermissions = () => request.get('/auth/permissions')

// 用户管理
export const listUsers = () => request.get('/users')
export const createUser = (data) => request.post('/users', data)
export const updateUser = (id, data) => request.put(`/users/${id}`, data)
export const deleteUser = (id) => request.delete(`/users/${id}`)
export const changePassword = (id, password) => request.put(`/users/${id}/password`, { password })
export const getUserRoles = (id) => request.get(`/users/${id}/roles`)
export const setUserRoles = (id, roleIds) => request.put(`/users/${id}/roles`, { role_ids: roleIds })

// 角色管理
export const listRoles = () => request.get('/roles')
export const createRole = (data) => request.post('/roles', data)
export const updateRole = (id, data) => request.put(`/roles/${id}`, data)
export const deleteRole = (id) => request.delete(`/roles/${id}`)
export const getRolePermissions = (id) => request.get(`/roles/${id}/permissions`)
export const setRolePermissions = (id, permissions) =>
  request.put(`/roles/${id}/permissions`, { permissions })

// 审计日志
export const listAudit = (params) => request.get('/audit', { params })
// 清理审计日志：days<=0 删除全部，否则删除 days 天前的记录
export const cleanupAudit = (days) => request.delete('/audit', { params: { days } })

// 全局设置
export const getSettings = () => request.get('/settings')
export const updateSettings = (data) => request.put('/settings', data)

// 权限操作项
export const ACTIONS = [
  { value: 'view', label: '查看' },
  { value: 'create', label: '新增' },
  { value: 'update', label: '编辑' },
  { value: 'delete', label: '删除' },
]

// 功能权限按模块划分「只读 / 编辑」两档，需与后端 model.AllFunctions / FuncImplies 保持一致。
// view=只读功能码；manage=编辑功能码（null 表示该模块本身只读、无编辑档）。编辑自动含只读。
export const FUNC_MODULES = [
  { key: 'model', label: '模型管理', desc: '模型分组、模型、字段、关联', view: 'model_view', manage: 'model_manage' },
  { key: 'topo', label: '业务拓扑', desc: '业务、集群、模块及资源分配', view: 'topo_view', manage: 'topo_manage' },
  { key: 'datasource', label: '数据源', desc: 'Prometheus / 云等数据源配置与同步', view: 'datasource_view', manage: 'datasource_manage' },
  { key: 'user', label: '用户与角色', desc: '用户、角色、权限分配', view: 'user_view', manage: 'user_manage' },
  { key: 'stats', label: '资产总览', desc: '资产总览统计看板', view: 'stats_view', manage: null },
  { key: 'audit', label: '审计日志', desc: '操作审计日志', view: 'audit_view', manage: null },
]

// 功能码 → 可读标签（用于角色列表展示）。
export const FUNC_LABELS = FUNC_MODULES.reduce((m, mod) => {
  m[mod.view] = `${mod.label}·只读`
  if (mod.manage) m[mod.manage] = `${mod.label}·编辑`
  return m
}, {})

// 数据源（Prometheus 等）
export const listDataSources = () => request.get('/data-sources')
export const createDataSource = (data) => request.post('/data-sources', data)
export const updateDataSource = (id, data) => request.put(`/data-sources/${id}`, data)
export const deleteDataSource = (id) => request.delete(`/data-sources/${id}`)
// 同步可能跨多地域、多云产品，耗时较长，单独放宽超时到 5 分钟
export const syncDataSource = (id) =>
  request.post(`/data-sources/${id}/sync`, null, { timeout: 300000 })
export const testDataSource = (data) => request.post('/data-sources/test', data)
export const reorderDataSources = (ids) => request.put('/data-sources/reorder', { ids })

// 告警源（与数据源解耦：阿里云/腾讯云拉取、webhook 回调）
export const listAlertSources = () => request.get('/alert-sources')
export const listAlertSourceOptions = () => request.get('/alert-sources/options')
export const listAlertProducts = () => request.get('/alerts/products')
export const createAlertSource = (data) => request.post('/alert-sources', data)
export const updateAlertSource = (id, data) => request.put(`/alert-sources/${id}`, data)
export const deleteAlertSource = (id) => request.delete(`/alert-sources/${id}`)
export const testAlertSource = (data) => request.post('/alert-sources/test', data)
export const reorderAlertSources = (ids) => request.put('/alert-sources/reorder', { ids })
export const syncAlertSource = (id) => request.post(`/alert-sources/${id}/sync`, null, { timeout: 300000 })

// 系统集成（外部 Web 服务入口）
export const listIntegrations = () => request.get('/integrations')
export const createIntegration = (data) => request.post('/integrations', data)
export const updateIntegration = (id, data) => request.put(`/integrations/${id}`, data)
export const deleteIntegration = (id) => request.delete(`/integrations/${id}`)
export const reorderIntegrations = (ids) => request.put('/integrations/reorder', { ids })

// 告警数据
export const listAlerts = (params) => request.get('/alerts', { params })
export const cleanupAlerts = (days, source) => request.delete('/alerts', { params: { days, source } })
