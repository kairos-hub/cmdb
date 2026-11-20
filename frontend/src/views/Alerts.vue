<template>
  <div>
    <div class="page-header">
      <div>
        <div class="ph-title">告警事件</div>
        <div class="ph-sub">各告警源的报警事件，按资源关联到 CMDB 资产</div>
      </div>
      <div class="ph-actions">
        <el-button :icon="Refresh" @click="load">刷新</el-button>
        <el-dropdown v-if="canManage" trigger="click" @command="onClean">
          <el-button>清理<el-icon class="el-icon--right"><ArrowDown /></el-icon></el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="7">清理 7 天前</el-dropdown-item>
              <el-dropdown-item command="30">清理 30 天前</el-dropdown-item>
              <el-dropdown-item command="90">清理 90 天前</el-dropdown-item>
              <el-dropdown-item command="all" divided>清空全部</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </div>

    <!-- 统计 -->
    <div class="stat-row">
      <div class="stat-card" @click="quickFilter('level', '')"><div class="sc-ic ic-blue"><el-icon><Bell /></el-icon></div><div><div class="sc-label">总告警</div><div class="sc-val">{{ total }}</div></div></div>
      <div class="stat-card" @click="quickFilter('level', 'critical')"><div class="sc-ic ic-red"><el-icon><CircleCloseFilled /></el-icon></div><div><div class="sc-label">紧急</div><div class="sc-val">{{ counts.critical }}</div></div></div>
      <div class="stat-card" @click="quickFilter('level', 'warning')"><div class="sc-ic ic-orange"><el-icon><WarningFilled /></el-icon></div><div><div class="sc-label">警告</div><div class="sc-val">{{ counts.warning }}</div></div></div>
      <div class="stat-card" @click="quickFilter('status', 'firing')"><div class="sc-ic ic-purple"><el-icon><AlarmClock /></el-icon></div><div><div class="sc-label">未恢复</div><div class="sc-val">{{ counts.firing }}</div></div></div>
    </div>

    <div class="panel panel-pad">
      <!-- 筛选 -->
      <div class="filters">
        <el-input v-model="q.keyword" placeholder="搜索规则 / 资源 / 指标" clearable :prefix-icon="Search" style="width: 220px" @keyup.enter="reload" @clear="reload" />
        <el-select v-model="q.level" placeholder="级别" clearable style="width: 110px" @change="reload">
          <el-option label="紧急" value="critical" /><el-option label="警告" value="warning" />
          <el-option label="提示" value="info" /><el-option label="恢复" value="ok" />
        </el-select>
        <el-select v-model="q.status" placeholder="状态" clearable style="width: 110px" @change="reload">
          <el-option label="未恢复" value="firing" /><el-option label="已恢复" value="resolved" />
        </el-select>
        <el-select v-model="q.source" placeholder="数据源" clearable filterable style="width: 160px" @change="reload">
          <el-option v-for="d in sources" :key="d.id" :label="d.name" :value="d.id" />
        </el-select>
        <el-select v-model="q.days" placeholder="时间范围" style="width: 120px" @change="reload">
          <el-option label="近 24 小时" :value="1" /><el-option label="近 7 天" :value="7" />
          <el-option label="近 30 天" :value="30" /><el-option label="近 1 年" :value="365" />
        </el-select>
        <el-button :icon="Search" @click="reload">查询</el-button>
      </div>

      <el-table :data="list" v-loading="loading">
        <el-table-column label="级别" width="90">
          <template #default="{ row }">
            <span class="soft-tag" :class="levelCls(row.level)"><span class="dot"></span>{{ levelText(row.level) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="规则" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="rule-name">{{ row.rule_name || '-' }}</div>
            <div class="muted sm">{{ [row.product, row.metric_name].filter(Boolean).join(' · ') || '—' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="资源" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <el-link v-if="row.res_inst_id" type="primary" @click="goDetail(row.res_inst_id)">{{ row.instance_name || row.instance_id }}</el-link>
            <span v-else>{{ row.instance_name || row.instance_id || '-' }}</span>
            <div class="muted sm">{{ [row.res_model || (row.instance_id ? '未纳管' : ''), row.dimensions].filter(Boolean).join(' · ') }}</div>
          </template>
        </el-table-column>
        <el-table-column label="触发条件" min-width="150">
          <template #default="{ row }">
            <div v-if="row.trigger_value">触发值 {{ row.trigger_value }}</div>
            <el-tooltip v-if="row.expression" :content="row.expression" placement="top" :show-after="300">
              <div class="muted sm expr-ellipsis">{{ row.expression }}</div>
            </el-tooltip>
            <span v-else-if="!row.trigger_value">—</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <span class="soft-tag" :class="row.status === 'resolved' ? 'is-success' : 'is-danger'">{{ row.status === 'resolved' ? '已恢复' : '未恢复' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="数据源" width="130" show-overflow-tooltip>
          <template #default="{ row }">{{ row.source_name || '—' }}</template>
        </el-table-column>
        <el-table-column label="触发时间" width="170">
          <template #default="{ row }">{{ formatTime(row.alert_time) }}</template>
        </el-table-column>
        <el-table-column label="恢复时间" width="170">
          <template #default="{ row }">{{ row.status === 'resolved' && row.resolved_at ? formatTime(row.resolved_at) : '—' }}</template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!loading && !list.length" description="暂无告警（在「系统管理 → 告警配置」接入云告警源或第三方回调）" />

      <el-pagination
        v-if="total > q.page_size"
        class="pager"
        background
        layout="total, prev, pager, next, sizes"
        :total="total"
        :current-page="q.page"
        :page-size="q.page_size"
        :page-sizes="[20, 50, 100]"
        @current-change="(p) => { q.page = p; load() }"
        @size-change="(s) => { q.page_size = s; q.page = 1; load() }"
      />
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Search, ArrowDown, Bell, WarningFilled, CircleCloseFilled, AlarmClock } from '@element-plus/icons-vue'
import * as api from '@/api/system'
import { useUserStore } from '@/store/user'

const router = useRouter()
const userStore = useUserStore()
const canManage = () => userStore.hasFunc('datasource_manage')

const list = ref([])
const total = ref(0)
const loading = ref(false)
const sources = ref([])
const counts = reactive({ critical: 0, warning: 0, firing: 0 })
const q = reactive({ keyword: '', level: '', status: '', source: '', days: 7, page: 1, page_size: 20 })

function levelText(l) { return { critical: '紧急', warning: '警告', info: '提示', ok: '恢复' }[l] || l }
function levelCls(l) { return { critical: 'is-danger', warning: 'is-warning', info: 'is-info', ok: 'is-success' }[l] || 'is-muted' }
function formatTime(t) { return t ? new Date(t).toLocaleString('zh-CN', { hour12: false }) : '-' }

async function load() {
  loading.value = true
  try {
    const params = { page: q.page, page_size: q.page_size, days: q.days, sort: 'activity' }
    if (q.keyword) params.keyword = q.keyword
    if (q.level) params.level = q.level
    if (q.status) params.status = q.status
    if (q.source) params.source = q.source
    const res = await api.listAlerts(params)
    list.value = res.list || []
    total.value = res.total || 0
  } finally {
    loading.value = false
  }
  loadCounts()
}
function reload() { q.page = 1; load() }
function quickFilter(key, val) {
  q.level = key === 'level' ? val : ''
  q.status = key === 'status' ? val : ''
  reload()
}

// 统计卡片：各维度条数（独立轻量查询，仅取 total）
async function loadCounts() {
  const base = { page: 1, page_size: 1, days: q.days }
  const [c1, c2, c3] = await Promise.all([
    api.listAlerts({ ...base, level: 'critical' }),
    api.listAlerts({ ...base, level: 'warning' }),
    api.listAlerts({ ...base, status: 'firing' }),
  ])
  counts.critical = c1.total || 0
  counts.warning = c2.total || 0
  counts.firing = c3.total || 0
}

async function onClean(cmd) {
  const days = cmd === 'all' ? 0 : Number(cmd)
  const srcName = q.source ? (sources.value.find((s) => s.id === q.source)?.name || '') : ''
  const scope = q.source ? `数据源「${srcName}」的` : '全部数据源的'
  const label = cmd === 'all' ? `清空${scope}告警` : `清理${scope} ${days} 天前的告警`
  await ElMessageBox.confirm(`确定${label}？此操作不可恢复。`, '提示', { type: 'warning' })
  const r = await api.cleanupAlerts(days, q.source || undefined)
  ElMessage.success(`已删除 ${r.deleted} 条`)
  reload()
}
function goDetail(id) { router.push(`/instances/${id}`) }

onMounted(async () => {
  try { sources.value = await api.listAlertSourceOptions() } catch { sources.value = [] }
  load()
})
</script>

<style scoped>
.stat-row { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; margin-bottom: 14px; }
.stat-card { display: flex; align-items: center; gap: 14px; background: var(--card-bg); border: 1px solid var(--card-border); border-radius: var(--radius-lg); box-shadow: var(--shadow-card); padding: 16px 18px; cursor: pointer; transition: box-shadow 0.15s; }
.stat-card:hover { box-shadow: 0 4px 14px rgba(36, 104, 242, 0.12); }
.sc-ic { width: 44px; height: 44px; border-radius: 11px; display: flex; align-items: center; justify-content: center; font-size: 22px; flex-shrink: 0; }
.ic-blue { background: #eaf1ff; color: #3b82f6; }
.ic-red { background: #fdeaea; color: #dc2626; }
.ic-orange { background: #fff2e6; color: #f59e0b; }
.ic-purple { background: #f3eefe; color: #8b5cf6; }
.sc-label { font-size: 13px; color: var(--text-secondary); margin-bottom: 4px; }
.sc-val { font-size: 24px; font-weight: 700; color: var(--text-primary); line-height: 1; }
.filters { display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 14px; }
.rule-name { font-weight: 500; color: var(--text-primary); }
.muted { color: var(--text-secondary); }
.sm { font-size: 12px; margin-top: 2px; }
.expr-ellipsis { max-width: 240px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; cursor: default; }
.pager { margin-top: 14px; display: flex; justify-content: flex-end; }
</style>
