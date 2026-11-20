<template>
  <div class="settings" v-loading="loading">
    <div class="page-header">
      <div>
        <div class="ph-title">系统设置</div>
        <div class="ph-sub">平台级运行与数据保留策略</div>
      </div>
      <el-tag v-if="!canManage" type="info" size="small">只读模式：无「用户与角色·编辑」权限</el-tag>
    </div>

    <div class="set-grid">
      <div class="panel set-card" v-for="m in cards" :key="m.key">
        <div class="sc-head">
          <div class="sc-ic" :class="m.tone"><el-icon><component :is="m.icon" /></el-icon></div>
          <div class="sc-meta">
            <div class="sc-title">{{ m.title }}</div>
            <div class="sc-desc">{{ m.desc }}</div>
          </div>
          <span class="soft-tag" :class="forms[m.key].enabled ? 'is-success' : 'is-muted'">
            <span class="dot"></span>{{ forms[m.key].enabled ? '已启用' : '已停用' }}
          </span>
        </div>

        <div class="sc-body">
          <div class="sc-row">
            <span class="sc-label">自动清理</span>
            <el-switch v-model="forms[m.key].enabled" :disabled="!canManage" />
          </div>
          <div class="sc-row">
            <span class="sc-label">{{ m.daysLabel }}</span>
            <div class="sc-ctl">
              <el-input-number v-model="forms[m.key].days" :min="1" :max="m.max"
                :disabled="!forms[m.key].enabled || !canManage" controls-position="right" />
              <span class="sc-unit">天</span>
            </div>
          </div>
          <div class="sc-row">
            <span class="sc-label">执行时间</span>
            <div class="sc-ctl">
              <el-input v-model="forms[m.key].cron" :disabled="!forms[m.key].enabled || !canManage"
                class="sc-cron" :placeholder="m.defaultCron" />
              <span class="sc-cronhint">5 段 cron</span>
            </div>
          </div>
        </div>

        <div class="sc-foot">
          <span class="sc-note">{{ m.hint }}</span>
          <el-button type="primary" size="small" :disabled="!canManage" :loading="saving === m.key" @click="saveCard(m)">保存</el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Delete, Document, Bell } from '@element-plus/icons-vue'
import * as api from '@/api/system'
import { useUserStore } from '@/store/user'

const userStore = useUserStore()
const canManage = computed(() => userStore.hasFunc('user_manage'))

// 三类数据保留策略：清理项元数据统一描述，表单值统一管理
const cards = [
  { key: 'offline', prefix: 'offline_cleanup', title: '下线资源清理', desc: '失联资源标记下线后自动删除',
    icon: Delete, tone: 'ic-warn', daysLabel: '下线超过', max: 365, defaultDays: 7, defaultCron: '0 3 * * *',
    defaultOn: false, hint: '删除「已下线」持续超过 N 天的实例（含已分配业务）。' },
  { key: 'audit', prefix: 'audit_cleanup', title: '审计日志清理', desc: '定时清理历史操作审计日志',
    icon: Document, tone: 'ic-info', daysLabel: '保留', max: 3650, defaultDays: 90, defaultCron: '0 4 * * *',
    defaultOn: false, hint: '删除创建时间早于 N 天的审计日志，保持列表轻量。' },
  { key: 'alert', prefix: 'alert_cleanup', title: '告警事件清理', desc: '定时清理历史告警记录',
    icon: Bell, tone: 'ic-danger', daysLabel: '保留', max: 3650, defaultDays: 90, defaultCron: '0 3 * * *',
    defaultOn: true, hint: '删除触发时间早于 N 天的告警（含已恢复），默认保留 90 天。' },
]

const loading = ref(false)
const saving = ref('')
const forms = reactive(Object.fromEntries(cards.map((m) => [m.key, { enabled: m.defaultOn, days: m.defaultDays, cron: m.defaultCron }])))

async function load() {
  loading.value = true
  try {
    const s = await api.getSettings()
    for (const m of cards) {
      const ev = s[`${m.prefix}_enabled`]
      forms[m.key].enabled = m.defaultOn ? ev !== '0' : ev === '1'
      forms[m.key].days = Number(s[`${m.prefix}_days`] || m.defaultDays)
      forms[m.key].cron = s[`${m.prefix}_cron`] || m.defaultCron
    }
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function saveCard(m) {
  saving.value = m.key
  try {
    await api.updateSettings({
      [`${m.prefix}_enabled`]: forms[m.key].enabled ? '1' : '0',
      [`${m.prefix}_days`]: String(forms[m.key].days),
      [`${m.prefix}_cron`]: (forms[m.key].cron || '').trim() || m.defaultCron,
    })
    ElMessage.success('已保存')
  } finally {
    saving.value = ''
  }
}
</script>

<style scoped>
.set-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 16px;
}
.set-card {
  display: flex;
  flex-direction: column;
  padding: 18px 20px 16px;
}
.sc-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding-bottom: 14px;
  margin-bottom: 14px;
  border-bottom: 1px solid var(--card-border, #eef0f4);
}
.sc-ic {
  width: 40px;
  height: 40px;
  border-radius: 11px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex-shrink: 0;
}
.ic-warn { background: #fff3e6; color: #e6882e; }
.ic-info { background: #eaf1ff; color: #3b82f6; }
.ic-danger { background: #fdecec; color: #e05858; }
.sc-meta { flex: 1; min-width: 0; }
.sc-title { font-weight: 600; font-size: 15px; color: var(--text-primary); }
.sc-desc { font-size: 12.5px; color: var(--text-muted, #98a2b3); margin-top: 2px; }
.sc-body { display: flex; flex-direction: column; gap: 12px; flex: 1; }
.sc-row { display: flex; align-items: center; min-height: 32px; }
.sc-label { width: 84px; flex-shrink: 0; color: var(--text-secondary); font-size: 13px; }
.sc-ctl { display: flex; align-items: center; gap: 8px; }
.sc-unit { color: var(--text-secondary); font-size: 13px; }
.sc-cron { width: 180px; }
.sc-cronhint { color: var(--text-muted, #98a2b3); font-size: 12px; }
.sc-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid var(--card-border, #eef0f4);
}
.sc-note { font-size: 12px; color: var(--text-muted, #98a2b3); line-height: 1.5; }
@media (prefers-color-scheme: dark) {
  .ic-warn { background: rgba(230, 136, 46, 0.16); }
  .ic-info { background: rgba(59, 130, 246, 0.16); }
  .ic-danger { background: rgba(224, 88, 88, 0.16); }
}
</style>
