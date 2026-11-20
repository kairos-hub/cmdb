<template>
  <div>
    <div class="page-header">
      <div>
        <div class="ph-title">告警配置</div>
        <div class="ph-sub">统一管理告警接入（阿里云/腾讯云拉取、第三方平台回调）与告警清理策略，与资源同步的数据源解耦</div>
      </div>
      <div class="ph-actions">
        <el-tag v-if="!canManage" type="info" size="small">只读模式：无「数据源·编辑」权限</el-tag>
        <el-button v-if="canManage" type="primary" :icon="Plus" @click="openCreate">新建告警源</el-button>
      </div>
    </div>

    <div class="panel panel-pad list-card">
      <div class="panel-head">
        <span class="panel-title">告警源</span>
        <span class="list-hint">阿里云/腾讯云按 cron 主动拉取报警历史；回调源由平台 POST 推送，无需拉取</span>
      </div>
      <el-table :data="list" v-loading="loading">
        <el-table-column label="名称" width="170">
          <template #default="{ row }"><span class="as-name">{{ row.name }}</span></template>
        </el-table-column>
        <el-table-column label="类型" width="120">
          <template #default="{ row }"><span class="soft-tag" :class="typeClass(row.type)">{{ typeLabel(row.type) }}</span></template>
        </el-table-column>
        <el-table-column label="接入信息" min-width="230" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="isCloud(row.type)">{{ row.region || '默认地域' }}</span>
            <span v-else-if="row.type === 'prometheus'">{{ row.url || '—' }}</span>
            <span v-else class="muted">回调推送（{{ webhookOf(row) ? 'URL 已生成' : '—' }}）</span>
          </template>
        </el-table-column>
        <el-table-column label="告警拉取" min-width="150">
          <template #default="{ row }">
            <template v-if="isPull(row.type)">
              <span v-if="row.alert_cron">{{ row.alert_cron }}<span class="muted sm">窗口 {{ row.alert_window_min || 20 }} 分钟</span></span>
              <span v-else class="muted">未配置（仅手动）</span>
            </template>
            <span v-else class="muted">由平台推送</span>
          </template>
        </el-table-column>
        <el-table-column label="上次告警" min-width="200">
          <template #default="{ row }">
            <div v-if="row.last_alert_at">
              <div>{{ formatTime(row.last_alert_at) }}</div>
              <div class="muted sm">{{ row.last_alert_msg }}</div>
            </div>
            <span v-else class="muted">—</span>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="90">
          <template #default="{ row }">
            <span class="soft-tag" :class="row.enabled ? 'is-success' : 'is-muted'"><span class="dot"></span>{{ row.enabled ? '启用' : '停用' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <template v-if="canManage">
              <el-button v-if="isPull(row.type)" link type="warning" :loading="syncingId === row.id" :disabled="!row.enabled" @click="doSync(row)">同步告警</el-button>
              <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            </template>
            <span v-else class="muted">—</span>
          </template>
        </el-table-column>
        <el-table-column v-if="canManage" label="排序" width="80" fixed="right">
          <template #default="{ $index }">
            <el-button link :icon="Top" :disabled="$index === 0" title="上移" @click="move($index, -1)" />
            <el-button link :icon="Bottom" :disabled="$index === list.length - 1" title="下移" @click="move($index, 1)" />
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!loading && !list.length" description="暂无告警源，点右上角「新建告警源」接入阿里云/腾讯云或第三方回调" />
    </div>

    <el-dialog v-model="dialog" :title="editing ? '编辑告警源' : '新建告警源'" width="520px">
      <el-form :model="form" label-width="120px">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="如 阿里云告警 / 夜莺" /></el-form-item>
        <el-form-item label="类型">
          <el-select v-model="form.type" style="width: 100%" :disabled="editing">
            <el-option label="阿里云" value="aliyun" />
            <el-option label="阿里云 ARMS(容器ACK告警)" value="arms" />
            <el-option label="腾讯云" value="tencent" />
            <el-option label="Prometheus(自建集群/应用告警)" value="prometheus" />
            <el-option label="告警回调(Webhook)" value="webhook" />
          </el-select>
        </el-form-item>

        <!-- 自建 Prometheus 告警源：拉 /api/v1/alerts -->
        <template v-if="form.type === 'prometheus'">
          <el-form-item label=" ">
            <span class="list-hint">主动拉取自建 Prometheus 的 firing 告警，按标签自动关联：namespace+pod→应用服务(k8s_app)、节点 IP→主机、集群标识→集群。需 Prometheus 已加载告警规则(kube-prometheus-stack 等)。</span>
          </el-form-item>
          <el-form-item label="Prometheus 地址">
            <el-input v-model="form.url" placeholder="http://<prometheus>:9090（同集群资源数据源的地址）" />
          </el-form-item>
          <el-form-item label="Basic Auth 账号">
            <el-input v-model="form.username" :placeholder="editing ? '留空=不修改（无鉴权可空）' : '可空'" autocomplete="off" />
          </el-form-item>
          <el-form-item label="Basic Auth 密码">
            <el-input v-model="form.password" :placeholder="editing ? '留空=不修改' : '可空'" show-password autocomplete="new-password" />
          </el-form-item>
          <el-form-item label="告警拉取">
            <el-input v-model="form.alert_cron" placeholder="cron（5 段），留空=仅手动同步" />
            <span class="list-hint">例：*/2 * * * * 每2分钟拉一次；本次不在活跃列表的旧告警自动置为已恢复。</span>
          </el-form-item>
        </template>

        <!-- 云告警源（阿里云/ARMS/腾讯云）-->
        <template v-if="isCloud(form.type)">
          <el-form-item v-if="form.type === 'arms'" label=" ">
            <span class="list-hint">拉取 ARMS 告警管理中的 ACK 集群告警（ETCD/Pod异常/副本拉起异常等），仅保留与 CMDB 已同步集群匹配的告警并自动关联到集群实例；需子账号具备 ARMS 只读权限。</span>
          </el-form-item>
          <el-form-item label="地域">
            <el-input v-model="form.region" :placeholder="form.type === 'tencent' ? '默认 ap-guangzhou；多地域逗号分隔' : '默认 cn-shanghai；多地域逗号分隔'" />
          </el-form-item>
          <el-form-item :label="form.type === 'tencent' ? 'SecretId' : 'AccessKeyId'">
            <el-input v-model="form.username" :placeholder="editing ? '留空=不修改' : '子账号凭证 ID'" autocomplete="off" />
          </el-form-item>
          <el-form-item :label="form.type === 'tencent' ? 'SecretKey' : 'AccessKeySecret'">
            <el-input v-model="form.password" :placeholder="editing ? '留空=不修改' : '凭证密钥'" show-password autocomplete="new-password" />
          </el-form-item>
          <el-form-item label="告警拉取">
            <el-input v-model="form.alert_cron" placeholder="cron（5 段），留空=仅手动同步" />
            <span class="list-hint">例：*/5 * * * * 每5分钟（阿里云需 CloudMonitor 只读 / 腾讯云需 QcloudMonitorReadOnlyAccess）</span>
          </el-form-item>
          <el-form-item label="拉取窗口(分钟)">
            <el-input-number v-model="form.alert_window_min" :min="0" :max="21600" :step="5" controls-position="right" />
            <span class="list-hint">每次拉最近 N 分钟，0=默认20。建议 ≥ cron 间隔以免漏（上限15天=21600）。</span>
          </el-form-item>
        </template>

        <!-- 回调源（webhook）-->
        <template v-else-if="form.type === 'webhook'">
          <el-form-item v-if="form.id" label="Webhook URL">
            <el-input :model-value="webhookUrl" readonly>
              <template #append><el-button :icon="CopyDocument" @click="copyWebhook" /></template>
            </el-input>
            <span class="list-hint">填到告警平台的「回调 / Webhook」里即可。已自动识别：夜莺(n9e)、Prometheus Alertmanager、Grafana 及常见自定义 JSON；触发/恢复都会 POST 过来。</span>
          </el-form-item>
          <el-form-item v-else label="说明">
            <span class="list-hint">保存后会生成带 token 的 Webhook URL（在此告警源的「编辑」里复制），填到平台回调地址即可。只填名称即可。</span>
          </el-form-item>
        </template>

        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <div class="dlg-footer">
          <el-button v-if="editing" type="danger" plain :icon="Delete" @click="removeFromEdit">删除</el-button>
          <span class="dlg-right">
            <el-button v-if="isPull(form.type)" :loading="testing" @click="doTest">测试连接</el-button>
            <el-button @click="dialog = false">取消</el-button>
            <el-button type="primary" @click="submit">确定</el-button>
          </span>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Top, Bottom, Delete, CopyDocument } from '@element-plus/icons-vue'
import * as api from '@/api/system'
import { useUserStore } from '@/store/user'

const userStore = useUserStore()
const canManage = computed(() => userStore.hasFunc('datasource_manage'))

const list = ref([])
const loading = ref(false)
const syncingId = ref(0)

const TYPE_LABELS = { aliyun: '阿里云', arms: 'ARMS', tencent: '腾讯云', prometheus: 'Prometheus', webhook: '告警回调' }
const typeLabel = (t) => TYPE_LABELS[t] || t
const isCloud = (t) => t === 'aliyun' || t === 'arms' || t === 'tencent'
// 支持主动拉取（有「同步告警 / 测试连接」）的告警源：云 + Prometheus
const isPull = (t) => isCloud(t) || t === 'prometheus'
function typeClass(t) {
  if (t === 'aliyun') return 'is-warning'
  if (t === 'tencent') return 'is-info'
  if (t === 'prometheus') return 'is-success'
  return 'is-purple'
}
const webhookOf = (row) => row.type === 'webhook' && row.token
function formatTime(t) {
  return t ? new Date(t).toLocaleString('zh-CN') : '-'
}

async function load() {
  loading.value = true
  try {
    list.value = await api.listAlertSources()
  } finally {
    loading.value = false
  }
}

onMounted(load)

async function move(index, dir) {
  const target = index + dir
  if (target < 0 || target >= list.value.length) return
  const arr = [...list.value]
  ;[arr[index], arr[target]] = [arr[target], arr[index]]
  list.value = arr
  try {
    await api.reorderAlertSources(arr.map((d) => d.id))
  } catch {
    ElMessage.error('排序失败')
    load()
  }
}

const dialog = ref(false)
const editing = ref(false)
const testing = ref(false)
const form = ref({})
const webhookUrl = computed(() =>
  form.value.id ? `${window.location.origin}/api/alerts/webhook/${form.value.id}?token=${form.value.token || ''}` : '',
)
async function copyWebhook() {
  try {
    await navigator.clipboard.writeText(webhookUrl.value)
    ElMessage.success('已复制 Webhook URL')
  } catch {
    ElMessage.warning('复制失败，请手动选择复制')
  }
}

async function doTest() {
  if (form.value.type === 'prometheus') {
    if (!form.value.url) { ElMessage.warning('请先填写 Prometheus 地址'); return }
  } else if (!form.value.username || !form.value.password) {
    ElMessage.warning(form.value.type === 'tencent' ? '请先填写 SecretId / SecretKey' : '请先填写 AccessKey')
    return
  }
  testing.value = true
  try {
    await api.testAlertSource(form.value)
    ElMessage.success('连接成功')
  } finally {
    testing.value = false
  }
}

function openCreate() {
  editing.value = false
  form.value = { name: '', type: 'aliyun', region: '', url: '', username: '', password: '', token: '', alert_cron: '', alert_window_min: 0, enabled: true }
  dialog.value = true
}
function openEdit(row) {
  editing.value = true
  form.value = { ...row, username: '', password: '' } // 凭证留空=不改
  dialog.value = true
}
async function submit() {
  if (!form.value.name) {
    ElMessage.warning('名称必填')
    return
  }
  if (isCloud(form.value.type) && !editing.value && (!form.value.username || !form.value.password)) {
    ElMessage.warning(form.value.type === 'tencent' ? '腾讯云需填 SecretId 与 SecretKey' : '阿里云需填 AccessKeyId 与 AccessKeySecret')
    return
  }
  if (editing.value) {
    await api.updateAlertSource(form.value.id, form.value)
  } else {
    const created = await api.createAlertSource(form.value)
    // 回调源：创建后带出 token，切编辑态展示 Webhook URL 供复制
    if (form.value.type === 'webhook' && created && created.id) {
      await load()
      form.value = { ...created }
      editing.value = true
      ElMessage.success('已创建，请复制下方 Webhook URL 填到告警平台的回调地址')
      return
    }
  }
  ElMessage.success('保存成功')
  dialog.value = false
  load()
}
async function removeFromEdit() {
  await ElMessageBox.confirm(`确定删除告警源「${form.value.name}」？`, '提示', { type: 'warning' })
  await api.deleteAlertSource(form.value.id)
  ElMessage.success('已删除')
  dialog.value = false
  load()
}
async function doSync(row) {
  syncingId.value = row.id
  try {
    const r = await api.syncAlertSource(row.id)
    ElMessage.success(`告警拉取完成：处理 ${r.count} 条`)
    await load()
  } finally {
    syncingId.value = 0
  }
}
</script>

<style scoped>
.list-card { padding: 18px 20px; }
.list-hint { font-size: 12.5px; color: var(--text-muted); }
.as-name { font-weight: 600; color: var(--text-primary); }
.muted { color: var(--text-secondary); }
.sm { font-size: 12px; margin-left: 8px; }
.dlg-footer { display: flex; align-items: center; justify-content: space-between; }
.dlg-right { display: inline-flex; gap: 8px; }
</style>
