<template>
  <div class="intg">
    <div class="page-header intg-header">
      <div class="ph-left">
        <div class="ph-title">系统集成</div>
        <div class="ph-sub">统一入口，在本系统内集成外部平台 · 支持内嵌或新标签页</div>
      </div>
      <div class="ph-actions">
        <el-button v-if="canManage" type="primary" :icon="Plus" @click="openCreate">新建集成</el-button>
      </div>
    </div>

    <div v-if="visibleList.length" class="card-grid">
      <div
        v-for="it in visibleList"
        :key="it.id"
        class="intg-card"
        :class="{ disabled: !it.enabled }"
        @click="open(it)"
      >
        <div class="ic-badge" :class="{ 'is-img': isImg(it.icon) }" :style="isImg(it.icon) ? {} : { background: colorOf(it), color: '#fff' }">
          <img v-if="isImg(it.icon)" :src="it.icon" alt="" @error="onImgError" />
          <span v-else>{{ badge(it) }}</span>
        </div>
        <div class="ic-body">
          <div class="ic-name">
            {{ it.name }}
            <span class="ic-mode">{{ it.open_mode === 'blank' ? '新标签' : '内嵌' }}</span>
            <span v-if="!it.enabled" class="ic-off">停用</span>
          </div>
          <div class="ic-url">{{ it.url }}</div>
        </div>
        <div v-if="canManage" class="ic-ops" @click.stop>
          <el-button link :icon="Edit" @click="openEdit(it)" />
          <el-button link type="danger" :icon="Delete" @click="remove(it)" />
        </div>
      </div>
    </div>
    <el-empty v-else description="暂无集成，点右上角「新建集成」接入 Grafana / 夜莺等" />

    <el-dialog v-model="dialog" :title="editing ? '编辑集成' : '新建集成'" width="480px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="如 Grafana / 夜莺" /></el-form-item>
        <el-form-item label="地址"><el-input v-model="form.url" placeholder="https://grafana.example.com" /></el-form-item>
        <el-form-item label="图标">
          <el-input v-model="form.icon" placeholder="1 个字/emoji，或图片 URL（http/https），留空用名称首字" />
          <span class="hint">卡片圆标里显示：填图片地址则显示 Logo，否则显示字符</span>
        </el-form-item>
        <el-form-item label="打开方式">
          <el-radio-group v-model="form.open_mode">
            <el-radio value="embed">内嵌(iframe)</el-radio>
            <el-radio value="blank">新标签页</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="说明"><el-input v-model="form.description" placeholder="选填" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" @click="submit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Edit, Delete } from '@element-plus/icons-vue'
import * as api from '@/api/system'
import { useUserStore } from '@/store/user'

const router = useRouter()
const userStore = useUserStore()
const canManage = computed(() => userStore.hasFunc('datasource_manage'))

const list = ref([])
// 管理员看全部（含停用，便于管理）；普通用户只看启用的
const visibleList = computed(() => (canManage.value ? list.value : list.value.filter((i) => i.enabled)))

const PALETTE = ['#2f66f0', '#16a34a', '#d97706', '#7c3aed', '#db2777', '#0891b2', '#e34948']
const colorOf = (it) => PALETTE[(it.id || 0) % PALETTE.length]
const badge = (it) => (it.icon ? it.icon : (it.name || '?').trim().charAt(0))
// 图标是否为图片地址（http/https 或以 / 开头的路径）
const isImg = (v) => /^(https?:\/\/|\/)/i.test((v || '').trim())
function onImgError(e) {
  // 图片加载失败时隐藏，回退到彩色圆标（badge 文本仍在 DOM 外，简单隐藏即可）
  e.target.style.display = 'none'
}

async function load() {
  try {
    list.value = await api.listIntegrations()
  } catch {
    list.value = []
  }
}
onMounted(load)

function open(it) {
  if (!it.enabled) {
    if (canManage.value) ElMessage.info('该集成已停用')
    return
  }
  if (it.open_mode === 'blank') {
    window.open(it.url, '_blank', 'noopener')
  } else {
    router.push(`/integrations/${it.id}`)
  }
}

const dialog = ref(false)
const editing = ref(false)
const form = ref({})
function openCreate() {
  editing.value = false
  form.value = { name: '', url: '', icon: '', open_mode: 'embed', description: '', enabled: true }
  dialog.value = true
}
function openEdit(it) {
  editing.value = true
  form.value = { ...it }
  dialog.value = true
}
async function submit() {
  if (!form.value.name) return ElMessage.warning('名称必填')
  if (!form.value.url) return ElMessage.warning('地址必填')
  if (editing.value) await api.updateIntegration(form.value.id, form.value)
  else await api.createIntegration(form.value)
  ElMessage.success('保存成功')
  dialog.value = false
  load()
}
async function remove(it) {
  await ElMessageBox.confirm(`确定删除集成「${it.name}」？`, '提示', { type: 'warning' })
  await api.deleteIntegration(it.id)
  ElMessage.success('已删除')
  load()
}
</script>

<style scoped>
.intg-header .ph-left { min-width: 0; }
.intg-header .ph-title {
  font-size: 18px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: 0.5px;
}
.intg-header .ph-sub {
  margin-top: 4px;
  font-size: 13px;
  color: var(--text-secondary);
  letter-spacing: 0.2px;
}
.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 14px;
}
.intg-card {
  display: flex;
  align-items: center;
  gap: 14px;
  background: var(--card-bg);
  border: 1px solid var(--card-border, #eef0f4);
  border-radius: var(--radius-lg, 12px);
  box-shadow: var(--shadow-card, 0 1px 3px rgba(0, 0, 0, 0.06));
  padding: 16px 18px;
  cursor: pointer;
  transition: box-shadow 0.15s, border-color 0.15s;
}
.intg-card:hover { box-shadow: 0 4px 14px rgba(36, 104, 242, 0.14); border-color: var(--brand, #2468f2); }
.intg-card.disabled { opacity: 0.55; }
.ic-badge {
  width: 46px; height: 46px; border-radius: 12px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  font-size: 20px; font-weight: 700; overflow: hidden;
}
.ic-badge.is-img { background: var(--fill-bg); }
.ic-badge img { width: 100%; height: 100%; object-fit: contain; }
.ic-body { flex: 1; min-width: 0; }
.ic-name { font-weight: 600; color: var(--text-primary); display: flex; align-items: center; gap: 8px; }
.ic-mode { font-size: 11px; font-weight: 500; color: var(--text-secondary); background: var(--fill-bg, #f3f5f9); padding: 1px 7px; border-radius: 999px; }
.ic-off { font-size: 11px; color: #e05858; }
.ic-desc { font-size: 12.5px; color: var(--text-secondary); margin-top: 3px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ic-url { font-size: 12px; color: var(--text-muted); margin-top: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ic-ops { flex-shrink: 0; }
.hint { color: var(--text-secondary); font-size: 12px; margin-left: 10px; }
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) .intg-card { background: var(--card-bg, #1f2430); }
}
</style>
