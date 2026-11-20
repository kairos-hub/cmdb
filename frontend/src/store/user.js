import { defineStore } from 'pinia'
import { ref } from 'vue'
import { login as loginApi, getMe } from '@/api/auth'
import { getMyPermissions } from '@/api/system'

export const useUserStore = defineStore('user', () => {
  const token = ref(localStorage.getItem('cmdb_token') || '')
  const user = ref(JSON.parse(localStorage.getItem('cmdb_user') || 'null'))
  const isAdmin = ref(localStorage.getItem('cmdb_is_admin') === '1')
  const permissions = ref(JSON.parse(localStorage.getItem('cmdb_perms') || '{}'))
  const functions = ref(JSON.parse(localStorage.getItem('cmdb_funcs') || '[]'))

  function persist() {
    localStorage.setItem('cmdb_token', token.value)
    localStorage.setItem('cmdb_user', JSON.stringify(user.value))
    localStorage.setItem('cmdb_is_admin', isAdmin.value ? '1' : '0')
    localStorage.setItem('cmdb_perms', JSON.stringify(permissions.value))
    localStorage.setItem('cmdb_funcs', JSON.stringify(functions.value))
  }

  async function fetchPermissions() {
    try {
      const p = await getMyPermissions()
      isAdmin.value = !!p.is_admin
      permissions.value = p.permissions || {}
      functions.value = p.functions || []
    } catch {
      isAdmin.value = false
      permissions.value = {}
      functions.value = []
    }
    persist()
  }

  // 是否拥有某功能权限（超管恒为真）
  function hasFunc(code) {
    return isAdmin.value || functions.value.includes(code)
  }

  // 是否对某模型拥有某操作权限（超管恒为真；model_id=0 为全模型通配）
  function hasPerm(modelId, action) {
    if (isAdmin.value) return true
    const wildcard = permissions.value['0']
    if (wildcard && wildcard.includes(action)) return true
    const p = permissions.value[String(modelId)]
    return !!(p && p.includes(action))
  }

  async function login(username, password) {
    const data = await loginApi({ username, password })
    token.value = data.token
    user.value = data.user
    persist()
    await fetchPermissions()
    return data
  }

  async function fetchMe() {
    const me = await getMe()
    user.value = me
    persist()
    await fetchPermissions()
    return me
  }

  // 活跃续期：后端下发的新令牌，静默更新并持久化
  function setToken(t) {
    if (!t || t === token.value) return
    token.value = t
    try { localStorage.setItem('cmdb_token', t) } catch {}
  }

  function logout() {
    token.value = ''
    user.value = null
    isAdmin.value = false
    permissions.value = {}
    functions.value = []
    localStorage.removeItem('cmdb_token')
    localStorage.removeItem('cmdb_user')
    localStorage.removeItem('cmdb_is_admin')
    localStorage.removeItem('cmdb_perms')
    localStorage.removeItem('cmdb_funcs')
  }

  return { token, user, isAdmin, permissions, functions, hasPerm, hasFunc, login, fetchMe, fetchPermissions, setToken, logout }
})
