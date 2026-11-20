<template>
  <div class="k8s-page" v-loading="loading">
    <!-- 集群头部 -->
    <div class="cl-head">
      <div class="clh-left">
        <div class="clh-logo"><ModelIcon obj-id="kubernetes" fallback="Box" :size="30" /></div>
        <div class="clh-main">
          <div class="clh-title">
            <el-select v-model="cluster" class="clh-select" filterable>
              <el-option v-for="c in clusters" :key="c.name" :label="c.name" :value="c.name" />
            </el-select>
            <span v-if="curCluster" class="clh-badge" :style="badgeStyle(curCluster._state)">
              <span class="dot" :style="{ background: stColor(curCluster._state) }"></span>{{ stText(curCluster._state) }}
            </span>
          </div>
          <div class="clh-meta">
            <span>集群类型：<b>{{ clusterKind(curCluster) }}</b></span>
            <i class="sep"></i><span>Kubernetes版本：<b>{{ curCluster?.version || '-' }}</b></span>
            <i class="sep"></i><span>节点数：<b>{{ curCluster?.node_count ?? '-' }}</b></span>
            <i class="sep"></i><span>地域：<b>{{ curCluster?.region || '-' }}</b></span>
            <i class="sep"></i><span>所属业务：<b>{{ bizName(curCluster?.biz_line) }}</b></span>
          </div>
        </div>
      </div>
      <div class="clh-actions">
        <el-radio-group v-model="mRange" size="small" @change="loadMetrics">
          <el-radio-button v-for="r in ranges" :key="r.v" :value="r.v">{{ r.t }}</el-radio-button>
        </el-radio-group>
        <el-button :icon="Refresh" circle title="刷新" @click="reload" />
      </div>
    </div>

    <!-- 统计卡 -->
    <div class="stat-row">
      <div class="stat-card"><div class="sc-ic ic-blue"><el-icon><Box /></el-icon></div><div><div class="sc-lab">服务总数</div><div class="sc-val">{{ scoped.length }}</div></div></div>
      <div class="stat-card"><div class="sc-ic ic-cyan"><el-icon><Grid /></el-icon></div><div><div class="sc-lab">Pod 总数</div><div class="sc-val">{{ podTotal }}</div></div></div>
      <div class="stat-card"><div class="sc-ic ic-indigo"><el-icon><Coin /></el-icon></div><div><div class="sc-lab">节点数</div><div class="sc-val">{{ curCluster?.node_count ?? '-' }}</div></div></div>
      <div class="stat-card"><div class="sc-ic ic-violet"><el-icon><Menu /></el-icon></div><div><div class="sc-lab">Namespace 数</div><div class="sc-val">{{ namespaces.length }}</div></div></div>
      <div class="stat-card ok" :title="`正常率=运行/(运行+异常)，不含 ${cnt.stopped} 个已停止`"><div class="sc-ic ic-green"><el-icon><CircleCheck /></el-icon></div><div class="sc-grow"><div class="sc-lab">运行正常服务</div><div class="sc-val">{{ cnt.running }} <span class="sc-pct">{{ pct(cnt.running) }}%</span></div><div class="sc-bar"><i :style="{ width: pct(cnt.running) + '%', background: '#22c55e' }"></i></div></div></div>
      <div class="stat-card danger" :title="`异常率=异常/(运行+异常)，不含 ${cnt.stopped} 个已停止`"><div class="sc-ic ic-red"><el-icon><WarningFilled /></el-icon></div><div class="sc-grow"><div class="sc-lab">异常服务<span v-if="cnt.stopped" class="sc-stopped">· {{ cnt.stopped }} 已停止</span></div><div class="sc-val" :class="{ danger: cnt.bad }">{{ cnt.bad }} <span class="sc-pct">{{ pct(cnt.bad) }}%</span></div><div class="sc-bar"><i :style="{ width: pct(cnt.bad) + '%', background: '#ef4444' }"></i></div></div></div>
    </div>

    <!-- 中部：集群拓扑 + 服务详情 -->
    <div class="mid-row">
      <div class="panel topo-panel">
        <div class="panel-head">
          <span class="panel-title">集群拓扑</span><span class="muted sm">集群 → 命名空间 → 服务</span>
          <el-button v-if="scoped.length" class="tp-max" text :icon="FullScreen" @click="openBigTopo">查看大图</el-button>
        </div>
        <el-empty v-if="!scoped.length" description="该集群暂无应用服务" :image-size="60" />
        <div v-else ref="topoScrollEl" class="topo-scroll" @scroll="onTopoScroll">
          <div class="topo-tree">
            <!-- 集群根 -->
            <div class="tt-cluster">
              <div ref="clusterCardEl" class="node-card nc-cluster">
                <div class="nc-ic ic-k8s"><ModelIcon obj-id="kubernetes" fallback="Box" :size="20" /></div>
                <div class="nc-body">
                  <div class="nc-name">{{ cluster }}</div>
                  <div class="nc-sub"><span class="dot" :style="{ background: stColor(curCluster?._state) }"></span>{{ stText(curCluster?._state) }}</div>
                </div>
              </div>
            </div>
            <!-- 命名空间行 -->
            <div class="tt-ns-row">
              <div v-for="g in nsGroups" :key="g.ns" class="ns-col">
                <div class="node-card nc-ns" :class="{ bad: g.abn }">
                  <div class="nc-ic ic-ns"><el-icon><Menu /></el-icon></div>
                  <div class="nc-body">
                    <div class="nc-name" :title="g.ns">{{ g.ns }}</div>
                    <div class="nc-sub">{{ g.list.length }} 服务<span v-if="g.abn" class="nc-abn">⚠{{ g.abn }}</span></div>
                  </div>
                </div>
                <div class="svc-list">
                  <div
                    v-for="a in g.show"
                    :key="a.id"
                    class="svc-card"
                    :class="{ on: sel && sel.id === a.id, bad: a._state === 'bad' }"
                    @click="selectApp(a)"
                  >
                    <div class="svc-ic"><ModelIcon obj-id="k8s_app" fallback="Grid" :size="13" /></div>
                    <span class="svc-name" :title="a.app_name">{{ a.app_name }}</span>
                    <span class="svc-pod">{{ a.replicas_available ?? 0 }} Pod</span>
                    <span class="dot" :style="{ background: stColor(a._state) }"></span>
                  </div>
                  <div v-if="g.more > 0" class="svc-more" @click="toggleNs(g.ns)">+{{ g.more }} 更多</div>
                  <div v-else-if="expandedNs.has(g.ns) && g.list.length > NS_CAP" class="svc-more" @click="toggleNs(g.ns)">收起</div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="panel detail-panel">
        <div class="panel-head">
          <span class="panel-title">服务详情</span>
          <span v-if="sel" class="clh-badge sm" :style="badgeStyle(sel._state)"><span class="dot" :style="{ background: stColor(sel._state) }"></span>{{ stText(sel._state) }}</span>
          <el-button v-if="sel && canEdit" class="dp-edit" text size="small" :icon="Edit" @click="openEdit">编辑</el-button>
        </div>
        <template v-if="sel">
          <div class="detail-body">
          <div class="d-title"><ModelIcon obj-id="k8s_app" fallback="Grid" :size="20" /><span class="d-name">{{ sel.app_name }}</span></div>
          <div class="d-kv">
            <div class="kv"><span class="k">类型</span><span class="v">{{ sel.workload_type || '-' }}</span></div>
            <div class="kv"><span class="k">命名空间</span><span class="v">{{ sel.namespace || '-' }}</span></div>
            <div class="kv"><span class="k">副本</span><span class="v" :style="{ color: repColor(sel) }">{{ sel.replicas_available ?? 0 }} / {{ sel.replicas_desired ?? 0 }}</span></div>
            <div class="kv"><span class="k">所属集群</span><span class="v link clk" title="查看集群详情" @click="goCluster(sel.cluster_name)">{{ sel.cluster_name }}</span></div>
            <div class="kv"><span class="k">负责人</span><span class="v">{{ sel.owner || '-' }}</span></div>
            <div class="kv"><span class="k">对外域名</span><span class="v">{{ sel.ingress_host || '-' }}</span></div>
            <div class="kv full"><span class="k">镜像</span><span class="v mono">{{ sel.image || '-' }}</span></div>
          </div>
          <div v-if="selAlerts.length" class="d-al">
            <span class="da-firing" v-if="selFiring">在触发 {{ selFiring }}</span>
            <span v-for="al in selAlerts.slice(0, 4)" :key="al.id" class="da-chip" :class="{ resolved: al.status === 'resolved' }">
              <span class="dot" :style="{ background: alvOf(al.level).c }"></span>{{ al.rule_name }}
            </span>
          </div>
          </div>
          <el-button class="d-more" type="primary" plain size="small" @click="openDrawer(sel)">监控 · 关联拓扑 · 全部告警</el-button>
        </template>
        <el-empty v-else description="点左侧拓扑节点或下方服务查看" :image-size="60" />
      </div>
    </div>

    <!-- 底部：服务列表 + 资源使用 -->
    <div class="bot-row">
      <div class="panel list-panel">
        <div class="panel-head">
          <span class="panel-title">服务列表</span>
          <div class="lp-filters">
            <el-select v-model="nsFilter" placeholder="全部命名空间" clearable filterable size="small" class="f-ns">
              <el-option label="全部命名空间" value="" />
              <el-option v-for="n in namespaces" :key="n" :label="n" :value="n" />
            </el-select>
            <el-select v-model="stFilter" placeholder="全部状态" clearable size="small" class="f-st">
              <el-option label="全部状态" value="" />
              <el-option label="运行中" value="running" />
              <el-option label="已停止" value="stopped" />
              <el-option label="异常" value="bad" />
            </el-select>
            <el-input v-model="kw" placeholder="服务名称 / 镜像" clearable size="small" :prefix-icon="Search" class="f-kw" />
          </div>
        </div>
        <div ref="listBodyEl" class="list-body">
        <el-table :data="tableRows" size="small" :height="tableH" @row-click="selectApp">
          <el-table-column label="服务名称" min-width="180">
            <template #default="{ row }"><span class="tl-name" :class="{ on: sel && sel.id === row.id }">{{ row.app_name }}</span></template>
          </el-table-column>
          <el-table-column label="命名空间" width="130"><template #default="{ row }">{{ row.namespace }}</template></el-table-column>
          <el-table-column label="类型" width="110"><template #default="{ row }">{{ row.workload_type }}</template></el-table-column>
          <el-table-column label="副本" width="80">
            <template #default="{ row }"><span :style="{ color: repColor(row) }">{{ row.replicas_available ?? 0 }}/{{ row.replicas_desired ?? 0 }}</span></template>
          </el-table-column>
          <el-table-column label="状态" width="150">
            <template #default="{ row }">
              <span class="tl-st" :style="{ color: stColor(row._state) }"><span class="dot" :style="{ background: stColor(row._state) }"></span>{{ stText(row._state) }}</span>
              <span v-if="alertMap[row.id]" class="tl-al" :style="{ background: alvOf(alertMap[row.id].level).c }">{{ alertMap[row.id].count }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="90">
            <template #default="{ row }"><el-button link type="primary" size="small" @click.stop="openDrawer(row)">详情</el-button></template>
          </el-table-column>
          <template #empty>该集群暂无匹配服务</template>
        </el-table>
        </div>
      </div>

      <div class="panel res-panel">
        <div class="panel-head"><span class="panel-title">资源使用情况</span><span class="muted sm">近 {{ rangeLabel }}</span></div>
        <div v-if="!metricsReady" class="res-empty"><el-empty :description="metricsMsg || '暂无监控数据'" :image-size="48" /></div>
        <template v-else>
          <div class="donut-row">
            <div class="donut" v-for="g in gauges" :key="g.key">
              <div class="ring" :style="{ background: `conic-gradient(${g.color} ${g.val * 3.6}deg, var(--track) 0)` }">
                <div class="ring-in"><span class="ring-val" :style="{ color: g.color }">{{ g.val.toFixed(1) }}%</span></div>
              </div>
              <div class="donut-name">{{ g.name }}</div>
              <div class="donut-sub">{{ g.sub }}</div>
            </div>
          </div>
          <div ref="lineEl" class="res-line"></div>
        </template>
      </div>
    </div>

    <!-- 编辑服务手动字段 -->
    <el-dialog v-model="editDialog" title="编辑服务信息" width="460px" append-to-body>
      <el-form label-width="90px">
        <el-form-item label="负责人"><el-input v-model="editForm.owner" placeholder="负责人" /></el-form-item>
        <el-form-item label="对外域名"><el-input v-model="editForm.ingress_host" placeholder="如 app.example.com" /></el-form-item>
        <el-form-item label="访问地址"><el-input v-model="editForm.endpoint" placeholder="如 svc:port / ClusterIP:port" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="editForm.comment" type="textarea" :rows="2" /></el-form-item>
      </el-form>
      <div class="ef-hint">应用名 / 集群 / 命名空间 / 副本 / 镜像 / 状态由 Prometheus 自动同步，不可手动修改（手填字段同步时会保留）。</div>
      <template #footer>
        <el-button @click="editDialog = false">取消</el-button>
        <el-button type="primary" @click="saveEdit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 全量详情抽屉（监控 + 关联拓扑 + 告警） -->
    <el-drawer v-model="drawer" :title="cur ? cur.app_name : '服务详情'" size="52%" append-to-body>
      <template v-if="cur">
        <InstanceMetrics :instance-id="cur.id" class="dw-metrics" />
        <div class="dw-head">关联拓扑</div>
        <RelationPanel :instance-id="cur.id" :self-label="cur.app_name" self-model-name="应用服务" self-obj-id="k8s_app" self-icon="Grid"
          :self-abnormal="cur._state === 'bad'" :self-status="stText(cur._state)" />
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { useRouter } from 'vue-router'
import * as echarts from 'echarts'
import { Refresh, Search, Box, Grid, Coin, Menu, CircleCheck, WarningFilled, FullScreen, Edit } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { searchInstances, getHostMetrics, updateInstance } from '@/api/instance'
import { listModels, listAttributes } from '@/api/model'
import { listAlerts } from '@/api/system'
import { useUserStore } from '@/store/user'
import ModelIcon from '@/components/ModelIcon.vue'
import RelationPanel from '@/components/RelationPanel.vue'
import InstanceMetrics from '@/components/InstanceMetrics.vue'

const userStore = useUserStore()
const router = useRouter()
const canEdit = computed(() => userStore.hasPerm(appModelId.value, 'update'))
// 点「所属集群」跳该集群(kubernetes 实例)详情页
function goCluster(name) {
  const c = clustersRaw.value.find((x) => x.name === name)
  if (c) router.push(`/instances/${c.id}`)
}

const loading = ref(false)
const appModelId = ref(0)
const apps = ref([])
const clustersRaw = ref([]) // kubernetes 实例
const bizMap = ref({}) // biz_line id -> name
const alertMap = ref({}) // inst_id -> {count, level}

const ALV = { critical: { c: '#ef4444', r: 3 }, warning: { c: '#f59e0b', r: 2 }, info: { c: '#3b82f6', r: 1 } }
const alvOf = (l) => ALV[l] || ALV.info
// 服务状态归并：running=运行中；stopped/offline(0 副本/下线)=已停止(中性，不算异常)；degraded/abnormal=异常
function appState(s) {
  if (s === 'running' || s === '1' || s === 'normal') return 'running'
  if (s === 'stopped' || s === 'offline') return 'stopped'
  return 'bad'
}
const ST = { running: { t: '运行中', c: '#22c55e' }, stopped: { t: '已停止', c: '#9ca3af' }, bad: { t: '异常', c: '#ef4444' } }
const stText = (s) => (ST[s] || ST.bad).t
const stColor = (s) => (ST[s] || ST.bad).c
// 排序权重：异常 → 运行中 → 已停止（停止的排最后）
const ST_ORDER = { bad: 0, running: 1, stopped: 2 }
const stRank = (s) => (ST_ORDER[s] ?? 1)
// 命名空间分类：系统/基础设施命名空间靠后，业务命名空间优先
const SYS_NS = new Set(['kube-system', 'kube-public', 'kube-node-lease', 'default', 'monitoring', 'logging', 'istio-system', 'ingress-nginx', 'cattle-system', 'cattle-fleet-system', 'cattle-fleet-local-system', 'kube-flannel', 'calico-system', 'tigera-operator', 'cert-manager', 'velero', 'metallb-system', 'koordinator-system', 'kruise-system', 'arms-prom', 'kubesphere-system'])
function isSysNs(ns) {
  if (!ns) return true
  const n = String(ns).toLowerCase()
  if (SYS_NS.has(n)) return true
  return /^kube-|^cattle-|^kubesphere|^istio|^calico|^arms|monitor|logging|prometheus|ingress|(^|-)system$/.test(n)
}
const nsRank = (ns) => (isSysNs(ns) ? 1 : 0) // 业务命名空间(0)优先，系统命名空间(1)靠后
function badgeStyle(s) { const c = stColor(s); return { color: c, background: c + '1f' } }

const ranges = [{ v: '1h', t: '1时' }, { v: '6h', t: '6时' }, { v: '24h', t: '24时' }]
const mRange = ref('1h')
const rangeLabel = computed(() => ({ '1h': '1 小时', '6h': '6 小时', '24h': '24 小时' }[mRange.value]))

const cluster = ref('')
const nsFilter = ref('')
const stFilter = ref('')
const kw = ref('')

// 集群下拉（kubernetes 实例；有应用的排前面）
const clusters = computed(() => {
  const withApp = new Set(apps.value.map((a) => a.cluster_name))
  return clustersRaw.value
    .map((c) => ({ ...c, _has: withApp.has(c.name) }))
    .sort((a, b) => (b._has ? 1 : 0) - (a._has ? 1 : 0) || a.name.localeCompare(b.name))
})
const curCluster = computed(() => clustersRaw.value.find((c) => c.name === cluster.value) || null)
// 集群类型：有阿里云集群ID(resource_id) 或 云厂商非 idc → 托管(ACK/TKE)，否则自建
function clusterKind(c) {
  if (!c) return '-'
  const rid = String(c.resource_id || '').trim()
  const cp = String(c.cloud_provider || '').toLowerCase()
  if (rid || (cp && cp !== 'idc' && cp !== 'other')) {
    if (cp === 'tencent') return '腾讯云 TKE'
    if (cp === 'huawei') return '华为云 CCE'
    const prof = String(c.cluster_profile || '')
    if (/edge/i.test(prof) || (!prof && /edge|边缘/i.test(c.name))) return '阿里云 ACK(边缘)'
    if (/serverless/i.test(prof)) return '阿里云 ACK Serverless'
    return '阿里云 ACK'
  }
  return '自建集群'
}
function bizName(id) { return bizMap.value[id] || '-' }

const scoped = computed(() => apps.value.filter((a) => a.cluster_name === cluster.value))
const namespaces = computed(() => [...new Set(scoped.value.map((a) => a.namespace).filter(Boolean))].sort())
const cnt = computed(() => {
  let running = 0, bad = 0, stopped = 0
  for (const a of scoped.value) {
    if (a._state === 'running') running++
    else if (a._state === 'bad') bad++
    else stopped++
  }
  return { running, bad, stopped }
})
// 正常率/异常率的分母=活跃服务(运行+异常，排除已停止的闲置服务)，两者相加=100%
const pct = (n) => {
  const base = cnt.value.running + cnt.value.bad
  return base ? Math.round((n / base) * 1000) / 10 : 0
}
const podTotal = computed(() => {
  if (metricsReady.value && podRunning.value != null) return podRunning.value
  return scoped.value.reduce((s, a) => s + (a.replicas_available || 0), 0)
})

const tableRows = computed(() => {
  const k = kw.value.trim().toLowerCase()
  return scoped.value
    .filter((a) => {
      if (nsFilter.value && a.namespace !== nsFilter.value) return false
      if (stFilter.value && a._state !== stFilter.value) return false
      if (k && !(`${a.app_name} ${a.image || ''}`.toLowerCase().includes(k))) return false
      return true
    })
    // 业务命名空间优先 → 异常/运行/停止 → 命名空间 → 名称
    .sort((a, b) => nsRank(a.namespace) - nsRank(b.namespace) || stRank(a._state) - stRank(b._state) || String(a.namespace).localeCompare(String(b.namespace)) || String(a.app_name).localeCompare(String(b.app_name)))
})

const sel = ref(null)
const selAlerts = ref([])
const selFiring = computed(() => selAlerts.value.filter((a) => a.status !== 'resolved').length)
async function selectApp(a) {
  sel.value = a
  selAlerts.value = []
  try {
    const r = await listAlerts({ inst: a.id, days: 365, page: 1, page_size: 20 })
    selAlerts.value = r.list || []
  } catch { selAlerts.value = [] }
}
function repColor(a) {
  const av = a.replicas_available ?? 0, de = a.replicas_desired ?? 0
  if (de <= 0) return '#9ca3af'
  if (av <= 0) return '#ef4444'
  if (av < de) return '#f59e0b'
  return '#16a34a'
}

// 全量抽屉
const drawer = ref(false)
const cur = ref(null)
function openDrawer(a) { cur.value = a; drawer.value = true }

// 编辑手动字段（负责人/对外域名/访问地址/备注）；同步字段只读、不覆盖
const editDialog = ref(false)
const editForm = ref({ id: null, owner: '', ingress_host: '', endpoint: '', comment: '' })
function openEdit() {
  if (!sel.value) return
  const s = sel.value
  editForm.value = { id: s.id, owner: s.owner || '', ingress_host: s.ingress_host || '', endpoint: s.endpoint || '', comment: s.comment || '' }
  editDialog.value = true
}
async function saveEdit() {
  const f = editForm.value
  const base = {}
  for (const k in sel.value) { if (k === 'id' || k.startsWith('_')) continue; base[k] = sel.value[k] }
  const edits = { owner: f.owner.trim(), ingress_host: f.ingress_host.trim(), endpoint: f.endpoint.trim(), comment: f.comment.trim() }
  try {
    await updateInstance(f.id, { data: { ...base, ...edits } })
    Object.assign(sel.value, edits) // 本地同步，无需整页刷新
    editDialog.value = false
    ElMessage.success('已保存')
  } catch (e) { /* 请求拦截器已提示 */ }
}

// ---- 数据加载 ----
async function load() {
  loading.value = true
  try {
    if (!appModelId.value) {
      const ms = await listModels()
      appModelId.value = ms.find((m) => m.obj_id === 'k8s_app')?.id || 0
      const k8sId = ms.find((m) => m.obj_id === 'kubernetes')?.id || 0
      if (k8sId) {
        const attrs = await listAttributes(k8sId)
        const bl = attrs.find((a) => a.property_id === 'biz_line')
        const map = {}
        for (const o of bl?.options || []) map[o.id] = o.name
        bizMap.value = map
        await loadClusters(k8sId)
      }
    }
    await loadApps()
    await loadAlerts()
    if (!cluster.value) cluster.value = clusters.value[0]?.name || ''
  } finally {
    loading.value = false
  }
}
async function loadClusters(k8sId) {
  const rows = []
  for (let page = 1; page <= 10; page++) {
    const r = await searchInstances({ model_id: k8sId, page, page_size: 500 })
    rows.push(...(r.list || []))
    if (rows.length >= (r.total || 0) || !(r.list || []).length) break
  }
  clustersRaw.value = rows.map((it) => ({ id: it.id, name: it.data.cluster_name, ...it.data, _state: appState(it.data.status) }))
}
async function loadApps() {
  const all = []
  for (let page = 1; page <= 50; page++) {
    const r = await searchInstances({ model_id: appModelId.value, page, page_size: 500 })
    all.push(...(r.list || []))
    if (all.length >= (r.total || 0) || !(r.list || []).length) break
  }
  apps.value = all.map((it) => ({ id: it.id, ...it.data, _state: appState(it.data.status) }))
}
async function loadAlerts() {
  try {
    const rows = []
    for (let page = 1; page <= 30; page++) {
      const r = await listAlerts({ status: 'firing', days: 30, page, page_size: 200 })
      rows.push(...(r.list || []))
      if (rows.length >= (r.total || 0) || !(r.list || []).length) break
    }
    const map = {}
    for (const a of rows) {
      if (!a.res_inst_id || a.res_model !== '应用服务') continue
      const e = map[a.res_inst_id] || { count: 0, level: 'info' }
      e.count++
      if (alvOf(a.level).r > alvOf(e.level).r) e.level = a.level
      map[a.res_inst_id] = e
    }
    alertMap.value = map
  } catch { alertMap.value = {} }
}
async function reload() { await loadApps(); await loadAlerts(); await loadMetrics() }

// ---- 集群拓扑（卡片式：集群 → 命名空间 → 服务）----
const expandedNs = ref(new Set())
const NS_CAP = 6 // 每个命名空间默认最多显示的服务卡片数
const nsGroups = computed(() => {
  const m = new Map()
  for (const a of scoped.value) {
    const ns = a.namespace || '—'
    if (!m.has(ns)) m.set(ns, [])
    m.get(ns).push(a)
  }
  return [...m.entries()]
    .map(([ns, list]) => {
      list.sort((x, y) => stRank(x._state) - stRank(y._state) || String(x.app_name).localeCompare(String(y.app_name)))
      const abn = list.filter((a) => a._state === 'bad').length
      const show = expandedNs.value.has(ns) ? list : list.slice(0, NS_CAP)
      return { ns, list, show, abn, more: list.length - show.length }
    })
    .sort((a, b) => nsRank(a.ns) - nsRank(b.ns) || (b.abn > 0) - (a.abn > 0) || b.list.length - a.list.length || a.ns.localeCompare(b.ns))
})
function toggleNs(ns) {
  const s = new Set(expandedNs.value)
  s.has(ns) ? s.delete(ns) : s.add(ns)
  expandedNs.value = s
  nextTick(() => onTopoScroll())
}
// 集群拓扑横向滚动：集群卡始终保持在视口水平中心（滑动时做位移补偿）
const topoScrollEl = ref()
const clusterCardEl = ref()
function onTopoScroll() {
  const el = topoScrollEl.value, card = clusterCardEl.value
  if (!el || !card) return
  card.style.transform = `translateX(${el.scrollLeft - (el.scrollWidth - el.clientWidth) / 2}px)`
}
function centerTopo() {
  // 集群卡由 onTopoScroll 做位移补偿保持居中，无需再滚动内容到中间
  nextTick(() => { onTopoScroll(); setTimeout(onTopoScroll, 120) })
}

// ---- 资源使用（集群监控）----
const lineEl = ref()
let lineChart = null
const metricsReady = ref(false)
const metricsMsg = ref('')
const metricsSeries = ref([])
const podRunning = ref(null)
async function loadMetrics() {
  metricsReady.value = false
  metricsSeries.value = []
  podRunning.value = null
  const cid = curCluster.value?.id
  if (!cid) { metricsMsg.value = '无集群实例'; return }
  try {
    const d = await getHostMetrics(cid, mRange.value)
    metricsSeries.value = d.series || []
    const pr = seriesLast('pod_running')
    if (pr != null) podRunning.value = Math.round(pr)
    metricsReady.value = !!d.supported && (seriesLast('cpu') != null || seriesLast('mem') != null)
    metricsMsg.value = d.message || ''
    await nextTick()
    if (metricsReady.value) renderLine()
  } catch { metricsReady.value = false; metricsMsg.value = '监控加载失败' }
}
function seriesOf(key) { return metricsSeries.value.find((s) => s.key === key) }
function seriesLast(key) { const s = seriesOf(key); return s && s.points && s.points.length ? s.points[s.points.length - 1][1] : null }
const gauges = computed(() => {
  const g = []
  const cpu = seriesLast('cpu'), mem = seriesLast('mem')
  const nr = seriesLast('node_ready'), nt = seriesLast('node_total')
  const pr = seriesLast('pod_running'), pf = seriesLast('pod_failed'), pp = seriesLast('pod_pending')
  if (cpu != null) g.push({ key: 'cpu', name: 'CPU 使用率', color: '#2f66f0', val: cpu, sub: '集群整体' })
  if (mem != null) g.push({ key: 'mem', name: '内存使用率', color: '#16a34a', val: mem, sub: '集群整体' })
  if (nt) g.push({ key: 'node', name: '节点就绪率', color: '#7c3aed', val: (nr / nt) * 100, sub: `${nr ?? 0} / ${nt} 节点` })
  else if (pr != null) {
    const tot = pr + (pf || 0) + (pp || 0)
    g.push({ key: 'pod', name: 'Pod 运行率', color: '#7c3aed', val: tot ? (pr / tot) * 100 : 0, sub: `${pr} / ${tot} Pod` })
  }
  return g
})
function renderLine() {
  if (!lineEl.value) return
  if (!lineChart || lineChart.getDom() !== lineEl.value) { lineChart && lineChart.dispose(); lineChart = echarts.init(lineEl.value) }
  const dark = document.documentElement.classList.contains('dark')
  const axis = dark ? '#8a94a6' : '#9aa4b2', split = dark ? 'rgba(255,255,255,.06)' : 'rgba(0,0,0,.05)'
  const mk = (key, name, color) => {
    const s = seriesOf(key)
    return s ? { name, type: 'line', smooth: true, showSymbol: false, lineStyle: { width: 1.6, color }, itemStyle: { color }, data: s.points.map((p) => [p[0] * 1000, p[1]]) } : null
  }
  const series = [mk('cpu', 'CPU', '#2f66f0'), mk('mem', '内存', '#16a34a')].filter(Boolean)
  lineChart.setOption({
    grid: { left: 8, right: 12, top: 26, bottom: 6, containLabel: true },
    tooltip: { trigger: 'axis', backgroundColor: dark ? '#1f2430' : '#fff', borderColor: dark ? '#333a48' : '#eee', textStyle: { color: dark ? '#dfe4ea' : '#333', fontSize: 12 }, formatter: (ps) => `${echarts.format.formatTime('hh:mm', ps[0].value[0])}<br/>` + ps.map((p) => `${p.marker}${p.seriesName}：<b>${p.value[1].toFixed(1)}%</b>`).join('<br/>') },
    legend: { right: 6, top: 0, itemWidth: 10, itemHeight: 8, textStyle: { color: axis, fontSize: 11 } },
    xAxis: { type: 'time', axisLine: { lineStyle: { color: split } }, axisLabel: { color: axis, fontSize: 10, hideOverlap: true }, axisTick: { show: false } },
    yAxis: { type: 'value', min: 0, max: 100, axisLabel: { color: axis, fontSize: 10, formatter: '{value}%' }, splitLine: { lineStyle: { color: split } } },
    series,
  }, true)
  nextTick(() => lineChart && lineChart.resize())
}

// 切集群：重置筛选、选中首个服务、重画拓扑、拉监控
watch(cluster, async () => {
  nsFilter.value = ''; stFilter.value = ''; kw.value = ''
  sel.value = null; selAlerts.value = []; expandedNs.value = new Set()
  // 默认选中排序后的第一个（业务命名空间优先 → 异常优先 → 命名空间 → 名称），与服务列表/拓扑一致
  const first = [...scoped.value].sort(
    (a, b) => nsRank(a.namespace) - nsRank(b.namespace) || stRank(a._state) - stRank(b._state) || String(a.namespace).localeCompare(String(b.namespace)) || String(a.app_name).localeCompare(String(b.app_name)),
  )[0]
  if (first) selectApp(first)
  centerTopo()
  await loadMetrics()
})

// 服务列表表格高度：按容器实际高度自适应（保证整页一屏放下）
const listBodyEl = ref()
const tableH = ref(240)
let ro = null
function resize() { lineChart && lineChart.resize() }
// 查看大图：新标签页打开独立的集群拓扑全页
function openBigTopo() {
  window.open('/app-topo?cluster=' + encodeURIComponent(cluster.value), '_blank', 'noopener')
}
onMounted(async () => {
  await load()
  window.addEventListener('resize', resize)
  if (window.ResizeObserver && listBodyEl.value) {
    ro = new ResizeObserver(() => { tableH.value = Math.max(160, listBodyEl.value.clientHeight) })
    ro.observe(listBodyEl.value)
  }
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', resize)
  ro && ro.disconnect()
  lineChart && lineChart.dispose()
})
</script>

<style scoped>
/* 整页按内容区高度自适应：大屏一屏铺满；屏幕不够高时各区块保持最小高度、整页改为滚动（不挤叠） */
.k8s-page { min-height: 100%; display: flex; flex-direction: column; gap: 12px; }
.cl-head, .stat-row { flex-shrink: 0; }
.panel { background: var(--card-bg); border: 1px solid var(--card-border); border-radius: var(--radius-lg); box-shadow: var(--shadow-card); padding: 14px 16px; }
.mid-row > .panel, .bot-row > .panel { min-height: 0; display: flex; flex-direction: column; overflow: hidden; }
.panel-head { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.panel-title { font-size: 15px; font-weight: 700; color: var(--text-primary); }
.muted { color: var(--text-muted); }
.muted.sm, .sm { font-size: 12px; }

/* 头部 */
.cl-head { display: flex; align-items: center; justify-content: space-between; gap: 16px; background: var(--card-bg); border: 1px solid var(--card-border); border-radius: var(--radius-lg); box-shadow: var(--shadow-card); padding: 16px 20px; flex-wrap: wrap; }
.clh-left { display: flex; align-items: center; gap: 14px; }
.clh-logo { width: 52px; height: 52px; border-radius: 14px; background: #eef4ff; display: flex; align-items: center; justify-content: center; }
.clh-title { display: flex; align-items: center; gap: 10px; }
.clh-select { width: 230px; }
.clh-select :deep(.el-input__wrapper) { box-shadow: none; padding-left: 0; }
.clh-select :deep(.el-input__inner) { font-size: 20px; font-weight: 700; color: var(--text-primary); }
.clh-badge { display: inline-flex; align-items: center; gap: 5px; font-size: 13px; font-weight: 600; padding: 2px 10px; border-radius: 8px; }
.clh-badge.sm { font-size: 12px; padding: 1px 8px; }
.dot { width: 7px; height: 7px; border-radius: 50%; }
.clh-meta { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; font-size: 13px; color: var(--text-secondary); margin-top: 6px; }
.clh-meta b { color: var(--text-primary); font-weight: 600; }
.clh-meta .sep { width: 1px; height: 11px; background: var(--border-light); display: inline-block; }
.clh-actions { display: flex; align-items: center; gap: 10px; }

/* 统计卡 */
.stat-row { display: grid; grid-template-columns: repeat(6, 1fr); gap: 12px; }
.stat-card { display: flex; align-items: center; gap: 12px; background: var(--card-bg); border: 1px solid var(--card-border); border-radius: var(--radius-lg); box-shadow: var(--shadow-card); padding: 14px 16px; }
.sc-ic { width: 42px; height: 42px; border-radius: 11px; display: flex; align-items: center; justify-content: center; font-size: 21px; flex-shrink: 0; }
.ic-blue { background: #eaf1ff; color: #3b82f6; } .ic-cyan { background: #e6f6fb; color: #0ea5e9; } .ic-indigo { background: #eef0ff; color: #6366f1; }
.ic-violet { background: #f3ecff; color: #8b5cf6; } .ic-green { background: #e8f8ee; color: #22c55e; } .ic-red { background: #fdecec; color: #ef4444; }
.sc-grow { flex: 1; min-width: 0; }
.sc-lab { font-size: 12.5px; color: var(--text-secondary); margin-bottom: 3px; }
.sc-val { font-size: 24px; font-weight: 700; color: var(--text-primary); line-height: 1; }
.sc-val.danger { color: #ef4444; }
.sc-pct { font-size: 12px; font-weight: 600; color: var(--text-muted); }
.sc-stopped { font-size: 11px; font-weight: 400; color: var(--text-muted); margin-left: 4px; }
.sc-bar { height: 4px; border-radius: 2px; background: var(--border-light); margin-top: 7px; overflow: hidden; }
.sc-bar i { display: block; height: 100%; border-radius: 2px; }

/* 中部 */
.mid-row { display: grid; grid-template-columns: 1fr 360px; gap: 12px; flex: 1.2 1 auto; min-height: 340px; }
/* 集群拓扑卡片树 */
.topo-panel { --line: #cbd5e1; }
.tp-max { margin-left: auto; }
.topo-scroll { flex: 1; min-height: 240px; overflow: auto; }
.topo-tree { min-width: max-content; padding: 6px 12px 12px; }
.tt-cluster { display: flex; justify-content: center; position: relative; padding-bottom: 20px; }
.tt-ns-row { display: flex; justify-content: center; align-items: flex-start; gap: 18px; }
.ns-col { position: relative; padding-top: 20px; display: flex; flex-direction: column; align-items: center; }
.ns-col::before { content: ''; position: absolute; top: 0; left: 0; right: 0; height: 2px; background: var(--line); }
.ns-col:first-child::before { left: 50%; }
.ns-col:last-child::before { right: 50%; }
.ns-col:only-child::before { display: none; }
.ns-col::after { content: ''; position: absolute; top: 0; left: 50%; width: 2px; height: 20px; background: var(--line); transform: translateX(-50%); }

.node-card { display: flex; align-items: center; gap: 9px; background: var(--card-bg); border: 1px solid #e5eaf2; border-radius: 10px; padding: 8px 12px; box-shadow: 0 1px 3px rgba(31, 45, 61, 0.07); }
.nc-cluster { background: linear-gradient(135deg, #eef4ff, #fff); border-color: #c3d6fb; min-width: 150px; position: relative; z-index: 2; }
.nc-cluster::after { content: ''; position: absolute; top: 100%; left: 50%; width: 2px; height: 20px; background: var(--line); transform: translateX(-50%); }
.nc-ns { min-width: 134px; }
.nc-ns.bad { border-color: #f4b4b4; }
.nc-ic { width: 32px; height: 32px; border-radius: 8px; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.ic-k8s { background: #e8f0fe; } .ic-ns { background: #f3ecff; color: #8b5cf6; }
.nc-body { min-width: 0; }
.nc-name { font-size: 13px; font-weight: 700; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 160px; }
.nc-sub { font-size: 11.5px; color: var(--text-secondary); margin-top: 2px; display: flex; align-items: center; gap: 5px; }
.nc-abn { color: #ef4444; margin-left: 4px; }
.dot { width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0; }

.svc-list { position: relative; margin-top: 12px; padding-left: 16px; width: 182px; }
.svc-list::before { content: ''; position: absolute; left: 7px; top: -12px; bottom: 14px; width: 2px; background: var(--line); }
.svc-card { position: relative; display: flex; align-items: center; gap: 6px; background: var(--card-bg); border: 1px solid #e8edf4; border-radius: 8px; padding: 5px 8px; margin-bottom: 7px; cursor: pointer; transition: border-color 0.12s, box-shadow 0.12s; }
.svc-card::before { content: ''; position: absolute; left: -9px; top: 50%; width: 9px; height: 2px; background: var(--line); transform: translateY(-50%); }
.svc-card:hover { border-color: #93b4fb; box-shadow: 0 2px 8px rgba(36, 104, 242, 0.12); }
.svc-card.on { border-color: #2468f2; box-shadow: 0 0 0 2px rgba(36, 104, 242, 0.15); }
.svc-card.bad { background: #fff6f6; border-color: #f4c5c5; }
.svc-ic { width: 20px; height: 20px; border-radius: 5px; background: #f1f5fb; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.svc-name { flex: 1; min-width: 0; font-size: 12px; font-weight: 500; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.svc-pod { font-size: 10.5px; color: var(--text-muted); flex-shrink: 0; }
.svc-more { position: relative; font-size: 11px; color: var(--brand); cursor: pointer; padding: 3px 8px; }
.svc-more::before { content: ''; position: absolute; left: -9px; top: 50%; width: 9px; height: 2px; background: var(--line); transform: translateY(-50%); }
.svc-more:hover { text-decoration: underline; }
html.dark .topo-panel { --line: #39414f; }
html.dark .node-card, html.dark .svc-card { border-color: #2c3342; }
html.dark .svc-ic { background: #222b38; }
html.dark .nc-cluster { background: linear-gradient(135deg, #1b2740, #141a24); border-color: #2d3a52; }
html.dark .svc-card.bad { background: #2a1d1d; border-color: #5a3838; }
.detail-panel { display: flex; flex-direction: column; }
.dp-edit { margin-left: auto; }
.ef-hint { font-size: 12px; color: var(--text-muted); margin-top: 2px; line-height: 1.5; }
.detail-body { flex: 1; min-height: 0; overflow: auto; }
.d-title { display: flex; align-items: center; gap: 8px; margin: 2px 0 12px; }
.d-name { font-size: 16px; font-weight: 700; color: var(--text-primary); }
.d-kv { display: flex; flex-direction: column; }
.d-kv .kv { display: flex; gap: 10px; padding: 8px 0; border-bottom: 1px dashed var(--border-light); font-size: 13px; }
.d-kv .k { color: var(--text-secondary); min-width: 68px; }
.d-kv .v { color: var(--text-primary); font-weight: 500; word-break: break-all; }
.d-kv .v.link { color: var(--brand); } .d-kv .v.mono { font-family: var(--font-mono, monospace); font-weight: 400; font-size: 12px; }
.d-kv .v.clk { cursor: pointer; } .d-kv .v.clk:hover { text-decoration: underline; }
.d-al { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin: 12px 0 10px; }
.da-firing { font-size: 12px; font-weight: 600; color: #ef4444; background: #fdecec; border-radius: 9px; padding: 1px 8px; }
.da-chip { display: inline-flex; align-items: center; gap: 4px; font-size: 12px; color: var(--text-regular); background: var(--fill-bg); border-radius: 8px; padding: 2px 8px; }
.da-chip.resolved { opacity: .5; }
.d-more { margin-top: auto; }

/* 底部 */
.bot-row { display: grid; grid-template-columns: 1fr 360px; gap: 12px; flex: 1 1 auto; min-height: 300px; }
.list-body { flex: 1; min-height: 0; }
.lp-filters { margin-left: auto; display: flex; gap: 8px; }
.f-ns { width: 140px; } .f-st { width: 110px; } .f-kw { width: 160px; }
.tl-name { color: var(--brand); cursor: pointer; font-weight: 500; }
.tl-name.on { font-weight: 700; }
.tl-st { display: inline-flex; align-items: center; gap: 5px; }
.tl-al { display: inline-block; margin-left: 6px; min-width: 16px; height: 16px; padding: 0 4px; border-radius: 8px; color: #fff; font-size: 11px; font-weight: 700; line-height: 16px; text-align: center; }
.list-panel :deep(.el-table) { cursor: pointer; }

/* 资源使用 */
.res-panel { --track: #eef1f6; display: flex; flex-direction: column; }
.res-empty { flex: 1; display: flex; align-items: center; justify-content: center; }
.donut-row { display: flex; justify-content: space-around; gap: 8px; margin: 4px 0 14px; flex-shrink: 0; }
.donut { text-align: center; }
.ring { width: 76px; height: 76px; border-radius: 50%; display: flex; align-items: center; justify-content: center; margin: 0 auto; }
.ring-in { width: 58px; height: 58px; border-radius: 50%; background: var(--card-bg); display: flex; align-items: center; justify-content: center; }
.ring-val { font-size: 15px; font-weight: 700; }
.donut-name { font-size: 12.5px; color: var(--text-regular); margin-top: 7px; font-weight: 500; }
.donut-sub { font-size: 11px; color: var(--text-muted); margin-top: 2px; }
.res-line { width: 100%; flex: 1; min-height: 160px; }

.dw-metrics { margin-bottom: 16px; }
.dw-head { font-size: 14px; font-weight: 700; color: var(--text-primary); margin: 4px 0 10px; }

html.dark .res-panel { --track: #262d3a; }
html.dark .clh-logo { background: #1c2636; }
@media (max-width: 1100px) {
  .stat-row { grid-template-columns: repeat(3, 1fr); }
  .mid-row, .bot-row { grid-template-columns: 1fr; }
}
</style>
