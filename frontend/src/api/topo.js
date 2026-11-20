import request from './request'

// 业务
export const listBusiness = () => request.get('/topo/business')
export const createBiz = (data) => request.post('/topo/business', data)
export const updateBiz = (id, data) => request.put(`/topo/business/${id}`, data)
export const deleteBiz = (id) => request.delete(`/topo/business/${id}`)
export const getTree = (bizId) => request.get(`/topo/business/${bizId}/tree`)
export const getSummary = (bizId) => request.get(`/topo/business/${bizId}/summary`)

// 导入导出
export const exportTopo = (bizId) =>
  request.get('/topo/export', { params: { biz_id: bizId }, responseType: 'blob' })
export const templateTopo = () => request.get('/topo/template', { responseType: 'blob' })
export const importTopo = (formData) => request.post('/topo/import', formData)

// 业务（业务线下一级，每个业务一棵独立拓扑）
export const createBusiness = (bizId, name) => request.post('/topo/businesses', { biz_id: bizId, name })
export const updateBusiness = (id, name) => request.put(`/topo/businesses/${id}`, { name })
export const deleteBusiness = (id) => request.delete(`/topo/businesses/${id}`)

// 集群 / 模块
export const createSet = (businessId, name) => request.post('/topo/sets', { business_id: businessId, name })
export const updateSet = (id, name) => request.put(`/topo/sets/${id}`, { name })
export const deleteSet = (id) => request.delete(`/topo/sets/${id}`)
export const createModule = (businessId, modelId, name, posX = 0, posY = 0) =>
  request.post('/topo/modules', { business_id: businessId, model_id: modelId, name, pos_x: posX, pos_y: posY })
export const updateModule = (id, name) => request.put(`/topo/modules/${id}`, { name })
export const updateModulePosition = (id, posX, posY) =>
  request.put(`/topo/modules/${id}/position`, { pos_x: Math.round(posX), pos_y: Math.round(posY) })
export const deleteModule = (id) => request.delete(`/topo/modules/${id}`)

// 连线（用户自建的模块间有向连接）
export const listEdges = (businessId) => request.get(`/topo/businesses/${businessId}/edges`)
export const createEdge = (businessId, src, dst) =>
  request.post('/topo/edges', { business_id: businessId, src_module_id: src, dst_module_id: dst })
export const deleteEdge = (id) => request.delete(`/topo/edges/${id}`)
export const moduleHosts = (id) => request.get(`/topo/modules/${id}/hosts`)

// 资源归属（模块级，任意资源类型）
export const idleHosts = (params) => request.get('/topo/idle-hosts', { params })
export const assignHosts = (moduleId, hostIds) =>
  request.post('/topo/assign', { module_id: moduleId, host_ids: hostIds })
export const transferHosts = (targetModuleId, hostIds) =>
  request.post('/topo/transfer', { target_module_id: targetModuleId, host_ids: hostIds })
export const recycleHosts = (hostIds) => request.post('/topo/recycle', { host_ids: hostIds })
