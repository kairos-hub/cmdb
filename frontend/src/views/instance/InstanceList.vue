<template>
  <div class="inst-page">
   <el-container class="inst-body">
    <el-aside v-show="catalogVisible" :width="catalogWidth + 'px'" class="catalog">
      <div class="catalog-head">资源类型</div>
      <div class="catalog-search">
        <el-input v-model="catalogKeyword" placeholder="搜索资源类型" clearable size="small" :prefix-icon="Search" />
      </div>
      <el-scrollbar class="catalog-scroll">
        <el-menu
          :key="catalogKeyword"
          :default-active="String(modelId)"
          :default-openeds="openedGroups"
          unique-opened
          class="catalog-menu"
          @select="onCatalogSelect"
        >
          <el-sub-menu v-for="g in catalog" :key="g.id" :index="'g' + g.id">
            <template #title><span class="cat-group">{{ g.name }}</span></template>
            <el-menu-item v-for="m in g.models" :key="m.id" :index="String(m.id)">
              <ModelIcon class="cat-icon" :obj-id="m.obj_id" :fallback="m.icon || 'Document'" :size="16" />
              <span>{{ m.name }}</span>
            </el-menu-item>
          </el-sub-menu>
        </el-menu>
        <el-empty v-if="!catalog.length" description="无匹配资源类型" :image-size="60" />
      </el-scrollbar>
    </el-aside>
    <div v-show="catalogVisible" class="resizer" @mousedown="startResize" title="拖动调整宽度"></div>
    <el-main class="inst-main">
    <div class="inst-card">
    <div class="toolbar">
      <el-button
        :icon="catalogVisible ? Fold : Expand"
        circle
        size="small"
        :title="catalogVisible ? '收起资源目录' : '展开资源目录'"
        @click="toggleCatalog"
      />
      <div class="tb-left">
        <span class="tb-model">{{ currentModelName || '资源实例' }}</span>
        <span v-if="modelId" class="tb-count">{{ total }}</span>
        <el-input
          v-model="keyword"
          placeholder="关键字搜索"
          :prefix-icon="Search"
          class="tb-kw"
          clearable
          @keyup.enter="reload"
        />
        <el-button type="primary" :icon="Search" @click="reload">搜索</el-button>
        <el-button
          :icon="Filter"
          :type="showFilter || activeFilterCount || quickChips.length ? 'primary' : ''"
          :disabled="!modelId"
          @click="showFilter = !showFilter"
        >
          筛选<span v-if="activeFilterCount + quickChips.length">({{ activeFilterCount + quickChips.length }})</span>
        </el-button>
        <el-tag
          v-for="chip in quickChips"
          :key="chip.key"
          closable
          size="small"
          class="tb-chip"
          @close="chip.clear()"
        >{{ chip.label }}</el-tag>
      </div>
      <div class="tb-right">
      <el-button v-if="canCreate && !isBizModel" type="primary" :icon="Plus" :disabled="!modelId" @click="openForm()">新建</el-button>
      <el-dropdown :disabled="!modelId" @command="onDataCommand">
        <el-button :disabled="!modelId">
          数据操作<el-icon class="el-icon--right"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="export" :icon="Download">导出…</el-dropdown-item>
            <el-dropdown-item v-if="canCreate && !isBizModel" command="import" :icon="Upload">导入</el-dropdown-item>
            <el-dropdown-item command="template" :icon="Document">模板</el-dropdown-item>
            <el-dropdown-item
              v-if="userStore.isAdmin && !isBizModel"
              command="batchDelete"
              :icon="Delete"
              :disabled="!selected.length"
            >批量删除{{ selected.length ? `（${selected.length}）` : '' }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <el-popover placement="bottom-end" :width="230" trigger="click">
        <template #reference>
          <el-button :disabled="!modelId" :icon="Operation">列</el-button>
        </template>
        <div class="col-pop">
          <div class="col-pop-head">
            <span>显示列</span>
            <el-button link type="primary" @click="resetCols">全选</el-button>
          </div>
          <el-checkbox-group v-model="visibleCols" class="col-pop-list">
            <el-checkbox v-for="a in columnAttrs" :key="a.property_id" :value="a.property_id">{{ a.name }}</el-checkbox>
          </el-checkbox-group>
        </div>
      </el-popover>
      <input ref="fileInput" type="file" accept=".xlsx" style="display: none" @change="onFilePick" />
      </div>
    </div>

    <!-- 批量选择操作条 -->
    <div v-if="selected.length" class="batch-bar">
      <span class="batch-count">已选择 <b>{{ selected.length }}</b> 项</span>
      <el-button size="small" :icon="Download" @click="exportSelected">导出所选</el-button>
      <el-button
        v-if="userStore.isAdmin && !isBizModel"
        size="small"
        type="danger"
        plain
        :icon="Delete"
        @click="onDataCommand('batchDelete')"
      >批量删除</el-button>
      <el-button size="small" link class="batch-cancel" @click="clearSelection">取消选择</el-button>
    </div>

    <!-- 字段筛选：自主选择「字段 + 条件 + 值」，可多条组合 -->
    <div v-if="showFilter && modelId" class="filter-panel">
      <!-- 快捷过滤：归属 / 业务线 / 数据源（收纳进筛选面板，工具栏以标签回显） -->
      <div class="quick-filters">
        <el-select v-model="assignedFilter" placeholder="全部归属" clearable style="width: 140px" @change="reload">
          <el-option label="已归属" value="assigned" />
          <el-option label="未归属" value="unassigned" />
        </el-select>
        <el-select v-model="bizFilter" placeholder="全部业务线" clearable filterable style="width: 160px" @change="reload">
          <el-option v-for="b in bizLines" :key="b.id" :label="b.name" :value="String(b.id)" />
        </el-select>
        <el-select v-model="sourceFilter" placeholder="全部数据源" clearable style="width: 180px" @change="reload">
          <el-option label="用户创建" value="manual" />
          <el-option label="自动同步（全部）" value="auto" />
          <el-option
            v-for="d in dataSources"
            :key="d.id"
            :label="`${d.name}（${sourceTypeLabel(d.type)}）`"
            :value="String(d.id)"
          />
        </el-select>
      </div>
      <div v-for="(c, i) in condRows" :key="i" class="cond-row">
        <el-select v-model="c.field" placeholder="字段" class="c-field" @change="onFieldChange(c)">
          <el-option v-for="a in filterableAttrs" :key="a.id" :label="a.name" :value="a.property_id" />
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
        <el-button link :icon="Plus" @click="addCond" title="添加条件" />
        <el-button link type="danger" :icon="Delete" :disabled="condRows.length === 1" @click="condRows.splice(i, 1)" title="删除" />
      </div>
      <div class="filter-actions">
        <el-button type="primary" :icon="Search" @click="applyConds">筛选搜索</el-button>
        <el-button @click="resetFilters">重置</el-button>
      </div>
    </div>

    <div class="table-wrap" ref="tableWrapRef">
    <el-table ref="tableRef" :key="modelId" :data="list" :fit="true" :height="tableHeight || undefined" :style="tableStyle" scrollbar-always-on v-loading="loading" @selection-change="selected = $event">
      <el-table-column type="selection" width="45" />
      <el-table-column
        v-for="(a, idx) in shownColumnAttrs"
        :key="a.id"
        :label="a.name"
        :width="colWidth(a)"
        show-overflow-tooltip
      >
        <template #default="{ row }">
          <el-tag
            v-if="isStatusAttr(a) && hasVal(row.data?.[a.property_id])"
            :type="statusTagType(displayValue(a, row.data?.[a.property_id]))"
            size="small"
          >{{ displayValue(a, row.data?.[a.property_id]) }}</el-tag>
          <el-link
            v-else-if="isLinkAttr(a, idx) && hasVal(row.data?.[a.property_id])"
            type="primary"
            :underline="false"
            @click="goDetail(row.id)"
          >{{ displayValue(a, row.data?.[a.property_id]) }}</el-link>
          <template v-else>{{ displayValue(a, row.data?.[a.property_id]) }}</template>
        </template>
      </el-table-column>
      <el-table-column label="数据源" width="100">
        <template #default="{ row }">
          <el-tag v-if="row.source_type" size="small" type="primary">{{ sourceTypeLabel(row.source_type) }}</el-tag>
          <el-tag v-else size="small" type="info" effect="plain">用户创建</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="140">
        <template #default="{ row }">
          <el-button link type="primary" @click="openRelations(row)">关联</el-button>
          <el-button v-if="canUpdate && !isBizModel" link type="primary" @click="openForm(row)">编辑</el-button>
        </template>
      </el-table-column>
      <!-- 末尾弹性占位列：唯一无固定宽度的列，吸收多余宽度，避免大屏/收起目录后右侧留白；
           列宽之和超出时照常横向滚动，其它列固定宽度不受影响 -->
      <el-table-column min-width="1" />
    </el-table>
    </div>

    <el-pagination
      class="pager"
      background
      layout="total, prev, pager, next, sizes"
      :total="total"
      :current-page="page"
      :page-size="pageSize"
      :page-sizes="[10, 20, 50, 100]"
      @current-change="(p) => { page = p; load() }"
      @size-change="(s) => { pageSize = s; page = 1; load() }"
    />
    </div>
    </el-main>
   </el-container>

    <!-- 动态表单 -->
    <el-dialog v-model="formDialog" :title="editing ? '编辑实例' : '新建实例'" width="560px">
      <el-form label-width="120px">
        <el-form-item v-for="a in enabledAttrs" :key="a.id" :label="a.name" :required="a.required">
          <el-input-number
            v-if="a.type === 'int' || a.type === 'float'"
            v-model="form[a.property_id]"
            :controls="false"
            style="width: 100%"
          />
          <el-switch v-else-if="a.type === 'bool'" v-model="form[a.property_id]" />
          <el-select v-else-if="a.type === 'enum'" v-model="form[a.property_id]" clearable style="width: 100%">
            <el-option v-for="o in a.options || []" :key="o.id" :label="o.name" :value="o.id" />
          </el-select>
          <el-select
            v-else-if="a.type === 'enummulti'"
            v-model="form[a.property_id]"
            multiple
            clearable
            style="width: 100%"
          >
            <el-option v-for="o in a.options || []" :key="o.id" :label="o.name" :value="o.id" />
          </el-select>
          <el-date-picker
            v-else-if="a.type === 'date'"
            v-model="form[a.property_id]"
            type="date"
            value-format="YYYY-MM-DD"
            style="width: 100%"
          />
          <el-date-picker
            v-else-if="a.type === 'datetime'"
            v-model="form[a.property_id]"
            type="datetime"
            value-format="YYYY-MM-DD HH:mm:ss"
            style="width: 100%"
          />
          <el-input
            v-else-if="a.type === 'longchar'"
            v-model="form[a.property_id]"
            type="textarea"
            :rows="3"
          />
          <el-input v-else v-model="form[a.property_id]" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="form-footer">
          <el-button
            v-if="editing && canDelete && !isBizModel"
            type="danger"
            plain
            @click="deleteFromForm"
          >删除</el-button>
          <span>
            <el-button @click="formDialog = false">取消</el-button>
            <el-button type="primary" @click="submitForm">确定</el-button>
          </span>
        </div>
      </template>
    </el-dialog>

    <!-- 导出：选择范围 -->
    <el-dialog v-model="exportDialog" title="导出" width="440px">
      <el-radio-group v-model="exportScope" class="exp-scope">
        <el-radio value="current">当前模型（按当前筛选条件）</el-radio>
        <el-radio value="selected" :disabled="!selected.length">当前所选（{{ selected.length }}）</el-radio>
        <el-radio value="all">全部资源 · 所有模型（全部来源）</el-radio>
      </el-radio-group>
      <template #footer>
        <el-button @click="exportDialog = false">取消</el-button>
        <el-button type="primary" @click="runExport">导出</el-button>
      </template>
    </el-dialog>

    <!-- 导入结果 -->
    <el-dialog v-model="reportDialog" title="导入结果" width="520px">
      <el-alert
        :title="`新增 ${importReport.created || 0} 条，更新 ${importReport.updated || 0} 条，失败 ${importReport.failed} 条`"
        :type="importReport.failed ? 'warning' : 'success'"
        :closable="false"
      />
      <div v-if="importReport.errors && importReport.errors.length" class="err-list">
        <p v-for="(e, i) in importReport.errors" :key="i" class="err-item">{{ e }}</p>
      </div>
      <template #footer>
        <el-button type="primary" @click="reportDialog = false">确定</el-button>
      </template>
    </el-dialog>

    <!-- 关联关系 -->
    <el-dialog v-model="relDialog" title="关联关系" width="640px">
      <div class="rel-head">
        <span class="rel-head-label">当前实例</span>
        <span class="rel-head-name">{{ relTitle }}</span>
        <el-tag size="small" effect="plain">{{ currentModelName }}</el-tag>
      </div>

      <el-alert
        v-if="isBizModel"
        type="info"
        :closable="false"
        show-icon
        title="业务的关联由业务拓扑（下辖主机）与各资源的「属于业务」关联自动维护，请在业务拓扑或对应资源处管理。"
        class="rel-alert"
      />

      <!-- 新建关联 -->
      <div v-if="!isBizModel && canUpdate" class="rel-section">
        <div class="rel-section-title">新建关联</div>
        <div class="rel-create">
          <el-select
            v-model="newRel.assocId"
            placeholder="选择关系"
            style="width: 190px"
            @change="onAssocChange"
          >
            <el-option v-for="o in applicableAssocs" :key="o.id" :label="o.label" :value="o.id" />
          </el-select>
          <el-select
            v-model="newRel.peerIds"
            :placeholder="newRel.assocId ? '选择目标实例（可多选）' : '请先选择关系'"
            :disabled="!newRel.assocId"
            multiple
            filterable
            collapse-tags
            collapse-tags-tooltip
            style="flex: 1; min-width: 240px"
          >
            <el-option v-for="p in peerOptions" :key="p.id" :label="p.label" :value="p.id" />
          </el-select>
          <el-button type="primary" :disabled="!newRel.assocId || !newRel.peerIds.length" @click="submitRelation">
            添加{{ newRel.peerIds.length ? `（${newRel.peerIds.length}）` : '' }}
          </el-button>
        </div>
      </div>

      <!-- 已有关联 -->
      <div class="rel-section">
        <div class="rel-section-title">
          <span>已有关联</span>
          <span class="rel-count">{{ relList.length }}</span>
          <el-button
            v-if="canUpdate && !isBizModel && relSelected.length"
            size="small"
            type="danger"
            plain
            :icon="Delete"
            class="rel-batch-btn"
            @click="batchDeleteRelations"
          >批量删除（{{ relSelected.length }}）</el-button>
        </div>
        <el-table :data="relList" v-loading="relLoading" size="small" class="rel-table" @selection-change="relSelected = $event">
          <el-table-column v-if="canUpdate && !isBizModel" type="selection" width="40" :selectable="(row) => !row.derived" />
          <el-table-column label="关系" width="180">
            <template #default="{ row }">
              <span class="soft-tag" :class="row.direction === 'out' ? 'is-info' : 'is-muted'">
                <span class="rel-dir">{{ row.direction === 'out' ? '→' : '←' }}</span>{{ row.note }}
              </span>
              <span v-if="row.derived" class="soft-tag is-success rel-tag">拓扑</span>
            </template>
          </el-table-column>
          <el-table-column label="对端模型" width="110">
            <template #default="{ row }"><span class="soft-tag is-muted">{{ row.peer_model_name }}</span></template>
          </el-table-column>
          <el-table-column label="对端实例">
            <template #default="{ row }"><span class="rel-peer">{{ row.peer_display || `#${row.peer_inst_id}` }}</span></template>
          </el-table-column>
          <el-table-column label="操作" width="64" align="center">
            <template #default="{ row }">
              <el-button v-if="!row.derived && !isBizModel && canUpdate" link type="danger" @click="removeRelation(row)">删除</el-button>
              <span v-else class="muted">—</span>
            </template>
          </el-table-column>
          <template #empty>暂无关联</template>
        </el-table>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, ElLoading } from 'element-plus'
import { Search, Plus, Delete, Download, Document, Upload, Filter, Fold, Expand, ArrowDown, Operation } from '@element-plus/icons-vue'
import * as mapi from '@/api/model'
import * as iapi from '@/api/instance'
import * as tapi from '@/api/topo'
import { useUserStore } from '@/store/user'
import ModelIcon from '@/components/ModelIcon.vue'

const route = useRoute()
const router = useRouter()
function goDetail(id) {
  router.push(`/instances/${id}`)
}
// 来源类型标签
const SOURCE_TYPE_LABEL = { prometheus: 'Prometheus', aliyun: '阿里云', tencent: '腾讯云' }
function sourceTypeLabel(t) {
  return SOURCE_TYPE_LABEL[t] || t
}
// 主标识字段（每个模型的第一个字段，如主机内网IP / 实例名）→ 可点击进详情
function isLinkAttr(a, idx) {
  return idx === 0
}
const userStore = useUserStore()
const canCreate = computed(() => userStore.hasPerm(modelId.value, 'create'))
const canUpdate = computed(() => userStore.hasPerm(modelId.value, 'update'))
const canDelete = computed(() => userStore.hasPerm(modelId.value, 'delete'))

const models = ref([])
const groups = ref([])
const catalogKeyword = ref('')

// 左侧目录宽度 / 显隐（记住上次选择）
const CATALOG_W_KEY = 'inst_catalog_width'
const CATALOG_V_KEY = 'inst_catalog_visible'
const catalogWidth = ref(240)
const catalogVisible = ref(true)
try {
  const w = Number(localStorage.getItem(CATALOG_W_KEY))
  if (w >= 180 && w <= 480) catalogWidth.value = w
  if (localStorage.getItem(CATALOG_V_KEY) === '0') catalogVisible.value = false
} catch (e) { /* 忽略存储不可用 */ }

function toggleCatalog() {
  catalogVisible.value = !catalogVisible.value
  try { localStorage.setItem(CATALOG_V_KEY, catalogVisible.value ? '1' : '0') } catch (e) { /* ignore */ }
}
function startResize(e) {
  const startX = e.clientX
  const startW = catalogWidth.value
  const onMove = (ev) => {
    const w = Math.max(180, Math.min(480, startW + ev.clientX - startX))
    catalogWidth.value = w
  }
  const onUp = () => {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    document.body.style.userSelect = ''
    try { localStorage.setItem(CATALOG_W_KEY, String(catalogWidth.value)) } catch (err) { /* ignore */ }
  }
  document.body.style.userSelect = 'none'
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}
const modelId = ref(null)
const attrs = ref([])

// 左侧资源目录：按分组组织模型，支持按名称搜索
const catalog = computed(() => {
  const kw = catalogKeyword.value.trim().toLowerCase()
  const byGroup = {}
  for (const m of models.value) {
    if (kw && !m.name.toLowerCase().includes(kw)) continue
    ;(byGroup[m.group_id] = byGroup[m.group_id] || []).push(m)
  }
  return [...groups.value]
    .sort((a, b) => (a.index || 0) - (b.index || 0))
    .map((g) => ({ id: g.id, name: g.name, models: byGroup[g.id] || [] }))
    .filter((g) => g.models.length)
})
const currentModelName = computed(() => models.value.find((m) => m.id === modelId.value)?.name || '')
// B 手风琴：配合 el-menu 的 unique-opened，默认只展开当前模型所在分组（无则第一组）
const openedGroups = computed(() => {
  const hit = catalog.value.find((g) => (g.models || []).some((m) => String(m.id) === String(modelId.value)))
  const target = hit || catalog.value[0]
  return target ? ['g' + target.id] : []
})

function onCatalogSelect(index) {
  const id = Number(index)
  if (!id || id === modelId.value) return
  modelId.value = id
  onModelChange()
}
const list = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')
const selected = ref([])
const tableRef = ref(null)
function clearSelection() {
  tableRef.value?.clearSelection()
  selected.value = []
}
const loading = ref(false)
// 表格高度按实际内容自适应，仅设一个很小的兜底高度防止空列表时塌陷。
// （不再按「每页条数」预留满页高度——否则每页条数越大、未满页时下方留白越多。）
const tableStyle = computed(() => ({ minHeight: '180px' }))

// 字段筛选（多条件「字段+操作符+值」）
const showFilter = ref(false)
const assignedFilter = ref('') // ''=全部 / assigned / unassigned
const sourceFilter = ref('') // ''=全部 / manual / auto / 具体数据源 id
const dataSources = ref([]) // 数据源选项（用于「来源」按具体来源值分类筛选）
const bizFilter = ref('') // ''=全部 / 业务实例 id（按业务线归属筛选）
const bizLines = ref([]) // 业务线选项
// 仅启用字段参与实例表单/列/筛选展示（停用字段数据仍保留在实例中）
const enabledAttrs = computed(() => attrs.value.filter((a) => a.enabled !== false))
// 列表展示列：排除「列表隐藏」字段（字段仍可在表单/筛选/详情中使用）
const columnAttrs = computed(() => enabledAttrs.value.filter((a) => a.list_hidden !== true))

// 列设置：用户自选显示哪些列，按模型记忆（localStorage）
const visibleCols = ref([])
const colKey = computed(() => `cmdb_cols_${modelId.value}`)
function initCols() {
  const all = columnAttrs.value.map((a) => a.property_id)
  let saved = null
  try { saved = JSON.parse(localStorage.getItem(colKey.value) || 'null') } catch { saved = null }
  if (Array.isArray(saved) && saved.length) {
    // 只保留仍存在的列；若过滤后为空则回退全选
    const v = all.filter((id) => saved.includes(id))
    visibleCols.value = v.length ? v : all
  } else {
    visibleCols.value = all
  }
}
// 模型/字段变化时重新初始化列选择
watch(columnAttrs, initCols, { immediate: true })
// 选择变化时持久化（按模型）
watch(visibleCols, (v) => {
  if (!modelId.value) return
  try { localStorage.setItem(colKey.value, JSON.stringify(v)) } catch { /* 忽略 */ }
}, { deep: true })
function resetCols() {
  visibleCols.value = columnAttrs.value.map((a) => a.property_id)
}
// 实际渲染的列 = 候选列 ∩ 用户勾选
const shownColumnAttrs = computed(() => columnAttrs.value.filter((a) => visibleCols.value.includes(a.property_id)))
// 业务模型：在资源实例中只读（统一在业务拓扑管理）
const isBizModel = computed(
  () => models.value.find((m) => m.id === modelId.value)?.obj_id === 'biz',
)
const filterableAttrs = computed(() => enabledAttrs.value.filter((a) => a.type !== 'enummulti'))

// 筛选条件构建器
const advConditions = ref([]) // 已应用的条件（传给查询/导出）
const condRows = ref([emptyCond()]) // 编辑中的条件行
const activeFilterCount = computed(() => advConditions.value.length)
// A 方案：归属/业务线/数据源三个快捷过滤移入筛选面板，已选项在工具栏以可删除标签回显
const quickChips = computed(() => {
  const chips = []
  if (assignedFilter.value) {
    chips.push({
      key: 'assigned',
      label: '归属：' + (assignedFilter.value === 'assigned' ? '已归属' : '未归属'),
      clear: () => { assignedFilter.value = ''; reload() },
    })
  }
  if (bizFilter.value) {
    const b = bizLines.value.find((x) => String(x.id) === String(bizFilter.value))
    chips.push({
      key: 'biz',
      label: '业务线：' + (b ? b.name : bizFilter.value),
      clear: () => { bizFilter.value = ''; reload() },
    })
  }
  if (sourceFilter.value) {
    let label
    if (sourceFilter.value === 'manual') label = '用户创建'
    else if (sourceFilter.value === 'auto') label = '自动同步（全部）'
    else {
      const d = dataSources.value.find((x) => String(x.id) === String(sourceFilter.value))
      label = d ? d.name : sourceFilter.value
    }
    chips.push({ key: 'source', label: '数据源：' + label, clear: () => { sourceFilter.value = ''; reload() } })
  }
  return chips
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
function emptyCond() {
  return { field: '', op: 'eq', value: '', value2: '', values: [] }
}
function catOf(c) {
  const a = attrs.value.find((x) => x.property_id === c.field)
  if (!a) return 'text'
  if (a.type === 'enum') return 'enum'
  if (a.type === 'bool') return 'bool'
  if (a.type === 'int' || a.type === 'float') return 'number'
  if (a.type === 'date' || a.type === 'datetime') return 'date'
  return 'text'
}
function optOf(c) {
  const a = attrs.value.find((x) => x.property_id === c.field)
  return (a && a.options) || []
}
function opsFor(c) {
  return OPS[catOf(c)] || OPS.text
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
function addCond() {
  condRows.value.push(emptyCond())
}
// 收集有效条件
function validConds() {
  return condRows.value
    .filter((c) => {
      if (!c.field || !c.op) return false
      if (c.op === 'in') return (c.values || []).length > 0
      if (c.op === 'between' || c.op === 'daterange') return c.value !== '' && c.value2 !== ''
      return c.value !== '' && c.value !== null && c.value !== undefined
    })
    .map((c) => ({
      field: c.field,
      op: c.op,
      value: c.value != null ? String(c.value) : '',
      value2: c.value2 != null ? String(c.value2) : '',
      values: c.values || [],
    }))
}
function applyConds() {
  advConditions.value = validConds()
  reload()
}
function resetFilters() {
  condRows.value = [emptyCond()]
  advConditions.value = []
  // 快捷过滤也一并重置（归属/业务线/数据源）
  assignedFilter.value = ''
  bizFilter.value = ''
  sourceFilter.value = ''
  reload()
}

async function loadModels() {
  // 集群/模块是业务拓扑的内部结构，不作为资源类型在资源实例中管理；
  // 仅展示当前用户有「查看」权限的模型
  const [all, grps] = await Promise.all([mapi.listModels(), mapi.listGroups()])
  models.value = all.filter(
    // set/module 是拓扑内部模型，不进通用资源目录；停用(enabled=false)的模型也不展示
    (m) =>
      !['set', 'module'].includes(m.obj_id) &&
      m.enabled !== false &&
      userStore.hasPerm(m.id, 'view'),
  )
  groups.value = grps || []
  if (!models.value.length) return
  const qid = Number(route.query.model_id)
  const match = models.value.find((m) => m.id === qid)
  // 无指定 model_id 时默认展示主机，无主机权限再退回第一个模型
  const host = models.value.find((m) => m.obj_id === 'host')
  modelId.value = match ? qid : (host ? host.id : models.value[0].id)
  await onModelChange()
  // 从 URL 带入：adv=高级筛选条件 JSON；assigned=归属过滤（来自首页/运营分析跳转）
  let needReload = false
  if (route.query.adv) {
    try {
      const conds = JSON.parse(route.query.adv)
      if (Array.isArray(conds) && conds.length) {
        advConditions.value = conds
        // 填入可编辑条件行并展开筛选面板
        condRows.value = conds.map((c) => ({
          field: c.field || '',
          op: c.op || 'eq',
          value: c.value || '',
          value2: c.value2 || '',
          values: c.values || [],
        }))
        showFilter.value = true
        needReload = true
      }
    } catch (e) { /* 忽略非法参数 */ }
  }
  if (route.query.assigned === 'assigned' || route.query.assigned === 'unassigned') {
    assignedFilter.value = route.query.assigned
    needReload = true
  }
  if (needReload) {
    page.value = 1
    await load()
  }
}
async function onModelChange() {
  // 先清空列与数据，避免切换时旧列按容器宽铺开又缩回的抖动
  list.value = []
  attrs.value = []
  loading.value = true
  attrs.value = await mapi.listAttributes(modelId.value)
  condRows.value = [emptyCond()]
  advConditions.value = []
  assignedFilter.value = ''
  sourceFilter.value = ''
  bizFilter.value = ''
  keyword.value = ''
  page.value = 1
  await load()
}
function reload() {
  page.value = 1
  load()
}
async function load() {
  if (!modelId.value) return
  loading.value = true
  try {
    const res = await iapi.searchInstances({
      model_id: modelId.value,
      page: page.value,
      page_size: pageSize.value,
      keyword: keyword.value,
      conditions: advConditions.value,
      assigned: assignedFilter.value,
      source: sourceFilter.value,
      biz: bizFilter.value,
    })
    list.value = res.list || []
    total.value = res.total || 0
  } finally {
    loading.value = false
  }
}

// 固定布局：表格占满卡片剩余高度并内部滚动。EP 的 height 只认像素，
// 这里测量表格槽位(.table-wrap)的高度传给 el-table，并用 ResizeObserver 跟随
// 窗口缩放/筛选区展开/侧栏折叠等高度变化自动重算。
const tableWrapRef = ref(null)
const tableHeight = ref(0)
let tableRO = null
function measureTable() {
  const el = tableWrapRef.value
  if (el) tableHeight.value = el.clientHeight
}
onMounted(async () => {
  loadModels()
  try {
    dataSources.value = (await iapi.listDataSourceOptions()) || []
  } catch {
    dataSources.value = []
  }
  try {
    bizLines.value = (await tapi.listBusiness()) || []
  } catch {
    bizLines.value = []
  }
  await nextTick()
  measureTable()
  if (window.ResizeObserver && tableWrapRef.value) {
    tableRO = new ResizeObserver(() => measureTable())
    tableRO.observe(tableWrapRef.value)
  } else {
    window.addEventListener('resize', measureTable)
  }
})
onBeforeUnmount(() => {
  if (tableRO) { tableRO.disconnect(); tableRO = null }
  window.removeEventListener('resize', measureTable)
})

// 值展示
function optionName(a, id) {
  const o = (a.options || []).find((x) => x.id === id)
  return o ? o.name : id
}
// 按字段类型给列一个贴合内容的固定宽度（配合末尾占位列，字段少时不被拉宽）
function colWidth(a) {
  switch (a.type) {
    case 'bool':
      return 90
    case 'int':
    case 'float':
      return 110
    case 'date':
      return 120
    case 'datetime':
      return 170
    case 'enum':
      return 130
    case 'enummulti':
      return 180
    case 'longchar':
      return 220
    default:
      return 160
  }
}
function displayValue(a, val) {
  if (val === undefined || val === null || val === '') return '-'
  if (a.type === 'bool') return val ? '是' : '否'
  if (a.type === 'enum') return optionName(a, val)
  if (a.type === 'enummulti')
    return Array.isArray(val) ? val.map((v) => optionName(a, v)).join('，') : '-'
  return val
}

// 状态类枚举字段：带颜色展示
function hasVal(v) {
  return v !== undefined && v !== null && v !== ''
}
function isStatusAttr(a) {
  return a.type === 'enum' && (/status/i.test(a.property_id) || a.name.includes('状态'))
}
function statusTagType(label) {
  const s = String(label)
  if (/(running|正常|在线|运行|健康|可用|已上线|启用)/i.test(s)) return 'success'
  if (/(down|异常|离线|故障|错误|宕|不可用|失败)/i.test(s)) return 'danger'
  if (/(停用|待回收|闲置|未知|下线|维护)/i.test(s)) return 'info'
  return 'warning'
}

// 表单
const formDialog = ref(false)
const editing = ref(false)
const editId = ref(null)
const form = ref({})

function initForm(data) {
  const f = {}
  for (const a of attrs.value) {
    if (data && data[a.property_id] !== undefined) {
      f[a.property_id] = data[a.property_id]
    } else if (a.type === 'bool') {
      f[a.property_id] = false
    } else if (a.type === 'enummulti') {
      f[a.property_id] = []
    } else if (a.type === 'int' || a.type === 'float') {
      f[a.property_id] = undefined
    } else {
      f[a.property_id] = ''
    }
  }
  return f
}
function openForm(row) {
  editing.value = !!row
  editId.value = row?.id || null
  form.value = initForm(row?.data)
  formDialog.value = true
}
async function submitForm() {
  const data = {}
  for (const a of attrs.value) {
    const v = form.value[a.property_id]
    if (v === '' || v === null || v === undefined || (Array.isArray(v) && !v.length)) continue
    data[a.property_id] = v
  }
  try {
    if (editing.value) {
      await iapi.updateInstance(editId.value, { data })
    } else {
      await iapi.createInstance({ model_id: modelId.value, data })
    }
    ElMessage.success('保存成功')
    formDialog.value = false
    load()
  } catch {
    // 错误已由拦截器提示
  }
}
async function removeOne(row) {
  await ElMessageBox.confirm('确定删除该实例？', '提示', { type: 'warning' })
  await iapi.deleteInstance(row.id)
  ElMessage.success('已删除')
  load()
}
// 编辑对话框内删除当前实例
async function deleteFromForm() {
  await ElMessageBox.confirm('确定删除该实例？', '提示', { type: 'warning' })
  await iapi.deleteInstance(editId.value)
  ElMessage.success('已删除')
  formDialog.value = false
  load()
}
async function batchDelete() {
  await ElMessageBox.confirm(`确定删除选中的 ${selected.value.length} 条实例？`, '提示', {
    type: 'warning',
  })
  await iapi.batchDeleteInstances(selected.value.map((r) => r.id))
  ElMessage.success('已删除')
  load()
}

// ---- 关联关系 ----
const relDialog = ref(false)
const relInstance = ref(null)
const relList = ref([])
const relLoading = ref(false)
const allAssocs = ref([])
const kinds = ref([])
const newRel = ref({ assocId: null, peerIds: [] })
const relSelected = ref([]) // 已有关联表格多选
const peerOptions = ref([])
let peerAttrsCache = {}

const relTitle = computed(() => {
  const r = relInstance.value
  if (!r) return ''
  const a = enabledAttrs.value[0]
  const v = a ? displayValue(a, r.data?.[a.property_id]) : ''
  return v && v !== '-' ? v : `#${r.id}`
})

// 当前模型可用的关联（当前模型作为源或目标），带方向语义文案
const applicableAssocs = computed(() => {
  const mid = modelId.value
  const kindOf = (id) => kinds.value.find((k) => k.id === id) || {}
  const modelOf = (id) => models.value.find((m) => m.id === id)?.name || `#${id}`
  const out = []
  for (const a of allAssocs.value) {
    const k = kindOf(a.kind_id)
    if (a.src_model_id === mid) {
      out.push({ id: a.id, label: `${k.src_to_dst_note || k.name} ${modelOf(a.dst_model_id)}`, peerModelId: a.dst_model_id, asSrc: true })
    }
    if (a.dst_model_id === mid) {
      out.push({ id: a.id, label: `${k.dst_to_src_note || k.name} ${modelOf(a.src_model_id)}`, peerModelId: a.src_model_id, asSrc: false })
    }
  }
  return out
})

async function openRelations(row) {
  relInstance.value = row
  newRel.value = { assocId: null, peerIds: [] }
  peerOptions.value = []
  relDialog.value = true
  if (!allAssocs.value.length) allAssocs.value = await mapi.listAssociations()
  if (!kinds.value.length) kinds.value = await mapi.listKinds()
  await loadRelations()
}
async function loadRelations() {
  relLoading.value = true
  try {
    relList.value = await iapi.listRelations(relInstance.value.id)
  } finally {
    relLoading.value = false
  }
}

// 用对端模型的前两个启用字段拼实例展示名
function peerLabel(inst, attrs) {
  const parts = []
  for (const a of attrs) {
    const v = inst.data?.[a.property_id]
    if (v !== undefined && v !== null && v !== '') {
      parts.push(displayValue(a, v))
      if (parts.length >= 2) break
    }
  }
  return parts.length ? parts.join(' / ') : `#${inst.id}`
}
async function onAssocChange(assocId) {
  newRel.value.peerIds = []
  peerOptions.value = []
  const opt = applicableAssocs.value.find((o) => o.id === assocId)
  if (!opt) return
  const pmId = opt.peerModelId
  if (!peerAttrsCache[pmId]) {
    peerAttrsCache[pmId] = (await mapi.listAttributes(pmId)).filter((a) => a.enabled !== false)
  }
  const attrs = peerAttrsCache[pmId]
  const res = await iapi.searchInstances({ model_id: pmId, page: 1, page_size: 200, keyword: '' })
  // 排除该关联下已关联的对端实例，避免重复关联
  const related = new Set(
    relList.value.filter((r) => r.association_id === opt.id).map((r) => r.peer_inst_id),
  )
  peerOptions.value = (res.list || [])
    .filter((inst) => !related.has(inst.id))
    .map((inst) => ({ id: inst.id, label: peerLabel(inst, attrs) }))
}
async function submitRelation() {
  const opt = applicableAssocs.value.find((o) => o.id === newRel.value.assocId)
  if (!opt) return
  const selfId = relInstance.value.id
  const peerIds = [...newRel.value.peerIds]
  if (!peerIds.length) return
  let ok = 0
  let fail = 0
  for (const peerId of peerIds) {
    const payload = opt.asSrc
      ? { association_id: opt.id, src_inst_id: selfId, dst_inst_id: peerId }
      : { association_id: opt.id, src_inst_id: peerId, dst_inst_id: selfId }
    try {
      await iapi.createRelation(payload)
      ok++
    } catch {
      fail++ // 拦截器已提示（如重复关联等），这里仅计数
    }
  }
  if (ok) ElMessage.success(`已添加 ${ok} 条关联${fail ? `，${fail} 条失败/已存在` : ''}`)
  else if (fail) ElMessage.warning(`${fail} 条未添加（可能已存在）`)
  newRel.value.peerIds = []
  await loadRelations()
  // 刷新可选对端：剔除刚添加的，避免再次重复关联
  if (newRel.value.assocId) await onAssocChange(newRel.value.assocId)
}
async function batchDeleteRelations() {
  const rows = relSelected.value.filter((r) => !r.derived)
  if (!rows.length) return
  await ElMessageBox.confirm(`确定删除选中的 ${rows.length} 条关联？`, '提示', { type: 'warning' })
  let ok = 0
  let fail = 0
  for (const row of rows) {
    try {
      await iapi.deleteRelation(row.id)
      ok++
    } catch {
      fail++
    }
  }
  if (ok) ElMessage.success(`已删除 ${ok} 条${fail ? `，${fail} 条失败` : ''}`)
  else if (fail) ElMessage.error(`删除失败（${fail} 条）`)
  relSelected.value = []
  await loadRelations()
}
async function removeRelation(row) {
  await ElMessageBox.confirm('确定删除该关联？', '提示', { type: 'warning' })
  await iapi.deleteRelation(row.id)
  ElMessage.success('已删除')
  await loadRelations()
}

// 导入导出
const reportDialog = ref(false)
const importReport = ref({ success: 0, failed: 0, errors: [] })
const exportDialog = ref(false)
const exportScope = ref('current') // current / selected / all

function triggerDownload(blob, filename) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}
// 打开导出弹层：有勾选时默认「当前所选」，否则「当前模型」
function openExportDialog() {
  exportScope.value = selected.value.length ? 'selected' : 'current'
  exportDialog.value = true
}
// 按选定范围导出
async function runExport() {
  const scope = exportScope.value
  exportDialog.value = false
  if (scope === 'all') return doExportAll()
  const ids = scope === 'selected' ? selected.value.map((r) => r.id) : undefined
  const blob = await iapi.exportInstances({
    model_id: modelId.value,
    keyword: keyword.value,
    conditions: advConditions.value,
    assigned: assignedFilter.value,
    source: sourceFilter.value,
    biz: bizFilter.value,
    ids,
  })
  triggerDownload(blob, `实例导出_${Date.now()}.xlsx`)
}
// 批量选择条「导出所选」：直接导出勾选项，不弹层
async function exportSelected() {
  const ids = selected.value.map((r) => r.id)
  if (!ids.length) return
  const blob = await iapi.exportInstances({ model_id: modelId.value, ids })
  triggerDownload(blob, `实例导出_${Date.now()}.xlsx`)
}
// 全部资源：全部来源、你有查看权限的全部模型（每个模型一个 sheet）
async function doExportAll() {
  const loading = ElLoading.service({ text: '正在导出全部数据…' })
  try {
    const blob = await iapi.exportAllResources()
    triggerDownload(blob, `全部资源导出_${Date.now()}.xlsx`)
    ElMessage.success('导出完成')
  } finally {
    loading.close()
  }
}
async function doTemplate() {
  const blob = await iapi.downloadTemplate(modelId.value)
  triggerDownload(blob, '导入模板.xlsx')
}
async function doImport(options) {
  const fd = new FormData()
  fd.append('model_id', modelId.value)
  fd.append('file', options.file)
  try {
    importReport.value = await iapi.importInstances(fd)
    reportDialog.value = true
    load()
  } catch {
    // 错误已由拦截器提示
  }
}

// 「数据操作」下拉
const fileInput = ref(null)
function onDataCommand(cmd) {
  if (cmd === 'export') openExportDialog()
  else if (cmd === 'template') doTemplate()
  else if (cmd === 'import') fileInput.value && fileInput.value.click()
  else if (cmd === 'batchDelete') batchDelete()
}
function onFilePick(e) {
  const file = e.target.files && e.target.files[0]
  if (file) doImport({ file })
  e.target.value = '' // 允许重复选择同一文件
}
</script>

<style scoped>
.inst-page {
  /* 固定外壳：本页不走整页滚动，卡片撑满 .content 可视区，由表格内部滚动 */
  height: 100%;
}
/* A 方案：把资源类型栏(②)与表格区(③)合并成同一张卡片，
   消除"第二条侧栏"带来的三栏割裂感。.inst-body 作为唯一卡片外壳。 */
.inst-body {
  height: 100%;
  background: var(--card-bg);
  border: 1px solid var(--card-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-card);
  overflow: hidden; /* 定高卡片：裁掉圆角外内容，内部各自滚动 */
}
/* 资源类型栏弱化为卡片内的左侧分区：去掉独立卡片外观，仅留一条极浅分隔线 */
.catalog {
  border: none;
  border-right: 1px solid var(--border-light);
  border-radius: 0;
  background: transparent;
  box-shadow: none;
  display: flex;
  flex-direction: column;
  margin-right: 0;
  overflow: hidden;
  /* 卡片已定高：左栏 stretch 填满整列高度，菜单过长由内部 catalog-scroll 滚动 */
}
.catalog-head {
  padding: 14px 16px 8px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.3px;
  color: var(--text-secondary);
}
.catalog-search {
  padding: 0 12px 12px;
  border-bottom: 1px solid var(--border-light);
}
.catalog-scroll {
  flex: 1;
  /* 关键：弹性子项默认 min-height:auto 会被内容撑满、溢出被裁切导致底部菜单(如"大数据")
     既看不到也滚不到；置 0 才能收缩到限高容器内并由内部滚动条滚动 */
  min-height: 0;
}
.catalog-menu {
  border-right: none;
}
.cat-icon {
  margin-right: 8px;
  font-size: 16px;
}
/* 主体表格卡片 */
.inst-card {
  padding: 16px 18px;
  /* 定高 flex 列：工具栏/筛选/分页固定，表格占满剩余空间内部滚动 */
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.toolbar,
.batch-bar,
.filter-panel {
  flex-shrink: 0;
}
/* 表格槽位：吃掉剩余高度，给 el-table 提供确定的可测量高度 */
.table-wrap {
  flex: 1;
  min-height: 0;
}
/* 批量选择操作条 */
.batch-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 14px;
  margin-bottom: 14px;
  background: var(--brand-light);
  border: 1px solid #cdddfb;
  border-radius: var(--radius);
}
.batch-count {
  font-size: 13px;
  color: var(--text-regular);
}
.batch-count b {
  color: var(--brand);
}
.batch-cancel {
  margin-left: auto;
  color: var(--text-secondary);
}
.cat-group {
  font-weight: 600;
}
.resizer {
  width: 6px;
  /* 压在分隔线上，便于拖动，不额外占出一列空隙 */
  margin: 0 2px 0 -4px;
  cursor: col-resize;
  border-radius: 3px;
  flex-shrink: 0;
  background: transparent;
  transition: background 0.2s;
}
.resizer:hover {
  background: #c0c4cc;
}
.inst-main {
  padding: 0;
  overflow: hidden;
  /* el-main 默认无 min-width:0，弹性项会被宽表格的最小内容宽度撑开、
     溢出作为卡片的 el-container；置 0 让它收缩到卡片内，由 el-table 自身横向滚动 */
  min-width: 0;
  /* 固定布局：撑满卡片高度，内部纵向排布工具栏/表格/分页 */
  display: flex;
  flex-direction: column;
}
.cur-model {
  font-size: 16px;
  font-weight: 600;
  display: flex;
  align-items: center;
  margin-right: 6px;
  white-space: nowrap;
}
.toolbar {
  display: flex;
  gap: 10px;
  margin-bottom: 16px;
  flex-wrap: wrap;
  align-items: center;
}
/* B 分组：左区(标识+搜索+筛选)自适应，右区(操作)靠右成组；
   窄屏时整组换行而非单个控件零散换行 */
.tb-left {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1 1 420px;
  min-width: 0;
  flex-wrap: wrap;
}
.tb-right {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-left: auto;
  flex-shrink: 0;
}
.tb-kw {
  flex: 1 1 180px;
  min-width: 150px;
  max-width: 300px;
}
.tb-chip {
  max-width: 220px;
}
.tb-spacer {
  flex: 1 1 auto;
  min-width: 12px;
}
/* 快捷过滤行（归属/业务线/数据源）置于筛选面板顶部 */
.quick-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 12px;
  padding-bottom: 12px;
  border-bottom: 1px dashed var(--border-light);
}
.tb-model {
  font-size: 15px;
  font-weight: 700;
  color: var(--text-primary);
  white-space: nowrap;
  margin-left: 2px;
}
.tb-count {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 8px;
  margin-left: -2px;
  border-radius: 999px;
  background: var(--fill-bg);
  color: var(--text-secondary);
  font-size: 12.5px;
  font-weight: 600;
}
.filter-panel {
  background: #f7f8fa;
  border: 1px solid #ebeef5;
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 16px;
}
.cond-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}
.c-field {
  width: 180px;
  flex-shrink: 0;
}
.c-op {
  width: 110px;
  flex-shrink: 0;
}
.c-val {
  width: 320px;
  display: flex;
  gap: 6px;
}
.filter-actions {
  margin-top: 6px;
}
.form-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
/* 表格底部横向滚动条：加粗、常显、好抓 */
:deep(.el-table .el-scrollbar__bar.is-horizontal) {
  height: 9px;
}
:deep(.el-table .el-scrollbar__bar.is-horizontal .el-scrollbar__thumb) {
  height: 7px;
  border-radius: 4px;
  background-color: #a3abb8;
  opacity: 0.85;
  transition: background-color 0.15s, opacity 0.15s, height 0.15s;
}
/* 悬停时略微加粗更好抓，但不悬停时保持纤细不挡数据 */
:deep(.el-table .el-scrollbar__bar.is-horizontal:hover) {
  height: 12px;
}
:deep(.el-table .el-scrollbar__bar.is-horizontal:hover .el-scrollbar__thumb) {
  height: 10px;
}
:deep(.el-table .el-scrollbar__bar.is-horizontal:hover .el-scrollbar__thumb),
:deep(.el-table .el-scrollbar__bar.is-horizontal .el-scrollbar__thumb:hover) {
  background-color: var(--brand, #2468f2);
  opacity: 1;
}
html.dark :deep(.el-table .el-scrollbar__bar.is-horizontal .el-scrollbar__thumb) {
  background-color: #5a6372;
}
/* scrollbar-always-on 会给 wrap 加 --hidden-default，使 EP 默认的 15px 底部留白失效，
   常显横向条便压住最后一行；这里补回底部内边距，让滚动条落在数据行下方的空白里 */
:deep(.el-table .el-table__body-wrapper .el-scrollbar__wrap) {
  padding-bottom: 14px;
}
.col-pop-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-regular);
  margin-bottom: 8px;
}
.col-pop-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 320px;
  overflow-y: auto;
}
.pager {
  flex-shrink: 0;
  margin-top: 12px;
  /* 固定在表格下方、整体右对齐 */
  justify-content: flex-end;
  margin-left: auto;
  max-width: 100%;
  flex-wrap: wrap;
  gap: 8px 0;
  padding: 2px 0 0;
}
.exp-scope {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 14px;
}
.exp-scope :deep(.el-radio) {
  margin: 0;
  height: auto;
  width: 100%;
  display: flex;
  align-items: center;
}
.exp-scope :deep(.el-radio__label) {
  white-space: normal;
}
.err-list {
  margin-top: 12px;
  max-height: 260px;
  overflow-y: auto;
}
.err-item {
  color: #f56c6c;
  font-size: 13px;
  margin: 2px 0;
}
.rel-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  margin-bottom: 16px;
  background: var(--fill-bg-soft);
  border: 1px solid var(--border-light);
  border-radius: var(--radius);
}
.rel-head-label {
  font-size: 12.5px;
  color: var(--text-muted);
}
.rel-head-name {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-primary);
}
.rel-section {
  margin-bottom: 18px;
}
.rel-section:last-child {
  margin-bottom: 0;
}
.rel-section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 10px;
}
.rel-count {
  display: inline-flex;
  align-items: center;
  height: 18px;
  padding: 0 7px;
  border-radius: 999px;
  background: var(--fill-bg);
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
}
.rel-batch-btn {
  margin-left: auto;
}
.rel-create {
  display: flex;
  gap: 10px;
  align-items: center;
}
.rel-table {
  margin-top: 4px;
}
.rel-dir {
  margin-right: 3px;
  font-weight: 700;
}
.rel-peer {
  color: var(--text-regular);
}
.muted {
  color: var(--text-muted);
}
.rel-tag {
  margin-left: 4px;
}
.biz-hint {
  color: #909399;
  font-size: 13px;
  align-self: center;
}
.rel-alert {
  margin-bottom: 14px;
}
</style>
