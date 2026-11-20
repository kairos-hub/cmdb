<template>
  <div class="panel panel-pad box im">
    <div class="panel-head im-head">
      <span class="panel-title">资源监控</span>
      <span v-if="result.source_name" class="im-src">{{ result.source_name }}<i v-if="result.target"> · {{ result.target }}</i></span>
      <div class="im-right">
        <el-radio-group v-model="range" size="small" @change="load">
          <el-radio-button v-for="r in ranges" :key="r.v" :value="r.v">{{ r.t }}</el-radio-button>
        </el-radio-group>
        <el-button size="small" :icon="Refresh" :loading="loading" circle @click="load" />
        <el-button v-if="result.detail_url" size="small" :icon="TopRight" @click="openDetail">完整监控</el-button>
      </div>
    </div>

    <div v-if="!loading && (!supported || !cards.length)" class="im-empty">
      <el-empty :description="result.message || '该来源暂不支持监控'" :image-size="76" />
    </div>

    <div v-else v-loading="loading" class="im-grid">
      <div v-for="(c, idx) in cards" :key="c.key" class="im-card" :class="{ 'im-span': cards.length % 2 === 1 && idx === cards.length - 1 }">
        <div class="im-card-head">
          <span class="im-name">{{ c.name }}</span>
          <span class="im-cur" :style="{ color: c.color }">{{ c.curText }}</span>
        </div>
        <div v-show="c.hasData" :ref="(el) => (chartEls[c.key] = el)" class="im-chart"></div>
        <div v-show="!c.hasData" class="im-nodata">暂无数据</div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import * as echarts from 'echarts'
import { Refresh, TopRight } from '@element-plus/icons-vue'
import { getHostMetrics } from '@/api/instance'

const props = defineProps({
  instanceId: { type: [Number, String], required: true },
})

const ranges = [
  { v: '1h', t: '1时' },
  { v: '6h', t: '6时' },
  { v: '24h', t: '24时' },
  { v: '7d', t: '7天' },
]
const range = ref('1h')
const loading = ref(false)
const supported = ref(true)
const result = reactive({ source_name: '', target: '', message: '', detail_url: '', cards: [], series: [] })

function openDetail() {
  if (result.detail_url) window.open(result.detail_url, '_blank', 'noopener')
}

const CARD_COLOR = {
  cpu: '#2f66f0', mem: '#16a34a', disk: '#d97706', net: '#7c3aed',
  conn: '#0891b2', qps: '#2f66f0', tps: '#16a34a', slow: '#e34948', iops: '#db2777',
}
const PALETTE = ['#2f66f0', '#16a34a', '#d97706', '#7c3aed', '#0891b2', '#db2777', '#e34948', '#64748b']

const cards = ref([])
const chartEls = reactive({})
const charts = {}

// 网络带宽：后端已统一为 bit/s，按 1000 进制显示 bps/Kbps/Mbps/Gbps。
function fmtBps(v, withUnit = true) {
  if (v == null || isNaN(v)) return '-'
  const u = ['bps', 'Kbps', 'Mbps', 'Gbps', 'Tbps']
  let i = 0; let n = v
  while (n >= 1000 && i < u.length - 1) { n /= 1000; i++ }
  const num = n.toFixed(n >= 100 || i === 0 ? 0 : 1)
  return withUnit ? `${num} ${u[i]}` : num
}
// 按单位格式化：%(百分比) / bps(带宽) / /s(速率) / 空(计数)
function fmtByUnit(unit, v, withUnit = true) {
  if (v == null || isNaN(v)) return '-'
  if (unit === '%') return withUnit ? `${v.toFixed(1)}%` : v.toFixed(1)
  if (unit === 'bps') return fmtBps(v, withUnit)
  if (unit === '/s' || unit === 'pps') {
    const suffix = unit === 'pps' ? ' pps' : '/s'
    const n = Math.abs(v) >= 100 ? Math.round(v).toLocaleString() : v.toFixed(1)
    return withUnit ? `${n}${suffix}` : `${n}`
  }
  if (unit === 'bytes' || unit === 'Bps') {
    const u = unit === 'Bps' ? ['B/s', 'KB/s', 'MB/s', 'GB/s', 'TB/s'] : ['B', 'KB', 'MB', 'GB', 'TB']
    let i = 0; let n = v
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
    const num = n.toFixed(n >= 100 || i === 0 ? 0 : 1)
    return withUnit ? `${num} ${u[i]}` : num
  }
  // 计数（连接数等）
  if (Math.abs(v) >= 1000) return Math.round(v).toLocaleString()
  return Number.isInteger(v) ? String(v) : v.toFixed(1)
}

function seriesOfCard(card) {
  const keys = card.series_keys || []
  return (result.series || []).filter((s) => keys.includes(s.key))
}
function lastVal(s) { return s && s.points && s.points.length ? s.points[s.points.length - 1][1] : null }

function lineColor(card, idx, seriesKey, total) {
  if (seriesKey === 'net_in') return '#16a34a'
  if (seriesKey === 'net_out') return '#e05858'
  if (total <= 1) return CARD_COLOR[card.key] || PALETTE[0]
  return PALETTE[idx % PALETTE.length]
}

function cardCurText(card, ss) {
  if (card.key === 'disk' && ss.length) {
    const vals = ss.map(lastVal).filter((v) => v != null)
    const mx = vals.length ? Math.max(...vals) : null
    return mx == null ? '-' : `最高 ${mx.toFixed(1)}%${ss.length > 1 ? ` · ${ss.length}分区` : ''}`
  }
  if (ss.length >= 2) {
    const a = lastVal(ss[0]); const b = lastVal(ss[1])
    if (card.unit === 'bps') return `↓${fmtBps(a)}  ↑${fmtBps(b)}`
    return `${fmtByUnit(card.unit, a)} / ${fmtByUnit(card.unit, b)}`
  }
  return fmtByUnit(card.unit, lastVal(ss[0]))
}

function buildCards() {
  cards.value = (result.cards || []).map((card) => {
    const ss = seriesOfCard(card)
    return {
      ...card,
      color: CARD_COLOR[card.key] || PALETTE[0],
      curText: cardCurText(card, ss),
      hasData: ss.some((s) => s.points && s.points.length),
    }
  })
}

function isDark() { return document.documentElement.classList.contains('dark') }

function chartOption(card) {
  const dark = isDark()
  const axisColor = dark ? '#8a94a6' : '#9aa4b2'
  const splitColor = dark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.05)'
  const ss = seriesOfCard(card)
  const multi = ss.length > 1
  const series = ss.map((s, i) => {
    const color = lineColor(card, i, s.key, ss.length)
    const data = s.points.map((p) => [p[0] * 1000, p[1]])
    return {
      name: s.label || s.name || card.name,
      type: 'line', smooth: true, showSymbol: false,
      lineStyle: { width: 1.6, color }, itemStyle: { color },
      areaStyle: multi ? undefined : {
        color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
          { offset: 0, color: color + '33' }, { offset: 1, color: color + '05' },
        ]),
      },
      data,
    }
  })
  return {
    grid: { left: 8, right: 12, top: multi ? 28 : 14, bottom: 6, containLabel: true },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'line', lineStyle: { color: axisColor, width: 1 } },
      backgroundColor: dark ? '#1f2430' : '#fff',
      borderColor: dark ? '#333a48' : '#eee',
      textStyle: { color: dark ? '#dfe4ea' : '#333', fontSize: 12 },
      formatter: (ps) => {
        const t = echarts.format.formatTime('yyyy-MM-dd hh:mm', ps[0].value[0])
        const rows = ps.map((p) => `${p.marker}${p.seriesName}：<b>${fmtByUnit(card.unit, p.value[1])}</b>`).join('<br/>')
        return `${t}<br/>${rows}`
      },
    },
    legend: multi
      ? { show: true, type: 'scroll', right: 6, top: 0, itemWidth: 10, itemHeight: 8, itemGap: 10, textStyle: { color: axisColor, fontSize: 11 } }
      : undefined,
    xAxis: {
      type: 'time',
      axisLine: { lineStyle: { color: splitColor } },
      axisLabel: { color: axisColor, fontSize: 11, hideOverlap: true },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value', min: 0,
      max: card.unit === '%' ? 100 : undefined,
      axisLabel: {
        color: axisColor, fontSize: 11,
        formatter: (v) => fmtByUnit(card.unit, v, false),
      },
      splitLine: { lineStyle: { color: splitColor } },
    },
    series,
  }
}

function renderCharts() {
  for (const card of cards.value) {
    const el = chartEls[card.key]
    if (!card.hasData || !el) {
      if (charts[card.key]) { charts[card.key].dispose(); delete charts[card.key] }
      continue
    }
    let ch = charts[card.key]
    if (!ch || ch.getDom() !== el) {
      ch && ch.dispose()
      ch = echarts.init(el)
      charts[card.key] = ch
    }
    ch.setOption(chartOption(card), true)
  }
}

async function load() {
  loading.value = true
  try {
    const d = await getHostMetrics(props.instanceId, range.value)
    supported.value = !!d.supported
    result.source_name = d.source_name || ''
    result.target = d.target || ''
    result.message = d.message || ''
    result.detail_url = d.detail_url || ''
    result.cards = d.cards || []
    result.series = d.series || []
    buildCards()
    await nextTick()
    if (supported.value) renderCharts()
  } catch (e) {
    supported.value = false
    result.message = '监控数据加载失败'
    result.detail_url = ''
  } finally {
    loading.value = false
  }
}

function resize() { for (const k in charts) charts[k] && charts[k].resize() }

let themeObs = null
onMounted(() => {
  load()
  window.addEventListener('resize', resize)
  themeObs = new MutationObserver(() => { if (supported.value) renderCharts() })
  themeObs.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', resize)
  themeObs && themeObs.disconnect()
  for (const k in charts) charts[k] && charts[k].dispose()
})

watch(() => props.instanceId, () => load())
</script>

<style scoped>
.im { margin-top: 0; }
.im-head { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; row-gap: 8px; }
.im-head .panel-title { flex-shrink: 0; }
.im-src { font-size: 12.5px; color: var(--text-secondary); flex: 1 1 auto; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.im-src i { color: var(--text-muted); font-style: normal; }
.im-right { margin-left: auto; display: flex; align-items: center; gap: 8px; flex-shrink: 0; }
.im-right :deep(.el-radio-button__inner) { padding: 5px 10px; }
.im-empty { padding: 10px 0 4px; }
.im-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 14px;
  margin-top: 6px;
}
.im-card {
  border: 1px solid var(--card-border, #eef0f4);
  border-radius: 10px;
  padding: 10px 12px 6px;
  background: var(--fill-bg, #fafbfc);
}
/* 奇数张时最后一卡占满整行，保证两列对称 */
.im-span { grid-column: 1 / -1; }
.im-card-head { display: flex; align-items: baseline; justify-content: space-between; margin-bottom: 2px; }
.im-name { font-size: 13px; font-weight: 600; color: var(--text-regular); }
.im-cur { font-size: 14px; font-weight: 700; font-variant-numeric: tabular-nums; }
.im-chart { width: 100%; height: 150px; }
.im-nodata {
  height: 150px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  font-size: 12.5px;
}
@media (max-width: 760px) {
  .im-grid { grid-template-columns: 1fr; }
}
html.dark .im-card { background: rgba(255, 255, 255, 0.03); border-color: rgba(255, 255, 255, 0.08); }
</style>
