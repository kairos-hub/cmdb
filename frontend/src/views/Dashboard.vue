<template>
  <div class="home">
    <div class="hero">
      <div class="hero-row">
        <div class="searchbar">
          <el-icon class="sb-ic"><Search /></el-icon>
          <input
            ref="inputEl"
            v-model="kw"
            class="sb-in"
            placeholder="请输入IP、实例、固资编号进行搜索"
            @keyup.enter="goSearch"
          />
          <el-icon v-if="kw" class="sb-clear" @click="kw = ''"><Close /></el-icon>
          <button class="sb-go" title="搜索" @click="goSearch">
            <el-icon><Right /></el-icon>
          </button>
        </div>
        <AdvancedFilterDrawer size="large" class="adv-btn" />
      </div>

      <div v-if="recent.length" class="recent">
        <span class="rc-label">最近搜索</span>
        <span v-for="(r, i) in recent" :key="i" class="chip" @click="runRecent(r)">{{ r }}</span>
        <span class="rc-clear" @click="clearRecent">清空</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Search, Close, Right } from '@element-plus/icons-vue'
import AdvancedFilterDrawer from '@/components/AdvancedFilterDrawer.vue'

const router = useRouter()
const kw = ref('')
const inputEl = ref(null)

function goSearch() {
  const keyword = kw.value.trim()
  if (!keyword) return
  router.push({ path: '/search', query: { keyword, mode: 'fuzzy' } })
}

// 最近搜索（与全局搜索页共享 localStorage）
const RECENT_KEY = 'cmdb_recent_search'
const recent = ref([])
try {
  recent.value = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]').slice(0, 8)
} catch { recent.value = [] }
function runRecent(k) {
  kw.value = k
  goSearch()
}
function clearRecent() {
  recent.value = []
  try { localStorage.removeItem(RECENT_KEY) } catch { /* 忽略 */ }
}

onMounted(() => inputEl.value?.focus())
</script>

<style scoped>
.home {
  position: relative;
  display: flex;
  justify-content: center;
  align-items: flex-start;
  /* 全屏铺满内容区（抵消 MainLayout .content 的 24px/28px 内边距） */
  margin: -24px -28px;
  padding: 24vh 24px 40px;
  min-height: calc(100vh - 60px);
  box-sizing: border-box;
  overflow: hidden;
  background:
    radial-gradient(circle, rgba(47, 102, 240, 0.12) 1px, transparent 1.4px) 0 0 / 22px 22px,
    linear-gradient(rgba(47, 102, 240, 0.045) 1px, transparent 1px) 0 0 / 110px 110px,
    linear-gradient(90deg, rgba(47, 102, 240, 0.045) 1px, transparent 1px) 0 0 / 110px 110px,
    linear-gradient(160deg, #eef3fb 0%, #f4f7fc 45%, #eef2fb 100%);
}
/* 角落柔光 */
.home::before,
.home::after {
  content: '';
  position: absolute;
  border-radius: 50%;
  filter: blur(90px);
  opacity: 0.55;
  pointer-events: none;
}
.home::before {
  width: 440px;
  height: 440px;
  left: -120px;
  bottom: -140px;
  background: radial-gradient(circle, rgba(120, 200, 230, 0.5) 0%, transparent 70%);
}
.home::after {
  width: 460px;
  height: 460px;
  right: -120px;
  bottom: -160px;
  background: radial-gradient(circle, rgba(150, 150, 235, 0.42) 0%, transparent 70%);
}

.hero {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 1040px;
}
.hero-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
}

/* 胶囊搜索框 */
.searchbar {
  flex: 1;
  max-width: 880px;
  display: flex;
  align-items: center;
  gap: 12px;
  height: 64px;
  padding: 0 8px 0 24px;
  background: var(--card-bg);
  border: 1px solid rgba(47, 102, 240, 0.12);
  border-radius: 999px;
  box-shadow: 0 16px 40px rgba(31, 55, 110, 0.12);
  transition: box-shadow 0.15s, border-color 0.15s;
}
.searchbar:focus-within {
  border-color: rgba(47, 102, 240, 0.4);
  box-shadow: 0 18px 46px rgba(47, 102, 240, 0.2);
}
.sb-ic {
  font-size: 21px;
  color: var(--text-muted);
}
.sb-in {
  flex: 1;
  border: none;
  outline: none;
  background: transparent;
  font-size: 16px;
  color: var(--text-primary);
}
.sb-in::placeholder { color: var(--text-muted); }
.sb-clear {
  color: var(--text-muted);
  cursor: pointer;
  font-size: 18px;
}
.sb-clear:hover { color: var(--text-regular); }
.sb-go {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  flex-shrink: 0;
  border: none;
  border-radius: 50%;
  background: var(--brand);
  color: #fff;
  font-size: 20px;
  cursor: pointer;
  transition: background 0.15s, transform 0.1s;
  box-shadow: 0 6px 16px rgba(47, 102, 240, 0.4);
}
.sb-go:hover { background: var(--brand-dark); }
.sb-go:active { transform: scale(0.94); }

/* 高级筛选胶囊 */
.adv-btn { flex-shrink: 0; }
.adv-btn :deep(.el-button) {
  height: 64px;
  padding: 0 26px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid rgba(47, 102, 240, 0.12);
  box-shadow: 0 16px 40px rgba(31, 55, 110, 0.1);
  font-size: 15px;
  color: var(--text-regular);
}
.adv-btn :deep(.el-button:hover) {
  color: var(--brand);
  border-color: rgba(47, 102, 240, 0.4);
  background: var(--card-bg);
}

/* 最近搜索 */
.recent {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 26px;
}
.rc-label {
  font-size: 13px;
  color: var(--text-muted);
}
.chip {
  padding: 5px 14px;
  background: rgba(255, 255, 255, 0.8);
  border: 1px solid rgba(47, 102, 240, 0.1);
  border-radius: 999px;
  font-size: 13px;
  color: var(--text-regular);
  cursor: pointer;
  transition: border-color 0.15s, color 0.15s, background 0.15s;
}
.chip:hover {
  border-color: var(--brand);
  color: var(--brand);
  background: var(--card-bg);
}
.rc-clear {
  font-size: 12.5px;
  color: var(--brand);
  cursor: pointer;
  margin-left: 2px;
}
.rc-clear:hover { text-decoration: underline; }

/* —— 深色模式：整页渐变底与玻璃卡改深 —— */
html.dark .home {
  background:
    radial-gradient(circle, rgba(99, 140, 230, 0.14) 1px, transparent 1.4px) 0 0 / 22px 22px,
    linear-gradient(rgba(99, 140, 230, 0.05) 1px, transparent 1px) 0 0 / 110px 110px,
    linear-gradient(90deg, rgba(99, 140, 230, 0.05) 1px, transparent 1px) 0 0 / 110px 110px,
    linear-gradient(160deg, #0e131b 0%, #121a27 45%, #0e131b 100%);
}
html.dark .adv-btn :deep(.el-button) {
  background: rgba(255, 255, 255, 0.06);
  border-color: rgba(120, 150, 240, 0.22);
  color: var(--text-regular);
}
html.dark .chip {
  background: rgba(255, 255, 255, 0.05);
  border-color: rgba(120, 150, 240, 0.2);
}
</style>
