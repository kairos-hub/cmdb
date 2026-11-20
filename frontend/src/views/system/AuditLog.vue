<template>
  <div>
    <div class="page-header">
      <div>
        <div class="ph-title">审计日志</div>
        <div class="ph-sub">记录平台的新增 / 编辑 / 删除等写操作，可按账号、对象、动作检索</div>
      </div>
      <div class="ph-actions">
        <el-dropdown trigger="click" @command="onCleanup">
          <el-button type="danger" plain :icon="Delete">
            清理日志<el-icon class="el-icon--right"><ArrowDown /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="7">删除 7 天前</el-dropdown-item>
              <el-dropdown-item command="30">删除 30 天前</el-dropdown-item>
              <el-dropdown-item command="90">删除 90 天前</el-dropdown-item>
              <el-dropdown-item command="custom">删除 N 天前…</el-dropdown-item>
              <el-dropdown-item command="all" divided>删除全部</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </div>

    <div class="panel panel-pad">
    <div class="filter-row">
      <el-input v-model="q.username" placeholder="操作账号" clearable style="width: 150px" :prefix-icon="Search" @keyup.enter="reload" />
      <el-input v-model="q.object" placeholder="操作对象" clearable style="width: 160px" @keyup.enter="reload" />
      <el-select v-model="q.action" placeholder="全部动作" clearable style="width: 140px">
        <el-option label="新增" value="POST" />
        <el-option label="编辑" value="PUT" />
        <el-option label="删除" value="DELETE" />
      </el-select>
      <el-button type="primary" :icon="Search" @click="reload">查询</el-button>
    </div>

    <el-table :data="list" v-loading="loading">
      <el-table-column label="时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作账号" prop="username" width="110" />
      <el-table-column label="动作" width="90">
        <template #default="{ row }">
          <span class="soft-tag" :class="actionSoftClass(row)">{{ row.action_text || row.action }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作对象" prop="object" width="110">
        <template #default="{ row }">{{ row.object || '-' }}</template>
      </el-table-column>
      <el-table-column label="实例" prop="instance" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">{{ row.instance || '-' }}</template>
      </el-table-column>
      <el-table-column label="所属业务" width="130">
        <template #default="{ row }">
          <span v-if="row.biz_name" class="soft-tag is-info">{{ row.biz_name }}</span>
          <span v-else class="muted">-</span>
        </template>
      </el-table-column>
      <el-table-column label="操作描述" prop="summary" min-width="200" show-overflow-tooltip>
        <template #default="{ row }">{{ row.summary || '-' }}</template>
      </el-table-column>
      <el-table-column label="结果" width="80">
        <template #default="{ row }">
          <span class="soft-tag" :class="row.status >= 400 ? 'is-danger' : 'is-success'">
            {{ row.status >= 400 ? '失败' : '成功' }}
          </span>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      class="pager"
      background
      layout="total, prev, pager, next"
      :total="total"
      :current-page="page"
      :page-size="pageSize"
      @current-change="(p) => { page = p; load() }"
    />
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Search, Delete, ArrowDown } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import * as api from '@/api/system'

const list = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const q = reactive({ username: '', object: '', action: '' })

function formatTime(t) {
  return t ? new Date(t).toLocaleString('zh-CN') : '-'
}
function actionSoftClass(row) {
  if (row.action === 'DELETE') return 'is-danger'
  if (row.action === 'POST') return 'is-success'
  if (row.action === 'PUT') return 'is-warning'
  return 'is-info'
}
async function load() {
  loading.value = true
  try {
    const res = await api.listAudit({
      username: q.username || undefined,
      object: q.object || undefined,
      action: q.action || undefined,
      page: page.value,
      page_size: pageSize.value,
    })
    list.value = res.list || []
    total.value = res.total || 0
  } finally {
    loading.value = false
  }
}
function reload() {
  page.value = 1
  load()
}

async function doCleanup(days, desc) {
  await ElMessageBox.confirm(
    `确定要${desc}吗？此操作不可恢复。`,
    '清理审计日志',
    { type: 'warning', confirmButtonText: '确定清理', cancelButtonText: '取消' }
  )
  const res = await api.cleanupAudit(days)
  ElMessage.success(`已清理 ${res?.deleted ?? 0} 条日志`)
  reload()
}

async function onCleanup(command) {
  try {
    if (command === 'all') {
      await doCleanup(0, '删除全部审计日志')
      return
    }
    if (command === 'custom') {
      const { value } = await ElMessageBox.prompt('请输入天数 N，将删除 N 天前的日志', '清理审计日志', {
        confirmButtonText: '下一步',
        cancelButtonText: '取消',
        inputPattern: /^[1-9]\d*$/,
        inputErrorMessage: '请输入正整数',
      })
      const n = parseInt(value, 10)
      await doCleanup(n, `删除 ${n} 天前的审计日志`)
      return
    }
    const n = parseInt(command, 10)
    await doCleanup(n, `删除 ${n} 天前的审计日志`)
  } catch (e) {
    // 取消操作：忽略
  }
}

onMounted(load)
</script>

<style scoped>
.filter-row {
  display: flex;
  gap: 10px;
  margin-bottom: 16px;
  align-items: center;
}
.pager {
  margin-top: 16px;
  justify-content: flex-end;
}
.muted {
  color: var(--text-muted);
}
</style>
