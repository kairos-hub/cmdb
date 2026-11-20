<template>
  <div class="topo-full" v-loading="loading">
    <div class="tf-bar">
      <span class="tf-title">集群拓扑</span>
      <el-select v-model="cluster" filterable class="tf-select" placeholder="选择集群" @change="onCluster">
        <el-option v-for="c in clusters" :key="c" :label="c" :value="c" />
      </el-select>
      <span v-if="scoped.length" class="tf-stat">{{ scoped.length }} 服务 · {{ namespaces.length }} 命名空间</span>
      <el-button class="tf-refresh" :icon="Refresh" circle size="small" title="刷新" @click="load" />
    </div>
    <div class="tf-body" ref="tfBodyEl" @scroll="onTopoScroll">
      <el-empty v-if="!loading && !scoped.length" description="该集群暂无应用服务" />
      <div v-else class="topo-tree">
        <div class="tt-cluster">
          <div ref="clusterCardEl" class="node-card nc-cluster">
            <div class="nc-ic ic-k8s"><ModelIcon obj-id="kubernetes" fallback="Box" :size="20" /></div>
            <div class="nc-body">
              <div class="nc-name">{{ cluster }}</div>
              <div class="nc-sub"><span class="dot" :style="{ background: stColor(clusterState) }"></span>{{ stText(clusterState) }}</div>
            </div>
          </div>
        </div>
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
                :class="{ bad: a._state === 'bad' }"
                :title="`${a.app_name} · ${a.workload_type || ''} · ${a.replicas_available ?? 0}/${a.replicas_desired ?? 0} · ${stText(a._state)}`"
              >
                <div class="svc-ic"><ModelIcon obj-id="k8s_app" fallback="Grid" :size="13" /></div>
                <span class="svc-name">{{ a.app_name }}</span>
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
</template>

<script setup>
import { ref, computed, onMounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { Refresh, Menu } from '@element-plus/icons-vue'
import { searchInstances } from '@/api/instance'
import { listModels } from '@/api/model'
import ModelIcon from '@/components/ModelIcon.vue'

const route = useRoute()
const loading = ref(false)
const appModelId = ref(0)
const apps = ref([])
const clusterState = ref('running')
const cluster = ref(String(route.query.cluster || ''))

function appState(s) {
  if (s === 'running' || s === '1' || s === 'normal') return 'running'
  if (s === 'stopped' || s === 'offline') return 'stopped'
  return 'bad'
}
const ST = { running: { t: '运行中', c: '#22c55e' }, stopped: { t: '已停止', c: '#9ca3af' }, bad: { t: '异常', c: '#ef4444' } }
const stText = (s) => (ST[s] || ST.bad).t
const stColor = (s) => (ST[s] || ST.bad).c
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
const nsRank = (ns) => (isSysNs(ns) ? 1 : 0)

const clusters = computed(() => [...new Set(apps.value.map((a) => a.cluster_name).filter(Boolean))].sort())
const scoped = computed(() => apps.value.filter((a) => a.cluster_name === cluster.value))
const namespaces = computed(() => [...new Set(scoped.value.map((a) => a.namespace).filter(Boolean))].sort())
const expandedNs = ref(new Set())
const NS_CAP = 10 // 每命名空间默认显示的服务数，多的折叠
function toggleNs(ns) {
  const s = new Set(expandedNs.value)
  s.has(ns) ? s.delete(ns) : s.add(ns)
  expandedNs.value = s
  nextTick(() => onTopoScroll())
}
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
      const show = expandedNs.value.has(ns) ? list : list.slice(0, NS_CAP)
      return { ns, list, show, more: list.length - show.length, abn: list.filter((a) => a._state === 'bad').length }
    })
    .sort((a, b) => nsRank(a.ns) - nsRank(b.ns) || (b.abn > 0) - (a.abn > 0) || b.list.length - a.list.length || a.ns.localeCompare(b.ns))
})

const tfBodyEl = ref()
const clusterCardEl = ref()
// 横向滑动时集群卡始终保持在视口水平中心（位移补偿）
function onTopoScroll() {
  const el = tfBodyEl.value, card = clusterCardEl.value
  if (!el || !card) return
  card.style.transform = `translateX(${el.scrollLeft - (el.scrollWidth - el.clientWidth) / 2}px)`
}
function centerView() {
  // 集群卡由 onTopoScroll 做位移补偿保持居中，无需再滚动内容到中间
  nextTick(() => { onTopoScroll(); setTimeout(onTopoScroll, 120) })
}
function onCluster() {
  expandedNs.value = new Set()
  const u = new URL(window.location.href)
  u.searchParams.set('cluster', cluster.value)
  window.history.replaceState(null, '', u)
  document.title = `集群拓扑 - ${cluster.value}`
  centerView()
}
async function load() {
  loading.value = true
  try {
    if (!appModelId.value) {
      const ms = await listModels()
      appModelId.value = ms.find((m) => m.obj_id === 'k8s_app')?.id || 0
    }
    if (!appModelId.value) { apps.value = []; return }
    const all = []
    for (let page = 1; page <= 50; page++) {
      const r = await searchInstances({ model_id: appModelId.value, page, page_size: 500 })
      all.push(...(r.list || []))
      if (all.length >= (r.total || 0) || !(r.list || []).length) break
    }
    apps.value = all.map((it) => ({ id: it.id, ...it.data, _state: appState(it.data.status) }))
    if (!cluster.value || !clusters.value.includes(cluster.value)) cluster.value = clusters.value[0] || ''
    onCluster()
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<style scoped>
.topo-full { --line: #cbd5e1; display: flex; flex-direction: column; height: 100vh; background: var(--app-bg, #f5f7fa); }
.tf-bar { display: flex; align-items: center; gap: 12px; padding: 12px 20px; border-bottom: 1px solid var(--card-border); background: var(--card-bg); position: sticky; top: 0; z-index: 2; }
.tf-title { font-size: 16px; font-weight: 700; color: var(--text-primary); }
.tf-select { width: 240px; }
.tf-stat { font-size: 13px; color: var(--text-secondary); }
.tf-refresh { margin-left: auto; }
.tf-body { flex: 1; min-height: 0; overflow: auto; padding: 20px; }

/* 单行组织图：集群 → 横向总线 → 各命名空间（正交连线）；超宽横向滚动 */
.topo-tree { min-width: max-content; padding: 4px 12px 14px; }
.tt-cluster { display: flex; justify-content: center; position: relative; padding-bottom: 22px; }
.tt-ns-row { display: flex; justify-content: center; align-items: flex-start; gap: 22px; }
.ns-col { position: relative; padding-top: 22px; display: flex; flex-direction: column; align-items: center; }
.ns-col::before { content: ''; position: absolute; top: 0; left: 0; right: 0; height: 2px; background: var(--line); }
.ns-col:first-child::before { left: 50%; }
.ns-col:last-child::before { right: 50%; }
.ns-col:only-child::before { display: none; }
.ns-col::after { content: ''; position: absolute; top: 0; left: 50%; width: 2px; height: 22px; background: var(--line); transform: translateX(-50%); }

.node-card { display: flex; align-items: center; gap: 9px; background: var(--card-bg); border: 1px solid #e5eaf2; border-radius: 10px; padding: 8px 12px; box-shadow: 0 1px 3px rgba(31, 45, 61, 0.07); }
.nc-cluster { background: linear-gradient(135deg, #eef4ff, #fff); border-color: #c3d6fb; display: inline-flex; position: relative; z-index: 2; }
.nc-cluster::after { content: ''; position: absolute; top: 100%; left: 50%; width: 2px; height: 22px; background: var(--line); transform: translateX(-50%); }
.nc-ns.bad { border-color: #f4b4b4; }
.nc-ic { width: 32px; height: 32px; border-radius: 8px; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.ic-k8s { background: #e8f0fe; } .ic-ns { background: #f3ecff; color: #8b5cf6; }
.nc-body { min-width: 0; }
.nc-name { font-size: 13px; font-weight: 700; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.nc-sub { font-size: 11.5px; color: var(--text-secondary); margin-top: 2px; display: flex; align-items: center; gap: 5px; }
.nc-abn { color: #ef4444; margin-left: 4px; }
.dot { width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0; }

.svc-list { position: relative; margin-top: 12px; padding-left: 16px; width: 190px; }
.svc-list::before { content: ''; position: absolute; left: 7px; top: -12px; bottom: 14px; width: 2px; background: var(--line); }
.svc-card { position: relative; display: flex; align-items: center; gap: 6px; background: var(--card-bg); border: 1px solid #e8edf4; border-radius: 8px; padding: 5px 8px; margin-bottom: 7px; }
.svc-card::before { content: ''; position: absolute; left: -9px; top: 50%; width: 9px; height: 2px; background: var(--line); transform: translateY(-50%); }
.svc-card.bad { background: #fff6f6; border-color: #f4c5c5; }
.svc-ic { width: 20px; height: 20px; border-radius: 5px; background: #f1f5fb; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.svc-name { flex: 1; min-width: 0; font-size: 12px; font-weight: 500; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.svc-pod { font-size: 10.5px; color: var(--text-muted); flex-shrink: 0; }
.svc-more { position: relative; font-size: 11px; color: var(--brand); cursor: pointer; padding: 3px 8px; }
.svc-more::before { content: ''; position: absolute; left: -9px; top: 50%; width: 9px; height: 2px; background: var(--line); transform: translateY(-50%); }
.svc-more:hover { text-decoration: underline; }

html.dark .topo-full { --line: #39414f; }
html.dark .node-card, html.dark .svc-card { border-color: #2c3342; }
html.dark .svc-ic { background: #222b38; }
html.dark .nc-cluster { background: linear-gradient(135deg, #1b2740, #141a24); border-color: #2d3a52; }
html.dark .svc-card.bad { background: #2a1d1d; border-color: #5a3838; }
</style>
