<template>
  <div>
    <div class="page-header">
      <div>
        <div class="ph-title">数据源</div>
        <div class="ph-sub">统一管理 Prometheus 与云账号同步，让资产数据按内网IP持续自动写入资源实例</div>
      </div>
      <div class="ph-actions">
        <el-tag v-if="!canManage" type="info" size="small">只读模式：无「数据源·编辑」权限</el-tag>
        <el-button v-if="canManage" type="primary" :icon="Plus" @click="openCreate">新建数据源</el-button>
      </div>
    </div>

    <!-- KPI -->
    <div class="kpi-row">
      <div class="stat-card">
        <div class="stat-label">数据源总数</div>
        <div class="stat-value">{{ list.length }}</div>
        <div class="stat-sub">Prometheus + 云账号</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">已启用</div>
        <div class="stat-value">{{ enabledCount }}</div>
        <div class="stat-sub" :class="enabledCount ? 'is-up' : ''">参与同步调度</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">定时同步</div>
        <div class="stat-value">{{ scheduledCount }}</div>
        <div class="stat-sub">配置了 cron 周期</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">云账号</div>
        <div class="stat-value">{{ cloudCount }}</div>
        <div class="stat-sub">阿里云 / 腾讯云</div>
      </div>
    </div>

    <div class="panel panel-pad list-card">
      <div class="panel-head">
        <span class="panel-title">数据源列表</span>
        <span class="list-hint">配置后点「同步」从中发现资源并按内网IP写入实例</span>
      </div>
    <el-table :data="list" v-loading="loading">
      <el-table-column label="名称" width="170">
        <template #default="{ row }"><span class="ds-name">{{ row.name }}</span></template>
      </el-table-column>
      <el-table-column label="类型" width="120">
        <template #default="{ row }"><span class="soft-tag" :class="typeClass(row.type)">{{ typeLabel(row.type) }}</span></template>
      </el-table-column>
      <el-table-column label="地址" prop="url" min-width="220" show-overflow-tooltip />
      <el-table-column label="查询(PromQL)" min-width="140">
        <template #default="{ row }">{{ row.query || 'up' }}</template>
      </el-table-column>
      <el-table-column label="定时同步" min-width="150">
        <template #default="{ row }">
          <span v-if="row.cron">{{ row.cron }}</span>
          <span v-else class="muted">—</span>
        </template>
      </el-table-column>
      <el-table-column label="上次同步" min-width="200">
        <template #default="{ row }">
          <div v-if="row.last_sync_at">
            <div>{{ formatTime(row.last_sync_at) }}</div>
            <div class="muted sm">{{ row.last_sync_msg }}</div>
          </div>
          <span v-else class="muted">—</span>
        </template>
      </el-table-column>
      <el-table-column label="启用" width="90">
        <template #default="{ row }">
          <span class="soft-tag" :class="row.enabled ? 'is-success' : 'is-muted'"><span class="dot"></span>{{ row.enabled ? '启用' : '停用' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="220" fixed="right">
        <template #default="{ row }">
          <template v-if="canManage">
            <el-button link type="primary" :loading="syncingId === row.id" :disabled="!row.enabled" @click="doSync(row)">同步</el-button>
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
    </div>

    <el-dialog v-model="dialog" :title="editing ? '编辑数据源' : '新建数据源'" width="520px">
      <el-form :model="form" label-width="110px">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="如 生产Prometheus" /></el-form-item>
        <el-form-item label="类型">
          <el-select v-model="form.type" style="width: 100%" :disabled="editing">
            <el-option label="Prometheus" value="prometheus" />
            <el-option label="阿里云" value="aliyun" />
            <el-option label="腾讯云" value="tencent" />
          </el-select>
        </el-form-item>

        <!-- Prometheus -->
        <template v-if="form.type === 'prometheus'">
          <el-form-item label="地址"><el-input v-model="form.url" placeholder="http://prometheus:9090" /></el-form-item>
          <el-form-item label="查询(PromQL)">
            <el-input v-model="form.query" placeholder="默认 up；可用 node_uname_info 获取主机名/系统" />
          </el-form-item>
          <el-form-item label="账号">
            <el-input v-model="form.username" placeholder="可选，Basic Auth 账号" autocomplete="off" />
          </el-form-item>
          <el-form-item label="密码">
            <el-input v-model="form.password" placeholder="可选，Basic Auth 密码" show-password autocomplete="new-password" />
          </el-form-item>
          <el-form-item label="Token">
            <el-input v-model="form.token" placeholder="可选，Bearer Token（填了则优先用 Token）" show-password />
          </el-form-item>
          <el-form-item label="仅同步IP目标">
            <el-switch v-model="form.only_ip" />
            <span class="sw-hint">开启后只同步 instance 为合法 IP 的目标，自动过滤域名/URL 类</span>
          </el-form-item>
        </template>

        <!-- 阿里云（当前同步 ECS → 主机）-->
        <template v-else-if="form.type === 'aliyun'">
          <el-form-item label="地域">
            <el-input v-model="form.region" placeholder="cn-shanghai" />
            <span class="sw-hint full">留空默认 cn-shanghai；多地域用逗号分隔，如 cn-shanghai,cn-hangzhou</span>
          </el-form-item>
          <el-form-item label="AccessKeyId">
            <el-input v-model="form.username" placeholder="RAM 子账号 AccessKey ID" autocomplete="off" />
          </el-form-item>
          <el-form-item label="AccessKeySecret">
            <el-input v-model="form.password" placeholder="AccessKey Secret" show-password autocomplete="new-password" />
          </el-form-item>
          <el-form-item label="说明">
            <span class="sw-hint">当前同步 ECS → 主机；建议用只读 RAM 子账号。</span>
          </el-form-item>
        </template>

        <!-- 腾讯云（当前同步 CVM → 主机）-->
        <template v-else-if="form.type === 'tencent'">
          <el-form-item label="地域">
            <el-input v-model="form.region" placeholder="ap-shanghai" />
            <span class="sw-hint full">留空默认 ap-shanghai；多地域用逗号分隔，如 ap-shanghai,ap-guangzhou</span>
          </el-form-item>
          <el-form-item label="SecretId">
            <el-input v-model="form.username" placeholder="子账号 SecretId" autocomplete="off" />
          </el-form-item>
          <el-form-item label="SecretKey">
            <el-input v-model="form.password" placeholder="SecretKey" show-password autocomplete="new-password" />
          </el-form-item>
          <el-form-item label="说明">
            <span class="sw-hint">当前同步 CVM → 主机；建议用只读子账号（CVM 只读权限）。</span>
          </el-form-item>
        </template>

        <el-form-item v-if="form.type === 'prometheus'" label="监控地址">
          <el-input
            v-model="form.monitor_url"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 5 }"
            placeholder="https://grafana/d/<uid>/node?var-node=${ip}&var-job=${job}&from=now-1h&to=now&kiosk"
          />
          <span class="sw-hint full">「完整监控」跳转地址（Grafana 等）；主机用 ${ip}、${job}(实时读真实 job 标签)，K8s 应用用 ${namespace}、${workload}、${pod}、${cluster}、${app}，留空则跳 Prometheus。</span>
        </el-form-item>
        <el-form-item label="定时同步">
          <el-input v-model="form.cron" placeholder="cron 表达式（5 段：分 时 日 月 周），留空=不自动" />
          <span class="sw-hint">资源同步节奏。例：0 * * * * 每小时整点；*/30 * * * * 每30分钟；0 2 * * * 每天2点</span>
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <div class="dlg-footer">
          <el-button v-if="editing" type="danger" plain :icon="Delete" @click="removeFromEdit">删除</el-button>
          <span class="dlg-right">
            <el-button :loading="testing" @click="doTest">测试连接</el-button>
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
import { Plus, Top, Bottom, Delete } from '@element-plus/icons-vue'
import * as api from '@/api/system'
import { useUserStore } from '@/store/user'

const userStore = useUserStore()
const canManage = computed(() => userStore.hasFunc('datasource_manage'))

const list = ref([])
const loading = ref(false)
const syncingId = ref(0)

const enabledCount = computed(() => list.value.filter((d) => d.enabled).length)
const scheduledCount = computed(() => list.value.filter((d) => d.cron).length)
const cloudCount = computed(() => list.value.filter((d) => d.type === 'aliyun' || d.type === 'tencent').length)

const TYPE_LABELS = { prometheus: 'Prometheus', aliyun: '阿里云', tencent: '腾讯云' }
const typeLabel = (t) => TYPE_LABELS[t] || t
function typeClass(t) {
  if (t === 'aliyun') return 'is-warning'
  if (t === 'tencent') return 'is-info'
  return 'is-purple'
}

async function load() {
  loading.value = true
  try {
    list.value = await api.listDataSources()
  } finally {
    loading.value = false
  }
}
onMounted(load)

// 自定义排序：上移(dir=-1)/下移(dir=1)并持久化顺序
async function move(index, dir) {
  const target = index + dir
  if (target < 0 || target >= list.value.length) return
  const arr = [...list.value]
  ;[arr[index], arr[target]] = [arr[target], arr[index]]
  list.value = arr // 先本地即时反馈
  try {
    await api.reorderDataSources(arr.map((d) => d.id))
  } catch {
    ElMessage.error('排序失败')
    load()
  }
}

function formatTime(t) {
  return t ? new Date(t).toLocaleString('zh-CN') : '-'
}

const dialog = ref(false)
const editing = ref(false)
const testing = ref(false)
const form = ref({})

async function doTest() {
  if (form.value.type === 'aliyun' || form.value.type === 'tencent') {
    if (!form.value.username || !form.value.password) {
      ElMessage.warning(form.value.type === 'tencent' ? '请先填写 SecretId / SecretKey' : '请先填写 AccessKey')
      return
    }
  } else if (!form.value.url) {
    ElMessage.warning('请先填写地址')
    return
  }
  testing.value = true
  try {
    const r = await api.testDataSource(form.value)
    ElMessage.success(`连接成功，查询到 ${r.count} 条数据`)
  } finally {
    testing.value = false
  }
}
function openCreate() {
  editing.value = false
  form.value = { name: '', type: 'prometheus', url: '', query: 'node_uname_info', region: '', username: '', password: '', token: '', only_ip: true, monitor_url: '', cron: '', enabled: true }
  dialog.value = true
}
function openEdit(row) {
  editing.value = true
  form.value = { ...row }
  dialog.value = true
}
async function submit() {
  if (!form.value.name) {
    ElMessage.warning('名称必填')
    return
  }
  if (form.value.type === 'aliyun' || form.value.type === 'tencent') {
    if (!form.value.username || !form.value.password) {
      ElMessage.warning(form.value.type === 'tencent' ? '腾讯云需填 SecretId 与 SecretKey' : '阿里云需填 AccessKeyId 与 AccessKeySecret')
      return
    }
  } else if (!form.value.url) {
    ElMessage.warning('地址必填')
    return
  }
  if (editing.value) {
    await api.updateDataSource(form.value.id, form.value)
  } else {
    await api.createDataSource(form.value)
  }
  ElMessage.success('保存成功')
  dialog.value = false
  load()
}
async function remove(row) {
  await ElMessageBox.confirm(`确定删除数据源「${row.name}」？`, '提示', { type: 'warning' })
  await api.deleteDataSource(row.id)
  ElMessage.success('已删除')
  load()
}
// 从编辑弹窗内删除当前数据源
async function removeFromEdit() {
  await ElMessageBox.confirm(`确定删除数据源「${form.value.name}」？`, '提示', { type: 'warning' })
  await api.deleteDataSource(form.value.id)
  ElMessage.success('已删除')
  dialog.value = false
  load()
}
async function doSync(row) {
  syncingId.value = row.id
  try {
    const r = await api.syncDataSource(row.id)
    const msg = `发现 ${r.discovered}，新增 ${r.created}，更新 ${r.updated}，跳过 ${r.skipped || 0}，失败 ${r.failed}`
    if (r.failed) {
      ElMessageBox.alert(msg + '\n' + (r.errors || []).join('\n'), '同步结果', { type: 'warning' })
    } else {
      ElMessage.success('同步完成：' + msg)
    }
  } finally {
    syncingId.value = 0
  }
}
</script>

<style scoped>
.kpi-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 18px;
}
.list-card {
  padding: 18px 20px;
}
.list-hint {
  font-size: 12.5px;
  color: var(--text-muted);
}
.ds-name {
  font-weight: 600;
  color: var(--text-primary);
}
.dlg-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.dlg-right {
  display: inline-flex;
  gap: 12px;
}
.sw-hint {
  color: var(--text-secondary);
  font-size: 12px;
  margin-left: 10px;
}
/* 监控地址提示：整行显示、可换行，不被截断 */
.sw-hint.full {
  display: block;
  width: 100%;
  margin-left: 0;
  margin-top: 4px;
  line-height: 1.5;
  white-space: normal;
  word-break: break-word;
}
/* textarea 里长 URL 换行显示 */
:deep(.el-textarea__inner) {
  word-break: break-all;
  font-family: inherit;
}
.muted {
  color: var(--text-muted);
}
.sm {
  font-size: 12px;
}
@media (max-width: 1200px) {
  .kpi-row { grid-template-columns: repeat(2, 1fr); }
}
</style>
