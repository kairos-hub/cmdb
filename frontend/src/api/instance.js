import request from './request'

export const searchInstances = (data) => request.post('/instances/search', data)
export const getInstance = (id) => request.get(`/instances/${id}`)
export const createInstance = (data) => request.post('/instances', data)
export const updateInstance = (id, data) => request.put(`/instances/${id}`, data)
export const deleteInstance = (id) => request.delete(`/instances/${id}`)
export const batchDeleteInstances = (ids) => request.post('/instances/batch-delete', { ids })

export const listRelations = (id) => request.get(`/instances/${id}/relations`)

// 主机资源监控时序（CPU/内存/磁盘/网络），range: 1h|6h|24h|7d
export const getHostMetrics = (id, range = '1h') =>
  request.get(`/instances/${id}/metrics`, { params: { range } })
export const createRelation = (data) => request.post('/instance-relations', data)
export const deleteRelation = (id) => request.delete(`/instance-relations/${id}`)

// 导入导出
export const exportInstances = (data) =>
  request.post('/instances/export', data, { responseType: 'blob' })
export const listDataSourceOptions = () => request.get('/data-sources/options')
export const exportAllResources = (sourceId) =>
  request.get('/export/all', {
    params: sourceId !== '' && sourceId !== null && sourceId !== undefined ? { source_id: sourceId } : {},
    responseType: 'blob',
  })
export const downloadTemplate = (modelId) =>
  request.post('/instances/template', { model_id: modelId }, { responseType: 'blob' })
export const importInstances = (formData) => request.post('/instances/import', formData)

// 全局搜索
export const globalSearch = (params) => request.get('/search', { params })
