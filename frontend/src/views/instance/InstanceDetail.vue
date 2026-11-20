<template>
  <div v-loading="loading">
    <!-- 吸顶检测哨兵：滚过它说明头部已吸顶 -->
    <div ref="heroSentinel" class="hero-sentinel"></div>
    <!-- 头部概览 -->
    <div class="panel detail-hero" :class="{ 'is-stuck': stuck }">
      <el-button class="dh-back" :icon="ArrowLeft" circle title="返回" @click="goBack" />
      <div class="dh-left">
        <span class="dh-icon"><ModelIcon :obj-id="modelInfo.obj_id" :fallback="modelInfo.icon || 'Document'" :size="26" /></span>
        <div class="dh-meta">
          <div class="dh-title-row">
            <span class="dh-name">{{ title }}</span>
            <span v-if="statusInfo" class="soft-tag" :class="statusInfo.cls"><span class="dot"></span>{{ statusInfo.text }}</span>
            <span class="soft-tag is-info">{{ modelInfo.name }}</span>
            <span v-if="alertFiring" class="al-chip" :style="{ background: alertTopColor + '22', color: alertTopColor }" title="点击查看告警" @click="scrollToAlerts">
              <el-icon><WarningFilled /></el-icon>{{ alertFiring }} 告警
            </span>
          </div>
          <div class="dh-sub">
            <span v-if="data.resource_id" class="mono">{{ data.resource_id }}</span>
            <span v-if="data.resource_id" class="sep">·</span>
            <span>#{{ instanceId }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 基础信息 -->
    <div class="panel panel-pad box">
      <div class="panel-head"><span class="panel-title">基础信息</span></div>
      <div class="kv-grid">
        <div v-for="a in gridAttrs" :key="a.id" class="kv">
          <div class="kv-label">{{ a.name }}</div>
          <div class="kv-value">
            <span v-if="!hasVal(data[a.property_id])" class="muted">-</span>
            <span v-else-if="a.type === 'enum'" class="soft-tag is-info">{{ displayValue(a, data[a.property_id]) }}</span>
            <span v-else-if="a.type === 'bool'" class="soft-tag" :class="data[a.property_id] ? 'is-success' : 'is-muted'">{{ data[a.property_id] ? '是' : '否' }}</span>
            <span v-else :class="{ mono: a.property_id === 'resource_id' }">{{ displayValue(a, data[a.property_id]) }}</span>
          </div>
        </div>
      </div>
      <div class="meta">
        <span class="meta-item"><i class="meta-label">数据源</i>{{ sourceText }}</span>
        <span class="meta-item"><i class="meta-label">创建人</i>{{ inst.creator || '-' }}</span>
        <span class="meta-item"><i class="meta-label">创建时间</i>{{ fmt(inst.created_at) }}</span>
        <span class="meta-item"><i class="meta-label">更新时间</i>{{ fmt(inst.updated_at) }}</span>
      </div>
    </div>

    <!-- 资源监控：主机/MySQL/Redis/EIP 等按来源(Prometheus/云监控)自动取指标 -->
    <InstanceMetrics v-if="['host', 'mysql', 'redis', 'kafka', 'clb', 'nlb', 'alb', 'eip', 'cen', 'vbr', 'minio', 'nacos', 'lindorm', 'elastic', 'natgw', 'apigw', 'kubernetes'].includes(modelInfo.obj_id)" :instance-id="instanceId" class="box" />

    <!-- DNS解析：该域名下全部解析记录明细 -->
    <el-card v-if="modelInfo.obj_id === 'dns_record'" class="box" shadow="never">
      <template #header>
        <span class="box-title">解析记录</span>
        <span class="box-sub">共 {{ dnsRecords.length }} 条</span>
        <el-input
          v-model="dnsKw"
          placeholder="搜索主机记录 / 完整域名 / 记录值 / 类型 / 线路 / 备注"
          clearable
          size="small"
          class="dns-search"
        />
      </template>
      <el-table :data="pagedDnsRecords" border size="small">
        <el-table-column type="index" label="#" width="55" :index="dnsIndex" />
        <el-table-column label="主机记录" prop="rr" min-width="110" show-overflow-tooltip>
          <template #default="{ row }"><span v-for="(s, i) in hlSegs(row.rr)" :key="i" :class="{ hl: s.hit }">{{ s.t }}</span></template>
        </el-table-column>
        <el-table-column label="完整域名" prop="fqdn" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="!row.fqdn">-</span>
            <span v-for="(s, i) in hlSegs(row.fqdn)" v-else :key="i" :class="{ hl: s.hit }">{{ s.t }}</span>
          </template>
        </el-table-column>
        <el-table-column label="类型" prop="type" width="100">
          <template #default="{ row }"><span v-for="(s, i) in hlSegs(row.type)" :key="i" :class="{ hl: s.hit }">{{ s.t }}</span></template>
        </el-table-column>
        <el-table-column label="记录值" prop="value" min-width="200" show-overflow-tooltip>
          <template #default="{ row }"><span v-for="(s, i) in hlSegs(row.value)" :key="i" :class="{ hl: s.hit }">{{ s.t }}</span></template>
        </el-table-column>
        <el-table-column label="TTL(秒)" prop="ttl" width="100" />
        <el-table-column label="线路" prop="line" width="120">
          <template #default="{ row }"><span v-for="(s, i) in hlSegs(row.line)" :key="i" :class="{ hl: s.hit }">{{ s.t }}</span></template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.status === 'disable' ? 'info' : 'success'">
              {{ row.status === 'disable' ? '暂停' : '启用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="备注" prop="remark" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="!row.remark">-</span>
            <span v-for="(s, i) in hlSegs(row.remark)" v-else :key="i" :class="{ hl: s.hit }">{{ s.t }}</span>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="160">
          <template #default="{ row }">{{ row.created_at || '-' }}</template>
        </el-table-column>
        <el-table-column label="更新时间" width="160">
          <template #default="{ row }">{{ row.updated_at || '-' }}</template>
        </el-table-column>
      </el-table>
      <el-pagination
        class="dns-pager"
        background
        layout="total, prev, pager, next, sizes"
        :total="filteredDnsRecords.length"
        :current-page="dnsPage"
        :page-size="dnsPageSize"
        :page-sizes="[10, 20, 50, 100]"
        @current-change="(p) => (dnsPage = p)"
        @size-change="(s) => { dnsPageSize = s; dnsPage = 1 }"
      />
    </el-card>

    <!-- 告警（云监控，关联到本资源） -->
    <el-card v-if="alerts.length" ref="alertCardRef" class="box" shadow="never">
      <template #header>
        <span class="box-title">告警</span>
        <span class="box-sub">在触发 {{ alertFiring }} · 共 {{ alerts.length }}</span>
      </template>
      <el-table :data="alerts" size="small">
        <el-table-column label="级别" width="80">
          <template #default="{ row }"><span class="soft-tag" :class="alLevelCls(row.level)">{{ alLevelText(row.level) }}</span></template>
        </el-table-column>
        <el-table-column label="规则" prop="rule_name" min-width="160" show-overflow-tooltip />
        <el-table-column label="指标" min-width="140" show-overflow-tooltip>
          <template #default="{ row }">{{ [row.product, row.metric_name].filter(Boolean).join(' · ') || '—' }}</template>
        </el-table-column>
        <el-table-column label="维度" min-width="130" show-overflow-tooltip>
          <template #default="{ row }">{{ row.dimensions || '—' }}</template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }"><span class="soft-tag" :class="row.status === 'resolved' ? 'is-success' : 'is-danger'">{{ row.status === 'resolved' ? '已恢复' : '未恢复' }}</span></template>
        </el-table-column>
        <el-table-column label="触发时间" width="160">
          <template #default="{ row }">{{ fmt(row.alert_time) }}</template>
        </el-table-column>
        <el-table-column label="恢复时间" width="160">
          <template #default="{ row }">{{ row.status === 'resolved' && row.resolved_at ? fmt(row.resolved_at) : '—' }}</template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 关联关系 + 拓扑图 -->
    <el-card class="box" shadow="never">
      <template #header><span class="box-title">关联关系与拓扑</span></template>
      <RelationPanel :instance-id="instanceId" :self-label="title" :self-model-name="modelInfo.name" :self-obj-id="modelInfo.obj_id" :self-icon="modelInfo.icon" :self-abnormal="selfAbnormal" :self-status="statusInfo?.text || ''" />
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, WarningFilled } from '@element-plus/icons-vue'
import * as iapi from '@/api/instance'
import * as mapi from '@/api/model'
import * as sapi from '@/api/system'
import RelationPanel from '@/components/RelationPanel.vue'
import ModelIcon from '@/components/ModelIcon.vue'
import InstanceMetrics from '@/components/InstanceMetrics.vue'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const instanceId = ref(Number(route.params.id))
const inst = ref({})
const data = ref({})
const modelInfo = ref({})
const attrs = ref([])

const enabledAttrs = computed(() => attrs.value.filter((a) => a.enabled !== false))
// 详情信息区排除 records（DNS解析明细另以表格展示）
const descAttrs = computed(() => enabledAttrs.value.filter((a) => a.property_id !== 'records'))
// DNS解析：records 字段存该域名全部解析记录明细（JSON 字符串）
const dnsRecords = computed(() => {
  const raw = data.value.records
  if (!raw) return []
  try {
    const arr = typeof raw === 'string' ? JSON.parse(raw) : raw
    return Array.isArray(arr) ? arr : []
  } catch {
    return []
  }
})
const dnsKw = ref(route.query.q ? String(route.query.q) : '')
const dnsPage = ref(1)
const dnsPageSize = ref(10)
const filteredDnsRecords = computed(() => {
  const kw = dnsKw.value.trim().toLowerCase()
  if (!kw) return dnsRecords.value
  // 与全局搜索（整段记录文本模糊匹配）保持一致：含备注/线路，否则搜 remark 命中后进详情定位不到
  return dnsRecords.value.filter((r) =>
    [r.rr, r.fqdn, r.value, r.type, r.line, r.remark].some((v) => String(v || '').toLowerCase().includes(kw)),
  )
})
const pagedDnsRecords = computed(() => {
  const start = (dnsPage.value - 1) * dnsPageSize.value
  return filteredDnsRecords.value.slice(start, start + dnsPageSize.value)
})
function dnsIndex(i) {
  return (dnsPage.value - 1) * dnsPageSize.value + i + 1
}
watch(dnsKw, () => (dnsPage.value = 1))
// 高亮命中子串
function hlSegs(text) {
  const kw = dnsKw.value.trim().toLowerCase()
  const src = String(text == null ? '' : text)
  if (!kw) return [{ t: src, hit: false }]
  const segs = []
  const lower = src.toLowerCase()
  let i = 0
  while (true) {
    const idx = lower.indexOf(kw, i)
    if (idx < 0) {
      if (i < src.length) segs.push({ t: src.slice(i), hit: false })
      break
    }
    if (idx > i) segs.push({ t: src.slice(i, idx), hit: false })
    segs.push({ t: src.slice(idx, idx + kw.length), hit: true })
    i = idx + kw.length
  }
  return segs
}
const title = computed(() => {
  const a = enabledAttrs.value[0]
  const v = a ? displayValue(a, data.value[a.property_id]) : ''
  return v && v !== '-' ? v : `实例 #${instanceId.value}`
})

// 状态字段（head 概览展示）
const statusAttr = computed(() =>
  descAttrs.value.find((a) => a.property_id === 'status' || a.property_id === 'bk_host_status'),
)
function statusClass(name) {
  const n = String(name || '')
  const l = n.toLowerCase()
  const has = (zh, ...en) => zh.some((k) => n.includes(k)) || en.some((k) => l.includes(k))
  if (has(['运行', '在线', '正常', '启用', '健康'], 'running', 'active', 'ready', 'healthy', 'online', 'ok', 'available')) return 'is-success'
  if (has(['异常', '故障', '失败', '错误'], 'abnormal', 'failed', 'error', 'unhealthy', 'fault')) return 'is-danger'
  if (has(['离线', '下线', '已停', '停止', '关闭', '过期', '暂停', '冻结'], 'stopped', 'offline', 'expired', 'paused', 'inactive', 'disable', 'deleting')) return 'is-warning'
  if (has(['创建', '初始', '变更', '升级', '扩容', '处理中', '中'], 'creating', 'pending', 'initializing', 'updating', 'scaling', 'upgrading')) return 'is-info'
  return 'is-muted'
}
const statusInfo = computed(() => {
  const a = statusAttr.value
  if (!a) return null
  const v = data.value[a.property_id]
  if (v === undefined || v === null || v === '') return null
  const text = displayValue(a, v)
  if (!text || text === '-') return null
  return { text, cls: statusClass(text) }
})
// 自身是否异常：非运行状态（异常/离线停止/过期/创建中…）均算；无状态/正常不算
const selfAbnormal = computed(() => {
  const s = statusInfo.value
  return !!s && (s.cls === 'is-danger' || s.cls === 'is-warning' || s.cls === 'is-info')
})
// 基础信息网格：排除标题字段(已作名称)与状态字段(已在概览)
const gridAttrs = computed(() => {
  const titleId = enabledAttrs.value[0]?.property_id
  const statusId = statusAttr.value?.property_id
  return descAttrs.value.filter((a) => a.property_id !== titleId && a.property_id !== statusId)
})

function optionName(a, id) {
  const o = (a.options || []).find((x) => x.id === id)
  return o ? o.name : id
}
function displayValue(a, val) {
  if (val === undefined || val === null || val === '') return '-'
  if (a.type === 'bool') return val ? '是' : '否'
  if (a.type === 'enum') return optionName(a, val)
  if (a.type === 'enummulti')
    return Array.isArray(val) ? val.map((v) => optionName(a, v)).join('，') : '-'
  return val
}
function fmt(t) {
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN')
}
function hasVal(v) {
  return !(v === undefined || v === null || v === '' || (Array.isArray(v) && v.length === 0))
}
const TYPE_CN = { prometheus: 'Prometheus', aliyun: '阿里云', tencent: '腾讯云' }
const sourceText = computed(() => {
  const i = inst.value || {}
  if (i.source_name) {
    const t = TYPE_CN[i.source_type] || i.source_type
    return t ? `${i.source_name}（${t}）` : i.source_name
  }
  return i.source_id ? '自动同步' : '用户创建'
})

async function load() {
  loading.value = true
  try {
    inst.value = await iapi.getInstance(instanceId.value)
    data.value = inst.value.data || {}
    const [m, at] = await Promise.all([
      mapi.getModel(inst.value.model_id),
      mapi.listAttributes(inst.value.model_id),
    ])
    modelInfo.value = m
    attrs.value = at
  } finally {
    loading.value = false
  }
  loadAlerts()
}

// 关联到本资源的告警（近一年）
const alerts = ref([])
const alertCardRef = ref()
const alertFiring = computed(() => alerts.value.filter((a) => a.status !== 'resolved').length)
// 未恢复告警里最高级别对应的颜色（紧急红/警告橙/提示蓝）
const alertTopColor = computed(() => {
  const rank = { critical: 3, warning: 2, info: 1, ok: 0 }
  const color = { critical: '#dc2626', warning: '#f59e0b', info: '#3b82f6', ok: '#22c55e' }
  let best = 'info', r = -1
  for (const a of alerts.value) {
    if (a.status === 'resolved') continue
    const rr = rank[a.level] ?? 0
    if (rr >= r) { r = rr; best = a.level }
  }
  return color[best] || '#dc2626'
})
function scrollToAlerts() {
  alertCardRef.value?.$el?.scrollIntoView({ behavior: 'smooth', block: 'center' })
}
function alLevelText(l) { return { critical: '紧急', warning: '警告', info: '提示', ok: '恢复' }[l] || l }
function alLevelCls(l) { return { critical: 'is-danger', warning: 'is-warning', info: 'is-info', ok: 'is-success' }[l] || 'is-muted' }
async function loadAlerts() {
  try {
    const res = await sapi.listAlerts({ inst: instanceId.value, days: 365, page: 1, page_size: 50 })
    alerts.value = res.list || []
  } catch {
    alerts.value = []
  }
}

function goBack() {
  // 返回到该实例所属模型的资源实例列表（带 model_id，避免回到默认的主机）
  const mid = inst.value?.model_id
  router.push(mid ? { path: '/instances', query: { model_id: mid } } : '/instances')
}

onMounted(load)

// 吸顶阴影：哨兵滚出滚动容器顶部时，给头部加阴影
const heroSentinel = ref(null)
const stuck = ref(false)
let stuckObserver = null
onMounted(() => {
  const el = heroSentinel.value
  if (!el) return
  const root = el.closest('.content') || null
  stuckObserver = new IntersectionObserver(
    ([e]) => { stuck.value = !e.isIntersecting },
    { root, threshold: 0 },
  )
  stuckObserver.observe(el)
})
onBeforeUnmount(() => { stuckObserver && stuckObserver.disconnect() })
watch(
  () => route.params.id,
  (id) => {
    if (id === undefined) return
    instanceId.value = Number(id)
    load()
  },
)
</script>

<style scoped>
/* 头部概览 */
.detail-hero {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 16px 22px;
  margin-bottom: 16px;
  /* 吸顶：长页面滚动时返回按钮/标题始终可见 */
  position: sticky;
  top: 0;
  z-index: 20;
  transition: box-shadow 0.18s ease;
}
/* 真正吸顶时才加浅阴影，和下方内容分层 */
.detail-hero.is-stuck {
  box-shadow: 0 6px 16px rgba(31, 55, 110, 0.1);
}
html.dark .detail-hero.is-stuck {
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.45);
}
.hero-sentinel {
  height: 0;
  margin: 0;
}
/* 左上角返回按钮（符合习惯、第一眼可见） */
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
.al-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  font-weight: 600;
  padding: 3px 9px;
  border-radius: 12px;
  cursor: pointer;
}
.al-chip .el-icon {
  font-size: 13px;
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
.dh-actions { flex-shrink: 0; }

.box {
  margin-bottom: 16px;
}
.box-title {
  font-weight: 700;
  color: var(--text-primary);
}

/* 基础信息键值网格 */
.kv-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 20px 32px;
}
.kv-label {
  font-size: 12.5px;
  color: var(--text-secondary);
  margin-bottom: 5px;
}
.kv-value {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  word-break: break-all;
}
@media (max-width: 900px) {
  .kv-grid { grid-template-columns: repeat(2, 1fr); }
}
.box-sub {
  margin-left: 10px;
  color: #909399;
  font-size: 12px;
}
.dns-search {
  width: 260px;
  margin-left: 16px;
  vertical-align: middle;
}
.dns-pager {
  margin-top: 12px;
  justify-content: flex-end;
}
.hl {
  background: #ffe8a3;
  color: #e6500a;
  border-radius: 2px;
  padding: 0 1px;
}
.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 28px;
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid var(--border-light);
  font-size: 12.5px;
  color: var(--text-regular);
}
.meta-item {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.meta-label {
  font-style: normal;
  color: var(--text-muted);
}
.muted {
  color: #c0c4cc;
}
</style>
