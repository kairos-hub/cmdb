<template>
  <div class="dash">
    <div class="page-header">
      <div>
        <div class="ph-sub">
          数据更新于 {{ nowText }} · 来源：Agent 采集 + 云厂商同步 + 手工维护
        </div>
      </div>
      <div class="ph-actions">
        <el-button type="primary" @click="router.push('/instances')">查看资源实例</el-button>
      </div>
    </div>

    <!-- KPI -->
    <div class="kpi-row">
      <div class="stat-card">
        <span class="stat-icon is-info"><el-icon><Coin /></el-icon></span>
        <div class="stat-body">
          <div class="stat-label">资源实例总数</div>
          <div class="stat-value">{{ fmt(ov.instance_count) }}</div>
          <div class="stat-sub">覆盖 {{ ov.model_count || 0 }} 类模型</div>
        </div>
      </div>
      <div class="stat-card">
        <span class="stat-icon is-success"><el-icon><Monitor /></el-icon></span>
        <div class="stat-body">
          <div class="stat-label">主机资产</div>
          <div class="stat-value">{{ fmt(ov.host_count) }}</div>
          <div class="stat-sub" :class="hostOnline.cls">{{ hostOnline.text }}</div>
        </div>
      </div>
      <div class="stat-card">
        <span class="stat-icon is-warning"><el-icon><OfficeBuilding /></el-icon></span>
        <div class="stat-body">
          <div class="stat-label">业务系统</div>
          <div class="stat-value">{{ fmt(ov.business_count) }}</div>
          <div class="stat-sub">已纳入拓扑管理</div>
        </div>
      </div>
      <div class="stat-card">
        <span class="stat-icon is-danger"><el-icon><WarningFilled /></el-icon></span>
        <div class="stat-body">
          <div class="stat-label">未纳管资产</div>
          <div class="stat-value">{{ hasAnalysis ? fmt(analysis.unassigned_total) : '—' }}</div>
          <div class="stat-sub is-warn">尚未分配到业务线</div>
        </div>
      </div>
    </div>

    <!-- 主区 -->
    <div class="grid-2">
      <div class="panel panel-pad">
        <div class="panel-head">
          <span class="panel-title">资产类型分布</span>
          <span class="muted-link" @click="router.push('/models')">模型管理</span>
        </div>
        <div v-show="assetDist.length" ref="assetChartEl" class="pie-chart"></div>
        <el-empty v-if="!assetDist.length" :image-size="72" description="暂无数据" />
      </div>

      <div class="panel panel-pad">
        <div class="panel-head">
          <span class="panel-title">业务系统分布</span>
        </div>
        <div v-show="bizDist.length" ref="bizChartEl" class="pie-chart"></div>
        <el-empty v-if="!bizDist.length" :image-size="72" description="暂无业务归属数据" />
      </div>

      <div class="panel panel-pad">
        <div class="panel-head">
          <span class="panel-title">主机状态分布</span>
        </div>
        <div v-show="hostStatus.length" ref="statusChartEl" class="pie-chart"></div>
        <el-empty v-if="!hostStatus.length" :image-size="72" description="暂无状态数据" />
      </div>

      <div class="panel panel-pad">
        <div class="panel-head">
          <span class="panel-title">告警资源分布</span>
          <span class="muted-link" @click="router.push('/alerts')">告警事件</span>
        </div>
        <div v-show="productDist.length" ref="alertChartEl" class="pie-chart"></div>
        <el-empty v-if="!productDist.length" :image-size="72" description="暂无告警" />
      </div>
    </div>

    <!-- 各模型实例数（左） + 最近告警事件（右）同行 -->
    <div class="grid-bottom">
      <div class="panel panel-pad models-panel">
        <div class="panel-head">
          <span class="panel-title">各模型实例数</span>
          <span class="muted-link" @click="router.push('/models')">模型管理</span>
        </div>
        <div v-if="modelInst.length" class="model-grid">
          <div
            v-for="m in modelInst"
            :key="m.name"
            class="model-cell"
            @click="goModel(m)"
          >
            <span class="mc-left">
              <span class="mc-icon">
                <ModelIcon :obj-id="modelMeta[m.model_id]?.obj_id" :fallback="modelMeta[m.model_id]?.icon || 'Document'" :size="18" />
              </span>
              <span class="mc-name">{{ m.name }}</span>
            </span>
            <span class="mc-count">{{ fmt(m.count) }}</span>
          </div>
        </div>
        <el-empty v-else :image-size="72" description="暂无数据" />
      </div>

      <div class="panel panel-pad alerts-panel">
        <div class="panel-head">
          <span class="panel-title">最近告警事件</span>
          <span class="ap-right">
            <span v-if="firingTotal" class="firing-badge">未恢复 {{ fmt(firingTotal) }}</span>
            <span class="muted-link" @click="router.push('/alerts')">查看全部</span>
          </span>
        </div>
        <div
          v-if="alerts.length"
          ref="alertListEl"
          class="alert-list"
          @mouseenter="scrollPaused = true"
          @mouseleave="scrollPaused = false"
        >
          <div
            v-for="a in alerts"
            :key="a.id"
            class="alert-row"
            :class="{ clickable: a.res_inst_id }"
            @click="a.res_inst_id && router.push(`/instances/${a.res_inst_id}`)"
          >
            <span class="soft-tag al-level" :class="levelCls(a.level)"><span class="dot"></span>{{ levelText(a.level) }}</span>
            <span class="al-rule" :title="a.rule_name">{{ a.rule_name || '—' }}</span>
            <span class="al-res" :title="a.instance_name || a.instance_id">{{ a.instance_name || a.instance_id || '—' }}</span>
            <span class="soft-tag al-status" :class="a.status === 'resolved' ? 'is-success' : 'is-danger'">{{ a.status === 'resolved' ? '已恢复' : '未恢复' }}</span>
            <span class="al-time">{{ fmtTime(a.status === 'resolved' && a.resolved_at ? a.resolved_at : a.alert_time) }}</span>
          </div>
        </div>
        <el-empty v-else :image-size="72" description="暂无告警事件" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import * as echarts from 'echarts'
import { getOverview, getAnalysis } from '@/api/stats'
import { listAlerts, listAlertProducts } from '@/api/system'
import { listModels } from '@/api/model'
import ModelIcon from '@/components/ModelIcon.vue'

// 分类色（与运营分析页一致）
const PALETTE = ['#2f66f0', '#16a34a', '#d97706', '#7c3aed', '#db2777', '#0891b2', '#e34948', '#64748b']
const bizChartEl = ref(null)
const statusChartEl = ref(null)
const assetChartEl = ref(null)
const alertChartEl = ref(null)
let bizChart = null
let statusChart = null
let assetChart = null
let alertChart = null

const router = useRouter()
// model_id → {obj_id, icon}，用于「各模型实例数」展示图标
const modelMeta = ref({})
const ov = ref({ model_count: 0, instance_count: 0, business_count: 0, host_count: 0 })
const analysis = ref(null)
const hasAnalysis = computed(() => !!analysis.value)

// 最近告警事件
const alerts = ref([])
const firingTotal = ref(0)
// 列表自动滚动（溢出时轮播，悬浮暂停）
const alertListEl = ref(null)
const scrollPaused = ref(false)
let alertScrollTimer = null
function startAlertScroll() {
  stopAlertScroll()
  alertScrollTimer = setInterval(() => {
    const el = alertListEl.value
    if (!el || scrollPaused.value) return
    if (el.scrollHeight - el.clientHeight <= 2) return // 不溢出不滚
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - 1) {
      el.scrollTop = 0 // 到底回到顶部循环
    } else {
      el.scrollTop += 1
    }
  }, 60)
}
function stopAlertScroll() {
  if (alertScrollTimer) {
    clearInterval(alertScrollTimer)
    alertScrollTimer = null
  }
}
// 告警产品分布（扇形图）
const productDist = ref([])
const ALERT_LEVELS = { critical: ['严重', 'is-danger'], warning: ['警告', 'is-warning'], info: ['提示', 'is-info'], ok: ['恢复', 'is-success'] }
const levelText = (l) => (ALERT_LEVELS[l] || [l || '—'])[0]
const levelCls = (l) => (ALERT_LEVELS[l] || ['', 'is-muted'])[1]
const fmtTime = (t) => (t ? new Date(t).toLocaleString('zh-CN') : '—')

const nowText = computed(() => {
  const d = new Date()
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
})

const fmt = (n) => (n == null ? '0' : Number(n).toLocaleString('en-US'))
function goModel(m) {
  if (m.model_id) router.push({ path: '/instances', query: { model_id: m.model_id } })
  else router.push('/instances')
}
const pct = (v, max) => (max > 0 ? `${Math.max(4, Math.round((v / max) * 100))}%` : '0%')

const bizDist = computed(() => (analysis.value?.biz_hosts_all || []).slice(0, 8))
const bizMax = computed(() => Math.max(1, ...bizDist.value.map((b) => b.count)))

const hostStatus = computed(() => analysis.value?.host_status || [])
const hostStatusMax = computed(() => Math.max(1, ...hostStatus.value.map((s) => s.count)))

const modelInst = computed(() => (analysis.value?.model_instances || []).slice(0, 12))

// 资产类型分布：按模型实例数，取前 8，其余并入「其他」
const assetDist = computed(() => {
  const all = analysis.value?.model_instances || []
  if (all.length <= 9) return all
  const top = all.slice(0, 8)
  const rest = all.slice(8).reduce((s, m) => s + (m.count || 0), 0)
  return rest > 0 ? [...top, { name: '其他', count: rest }] : top
})

const hostOnline = computed(() => {
  const list = analysis.value?.host_status || []
  const on = list.find((s) => s.name.includes('运行') || s.name.includes('在线') || s.name.includes('正常'))
  const abn = list.find((s) => s.name.includes('异常') || s.name.includes('离线') || s.name.includes('故障'))
  if (on) return { text: `在线 ${fmt(on.count)}${abn ? ` · 异常 ${fmt(abn.count)}` : ''}`, cls: 'is-up' }
  return { text: '按状态分布见下方', cls: '' }
})

function statusClass(name) {
  const n = String(name || '')
  const l = n.toLowerCase()
  const has = (zh, ...en) => zh.some((k) => n.includes(k)) || en.some((k) => l.includes(k))
  if (has(['运行', '在线', '正常', '启用', '健康'], 'running', 'active', 'ready', 'healthy', 'online', 'ok')) return 'is-success'
  if (has(['异常', '故障', '失败', '错误'], 'abnormal', 'failed', 'error', 'unhealthy', 'fault')) return 'is-danger'
  if (has(['离线', '下线', '停', '关闭', '过期', '暂停'], 'stopped', 'offline', 'expired', 'paused', 'inactive')) return 'is-warning'
  if (has(['创建', '初始', '变更', '升级', '中'], 'creating', 'pending', 'initializing', 'updating')) return 'is-info'
  return 'is-muted'
}
// 主机状态 → 语义色（绿运行/红异常/橙离线停/蓝创建/灰其它）
function statusColor(name) {
  const c = statusClass(name)
  return { 'is-success': '#16a34a', 'is-danger': '#e34948', 'is-warning': '#d97706', 'is-info': '#2f66f0' }[c] || '#94a3b8'
}

// 环形扇形图配置
function donutOption(data, colorFn) {
  return {
    tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
    legend: {
      type: 'scroll',
      orient: 'vertical',
      right: 4,
      top: 'center',
      itemWidth: 10,
      itemHeight: 10,
      itemGap: 12,
      icon: 'circle',
      textStyle: { color: '#64748b', fontSize: 12 },
      formatter: (name) => (name.length > 7 ? name.slice(0, 7) + '…' : name),
    },
    series: [
      {
        type: 'pie',
        radius: ['44%', '66%'],
        center: ['32%', '50%'],
        avoidLabelOverlap: true,
        itemStyle: { borderColor: '#fff', borderWidth: 2, borderRadius: 4 },
        label: { show: false },
        emphasis: { label: { show: true, formatter: '{b}\n{d}%', fontSize: 13, color: '#1e293b' } },
        data: data.map((d, i) => ({
          name: d.name,
          value: d.count,
          itemStyle: { color: colorFn ? colorFn(d.name) : PALETTE[i % PALETTE.length] },
        })),
      },
    ],
  }
}
function renderCharts() {
  if (bizDist.value.length && bizChartEl.value) {
    if (!bizChart || bizChart.getDom() !== bizChartEl.value) {
      bizChart && bizChart.dispose()
      bizChart = echarts.init(bizChartEl.value)
    }
    bizChart.setOption(donutOption(bizDist.value, null))
  }
  if (assetDist.value.length && assetChartEl.value) {
    if (!assetChart || assetChart.getDom() !== assetChartEl.value) {
      assetChart && assetChart.dispose()
      assetChart = echarts.init(assetChartEl.value)
    }
    assetChart.setOption(donutOption(assetDist.value, null))
  }
  if (hostStatus.value.length && statusChartEl.value) {
    if (!statusChart || statusChart.getDom() !== statusChartEl.value) {
      statusChart && statusChart.dispose()
      statusChart = echarts.init(statusChartEl.value)
    }
    statusChart.setOption(donutOption(hostStatus.value, statusColor))
  }
  if (productDist.value.length && alertChartEl.value) {
    if (!alertChart || alertChart.getDom() !== alertChartEl.value) {
      alertChart && alertChart.dispose()
      alertChart = echarts.init(alertChartEl.value)
    }
    alertChart.setOption(donutOption(productDist.value, null))
  }
}
function resizeCharts() {
  bizChart && bizChart.resize()
  assetChart && assetChart.resize()
  statusChart && statusChart.resize()
  alertChart && alertChart.resize()
}

onMounted(async () => {
  try {
    const models = await listModels()
    const map = {}
    for (const m of models) map[m.id] = { obj_id: m.obj_id, icon: m.icon }
    modelMeta.value = map
  } catch (e) { /* ignore */ }
  try {
    ov.value = await getOverview()
  } catch (e) { /* ignore */ }
  try {
    analysis.value = await getAnalysis()
    if (analysis.value?.overview) ov.value = analysis.value.overview
  } catch (e) {
    analysis.value = null // 无运营分析权限：仅展示 KPI
  }
  try {
    const res = await listAlerts({ days: 1, page_size: 100, sort: 'activity' })
    alerts.value = res.list || []
    const [firing, products] = await Promise.all([
      listAlerts({ status: 'firing', page_size: 1 }),
      listAlertProducts(),
    ])
    firingTotal.value = firing.total || 0
    productDist.value = (products || []).filter((x) => x.count > 0)
  } catch (e) { /* 忽略 */ }
  await nextTick()
  renderCharts()
  startAlertScroll()
  window.addEventListener('resize', resizeCharts)
})
onBeforeUnmount(() => {
  stopAlertScroll()
  window.removeEventListener('resize', resizeCharts)
  bizChart && bizChart.dispose()
  assetChart && assetChart.dispose()
  statusChart && statusChart.dispose()
  alertChart && alertChart.dispose()
})
</script>

<style scoped>
.kpi-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 18px;
}
html:not(.dark) .kpi-row .stat-card {
  background: linear-gradient(130deg, var(--stat-tint), var(--card-bg) 70%);
  border-color: #dce7f4;
}
.kpi-row .stat-card { --stat-tint: #f0f6ff; }
.kpi-row .stat-card:has(.is-success) { --stat-tint: #f0f9f6; }
.kpi-row .stat-card:has(.is-warning) { --stat-tint: #fff8ef; }
.kpi-row .stat-card:has(.is-danger) { --stat-tint: #fff5f6; }
.stat-icon.is-info { color: var(--brand); background: #e8f0fe; }
.stat-icon.is-success { color: var(--success); background: #e6f7ef; }
.stat-icon.is-warning { color: var(--warning); background: #fdf0e5; }
.stat-icon.is-danger { color: var(--danger); background: #fdeaea; }
html.dark .stat-icon.is-info { background: var(--info-bg); }
html.dark .stat-icon.is-success { background: var(--success-bg); }
html.dark .stat-icon.is-warning { background: var(--warning-bg); }
html.dark .stat-icon.is-danger { background: var(--danger-bg); }
.kpi-row .stat-card {
  display: flex;
  align-items: center;
  gap: 14px;
}
.kpi-row .stat-body { min-width: 0; }
.stat-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  border-radius: 14px;
  font-size: 30px;
  flex-shrink: 0;
}
.pie-chart {
  width: 100%;
  height: 260px;
}
.grid-2 {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 18px;
}
/* 大屏 4 张一排；笔记本 2×2；窄屏 1 列（避免 3+1 折行） */
@media (max-width: 1560px) {
  .grid-2 { grid-template-columns: repeat(2, 1fr); }
}
@media (max-width: 820px) {
  .grid-2 { grid-template-columns: 1fr; }
}
.muted-link {
  font-size: 13px;
  color: var(--brand);
  cursor: pointer;
}
.muted-link:hover { text-decoration: underline; }

/* 主机状态分布 */
.status-list { display: flex; flex-direction: column; gap: 14px; }
.status-row { display: flex; align-items: center; gap: 12px; }
.status-row .soft-tag { width: 92px; justify-content: flex-start; }
.status-bar {
  flex: 1;
  height: 8px;
  border-radius: 999px;
  background: var(--fill-bg);
  overflow: hidden;
}
.status-bar > i { display: block; height: 100%; border-radius: 999px; }
.bar-success { background: var(--success); }
.bar-danger { background: var(--danger); }
.bar-warning { background: var(--warning); }
.bar-info { background: var(--info); }
.bar-muted { background: var(--text-muted); }
.status-val { min-width: 48px; text-align: right; font-weight: 700; color: var(--text-primary); }

/* 最近告警事件 */
.ap-right { display: inline-flex; align-items: center; gap: 14px; }
.firing-badge {
  font-size: 12px;
  font-weight: 600;
  color: var(--danger, #e34948);
  background: var(--danger-light, #fdeaea);
  padding: 2px 10px;
  border-radius: 999px;
}
.alert-list { display: flex; flex-direction: column; }
.alert-row {
  display: grid;
  grid-template-columns: 76px 1.6fr 1.4fr 80px 170px;
  align-items: center;
  gap: 12px;
  padding: 11px 6px;
  border-bottom: 1px solid var(--border-light);
  font-size: 13.5px;
}
.alert-row:last-child { border-bottom: none; }
.alert-row.clickable { cursor: pointer; }
.alert-row.clickable:hover { background: var(--brand-light); }
.al-level { justify-content: flex-start; }
.al-rule { color: var(--text-primary); font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.al-res { color: var(--text-regular); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.al-status { justify-content: flex-start; }
.al-time { color: var(--text-secondary); font-size: 12.5px; text-align: right; }

@media (max-width: 1200px) {
  .alert-row { grid-template-columns: 70px 1.5fr 80px; }
  .alert-row .al-res, .alert-row .al-time { display: none; }
}

/* 各模型实例数（左） + 最近告警事件（右） */
.grid-bottom {
  display: flex;
  gap: 16px;
  align-items: stretch; /* flex 等高列：两栏严格等高 */
  margin-bottom: 18px;
}
.grid-bottom > .panel {
  flex: 1 1 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
/* 内容区填满卡片剩余高度；超出则卡内滚动 */
.grid-bottom .model-grid,
.grid-bottom .alert-list {
  flex: 1 1 auto;
  min-height: 0;
  max-height: 188px; /* 约 4 行告警，超出卡内滚动；两栏等高 */
  overflow-y: auto;
  align-content: start;
}

/* 模型实例数网格 */
.model-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 10px 16px;
}
.model-cell {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  border: 1px solid var(--border-light);
  border-radius: 10px;
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}
.model-cell:hover { border-color: var(--brand); background: var(--brand-light); }
.mc-left { display: flex; align-items: center; gap: 10px; min-width: 0; }
.mc-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 7px;
  background: var(--fill-bg);
  color: var(--brand);
  font-size: 16px;
  flex-shrink: 0;
}
.mc-name { font-size: 13.5px; color: var(--text-regular); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.mc-count { font-size: 18px; font-weight: 700; color: var(--text-primary); flex-shrink: 0; }

@media (max-width: 1200px) {
  .kpi-row { grid-template-columns: repeat(2, 1fr); }
  .grid-bottom { flex-direction: column; }
}
</style>
