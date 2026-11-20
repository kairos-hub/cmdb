<template>
  <div class="frame-wrap">
    <div class="frame-bar">
      <el-button :icon="Back" size="small" @click="router.push('/integrations')">返回集成</el-button>
      <span class="fb-name">{{ cur.name || '外部系统' }}</span>
      <span class="fb-spacer"></span>
      <span class="fb-hint">若下方空白，可能对方禁止被嵌入</span>
      <el-button :icon="TopRight" size="small" @click="openBlank">新标签打开</el-button>
    </div>
    <iframe v-if="cur.url" :src="cur.url" class="frame-body" referrerpolicy="no-referrer"></iframe>
    <el-empty v-else description="集成不存在或已停用" />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Back, TopRight } from '@element-plus/icons-vue'
import * as api from '@/api/system'

const route = useRoute()
const router = useRouter()
const cur = ref({})

onMounted(async () => {
  try {
    const id = Number(route.params.id)
    const list = await api.listIntegrations()
    cur.value = list.find((i) => i.id === id && i.enabled) || {}
  } catch {
    cur.value = {}
  }
})

function openBlank() {
  if (cur.value.url) window.open(cur.value.url, '_blank', 'noopener')
}
</script>

<style scoped>
.frame-wrap {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 92px); /* 填满内容区（减去顶栏/内边距） */
  min-height: 420px;
}
.frame-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding-bottom: 10px;
}
.fb-name { font-weight: 600; color: var(--text-primary); }
.fb-spacer { flex: 1; }
.fb-hint { font-size: 12px; color: var(--text-muted, #98a2b3); }
.frame-body {
  flex: 1;
  width: 100%;
  border: 1px solid var(--card-border, #eef0f4);
  border-radius: var(--radius-lg, 12px);
  background: var(--card-bg);
}
</style>
