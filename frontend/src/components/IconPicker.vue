<template>
  <el-popover ref="popRef" :width="340" trigger="click" placement="bottom-start">
    <template #reference>
      <el-input
        :model-value="modelValue"
        readonly
        placeholder="点击选择图标"
        class="icon-input"
      >
        <template #prefix>
          <el-icon v-if="modelValue"><component :is="modelValue" /></el-icon>
        </template>
      </el-input>
    </template>

    <div class="picker">
      <el-input v-model="kw" placeholder="搜索图标名" clearable size="small" :prefix-icon="Search" />
      <div class="grid">
        <div
          v-for="name in filtered"
          :key="name"
          :class="['cell', name === modelValue && 'active']"
          :title="name"
          @click="select(name)"
        >
          <el-icon><component :is="name" /></el-icon>
        </div>
        <el-empty v-if="!filtered.length" :image-size="60" description="无匹配图标" />
      </div>
      <div class="footer">
        <span class="hint">共 {{ filtered.length }} 个</span>
        <el-button link size="small" @click="select('')">清除</el-button>
      </div>
    </div>
  </el-popover>
</template>

<script setup>
import { ref, computed } from 'vue'
import { Search } from '@element-plus/icons-vue'

defineProps({ modelValue: { type: String, default: '' } })
const emit = defineEmits(['update:modelValue'])

const popRef = ref()
const kw = ref('')

// 预置的常用图标（均为 Element Plus Icons 全局注册名）
const ICONS = [
  'Monitor', 'Cpu', 'Platform', 'Grid', 'Coin', 'OfficeBuilding', 'House', 'School',
  'Shop', 'Goods', 'Box', 'Management', 'Document', 'Folder', 'Files', 'Notebook',
  'Memo', 'Reading', 'Postcard', 'Tickets', 'Collection', 'List', 'Menu', 'Setting',
  'Operation', 'Tools', 'Histogram', 'PieChart', 'DataBoard', 'DataLine', 'DataAnalysis',
  'TrendCharts', 'Odometer', 'Connection', 'Link', 'Share', 'Location', 'Position',
  'MapLocation', 'Compass', 'Guide', 'Flag', 'Bell', 'Star', 'Medal', 'Trophy',
  'Avatar', 'User', 'UserFilled', 'Message', 'ChatDotRound', 'Service', 'Phone',
  'Cellphone', 'Printer', 'Watch', 'Timer', 'AlarmClock', 'Calendar', 'Clock',
  'Cloudy', 'Van', 'Ship', 'Suitcase', 'Key', 'Lock', 'Opportunity', 'Promotion',
]

const filtered = computed(() => {
  const k = kw.value.trim().toLowerCase()
  return k ? ICONS.filter((n) => n.toLowerCase().includes(k)) : ICONS
})

function select(name) {
  emit('update:modelValue', name)
  popRef.value?.hide()
}
</script>

<style scoped>
.icon-input {
  cursor: pointer;
}
.grid {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 8px;
  max-height: 240px;
  overflow-y: auto;
}
.cell {
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid #ebeef5;
  border-radius: 4px;
  cursor: pointer;
  font-size: 18px;
  color: #606266;
}
.cell:hover {
  border-color: var(--brand);
  color: var(--brand);
}
.cell.active {
  border-color: var(--brand);
  color: var(--brand);
  background: #ecf5ff;
}
.footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 8px;
}
.hint {
  color: #909399;
  font-size: 12px;
}
</style>
