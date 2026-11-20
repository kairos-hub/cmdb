<template>
  <div v-loading="loading">
    <!-- 头部概览 -->
    <div class="panel detail-hero">
      <el-button class="dh-back" :icon="ArrowLeft" circle title="返回" @click="$router.push('/models')" />
      <div class="dh-left">
        <span class="dh-icon"><ModelIcon :obj-id="modelInfo.obj_id" :fallback="modelInfo.icon || 'Document'" :size="26" /></span>
        <div class="dh-meta">
          <div class="dh-title-row">
            <span class="dh-name">{{ modelInfo.name }}</span>
            <span class="soft-tag" :class="modelInfo.is_builtin ? 'is-info' : 'is-purple'">
              {{ modelInfo.is_builtin ? '内置' : '自定义' }}
            </span>
          </div>
          <div class="dh-sub">
            <span class="mono">{{ modelInfo.obj_id }}</span>
            <span class="sep">·</span>
            <span>{{ attrs.length }} 个字段</span>
          </div>
        </div>
      </div>
      <div class="dh-actions">
        <el-button v-if="canManage && tab === 'attrs'" type="primary" :icon="Plus" @click="openAttrDialog()">新建字段</el-button>
        <el-button v-if="canManage && tab === 'assoc'" type="primary" :icon="Plus" @click="openAssocDialog()">新建关联</el-button>
      </div>
    </div>

    <el-tabs v-model="tab" class="tabs panel">
      <!-- 字段管理 -->
      <el-tab-pane label="字段配置" name="attrs">
        <div class="field-toolbar">
          <el-input
            v-model="attrKeyword"
            class="field-search"
            placeholder="搜索字段名称或标识"
            :prefix-icon="Search"
            clearable
          />
          <span v-if="canManage" class="field-hint">点击「排序」可调整字段展示顺序</span>
        </div>
        <el-table :data="filteredAttrs">
          <el-table-column prop="name" label="字段名称" min-width="120">
            <template #default="{ row }"><span class="f-name">{{ row.name }}</span></template>
          </el-table-column>
          <el-table-column prop="property_id" label="唯一标识" min-width="130">
            <template #default="{ row }"><span class="f-id">{{ row.property_id }}</span></template>
          </el-table-column>
          <el-table-column label="类型" width="110">
            <template #default="{ row }"><span class="soft-tag is-muted">{{ fieldTypeLabel(row.type) }}</span></template>
          </el-table-column>
          <el-table-column label="必填" width="70">
            <template #default="{ row }"><span v-if="row.required" class="soft-tag is-danger">是</span><span v-else class="f-dash">—</span></template>
          </el-table-column>
          <el-table-column width="86">
            <template #header>
              <span>唯一</span>
              <el-tooltip placement="top" content="唯一校验作用域为「同一数据来源内」：同一来源（阿里云/Prometheus/用户创建）不可重复，不同来源相同值可各自共存。">
                <el-icon class="hint-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </template>
            <template #default="{ row }"><span v-if="row.unique" class="soft-tag is-warning">是</span><span v-else class="f-dash">—</span></template>
          </el-table-column>
          <el-table-column label="来源" width="90">
            <template #default="{ row }">
              <span class="soft-tag" :class="row.is_builtin ? 'is-info' : 'is-purple'">{{ row.is_builtin ? '内置' : '自定义' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="启用" width="80">
            <template #default="{ row }">
              <el-switch
                v-model="row.enabled"
                :disabled="!canManage"
                @change="toggleEnabled(row)"
              />
            </template>
          </el-table-column>
          <el-table-column v-if="canManage && !attrKeyword.trim()" label="排序" width="90">
            <template #default="{ $index }">
              <el-button link :icon="Top" :disabled="$index === 0" @click="move($index, -1)" />
              <el-button
                link
                :icon="Bottom"
                :disabled="$index === attrs.length - 1"
                @click="move($index, 1)"
              />
            </template>
          </el-table-column>
          <el-table-column v-if="canManage" label="操作" width="140">
            <template #default="{ row }">
              <el-button link type="primary" @click="openAttrDialog(row)">编辑</el-button>
              <el-button link type="danger" :disabled="row.is_builtin" @click="removeAttr(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 关联关系 -->
      <el-tab-pane label="模型关联" name="assoc">
        <el-table :data="relatedAssocs">
          <el-table-column label="关联类型">
            <template #default="{ row }">{{ kindName(row.kind_id) }}</template>
          </el-table-column>
          <el-table-column label="源模型">
            <template #default="{ row }">{{ modelName(row.src_model_id) }}</template>
          </el-table-column>
          <el-table-column label="目标模型">
            <template #default="{ row }">{{ modelName(row.dst_model_id) }}</template>
          </el-table-column>
          <el-table-column prop="mapping" label="基数" width="90" />
          <el-table-column v-if="canManage" label="操作" width="90">
            <template #default="{ row }">
              <el-button link type="danger" @click="removeAssoc(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <!-- 字段对话框 -->
    <el-dialog v-model="attrDialog" :title="attrEditing ? '编辑字段' : '新建字段'" width="520px">
      <el-form :model="attrForm" label-width="90px">
        <el-form-item label="名称"><el-input v-model="attrForm.name" /></el-form-item>
        <el-form-item label="标识">
          <el-input v-model="attrForm.property_id" :disabled="attrEditing" placeholder="英文唯一标识" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="attrForm.type" :disabled="attrEditing && attrForm.is_builtin">
            <el-option v-for="t in FIELD_TYPES" :key="t.value" :label="t.label" :value="t.value" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="isEnum" label="枚举选项">
          <el-input v-model="attrForm.optionsText" type="textarea" :rows="4" placeholder="每行一个选项名" />
        </el-form-item>
        <el-form-item label="默认值"><el-input v-model="attrForm.default" /></el-form-item>
        <el-form-item label="校验正则"><el-input v-model="attrForm.regex" /></el-form-item>
        <el-form-item label="属性">
          <el-checkbox v-model="attrForm.required">必填</el-checkbox>
          <el-checkbox v-model="attrForm.unique">唯一</el-checkbox>
          <el-checkbox v-model="attrForm.editable">可编辑</el-checkbox>
          <el-checkbox v-model="attrForm.enabled">启用</el-checkbox>
          <el-checkbox v-model="attrForm.list_hidden">列表隐藏</el-checkbox>
          <div v-if="attrForm.unique" class="form-hint">
            「唯一」作用域为同一数据来源内：同一来源不可重复，不同来源（阿里云/Prometheus/用户创建）的相同值可各自共存。
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="attrDialog = false">取消</el-button>
        <el-button type="primary" @click="submitAttr">确定</el-button>
      </template>
    </el-dialog>

    <!-- 关联对话框 -->
    <el-dialog v-model="assocDialog" title="新建关联" width="460px">
      <el-form :model="assocForm" label-width="90px">
        <el-form-item label="关联类型">
          <el-select v-model="assocForm.kind_id">
            <el-option v-for="k in kinds" :key="k.id" :label="k.name || k.code" :value="k.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="源模型">
          <el-input :value="modelInfo.name" disabled />
        </el-form-item>
        <el-form-item label="目标模型">
          <el-select v-model="assocForm.dst_model_id" filterable>
            <el-option v-for="m in allModels" :key="m.id" :label="m.name" :value="m.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="基数">
          <el-select v-model="assocForm.mapping">
            <el-option v-for="mp in MAPPINGS" :key="mp.value" :label="mp.label" :value="mp.value" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="assocDialog = false">取消</el-button>
        <el-button type="primary" @click="submitAssoc">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, Plus, Top, Bottom, QuestionFilled, Search } from '@element-plus/icons-vue'
import * as api from '@/api/model'
import { FIELD_TYPES, MAPPINGS, fieldTypeLabel } from '@/api/model'
import { useUserStore } from '@/store/user'
import ModelIcon from '@/components/ModelIcon.vue'

const userStore = useUserStore()
const canManage = computed(() => userStore.hasFunc('model_manage'))

const route = useRoute()
const modelId = Number(route.params.id)

const loading = ref(false)
const tab = ref('attrs')
const modelInfo = ref({})
const attrs = ref([])
const attrKeyword = ref('')
const filteredAttrs = computed(() => {
  const kw = attrKeyword.value.trim().toLowerCase()
  if (!kw) return attrs.value
  return attrs.value.filter(
    (a) =>
      (a.name || '').toLowerCase().includes(kw) ||
      (a.property_id || '').toLowerCase().includes(kw),
  )
})
const associations = ref([])
const kinds = ref([])
const allModels = ref([])

const relatedAssocs = computed(() =>
  associations.value.filter((a) => a.src_model_id === modelId || a.dst_model_id === modelId),
)
const modelName = (id) => allModels.value.find((m) => m.id === id)?.name || `#${id}`
const kindName = (id) => {
  const k = kinds.value.find((i) => i.id === id)
  return k ? k.name || k.code : `#${id}`
}

async function load() {
  loading.value = true
  try {
    modelInfo.value = await api.getModel(modelId)
    attrs.value = await api.listAttributes(modelId)
    associations.value = await api.listAssociations()
    kinds.value = await api.listKinds()
    allModels.value = await api.listModels()
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---- 字段 ----
const attrDialog = ref(false)
const attrEditing = ref(false)
const attrForm = ref({})
const isEnum = computed(() => ['enum', 'enummulti'].includes(attrForm.value.type))

function openAttrDialog(row) {
  attrEditing.value = !!row
  if (row) {
    attrForm.value = {
      id: row.id,
      is_builtin: row.is_builtin,
      name: row.name,
      property_id: row.property_id,
      type: row.type,
      required: row.required,
      unique: row.unique,
      editable: row.editable,
      enabled: row.enabled !== false,
      list_hidden: row.list_hidden === true,
      default: row.default,
      regex: row.regex,
      index: row.index,
      optionsText: Array.isArray(row.options) ? row.options.map((o) => o.name).join('\n') : '',
    }
  } else {
    attrForm.value = {
      name: '', property_id: '', type: 'singlechar',
      required: false, unique: false, editable: true, enabled: true, list_hidden: false,
      default: '', regex: '', optionsText: '',
    }
  }
  attrDialog.value = true
}

function buildAttrPayload() {
  const f = attrForm.value
  const payload = {
    property_id: f.property_id,
    name: f.name,
    type: f.type,
    required: f.required,
    unique: f.unique,
    editable: f.editable,
    enabled: f.enabled !== false,
    list_hidden: f.list_hidden === true,
    default: f.default,
    regex: f.regex,
    group: 'default',
    index: f.index || 0,
  }
  if (['enum', 'enummulti'].includes(f.type)) {
    const lines = (f.optionsText || '').split('\n').map((s) => s.trim()).filter(Boolean)
    payload.options = lines.map((name, i) => ({ id: String(i + 1), name }))
  }
  return payload
}

async function submitAttr() {
  const f = attrForm.value
  if (!f.name || (!attrEditing.value && !f.property_id)) {
    ElMessage.warning('名称和标识必填')
    return
  }
  const payload = buildAttrPayload()
  if (attrEditing.value) {
    await api.updateAttribute(f.id, payload)
  } else {
    await api.createAttribute(modelId, payload)
  }
  ElMessage.success('保存成功')
  attrDialog.value = false
  attrs.value = await api.listAttributes(modelId)
}

// 切换字段启用/停用（停用后在实例表单、列表、导出中隐藏，数据保留）
async function toggleEnabled(row) {
  try {
    await api.updateAttribute(row.id, {
      property_id: row.property_id,
      name: row.name,
      type: row.type,
      required: row.required,
      unique: row.unique,
      editable: row.editable,
      enabled: row.enabled,
      list_hidden: row.list_hidden === true,
      options: row.options,
      default: row.default,
      regex: row.regex,
      group: row.group || 'default',
      index: row.index,
    })
    ElMessage.success(row.enabled ? '已启用' : '已停用')
  } catch {
    row.enabled = !row.enabled // 失败回滚开关
  }
}

async function removeAttr(row) {
  await ElMessageBox.confirm(`确定删除字段「${row.name}」？`, '提示', { type: 'warning' })
  await api.deleteAttribute(row.id)
  ElMessage.success('已删除')
  attrs.value = await api.listAttributes(modelId)
}

// 上移(dir=-1)/下移(dir=1)字段并持久化顺序
async function move(index, dir) {
  const target = index + dir
  if (target < 0 || target >= attrs.value.length) return
  const arr = [...attrs.value]
  ;[arr[index], arr[target]] = [arr[target], arr[index]]
  attrs.value = arr
  await api.reorderAttributes(modelId, arr.map((a) => a.id))
  attrs.value = await api.listAttributes(modelId)
}

// ---- 关联 ----
const assocDialog = ref(false)
const assocForm = ref({})
function openAssocDialog() {
  assocForm.value = { kind_id: null, dst_model_id: null, mapping: '1:n' }
  assocDialog.value = true
}
async function submitAssoc() {
  const f = assocForm.value
  if (!f.kind_id || !f.dst_model_id || !f.mapping) {
    ElMessage.warning('请完整选择关联类型、目标模型与基数')
    return
  }
  await api.createAssociation({
    kind_id: f.kind_id,
    src_model_id: modelId,
    dst_model_id: f.dst_model_id,
    mapping: f.mapping,
  })
  ElMessage.success('创建成功')
  assocDialog.value = false
  associations.value = await api.listAssociations()
}
async function removeAssoc(row) {
  await ElMessageBox.confirm('确定删除该关联？', '提示', { type: 'warning' })
  await api.deleteAssociation(row.id)
  ElMessage.success('已删除')
  associations.value = await api.listAssociations()
}
</script>

<style scoped>
/* 头部概览 */
.detail-hero {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px 22px;
  margin-bottom: 16px;
}
/* 左上角返回按钮（与资源实例详情一致：圆形图标、第一眼可见） */
.dh-back {
  flex-shrink: 0;
  font-weight: 600;
}
.dh-left {
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: 0;
}
.dh-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 52px;
  height: 52px;
  border-radius: 13px;
  background: var(--brand-light);
  color: var(--brand);
  font-size: 26px;
  flex-shrink: 0;
}
.dh-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.dh-name {
  font-size: 22px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.3px;
}
.dh-sub {
  margin-top: 5px;
  font-size: 13px;
  color: var(--text-muted);
  display: flex;
  align-items: center;
  gap: 8px;
}
.dh-sub .mono {
  font-family: 'SFMono-Regular', Menlo, Consolas, monospace;
}
.dh-sub .sep { color: var(--border-base); }
.dh-actions { flex-shrink: 0; display: flex; gap: 10px; margin-left: auto; }

/* 选项卡面板 */
.tabs {
  padding: 4px 18px 18px;
}
.tabs :deep(.el-tabs__header) {
  margin-bottom: 16px;
}
.tabs :deep(.el-tabs__item) {
  font-size: 14px;
  font-weight: 500;
}
.tabs :deep(.el-tabs__item.is-active) {
  font-weight: 600;
}

/* 字段工具栏 */
.field-toolbar {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 14px;
}
.field-search {
  width: 300px;
}
.field-hint {
  font-size: 12.5px;
  color: var(--text-muted);
}

/* 字段表格单元 */
.f-name {
  font-weight: 600;
  color: var(--text-primary);
}
.f-id {
  font-family: 'SFMono-Regular', Menlo, Consolas, monospace;
  font-size: 12.5px;
  color: var(--text-secondary);
}
.f-dash { color: var(--text-muted); }

.hint-icon {
  margin-left: 2px;
  color: var(--text-muted);
  vertical-align: -2px;
  cursor: help;
}
.form-hint {
  width: 100%;
  margin-top: 6px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-secondary);
}
</style>
