<template>
  <div>
    <div class="page-header">
      <div>
        <div class="ph-title">角色管理</div>
        <div class="ph-sub">配置角色的功能权限与按模型的数据权限，再分配给用户</div>
      </div>
      <div class="ph-actions">
        <el-tag v-if="!canManage" type="info" size="small">只读模式：无「用户与角色·编辑」权限</el-tag>
        <el-button v-if="canManage" type="primary" :icon="Plus" @click="openCreate">新建角色</el-button>
      </div>
    </div>
    <div class="panel panel-pad">
    <el-table :data="roles" v-loading="loading">
      <el-table-column label="ID" prop="id" width="64" />
      <el-table-column label="名称" width="150">
        <template #default="{ row }"><span class="r-name">{{ row.name }}</span></template>
      </el-table-column>
      <el-table-column label="标识" width="140">
        <template #default="{ row }"><span class="r-code">{{ row.code }}</span></template>
      </el-table-column>
      <el-table-column label="功能权限" min-width="260">
        <template #default="{ row }">
          <span v-if="row.is_admin" class="soft-tag is-danger">全部（超级管理员）</span>
          <template v-else>
            <span
              v-for="f in funcLabels(row.functions)"
              :key="f"
              class="soft-tag is-info fn-tag"
            >{{ f }}</span>
            <span v-if="!funcLabels(row.functions).length" class="muted">无</span>
          </template>
        </template>
      </el-table-column>
      <el-table-column label="描述" prop="description" min-width="160" show-overflow-tooltip />
      <el-table-column label="操作" width="200" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" :disabled="row.is_admin || !canManage" @click="openEdit(row)">编辑</el-button>
          <el-button link type="primary" :disabled="row.is_admin || !canManage" @click="openPerm(row)">数据权限</el-button>
          <el-button link type="danger" :disabled="row.is_builtin || !canManage" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
    </div>

    <!-- 新建/编辑角色（含功能权限） -->
    <el-dialog v-model="formDialog" :title="editing ? '编辑角色' : '新建角色'" width="520px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="标识"><el-input v-model="form.code" :disabled="editing" placeholder="英文唯一标识" /></el-form-item>
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="描述"><el-input v-model="form.description" /></el-form-item>
        <el-form-item label="功能权限">
          <el-table :data="FUNC_MODULES" border size="small" class="fn-table">
            <el-table-column label="功能模块" min-width="200">
              <template #default="{ row }">
                <div class="fn-name">{{ row.label }}</div>
                <div class="fn-desc">{{ row.desc }}</div>
              </template>
            </el-table-column>
            <el-table-column label="只读" width="90" align="center">
              <template #default="{ row }">
                <el-checkbox
                  :model-value="hasFn(row.view)"
                  :disabled="!!(row.manage && hasFn(row.manage))"
                  @change="(v) => toggleView(row, v)"
                />
              </template>
            </el-table-column>
            <el-table-column label="编辑" width="90" align="center">
              <template #default="{ row }">
                <el-checkbox
                  v-if="row.manage"
                  :model-value="hasFn(row.manage)"
                  @change="(v) => toggleManage(row, v)"
                />
                <span v-else class="muted">—</span>
              </template>
            </el-table-column>
          </el-table>
          <p class="fn-hint">勾选「编辑」自动包含「只读」。资产总览 / 审计本身只读，无编辑档。资源实例的查看/增删改按「数据权限」逐模型配置。</p>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="formDialog = false">取消</el-button>
        <el-button type="primary" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>

    <!-- 数据权限矩阵（模型 × 操作） -->
    <el-dialog v-model="permDialog" title="数据权限（按模型 × 操作）" width="680px">
      <div class="perm-tools">
        <el-input v-model="permKeyword" placeholder="搜索模型" clearable size="small" style="width: 200px" :prefix-icon="Search" />
        <div class="perm-tools-right">
          <el-button size="small" @click="selectAll(true)">全选</el-button>
          <el-button size="small" @click="selectAll(false)">清空</el-button>
        </div>
      </div>
      <el-table :data="filteredRows" border max-height="440">
        <el-table-column label="模型" min-width="180">
          <template #default="{ row }">
            <el-checkbox
              :model-value="rowAll(row.model_id)"
              :indeterminate="rowSome(row.model_id) && !rowAll(row.model_id)"
              @change="(v) => toggleRow(row.model_id, v)"
            >
              <span :class="{ wildcard: row.model_id === 0 }">{{ row.name }}</span>
            </el-checkbox>
          </template>
        </el-table-column>
        <el-table-column v-for="a in ACTIONS" :key="a.value" width="100" align="center">
          <template #header>
            <el-checkbox
              :model-value="colAll(a.value)"
              :indeterminate="colSome(a.value) && !colAll(a.value)"
              @change="(v) => toggleCol(a.value, v)"
            >{{ a.label }}</el-checkbox>
          </template>
          <template #default="{ row }">
            <el-checkbox
              :model-value="hasAction(row.model_id, a.value)"
              @change="(v) => toggle(row.model_id, a.value, v)"
            />
          </template>
        </el-table-column>
      </el-table>
      <p class="perm-hint">提示：「全部模型（通配）」勾选后，对所有模型（含以后新增）均生效。</p>
      <template #footer>
        <el-button @click="permDialog = false">取消</el-button>
        <el-button type="primary" @click="submitPerm">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import * as api from '@/api/system'
import { ACTIONS, FUNC_MODULES, FUNC_LABELS } from '@/api/system'
import { listModels } from '@/api/model'
import { useUserStore } from '@/store/user'

const userStore = useUserStore()
const canManage = computed(() => userStore.hasFunc('user_manage'))

const roles = ref([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    roles.value = await api.listRoles()
  } finally {
    loading.value = false
  }
}
onMounted(load)

// 角色列表展示：每模块只显示最高档（有编辑显示「·编辑」，否则「·只读」），避免编辑+只读重复。
function funcLabels(functions) {
  const arr = Array.isArray(functions) ? functions : []
  const labels = []
  for (const m of FUNC_MODULES) {
    if (m.manage && arr.includes(m.manage)) labels.push(FUNC_LABELS[m.manage])
    else if (arr.includes(m.view)) labels.push(FUNC_LABELS[m.view])
  }
  return labels
}

// 功能权限勾选（form.functions 存功能码数组）
function hasFn(code) {
  return !!code && form.value.functions.includes(code)
}
function addFn(code) {
  if (code && !form.value.functions.includes(code)) form.value.functions.push(code)
}
function removeFn(code) {
  form.value.functions = form.value.functions.filter((c) => c !== code)
}
// 只读开关：勾上加 view；取消去掉 view（编辑开启时该框被禁用，不会走到取消）
function toggleView(mod, val) {
  if (val) addFn(mod.view)
  else removeFn(mod.view)
}
// 编辑开关：勾上同时加 manage+view（编辑含只读）；取消仅去掉 manage，保留只读
function toggleManage(mod, val) {
  if (val) {
    addFn(mod.manage)
    addFn(mod.view)
  } else {
    removeFn(mod.manage)
  }
}

// 新建 / 编辑
const formDialog = ref(false)
const editing = ref(false)
const form = ref({ code: '', name: '', description: '', functions: [] })
function openCreate() {
  editing.value = false
  form.value = { code: '', name: '', description: '', functions: [] }
  formDialog.value = true
}
function openEdit(row) {
  editing.value = true
  form.value = {
    id: row.id,
    code: row.code,
    name: row.name,
    description: row.description,
    functions: Array.isArray(row.functions) ? [...row.functions] : [],
  }
  formDialog.value = true
}
async function submitForm() {
  const f = form.value
  if (editing.value) {
    await api.updateRole(f.id, { name: f.name, description: f.description, functions: f.functions })
  } else {
    if (!f.code || !f.name) {
      ElMessage.warning('标识和名称必填')
      return
    }
    await api.createRole({ code: f.code, name: f.name, description: f.description, functions: f.functions })
  }
  ElMessage.success('保存成功')
  formDialog.value = false
  load()
}

async function remove(row) {
  await ElMessageBox.confirm(`确定删除角色「${row.name}」？`, '提示', { type: 'warning' })
  await api.deleteRole(row.id)
  ElMessage.success('已删除')
  load()
}

// 数据权限矩阵
const permDialog = ref(false)
const permRoleId = ref(null)
const permRows = ref([])
const permMatrix = ref({})
const permKeyword = ref('')

const filteredRows = computed(() => {
  const kw = permKeyword.value.trim().toLowerCase()
  if (!kw) return permRows.value
  // 通配行始终保留，便于一键全选
  return permRows.value.filter((r) => r.model_id === 0 || r.name.toLowerCase().includes(kw))
})

async function openPerm(row) {
  permRoleId.value = row.id
  permKeyword.value = ''
  const models = await listModels()
  permRows.value = [
    { model_id: 0, name: '全部模型（通配）' },
    ...models.map((m) => ({ model_id: m.id, name: m.name })),
  ]
  const perms = await api.getRolePermissions(row.id)
  const matrix = {}
  for (const p of perms) {
    matrix[p.model_id] = Array.isArray(p.actions) ? p.actions : []
  }
  permMatrix.value = matrix
  permDialog.value = true
}

function hasAction(mid, action) {
  return (permMatrix.value[mid] || []).includes(action)
}
function toggle(mid, action, checked) {
  const arr = permMatrix.value[mid] ? [...permMatrix.value[mid]] : []
  const i = arr.indexOf(action)
  if (checked && i < 0) arr.push(action)
  if (!checked && i >= 0) arr.splice(i, 1)
  permMatrix.value = { ...permMatrix.value, [mid]: arr }
}

// 行级：某模型全部操作
function rowAll(mid) {
  const arr = permMatrix.value[mid] || []
  return ACTIONS.every((a) => arr.includes(a.value))
}
function rowSome(mid) {
  const arr = permMatrix.value[mid] || []
  return ACTIONS.some((a) => arr.includes(a.value))
}
function toggleRow(mid, checked) {
  permMatrix.value = {
    ...permMatrix.value,
    [mid]: checked ? ACTIONS.map((a) => a.value) : [],
  }
}

// 列级：某操作对（当前筛选出的）所有模型
function colAll(action) {
  return filteredRows.value.every((r) => hasAction(r.model_id, action))
}
function colSome(action) {
  return filteredRows.value.some((r) => hasAction(r.model_id, action))
}
function toggleCol(action, checked) {
  const next = { ...permMatrix.value }
  for (const r of filteredRows.value) {
    const arr = next[r.model_id] ? [...next[r.model_id]] : []
    const i = arr.indexOf(action)
    if (checked && i < 0) arr.push(action)
    if (!checked && i >= 0) arr.splice(i, 1)
    next[r.model_id] = arr
  }
  permMatrix.value = next
}

// 全选 / 清空（当前筛选范围）
function selectAll(checked) {
  const next = { ...permMatrix.value }
  for (const r of filteredRows.value) {
    next[r.model_id] = checked ? ACTIONS.map((a) => a.value) : []
  }
  permMatrix.value = next
}

async function submitPerm() {
  const permissions = Object.entries(permMatrix.value)
    .filter(([, actions]) => actions.length)
    .map(([mid, actions]) => ({ model_id: Number(mid), actions }))
  await api.setRolePermissions(permRoleId.value, permissions)
  ElMessage.success('权限已保存')
  permDialog.value = false
}
</script>

<style scoped>
.r-name {
  font-weight: 600;
  color: var(--text-primary);
}
.r-code {
  font-family: 'SFMono-Regular', Menlo, Consolas, monospace;
  font-size: 12.5px;
  color: var(--text-secondary);
}
.fn-tag {
  margin: 2px 4px 2px 0;
}
.muted {
  color: var(--text-muted);
}
.fn-table {
  width: 100%;
}
.fn-hint {
  color: #909399;
  font-size: 12px;
  margin: 8px 0 0;
  line-height: 1.6;
}
.fn-name {
  font-weight: 600;
}
.fn-desc {
  color: #909399;
  font-size: 12px;
}
.perm-tools {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}
.perm-hint {
  color: #909399;
  font-size: 12px;
  margin: 8px 0 0;
}
.wildcard {
  font-weight: 600;
  color: #e6a23c;
}
</style>
