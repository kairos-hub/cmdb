import request from './request'

// 字段类型 -> 中文名
export const FIELD_TYPES = [
  { value: 'singlechar', label: '单行文本' },
  { value: 'longchar', label: '长文本' },
  { value: 'int', label: '整数' },
  { value: 'float', label: '浮点数' },
  { value: 'bool', label: '布尔' },
  { value: 'enum', label: '枚举(单选)' },
  { value: 'enummulti', label: '枚举(多选)' },
  { value: 'date', label: '日期' },
  { value: 'datetime', label: '日期时间' },
  { value: 'user', label: '用户' },
  { value: 'list', label: '列表' },
]

export const fieldTypeLabel = (t) => FIELD_TYPES.find((i) => i.value === t)?.label || t

export const MAPPINGS = [
  { value: '1:1', label: '一对一 (1:1)' },
  { value: '1:n', label: '一对多 (1:n)' },
  { value: 'n:n', label: '多对多 (n:n)' },
]

// 分组
export const listGroups = () => request.get('/model-groups')
export const createGroup = (data) => request.post('/model-groups', data)
export const updateGroup = (id, data) => request.put(`/model-groups/${id}`, data)
export const deleteGroup = (id) => request.delete(`/model-groups/${id}`)

// 模型
export const listModels = (groupId) =>
  request.get('/models', { params: groupId ? { group_id: groupId } : {} })
export const getModel = (id) => request.get(`/models/${id}`)
export const createModel = (data) => request.post('/models', data)
export const updateModel = (id, data) => request.put(`/models/${id}`, data)
export const deleteModel = (id) => request.delete(`/models/${id}`)
export const reorderModels = (groupId, ids) =>
  request.put(`/model-groups/${groupId}/models/reorder`, { ids })

// 字段
export const listAttributes = (modelId) => request.get(`/models/${modelId}/attributes`)
export const createAttribute = (modelId, data) =>
  request.post(`/models/${modelId}/attributes`, data)
export const updateAttribute = (aid, data) => request.put(`/attributes/${aid}`, data)
export const deleteAttribute = (aid) => request.delete(`/attributes/${aid}`)
export const reorderAttributes = (modelId, ids) =>
  request.put(`/models/${modelId}/attributes/reorder`, { ids })

// 关联类型与模型关联
export const listKinds = () => request.get('/association-kinds')
export const listAssociations = () => request.get('/model-associations')
export const createAssociation = (data) => request.post('/model-associations', data)
export const deleteAssociation = (id) => request.delete(`/model-associations/${id}`)
