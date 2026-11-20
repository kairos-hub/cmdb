<template>
  <span class="adv-wrap">
    <el-button :size="size" :icon="Filter" @click="openAdv">高级筛选</el-button>

    <el-drawer v-model="advOpen" title="高级筛选" size="480px" :close-on-click-modal="false">
      <div class="adv-body">
        <el-form label-width="72px">
          <el-form-item label="资源类型">
            <el-select
              v-model="advModelId"
              placeholder="选择资源类型（可搜名称或标识）"
              filterable
              :filter-method="onModelFilter"
              style="width: 100%"
              @change="onAdvModelChange"
              @visible-change="(v) => { if (!v) modelQuery = '' }"
            >
              <el-option-group v-for="g in groupedModels" :key="g.name" :label="g.name">
                <el-option
                  v-for="m in g.models"
                  :key="m.id"
                  :label="m.name"
                  :value="m.id"
                >
                  <span>{{ m.name }}</span>
                  <span class="opt-obj">{{ m.obj_id }}</span>
                </el-option>
              </el-option-group>
            </el-select>
          </el-form-item>
        </el-form>

        <template v-if="advModelId">
          <div class="cond-head">
            <span>筛选条件（满足全部）</span>
            <el-button link type="primary" :icon="Plus" @click="addCond">添加条件</el-button>
          </div>

          <el-empty v-if="!conds.length" description="点击「添加条件」开始筛选" :image-size="60" />

          <div v-for="(c, i) in conds" :key="i" class="cond-row">
            <el-select v-model="c.field" placeholder="字段" class="c-field" @change="onFieldChange(c)">
              <el-option v-for="a in advAttrs" :key="a.id" :label="a.name" :value="a.property_id" />
            </el-select>
            <el-select v-model="c.op" placeholder="条件" class="c-op" @change="onOpChange(c)">
              <el-option v-for="o in opsFor(c)" :key="o.value" :label="o.label" :value="o.value" />
            </el-select>
            <div class="c-val">
              <el-select v-if="c.op === 'in'" v-model="c.values" multiple collapse-tags placeholder="选择" style="width: 100%">
                <el-option v-for="o in optOf(c)" :key="o.id" :label="o.name" :value="o.id" />
              </el-select>
              <el-select v-else-if="catOf(c) === 'enum'" v-model="c.value" placeholder="选择" style="width: 100%">
                <el-option v-for="o in optOf(c)" :key="o.id" :label="o.name" :value="o.id" />
              </el-select>
              <el-select v-else-if="catOf(c) === 'bool'" v-model="c.value" placeholder="选择" style="width: 100%">
                <el-option label="是" value="true" />
                <el-option label="否" value="false" />
              </el-select>
              <template v-else-if="catOf(c) === 'date'">
                <el-date-picker v-if="c.op === 'daterange'" v-model="c.value" type="date" value-format="YYYY-MM-DD" placeholder="开始" style="width: 50%" />
                <el-date-picker v-if="c.op === 'daterange'" v-model="c.value2" type="date" value-format="YYYY-MM-DD" placeholder="结束" style="width: 50%" />
                <el-date-picker v-else v-model="c.value" type="date" value-format="YYYY-MM-DD" placeholder="日期" style="width: 100%" />
              </template>
              <template v-else-if="catOf(c) === 'number'">
                <el-input v-if="c.op === 'between'" v-model="c.value" placeholder="最小" style="width: 50%" />
                <el-input v-if="c.op === 'between'" v-model="c.value2" placeholder="最大" style="width: 50%" />
                <el-input v-else v-model="c.value" placeholder="数值" style="width: 100%" />
              </template>
              <el-input v-else v-model="c.value" placeholder="值" :disabled="!c.field" style="width: 100%" />
            </div>
            <el-button link type="danger" :icon="Delete" class="c-del" @click="conds.splice(i, 1)" />
          </div>
        </template>
      </div>

      <template #footer>
        <el-button @click="conds = []">重置</el-button>
        <el-button type="primary" :icon="Search" :disabled="!validConds.length" @click="applyAdv">
          筛选{{ validConds.length ? `（${validConds.length}）` : '' }}
        </el-button>
      </template>
    </el-drawer>
  </span>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { Search, Filter, Plus, Delete } from '@element-plus/icons-vue'
import * as mapi from '@/api/model'
import { useUserStore } from '@/store/user'

defineProps({
  size: { type: String, default: 'large' },
})

const router = useRouter()
const userStore = useUserStore()

const advOpen = ref(false)
const advModels = ref([])
const advGroups = ref([])
const advModelId = ref(null)
const advAttrs = ref([])
const conds = ref([])
const advLoaded = ref(false)
const modelQuery = ref('') // 资源类型下拉的搜索词（匹配名称 + 英文标识）

// 资源类型按模型分组归类展示；搜索同时匹配中文名与 obj_id；空分组不展示
const groupedModels = computed(() => {
  const q = modelQuery.value.trim().toLowerCase()
  const match = (m) =>
    !q || m.name.toLowerCase().includes(q) || (m.obj_id || '').toLowerCase().includes(q)
  const byGroup = []
  for (const g of advGroups.value) {
    const models = advModels.value.filter((m) => m.group_id === g.id && match(m))
    if (models.length) byGroup.push({ name: g.name, models })
  }
  // 兜底：无分组归属的模型
  const grouped = new Set(advGroups.value.map((g) => g.id))
  const orphans = advModels.value.filter((m) => !grouped.has(m.group_id) && match(m))
  if (orphans.length) byGroup.push({ name: '其他', models: orphans })
  return byGroup
})
function onModelFilter(q) {
  modelQuery.value = q || ''
}

// DNS解析：真实字段是 records(JSON)，用虚拟字段按解析记录维度筛选（映射到 records 文本匹配）
const DNS_FIELDS = [
  { id: 'rr', property_id: 'rr', name: '主机记录', type: 'singlechar' },
  { id: 'fqdn', property_id: 'fqdn', name: '完整域名', type: 'singlechar' },
  { id: 'value', property_id: 'value', name: '记录值', type: 'singlechar' },
  { id: 'type', property_id: 'type', name: '类型', type: 'singlechar' },
]
const isDnsModel = computed(() => {
  const m = advModels.value.find((x) => x.id === advModelId.value)
  return m && m.obj_id === 'dns_record'
})

const OPS = {
  text: [
    { value: 'eq', label: '等于' },
    { value: 'ne', label: '不等于' },
    { value: 'contains', label: '包含' },
  ],
  number: [
    { value: 'eq', label: '等于' },
    { value: 'gt', label: '大于' },
    { value: 'lt', label: '小于' },
    { value: 'between', label: '区间' },
  ],
  enum: [
    { value: 'eq', label: '等于' },
    { value: 'ne', label: '不等于' },
    { value: 'in', label: '属于' },
  ],
  bool: [{ value: 'eq', label: '等于' }],
  date: [
    { value: 'eq', label: '等于' },
    { value: 'daterange', label: '日期范围' },
  ],
}

function catOf(c) {
  const a = advAttrs.value.find((x) => x.property_id === c.field)
  if (!a) return 'text'
  if (a.type === 'enum') return 'enum'
  if (a.type === 'bool') return 'bool'
  if (a.type === 'int' || a.type === 'float') return 'number'
  if (a.type === 'date' || a.type === 'datetime') return 'date'
  return 'text'
}
function optOf(c) {
  const a = advAttrs.value.find((x) => x.property_id === c.field)
  return (a && a.options) || []
}
function opsFor(c) {
  // DNS解析虚拟字段只支持「等于/包含」（底层对 records 文本匹配）
  if (isDnsModel.value) {
    return [
      { value: 'eq', label: '等于' },
      { value: 'contains', label: '包含' },
    ]
  }
  return OPS[catOf(c)] || OPS.text
}

async function openAdv() {
  advOpen.value = true
  if (advLoaded.value) return
  const [all, groups] = await Promise.all([mapi.listModels(), mapi.listGroups()])
  advModels.value = all.filter(
    (m) => !['set', 'module'].includes(m.obj_id) && userStore.hasPerm(m.id, 'view'),
  )
  advGroups.value = groups || []
  advLoaded.value = true
}
async function onAdvModelChange() {
  conds.value = []
  const list = await mapi.listAttributes(advModelId.value)
  const real = (list || []).filter((a) => a.enabled !== false && a.type !== 'enummulti')
  // DNS解析用解析记录维度的虚拟字段替代真实字段
  advAttrs.value = isDnsModel.value ? DNS_FIELDS : real
}
function addCond() {
  conds.value.push({ field: '', op: 'eq', value: '', value2: '', values: [] })
}
function onFieldChange(c) {
  c.op = (opsFor(c)[0] || {}).value || 'eq'
  c.value = ''
  c.value2 = ''
  c.values = []
}
function onOpChange(c) {
  c.value = ''
  c.value2 = ''
  c.values = []
}

const validConds = computed(() =>
  conds.value.filter((c) => {
    if (!c.field || !c.op) return false
    if (c.op === 'in') return (c.values || []).length > 0
    if (c.op === 'between' || c.op === 'daterange') return c.value !== '' && c.value2 !== ''
    return c.value !== '' && c.value !== null && c.value !== undefined
  }),
)

function applyAdv() {
  const payload = validConds.value.map((c) => {
    // DNS解析：虚拟字段(rr/fqdn/value/type) → 对 records JSON 文本匹配
    if (isDnsModel.value) {
      const v = c.value != null ? String(c.value) : ''
      // 等于：按 "key":"值" 精确匹配该记录字段；包含：子串模糊
      const pattern = c.op === 'eq' ? `"${c.field}":"${v}"` : v
      return { field: 'records', op: 'contains', value: pattern, value2: '', values: [] }
    }
    return {
      field: c.field,
      op: c.op,
      value: c.value != null ? String(c.value) : '',
      value2: c.value2 != null ? String(c.value2) : '',
      values: c.values || [],
    }
  })
  if (!payload.length) return
  advOpen.value = false
  const query = { model_id: advModelId.value, adv: JSON.stringify(payload) }
  // DNS解析：带上原始定位值，详情页据此自动过滤并高亮命中记录
  if (isDnsModel.value) {
    const first = validConds.value[0]
    if (first && first.value != null) query.locate = String(first.value)
  }
  router.push({ path: '/search', query })
}
</script>

<style scoped>
.adv-wrap {
  display: inline-flex;
}
.adv-body {
  padding-right: 4px;
}
.cond-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 4px 0 12px;
  color: #606266;
  font-size: 13px;
}
.cond-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}
.c-field {
  width: 130px;
  flex-shrink: 0;
}
.c-op {
  width: 96px;
  flex-shrink: 0;
}
.c-val {
  flex: 1;
  display: flex;
  gap: 6px;
}
.c-del {
  flex-shrink: 0;
}
.opt-obj {
  float: right;
  color: #a8abb2;
  font-size: 12px;
  margin-left: 12px;
}
</style>
