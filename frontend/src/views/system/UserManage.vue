<template>
  <div>
    <div class="page-header">
      <div>
        <div class="ph-title">用户管理</div>
        <div class="ph-sub">管理平台账号、状态与角色分配</div>
      </div>
      <div class="ph-actions">
        <el-tag v-if="!canManage" type="info" size="small">只读模式：无「用户与角色·编辑」权限</el-tag>
        <el-button v-if="canManage" type="primary" :icon="Plus" @click="openCreate">新建用户</el-button>
      </div>
    </div>
    <div class="panel panel-pad">
    <el-table :data="users" v-loading="loading">
      <el-table-column label="ID" prop="id" width="64" />
      <el-table-column label="用户名" min-width="140">
        <template #default="{ row }"><span class="u-name">{{ row.username }}</span></template>
      </el-table-column>
      <el-table-column label="显示名" prop="display_name" min-width="120" />
      <el-table-column label="邮箱" prop="email" min-width="180" show-overflow-tooltip />
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <span class="soft-tag" :class="row.status === 1 ? 'is-success' : 'is-muted'"><span class="dot"></span>{{ row.status === 1 ? '启用' : '禁用' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="240">
        <template #default="{ row }">
          <template v-if="canManage">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="primary" @click="openPassword(row)">改密</el-button>
            <el-button link type="primary" @click="openRoles(row)">角色</el-button>
            <el-button link type="danger" :disabled="row.username === 'admin'" @click="remove(row)">删除</el-button>
          </template>
          <span v-else class="muted">—</span>
        </template>
      </el-table-column>
    </el-table>
    </div>

    <!-- 新建/编辑 -->
    <el-dialog v-model="formDialog" :title="editing ? '编辑用户' : '新建用户'" width="460px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="用户名"><el-input v-model="form.username" :disabled="editing" /></el-form-item>
        <el-form-item v-if="!editing" label="密码"><el-input v-model="form.password" type="password" show-password /></el-form-item>
        <el-form-item label="显示名"><el-input v-model="form.display_name" /></el-form-item>
        <el-form-item label="邮箱"><el-input v-model="form.email" /></el-form-item>
        <el-form-item v-if="editing" label="状态">
          <el-switch v-model="form.status" :active-value="1" :inactive-value="0" active-text="启用" inactive-text="禁用" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="formDialog = false">取消</el-button>
        <el-button type="primary" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>

    <!-- 改密 -->
    <el-dialog v-model="pwdDialog" title="修改密码" width="420px">
      <el-form label-width="90px">
        <el-form-item label="新密码"><el-input v-model="newPassword" type="password" show-password /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="pwdDialog = false">取消</el-button>
        <el-button type="primary" @click="submitPassword">确定</el-button>
      </template>
    </el-dialog>

    <!-- 角色分配 -->
    <el-dialog v-model="roleDialog" title="分配角色" width="420px">
      <el-checkbox-group v-model="selectedRoleIds">
        <el-checkbox v-for="r in roles" :key="r.id" :value="r.id" :label="r.id">{{ r.name }}</el-checkbox>
      </el-checkbox-group>
      <template #footer>
        <el-button @click="roleDialog = false">取消</el-button>
        <el-button type="primary" @click="submitRoles">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import * as api from '@/api/system'
import { useUserStore } from '@/store/user'

const userStore = useUserStore()
const canManage = computed(() => userStore.hasFunc('user_manage'))

const users = ref([])
const roles = ref([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    users.value = await api.listUsers()
  } finally {
    loading.value = false
  }
}
onMounted(async () => {
  await load()
  roles.value = await api.listRoles()
})

// 新建 / 编辑
const formDialog = ref(false)
const editing = ref(false)
const form = ref({})
function openCreate() {
  editing.value = false
  form.value = { username: '', password: '', display_name: '', email: '' }
  formDialog.value = true
}
function openEdit(row) {
  editing.value = true
  form.value = { id: row.id, username: row.username, display_name: row.display_name, email: row.email, status: row.status }
  formDialog.value = true
}
async function submitForm() {
  const f = form.value
  if (editing.value) {
    await api.updateUser(f.id, { display_name: f.display_name, email: f.email, status: f.status })
  } else {
    if (!f.username || !f.password) {
      ElMessage.warning('用户名和密码必填')
      return
    }
    await api.createUser(f)
  }
  ElMessage.success('保存成功')
  formDialog.value = false
  load()
}

// 改密
const pwdDialog = ref(false)
const pwdUserId = ref(null)
const newPassword = ref('')
function openPassword(row) {
  pwdUserId.value = row.id
  newPassword.value = ''
  pwdDialog.value = true
}
async function submitPassword() {
  if (!newPassword.value) {
    ElMessage.warning('请输入新密码')
    return
  }
  await api.changePassword(pwdUserId.value, newPassword.value)
  ElMessage.success('密码已修改')
  pwdDialog.value = false
}

// 角色
const roleDialog = ref(false)
const roleUserId = ref(null)
const selectedRoleIds = ref([])
async function openRoles(row) {
  roleUserId.value = row.id
  selectedRoleIds.value = await api.getUserRoles(row.id)
  roleDialog.value = true
}
async function submitRoles() {
  await api.setUserRoles(roleUserId.value, selectedRoleIds.value)
  ElMessage.success('角色已更新')
  roleDialog.value = false
}

async function remove(row) {
  await ElMessageBox.confirm(`确定删除用户「${row.username}」？`, '提示', { type: 'warning' })
  await api.deleteUser(row.id)
  ElMessage.success('已删除')
  load()
}
</script>

<style scoped>
.u-name {
  font-weight: 600;
  color: var(--text-primary);
}
.muted {
  color: var(--text-muted);
}
</style>
