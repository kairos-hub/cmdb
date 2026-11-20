<template>
  <div class="search-page">
    <div class="page-header">
      <div>
        <div class="ph-title">全局搜索</div>
        <div class="ph-sub">跨全部资源模型检索 IP、实例名、字段值，支持精确与模糊匹配</div>
      </div>
    </div>

    <!-- 顶部搜索条 -->
    <div class="panel search-header">
      <div class="search-box">
        <el-icon class="sb-icon"><Search /></el-icon>
        <input
          v-model="kw"
          class="sb-input"
          placeholder="请输入 IP、实例名、固资编号、字段值进行搜索"
          @keyup.enter="submit"
        />
        <el-icon v-if="kw" class="sb-clear" @click="kw = ''"><Close /></el-icon>
      </div>
      <el-radio-group v-model="mode" class="mode-switch" @change="onModeChange">
        <el-radio-button value="exact">精确匹配</el-radio-button>
        <el-radio-button value="fuzzy">模糊匹配</el-radio-button>
      </el-radio-group>
      <el-button type="primary" size="large" @click="submit">搜索</el-button>
      <AdvancedFilterDrawer size="large" />
    </div>

    <!-- 未搜索提示 -->
    <el-empty v-if="!searched" description="输入关键词，检索全部资源实例" />

    <!-- 结果区 -->
    <div v-else class="search-body" v-loading="loading">
      <!-- 左侧分类 -->
      <div class="panel side">
        <div class="side-title">模型</div>
        <div :class="['cat', activeModel === 0 && 'active']" @click="selectModel(0)">
          <span class="cat-name">全部</span>
          <span class="cat-count">{{ totalHits }}</span>
        </div>
        <div
          v-for="c in route.query.adv ? [] : categories"
          :key="c.model_id"
          :class="['cat', activeModel === c.model_id && 'active']"
          @click="selectModel(c.model_id)"
        >
          <span class="cat-name">
            <el-icon><component :is="c.icon || 'Document'" /></el-icon>
            {{ c.name }}
          </span>
          <span class="cat-count">{{ c.count }}</span>
        </div>
      </div>

      <!-- 中间结果 -->
      <div class="main">
        <div class="result-summary">
          共 <b>{{ totalHits }}</b> 条结果<span v-if="activeModel"> · 当前分类 {{ resultTotal }} 条</span>
        </div>
        <el-empty v-if="!loading && !hits.length" description="无匹配资源" />
        <div
          v-for="h in hits"
          :key="h.instance_id"
          class="result-item"
          @click="goInstance(h)"
        >
          <span class="ri-icon"><ModelIcon :obj-id="h.model_obj_id" :fallback="h.icon || 'Document'" :size="20" /></span>
          <div class="ri-body">
            <div class="ri-title">
              <span class="ri-name">
                <span v-for="(s, i) in segments(h.display)" :key="i" :class="{ hl: s.hit }">{{ s.t }}</span>
              </span>
              <span class="soft-tag is-muted">{{ h.model_name }}</span>
              <span class="soft-tag is-info">{{ mode === 'exact' ? '精确匹配' : '模糊匹配' }}</span>
            </div>
            <div class="ri-id">#{{ h.instance_id }}</div>
          </div>
          <el-icon class="ri-arrow"><ArrowRight /></el-icon>
        </div>
        <div v-if="resultTotal > 0" class="result-pagination">
          <el-pagination
            background
            :current-page="page"
            :page-size="pageSize"
            :page-sizes="[20, 50, 100]"
            :total="resultTotal"
            :pager-count="5"
            :disabled="loading"
            layout="sizes, prev, pager, next"
            @current-change="changePage"
            @size-change="changePageSize"
          />
        </div>
      </div>

      <!-- 右侧：搜索语法 + 最近搜索 -->
      <div class="rail">
        <div class="panel panel-pad">
          <div class="rail-title">搜索语法</div>
          <div class="syntax-item">
            <code>prod-app</code>
            <p>关键字在实例名 / IP / 字段值等跨字段模糊检索</p>
          </div>
          <div class="syntax-item">
            <code>10.20.8.15</code>
            <p>完整 IP / 实例名等，按包含匹配更易命中</p>
          </div>
          <div class="syntax-item">
            <p class="syntax-note">开启上方「精确匹配」后，关键词需与某字段的<b>完整值</b>一致才命中。</p>
          </div>
        </div>
        <div class="panel panel-pad" v-if="recent.length">
          <div class="rail-head">
            <span class="rail-title">最近搜索</span>
            <span class="rail-clear" @click="clearRecent">清空</span>
          </div>
          <div
            v-for="(r, i) in recent"
            :key="i"
            class="recent-item"
            @click="applyRecent(r)"
          >{{ r }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Search, ArrowRight, Close } from '@element-plus/icons-vue'
import { globalSearch, searchInstances } from '@/api/instance'
import * as mapi from '@/api/model'
import AdvancedFilterDrawer from '@/components/AdvancedFilterDrawer.vue'
import ModelIcon from '@/components/ModelIcon.vue'

const route = useRoute()
const router = useRouter()
const kw = ref('')
const mode = ref('fuzzy')
const hits = ref([])
const searched = ref(false)
const loading = ref(false)
const activeModel = ref(0)
const categories = ref([])
const resultTotal = ref(0)
const totalHits = computed(() => categories.value.reduce((sum, category) => sum + category.count, 0))
const page = ref(1)
const pageSize = ref(20)
let requestID = 0
const locate = ref('') // DNS解析命中后用于在详情页定位具体记录的值

// 最近搜索（仅存本机 localStorage，按浏览器保存，容错）
const RECENT_KEY = 'cmdb_recent_search'
const recent = ref([])
try {
  recent.value = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')
} catch { recent.value = [] }
function pushRecent(k) {
  if (!k) return
  const list = [k, ...recent.value.filter((x) => x !== k)].slice(0, 8)
  recent.value = list
  try { localStorage.setItem(RECENT_KEY, JSON.stringify(list)) } catch { /* 忽略 */ }
}
function clearRecent() {
  recent.value = []
  try { localStorage.removeItem(RECENT_KEY) } catch { /* 忽略 */ }
}
function applyRecent(k) {
  kw.value = k
  submit()
}
function onModeChange() {
  if (kw.value.trim()) submit()
}

function selectModel(id) {
  if (loading.value || id === activeModel.value) return
  router.replace({ query: { ...route.query, model_id: id || undefined, page: undefined } })
}
function changePage(newPage) {
  if (loading.value || newPage === page.value) return
  router.replace({ query: { ...route.query, page: newPage === 1 ? undefined : newPage } })
}
function changePageSize(size) {
  if (loading.value || size === pageSize.value) return
  router.replace({ query: { ...route.query, page: undefined, page_size: size } })
}

function segments(text) {
  const keyword = (route.query.keyword || '').toString().trim()
  const src = text || ''
  if (!keyword) return [{ t: src, hit: false }]
  const parts = []
  const lower = src.toLowerCase()
  const klower = keyword.toLowerCase()
  let i = 0
  while (true) {
    const idx = lower.indexOf(klower, i)
    if (idx < 0) {
      if (i < src.length) parts.push({ t: src.slice(i), hit: false })
      break
    }
    if (idx > i) parts.push({ t: src.slice(i, idx), hit: false })
    parts.push({ t: src.slice(idx, idx + keyword.length), hit: true })
    i = idx + keyword.length
  }
  return parts
}

function submit() {
  const keyword = kw.value.trim()
  if (!keyword) {
    router.replace({ path: '/search' })
    return
  }
  router.replace({ path: '/search', query: { keyword, mode: mode.value, page_size: pageSize.value } })
}

async function loadResults(id, load) {
  loading.value = true
  searched.value = true
  try {
    const result = await load()
    if (id !== requestID) return
    hits.value = result.list
    categories.value = result.categories
    resultTotal.value = result.total
    page.value = result.page
  } catch {
    if (id !== requestID) return
    hits.value = []
    categories.value = []
    resultTotal.value = 0
  } finally {
    if (id === requestID) loading.value = false
  }
}

function runSearch(id, keyword, searchMode, selectedPage, size, modelId) {
  return loadResults(id, async () => {
    const result = await globalSearch({
      keyword, mode: searchMode, page: selectedPage, page_size: size, model_id: modelId || undefined,
    })
    if (id === requestID) pushRecent(keyword)
    return result
  })
}

function goInstance(h) {
  // DNS解析命中：带定位值进详情，自动过滤+高亮命中的解析记录
  if (h.model_obj_id === 'dns_record' && locate.value) {
    router.push({ path: `/instances/${h.instance_id}`, query: { q: locate.value } })
    return
  }
  router.push(`/instances/${h.instance_id}`)
}

// 高级筛选结果：按模型+条件查询，映射为与全局搜索一致的命中卡片
function optName(a, v) {
  const o = (a.options || []).find((x) => x.id === v)
  return o ? o.name : v
}
function dispVal(a, v) {
  if (v === undefined || v === null || v === '') return ''
  if (a.type === 'bool') return v ? '是' : '否'
  if (a.type === 'enum') return optName(a, v)
  if (a.type === 'enummulti') return Array.isArray(v) ? v.map((x) => optName(a, x)).join('，') : ''
  return String(v)
}
function summarize(data, attrs) {
  const parts = []
  for (const a of attrs) {
    const v = data?.[a.property_id]
    if (v === undefined || v === null || v === '') continue
    parts.push(a.name + '=' + dispVal(a, v))
    if (parts.length >= 5) break
  }
  return parts.join(' / ')
}
function runAdvanced(id, modelId, conds, selectedPage, size) {
  return loadResults(id, async () => {
    const search = (page) => searchInstances({ model_id: modelId, conditions: conds, page, page_size: size })
    const [models, attrs, firstResult] = await Promise.all([
      mapi.listModels(), mapi.listAttributes(modelId), search(selectedPage),
    ])
    const m = models.find((x) => x.id === modelId)
    const cols = (attrs || [])
      .filter((a) => a.enabled !== false)
      .sort((x, y) => (x.index || 0) - (y.index || 0))
    const lastPage = Math.max(1, Math.ceil(firstResult.total / size))
    const res = selectedPage > lastPage ? await search(lastPage) : firstResult
    return {
      total: res.total,
      page: res.page,
      categories: res.total ? [{ model_id: modelId, name: m?.name, icon: m?.icon, count: res.total }] : [],
      list: (res.list || []).map((i) => ({
        instance_id: i.id,
        model_id: modelId,
        model_name: m?.name || '',
        model_obj_id: m?.obj_id,
        icon: m?.icon,
        display: summarize(i.data, cols),
      })),
    }
  })
}

watch(
  () => route.query,
  (q) => {
    const id = ++requestID
    const requestedPage = Number(q.page)
    const requestedSize = Number(q.page_size)
    const currentPage = Number.isSafeInteger(requestedPage) && requestedPage > 0 ? requestedPage : 1
    const size = [20, 50, 100].includes(requestedSize) ? requestedSize : 20
    pageSize.value = size
    kw.value = (q.keyword || '').toString()
    mode.value = (q.mode || 'fuzzy').toString()
    // 高级筛选模式：带 model_id + adv(conditions)
    const mid = Number(q.model_id)
    if (mid && q.adv) {
      try {
        const conds = JSON.parse(q.adv)
        locate.value = (q.locate || '').toString()
        activeModel.value = 0
        runAdvanced(id, mid, Array.isArray(conds) ? conds : [], currentPage, size)
        return
      } catch (e) { /* 忽略非法参数 */ }
    }
    const keyword = kw.value.trim()
    locate.value = keyword
    activeModel.value = mid || 0
    if (keyword) runSearch(id, keyword, mode.value, currentPage, size, activeModel.value)
    else {
      hits.value = []
      categories.value = []
      resultTotal.value = 0
      searched.value = false
      loading.value = false
    }
  },
  { immediate: true },
)
</script>

<style scoped>
/* 搜索条 */
.search-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 18px;
  margin-bottom: 20px;
}
.search-box {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 10px;
  height: 46px;
  padding: 0 16px;
  border: 1px solid var(--border-base);
  border-radius: var(--radius);
  background: var(--fill-bg-soft);
  transition: border-color 0.15s, background 0.15s;
}
.search-box:focus-within {
  border-color: var(--brand);
  background: var(--card-bg);
}
.sb-icon { color: var(--text-muted); font-size: 18px; }
.sb-input {
  flex: 1;
  border: none;
  outline: none;
  background: transparent;
  font-size: 15px;
  color: var(--text-primary);
}
.sb-input::placeholder { color: var(--text-muted); }
.sb-clear { color: var(--text-muted); cursor: pointer; }
.sb-clear:hover { color: var(--text-regular); }
.mode-switch { flex-shrink: 0; }

.search-body {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}
/* 左侧模型 */
.side {
  width: 220px;
  flex-shrink: 0;
  padding: 12px 10px;
}
.side-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-primary);
  padding: 2px 8px 10px;
}
.cat {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 9px 12px;
  border-radius: 8px;
  cursor: pointer;
  margin-bottom: 2px;
  font-size: 14px;
  color: var(--text-regular);
}
.cat:hover { background: var(--fill-bg); }
.cat.active {
  background: var(--brand-light);
  color: var(--brand);
  font-weight: 600;
}
.cat-name {
  display: flex;
  align-items: center;
  gap: 8px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cat-count {
  font-size: 13px;
  color: var(--text-muted);
  font-weight: 600;
}
.cat.active .cat-count { color: var(--brand); }

/* 中间结果 */
.main {
  flex: 1;
  min-width: 0;
}
.result-summary {
  color: var(--text-secondary);
  font-size: 13px;
  margin-bottom: 12px;
}
.result-summary b { color: var(--text-primary); }
.result-item {
  display: flex;
  align-items: center;
  gap: 14px;
  background: var(--card-bg);
  border: 1px solid var(--card-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-card);
  padding: 16px 18px;
  margin-bottom: 12px;
  cursor: pointer;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.result-item:hover {
  border-color: var(--brand);
  box-shadow: var(--shadow-card-hover);
}
.ri-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: 10px;
  background: var(--brand-light);
  color: var(--brand);
  font-size: 20px;
  flex-shrink: 0;
}
.ri-body {
  flex: 1;
  min-width: 0;
}
.ri-title {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.ri-name {
  font-size: 15px;
  font-weight: 700;
  color: var(--brand);
}
.ri-id {
  margin-top: 4px;
  color: var(--text-muted);
  font-size: 12px;
}
.hl {
  background: #fff3c4;
  color: #b45309;
  border-radius: 3px;
  padding: 0 2px;
  font-weight: 700;
}
.ri-arrow {
  color: var(--text-muted);
  flex-shrink: 0;
}

/* 右侧栏 */
.rail {
  width: 260px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.rail-title {
  font-size: 15px;
  font-weight: 700;
  color: var(--text-primary);
}
.rail-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}
.rail-clear {
  font-size: 13px;
  color: var(--brand);
  cursor: pointer;
}
.rail-clear:hover { text-decoration: underline; }
.syntax-item {
  margin-top: 12px;
}
.syntax-item code {
  display: inline-block;
  font-family: 'SFMono-Regular', Menlo, Consolas, monospace;
  font-size: 12.5px;
  color: var(--brand);
  background: var(--fill-bg);
  border-radius: 5px;
  padding: 2px 8px;
}
.syntax-item p {
  margin: 5px 0 0;
  font-size: 12.5px;
  color: var(--text-secondary);
}
.syntax-note {
  line-height: 1.6;
}
.syntax-note b { color: var(--text-regular); }
.recent-item {
  padding: 8px 10px;
  border-radius: 7px;
  font-size: 13.5px;
  color: var(--text-regular);
  cursor: pointer;
}
.recent-item:hover {
  background: var(--fill-bg);
  color: var(--brand);
}

@media (max-width: 1200px) {
  .rail { display: none; }
}
.result-pagination {
  display: flex;
  justify-content: flex-end;
  overflow-x: auto;
  margin-top: 16px;
  padding-bottom: 4px;
}
</style>
