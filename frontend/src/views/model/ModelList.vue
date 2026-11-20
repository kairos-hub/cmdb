<template>
  <div class="model-page">
    <div class="page-header">
      <div>
        <div class="ph-sub">定义资产类型、字段与模型之间的关联关系，新增模型后即可录入和采集实例</div>
      </div>
      <div class="ph-actions">
        <span v-if="!canManage" class="readonly-hint">只读模式（需「模型管理」权限才能编辑）</span>
        <el-button v-if="canManage" :icon="FolderAdd" @click="openGroupDialog">新建分组</el-button>
        <el-button v-if="canManage" type="primary" :icon="Plus" @click="openModelDialog">新建模型</el-button>
      </div>
    </div>

    <div class="body">
      <!-- 左侧分组 -->
      <div class="panel groups">
        <div class="groups-title">模型分组</div>
        <div :class="['group-item', activeGroup === 0 && 'active']" @click="activeGroup = 0">
          <span>全部模型</span>
          <span class="g-count">{{ models.length }}</span>
        </div>
        <div
          v-for="g in groups"
          :key="g.id"
          :class="['group-item', activeGroup === g.id && 'active']"
          @click="activeGroup = g.id"
        >
          <span class="g-name">{{ g.name }}</span>
          <el-icon v-if="!g.is_builtin && canManage" class="del-group" @click.stop="removeGroup(g)"><Delete /></el-icon>
          <span class="g-count">{{ groupCount(g.id) }}</span>
        </div>
      </div>

      <!-- 中间模型卡片 -->
      <div class="models-col">
        <div class="models-toolbar">
          <el-input
            v-model="keyword"
            class="model-search"
            placeholder="搜索模型名称或唯一标识"
            :prefix-icon="Search"
            clearable
          />
        </div>
        <el-empty v-if="!filteredModels.length" description="暂无模型" />
        <div class="models-grid">
          <div
            v-for="(m, idx) in filteredModels"
            :key="m.id"
            class="model-card"
            @click="goDetail(m)"
          >
            <div class="mc-head">
              <span class="mc-icon" :class="'tint-' + (idx % 6)">
                <ModelIcon :obj-id="m.obj_id" :fallback="m.icon || 'Document'" :size="22" />
              </span>
              <div class="mc-meta">
                <div class="mc-name">{{ m.name }}</div>
                <div class="mc-obj">{{ m.obj_id }}</div>
              </div>
              <span class="mc-tag soft-tag" :class="m.is_builtin ? 'is-info' : 'is-purple'">
                {{ m.is_builtin ? '内置' : '自定义' }}
              </span>
            </div>
            <div class="mc-foot">
              <span class="mc-link" @click.stop="goDetail(m)">字段配置</span>
              <span class="mc-link" @click.stop="goInstances(m)">查看实例</span>
              <span v-if="!m.enabled" class="soft-tag is-warning mc-disabled">停用</span>
              <span v-if="canManage" class="mc-ops">
                <el-icon title="编辑模型" @click.stop="openEditModel(m)"><Edit /></el-icon>
                <el-icon v-if="!m.is_builtin" title="删除模型" class="op-del" @click.stop="removeModel(m)"><Delete /></el-icon>
                <template v-if="canSort">
                  <el-icon :class="['op-sort', idx === 0 && 'is-disabled']" title="前移" @click.stop="idx > 0 && move(idx, -1)"><ArrowLeft /></el-icon>
                  <el-icon :class="['op-sort', idx === filteredModels.length - 1 && 'is-disabled']" title="后移" @click.stop="idx < filteredModels.length - 1 && move(idx, 1)"><ArrowRight /></el-icon>
                </template>
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- 右侧提示 -->
      <div class="panel panel-pad tips">
        <div class="tips-title">建模小提示</div>
        <p class="tips-text">内置模型的核心字段不可删除，仅可补充自定义字段；自定义模型可设置主键字段，用于导入时的去重与更新匹配。</p>
      </div>
    </div>

    <!-- 新建分组 -->
    <el-dialog v-model="groupDialog" title="新建分组" width="420px">
      <el-form :model="groupForm" label-width="80px">
        <el-form-item label="标识"><el-input v-model="groupForm.code" placeholder="英文唯一标识" /></el-form-item>
        <el-form-item label="名称"><el-input v-model="groupForm.name" /></el-form-item>
        <el-form-item label="排序"><el-input-number v-model="groupForm.index" :min="0" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="groupDialog = false">取消</el-button>
        <el-button type="primary" @click="submitGroup">确定</el-button>
      </template>
    </el-dialog>

    <!-- 新建/编辑模型 -->
    <el-dialog v-model="modelDialog" :title="editingModel ? '编辑模型' : '新建模型'" width="420px">
      <el-form :model="modelForm" label-width="80px">
        <el-form-item label="所属分组">
          <el-select v-model="modelForm.group_id" placeholder="请选择">
            <el-option v-for="g in groups" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="标识">
          <el-input v-model="modelForm.obj_id" :disabled="editingModel" placeholder="英文唯一标识" />
        </el-form-item>
        <el-form-item label="名称"><el-input v-model="modelForm.name" /></el-form-item>
        <el-form-item label="图标"><IconPicker v-model="modelForm.icon" /></el-form-item>
        <el-form-item v-if="editingModel" label="启用">
          <el-switch v-model="modelForm.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="modelDialog = false">取消</el-button>
        <el-button type="primary" @click="submitModel">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, FolderAdd, Delete, Edit, ArrowLeft, ArrowRight, Search } from '@element-plus/icons-vue'
import * as api from '@/api/model'
import IconPicker from '@/components/IconPicker.vue'
import ModelIcon from '@/components/ModelIcon.vue'
import { useUserStore } from '@/store/user'

const userStore = useUserStore()
const canManage = computed(() => userStore.hasFunc('model_manage'))

const router = useRouter()
const groups = ref([])
const models = ref([])
const activeGroup = ref(0)
const keyword = ref('')

const filteredModels = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  // 有搜索词时跨所有分组按「名称 / 标识」过滤，忽略当前分组
  if (kw) {
    return models.value.filter(
      (m) =>
        (m.name || '').toLowerCase().includes(kw) ||
        (m.obj_id || '').toLowerCase().includes(kw),
    )
  }
  return activeGroup.value === 0
    ? models.value
    : models.value.filter((m) => m.group_id === activeGroup.value)
})
// 仅在选中具体分组且未处于搜索态时可排序（「全部模型」/搜索结果跨分组排序无意义）
const canSort = computed(() => canManage.value && activeGroup.value !== 0 && !keyword.value.trim())

// 前移(dir=-1)/后移(dir=1)分组内模型并持久化顺序
async function move(index, dir) {
  const arr = [...filteredModels.value]
  const t = index + dir
  if (t < 0 || t >= arr.length) return
  ;[arr[index], arr[t]] = [arr[t], arr[index]]
  try {
    await api.reorderModels(activeGroup.value, arr.map((m) => m.id))
    await load()
  } catch {
    ElMessage.error('排序失败')
  }
}

async function load() {
  groups.value = await api.listGroups()
  models.value = await api.listModels()
}
onMounted(load)

function goDetail(m) {
  router.push(`/models/${m.id}`)
}
function goInstances(m) {
  router.push({ path: '/instances', query: { model_id: m.id } })
}
// 各分组模型数（前端真实统计）
function groupCount(gid) {
  return models.value.filter((m) => m.group_id === gid).length
}

// 分组
const groupDialog = ref(false)
const groupForm = ref({ code: '', name: '', index: 0 })
function openGroupDialog() {
  groupForm.value = { code: '', name: '', index: 0 }
  groupDialog.value = true
}
async function submitGroup() {
  if (!groupForm.value.code || !groupForm.value.name) {
    ElMessage.warning('标识和名称必填')
    return
  }
  await api.createGroup(groupForm.value)
  ElMessage.success('创建成功')
  groupDialog.value = false
  load()
}
async function removeGroup(g) {
  await ElMessageBox.confirm(`确定删除分组「${g.name}」？`, '提示', { type: 'warning' })
  await api.deleteGroup(g.id)
  ElMessage.success('已删除')
  if (activeGroup.value === g.id) activeGroup.value = 0
  load()
}

// 模型
const modelDialog = ref(false)
const editingModel = ref(false)
const modelForm = ref({ group_id: null, obj_id: '', name: '', icon: '', enabled: true })
function openModelDialog() {
  editingModel.value = false
  modelForm.value = {
    group_id: activeGroup.value || groups.value[0]?.id || null,
    obj_id: '',
    name: '',
    icon: '',
    enabled: true,
  }
  modelDialog.value = true
}
function openEditModel(m) {
  editingModel.value = true
  modelForm.value = {
    id: m.id,
    group_id: m.group_id,
    obj_id: m.obj_id,
    name: m.name,
    icon: m.icon,
    enabled: m.enabled,
  }
  modelDialog.value = true
}
async function removeModel(m) {
  await ElMessageBox.confirm(`确定删除模型「${m.name}」？其下所有字段将一并删除。`, '提示', {
    type: 'warning',
  })
  await api.deleteModel(m.id)
  ElMessage.success('已删除')
  load()
}
async function submitModel() {
  const f = modelForm.value
  if (editingModel.value) {
    if (!f.name) {
      ElMessage.warning('名称必填')
      return
    }
    await api.updateModel(f.id, { name: f.name, icon: f.icon, group_id: f.group_id, enabled: f.enabled })
  } else {
    if (!f.group_id || !f.obj_id || !f.name) {
      ElMessage.warning('分组、标识、名称必填')
      return
    }
    await api.createModel(f)
  }
  ElMessage.success('保存成功')
  modelDialog.value = false
  load()
}
</script>

<style scoped>
.readonly-hint {
  color: var(--text-secondary);
  font-size: 13px;
}
.body {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}

/* 左侧分组 */
.groups {
  width: 210px;
  flex-shrink: 0;
  padding: 14px 12px;
}
.groups-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-primary);
  padding: 2px 8px 10px;
}
.group-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 9px 12px;
  border-radius: 8px;
  cursor: pointer;
  margin-bottom: 2px;
  font-size: 14px;
  color: var(--text-regular);
}
.group-item .g-name,
.group-item > span:first-child {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.group-item:hover {
  background: var(--fill-bg);
}
.group-item.active {
  background: var(--brand-light);
  color: var(--brand);
  font-weight: 600;
}
.g-count {
  font-size: 13px;
  color: var(--text-muted);
  font-weight: 600;
}
.group-item.active .g-count {
  color: var(--brand);
}
.del-group {
  color: var(--text-muted);
  visibility: hidden;
}
.group-item:hover .del-group {
  visibility: visible;
}
.del-group:hover {
  color: var(--danger);
}

/* 中间卡片列 */
.models-col {
  flex: 1;
  min-width: 0;
}
.models-toolbar {
  margin-bottom: 14px;
}
.model-search {
  width: 100%;
  max-width: 420px;
}
.models-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 14px;
}
.model-card {
  background: var(--card-bg);
  border: 1px solid var(--card-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-card);
  padding: 16px 18px;
  cursor: pointer;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.model-card:hover {
  border-color: var(--brand);
  box-shadow: var(--shadow-card-hover);
}
.mc-head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.mc-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 10px;
  font-size: 20px;
  flex-shrink: 0;
}
.mc-icon.tint-0 { background: #e8f0fe; color: #2f66f0; }
.mc-icon.tint-1 { background: #e6f7ef; color: #16a34a; }
.mc-icon.tint-2 { background: #fdf0e5; color: #d97706; }
.mc-icon.tint-3 { background: #eef0fe; color: #4f46e5; }
.mc-icon.tint-4 { background: #fdeaf0; color: #db2777; }
.mc-icon.tint-5 { background: #e8f6fb; color: #0891b2; }
.mc-meta {
  flex: 1;
  min-width: 0;
}
.mc-name {
  font-size: 15px;
  font-weight: 700;
  color: var(--text-primary);
  line-height: 1.3;
}
.mc-obj {
  font-size: 12px;
  color: var(--text-muted);
  font-family: 'SFMono-Regular', Menlo, Consolas, monospace;
  margin-top: 2px;
}
.mc-tag {
  flex-shrink: 0;
  align-self: flex-start;
}
.mc-foot {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid var(--border-light);
}
.mc-link {
  font-size: 13px;
  color: var(--brand);
}
.mc-link:hover {
  text-decoration: underline;
}
.mc-disabled {
  font-size: 11px;
  padding: 1px 7px;
}
.mc-ops {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--text-muted);
  font-size: 15px;
}
.mc-ops .el-icon {
  cursor: pointer;
}
.mc-ops .el-icon:hover {
  color: var(--brand);
}
.mc-ops .op-del:hover {
  color: var(--danger);
}
.op-sort.is-disabled {
  color: var(--border-base);
  cursor: not-allowed;
}

/* 右侧提示 */
.tips {
  width: 260px;
  flex-shrink: 0;
}
.tips-title {
  font-size: 15px;
  font-weight: 700;
  color: var(--text-primary);
  margin-bottom: 10px;
}
.tips-text {
  font-size: 13px;
  line-height: 1.7;
  color: var(--text-secondary);
  margin: 0;
}

@media (max-width: 1200px) {
  .models-grid { grid-template-columns: 1fr; }
  .tips { display: none; }
}
</style>
