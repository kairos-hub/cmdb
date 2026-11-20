<template>
  <div v-loading="loading">
    <el-table :data="pagedRelations" border size="small">
      <el-table-column label="关系" width="180">
        <template #default="{ row }">
          <el-tag size="small" :type="row.direction === 'out' ? 'primary' : 'info'">{{ row.note }}</el-tag>
          <el-tag v-if="row.derived" size="small" type="success" effect="plain" class="rel-tag">拓扑</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="对端模型" width="120">
        <template #default="{ row }">{{ row.peer_model_name }}</template>
      </el-table-column>
      <el-table-column label="对端实例">
        <template #default="{ row }">
          <el-button link :type="row.peer_abnormal ? 'danger' : 'primary'" @click="goInstance(row.peer_inst_id)">
            {{ row.peer_display || `#${row.peer_inst_id}` }}
          </el-button>
          <el-tag v-if="row.peer_abnormal" size="small" type="danger" effect="dark" class="rel-tag">
            {{ row.peer_status || '异常' }}
          </el-tag>
        </template>
      </el-table-column>
      <template #empty>暂无关联</template>
    </el-table>

    <el-pagination
      v-if="relations.length > pageSize"
      class="rel-pager"
      small
      background
      layout="total, prev, pager, next"
      :total="relations.length"
      :current-page="page"
      :page-size="pageSize"
      @current-change="(p) => (page = p)"
    />

    <div class="graph-wrap">
      <el-empty v-if="!relations.length" description="暂无关联，无法生成拓扑" />
      <div v-show="relations.length" class="rel-legend">
        <span
          v-for="it in legendItems"
          :key="it.id"
          class="rel-legend-item"
          :class="{ off: hiddenModels.has(it.id) }"
          @click="toggleModel(it.id)"
        >
          <i class="rel-sw" :style="{ background: it.color }"></i>{{ it.name }}
        </span>
      </div>
      <div v-show="relations.length" ref="graphEl" class="graph"></div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { useRouter } from 'vue-router'
import * as echarts from 'echarts'
import { nodeSymbol, catColor, DANGER } from '@/utils/topoSymbol'
import { listRelations } from '@/api/instance'
import { listModels } from '@/api/model'

const props = defineProps({
  instanceId: { type: [Number, String], required: true },
  selfLabel: { type: String, default: '' },
  selfModelName: { type: String, default: '当前' },
  selfObjId: { type: String, default: '' },
  selfIcon: { type: String, default: '' },
  selfAbnormal: { type: Boolean, default: false },
  selfStatus: { type: String, default: '' },
})

const router = useRouter()
const loading = ref(false)
const relations = ref([])
const modelMeta = ref({}) // model_id -> { obj_id, name, icon }
const page = ref(1)
const pageSize = ref(10)
const pagedRelations = computed(() =>
  relations.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value),
)
// 图例：每个关联对端模型一个「色块 + 模型名」，颜色与拓扑节点一致；可点击隐藏/显示
const legendItems = computed(() => {
  const seen = new Map()
  for (const r of relations.value) {
    if (seen.has(r.peer_model_id)) continue
    const meta = modelMeta.value[r.peer_model_id] || {}
    const name = r.peer_model_name || meta.name || '其它'
    seen.set(r.peer_model_id, { id: r.peer_model_id, name, color: catColor(meta.obj_id || name) })
  }
  return [...seen.values()]
})
const hiddenModels = ref(new Set()) // 图例隐藏的对端模型 id
function toggleModel(id) {
  const s = new Set(hiddenModels.value)
  s.has(id) ? s.delete(id) : s.add(id)
  hiddenModels.value = s
  renderGraph()
}

let chart = null
const graphEl = ref()

async function load() {
  loading.value = true
  hiddenModels.value = new Set() // 换实例时重置图例隐藏（树折叠状态由 ECharts initialTreeDepth 控制）
  try {
    const [rels, models] = await Promise.all([
      listRelations(props.instanceId),
      Object.keys(modelMeta.value).length ? Promise.resolve(null) : listModels(),
    ])
    relations.value = rels || []
    if (models) {
      const map = {}
      for (const m of models) map[m.id] = { obj_id: m.obj_id, name: m.name, icon: m.icon }
      modelMeta.value = map
    }
    page.value = 1
  } finally {
    loading.value = false
  }
  await nextTick()
  renderGraph()
}

function renderGraph() {
  if (!relations.value.length || !graphEl.value) return
  if (!chart) chart = echarts.init(graphEl.value)

  // 1) 按对端模型分组；每个对端实例合并多条关系（保留各自 note/方向用于 tooltip）
  const groups = new Map() // model_id -> { meta, modelName, peers: Map(inst_id -> {display, notes[]}) }
  for (const r of relations.value) {
    let g = groups.get(r.peer_model_id)
    if (!g) {
      const meta = modelMeta.value[r.peer_model_id] || {}
      g = { meta, modelName: r.peer_model_name || meta.name || '其它', peers: new Map() }
      groups.set(r.peer_model_id, g)
    }
    let p = g.peers.get(r.peer_inst_id)
    if (!p) {
      p = { display: r.peer_display || `#${r.peer_inst_id}`, notes: [], abnormal: !!r.peer_abnormal, status: r.peer_status || '' }
      g.peers.set(r.peer_inst_id, p)
    }
    p.notes.push({ note: r.note, direction: r.direction, derived: r.derived })
  }
  const modelIds = [...groups.keys()].filter((mid) => !hiddenModels.value.has(mid)) // 图例隐藏的模型不画

  // 2) 横向树：self(根) → 各模型分组 hub → 该模型各对端实例。默认折叠(initialTreeDepth=1，只显 self+hub)。
  const selfName = props.selfLabel || `实例 #${props.instanceId}`
  const children = modelIds.map((mid) => {
    const g = groups.get(mid)
    const color = catColor(g.meta.obj_id || g.modelName)
    let abnN = 0
    for (const [, p] of g.peers) if (p.abnormal) abnN++
    const leaves = [...g.peers.entries()].map(([iid, p]) => ({
      name: p.display,
      symbol: nodeSymbol(g.meta.obj_id, g.meta.icon, color, p.abnormal),
      symbolSize: 32,
      label: { color: p.abnormal ? DANGER : '#475569', fontWeight: p.abnormal ? 600 : 400 },
      _inst: iid,
      _notes: p.notes,
      _status: p.status,
      _abnormal: p.abnormal,
    }))
    return {
      name: `${g.modelName} ×${g.peers.size}${abnN ? ` ⚠${abnN}` : ''}`,
      symbol: nodeSymbol(g.meta.obj_id, g.meta.icon, color, false),
      symbolSize: 40,
      // 模型名完整展示（放宽、换行不截断）
      label: { color: abnN ? DANGER : '#334155', fontWeight: 600, width: 150, overflow: 'break' },
      children: leaves,
    }
  })
  const root = {
    name: props.selfAbnormal && props.selfStatus ? `${selfName}（${props.selfStatus}）` : selfName,
    symbol: nodeSymbol(props.selfObjId, props.selfIcon, undefined, props.selfAbnormal),
    symbolSize: 50,
    label: { color: props.selfAbnormal ? DANGER : '#1e293b', fontWeight: 700 },
    _status: props.selfStatus,
    _abnormal: props.selfAbnormal,
    children,
  }

  chart.setOption(
    {
      tooltip: { trigger: 'item', formatter: graphTooltip, confine: true, borderColor: '#e2e8f0' },
      series: [
        {
          type: 'tree',
          orient: 'TB',
          roam: false, // 不手动缩放，始终自动适配容器，避免「进去就遮挡、要手动缩小才看全」
          top: '22%',
          left: '12%',
          right: '12%',
          bottom: '22%',
          symbolSize: 40,
          initialTreeDepth: 1, // 默认只展开到第一层：self + 各模型 hub（实例折叠）
          expandAndCollapse: true,
          animationDurationUpdate: 450,
          edgeShape: 'polyline', // 正交直线（非曲线）
          edgeForkPosition: '50%',
          lineStyle: { color: '#cbd5e1', width: 1.4 },
          label: {
            position: 'top',
            verticalAlign: 'bottom',
            align: 'center',
            fontSize: 12,
            color: '#475569',
            distance: 6,
            width: 96,
            overflow: 'truncate',
            ellipsis: '…',
            formatter: (p) => p.data.name,
          },
          leaves: { label: { position: 'bottom', verticalAlign: 'top', align: 'center' } },
          // 真实重叠检测：只隐藏实际糊在一起的标签（随折叠/展开自动生效），hover 时再内联显示
          labelLayout: { hideOverlap: true },
          // hover 高亮时强制显示该节点标签（即便被 hideOverlap 隐藏）
          emphasis: { focus: 'relative', label: { show: true, overflow: 'break', width: 140 } },
          data: [root],
        },
      ],
    },
    true,
  )
  chart.off('click')
  // 叶子(对端实例)点击跳详情；hub/root 点击由 ECharts 原生折叠展开处理
  chart.on('click', (p) => {
    if (p.data && p.data._inst) goInstance(p.data._inst)
  })
  // 渲染后多次按容器真实尺寸重算（覆盖初始化时容器尺寸未就绪的情况），避免错位/遮挡
  const fit = () => chart && chart.resize()
  nextTick(fit)
  requestAnimationFrame(fit)
  setTimeout(fit, 180)
}

// tooltip：对端实例显示其各条关系（方向 + note + 是否拓扑派生）；异常显示状态
function graphTooltip(p) {
  const d = p && p.data
  if (!d) return ''
  const title = d.name || ''
  if (d._notes && d._notes.length) {
    const head = d._abnormal
      ? `<b>${title}</b> <span style="color:${DANGER}">● ${d._status || '异常'}</span>`
      : `<b>${title}</b>${d._status ? ` · ${d._status}` : ''}`
    const lines = d._notes.map(
      (n) => `${n.direction === 'out' ? '→ ' : '← '}${n.note || ''}${n.derived ? ' （拓扑）' : ''}`,
    )
    return `${head}<br/>${lines.join('<br/>')}`
  }
  if (d._abnormal) return `<b>${title}</b> <span style="color:${DANGER}">● ${d._status || '异常'}</span>`
  return title
}

function resize() {
  chart && chart.resize()
}
function goInstance(id) {
  router.push(`/instances/${id}`)
}

let ro = null
onMounted(() => {
  load()
  window.addEventListener('resize', resize)
  // 容器自身尺寸变化(显示/布局变动)时自动重算画布，根治「画布尺寸没跟容器对齐、溢出盖住相邻元素」
  if (window.ResizeObserver && graphEl.value) {
    ro = new ResizeObserver(() => resize())
    ro.observe(graphEl.value)
  }
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', resize)
  if (ro) {
    ro.disconnect()
    ro = null
  }
  chart && chart.dispose()
})
watch(
  () => props.instanceId,
  () => load(),
)
// 自身名称/模型是父组件异步算出的，到达后若关联已加载则重画，避免自身节点定格成回退值
watch(
  () => [props.selfLabel, props.selfModelName, props.selfObjId, props.selfAbnormal, props.selfStatus],
  () => {
    if (relations.value.length) nextTick(renderGraph)
  },
)
</script>

<style scoped>
.rel-tag {
  margin-left: 4px;
}
.rel-pager {
  margin-top: 10px;
  justify-content: flex-end;
}
.graph-wrap {
  margin-top: 12px;
}
.rel-legend {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px 16px;
  padding: 2px 4px 8px;
}
.rel-legend-item {
  display: inline-flex;
  align-items: center;
  font-size: 12px;
  color: #475569;
  cursor: pointer;
  user-select: none;
}
.rel-legend-item.off {
  opacity: 0.4;
  text-decoration: line-through;
}
.rel-sw {
  width: 10px;
  height: 10px;
  border-radius: 3px;
  margin-right: 5px;
  flex-shrink: 0;
}
.graph {
  height: 420px;
  width: 100%;
  position: relative;
  overflow: hidden;
}
</style>
