<template>
  <div class="topo-page">
    <!-- 工具栏（标题已去除，让拓扑图占更大屏占比） -->
    <div class="page-header topo-toolbar">
      <div class="tb-left">
        <span class="tb-label">业务线</span>
        <el-select v-model="bizId" placeholder="选择业务线" style="width: 158px" @change="onBizChange">
          <el-option v-for="b in businessList" :key="b.id" :label="b.name" :value="b.id" />
        </el-select>
        <el-icon class="tb-sep"><ArrowRight /></el-icon>
        <span class="tb-label">业务</span>
        <el-select v-model="businessId" placeholder="选择业务" style="width: 148px" :disabled="!businesses.length" @change="onBusinessChange">
          <el-option v-for="b in businesses" :key="b.id" :label="b.name" :value="b.id" />
        </el-select>
        <el-dropdown v-if="canEdit" trigger="click" @command="onBusinessCmd">
          <el-button text class="tb-more" :icon="MoreFilled" title="业务操作" />
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="new" :icon="Plus" :disabled="!bizId">新建业务</el-dropdown-item>
              <el-dropdown-item command="edit" :icon="Edit" :disabled="!businessId">重命名业务</el-dropdown-item>
              <el-dropdown-item command="delete" :icon="Delete" :disabled="!businessId" divided>删除业务</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
      <div class="tb-right ph-actions">
        <span v-if="!canEdit" class="readonly-hint">只读模式</span>
        <el-button :icon="Refresh" circle @click="refreshAll" title="刷新" />
        <el-button :icon="maximized ? Close : FullScreen" circle @click="toggleFull" :title="maximized ? '退出全屏' : '全屏'" />
        <el-button v-if="canEdit" type="primary" :icon="Plus" @click="newBiz">新建业务线</el-button>
        <el-dropdown trigger="click" @command="onBizCmd">
          <el-button>业务线<el-icon class="el-icon--right"><ArrowDown /></el-icon></el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-if="canEdit" command="edit" :icon="Edit" :disabled="!bizId">编辑业务线</el-dropdown-item>
              <el-dropdown-item v-if="canEdit" command="delete" :icon="Delete" :disabled="!bizId">删除业务线</el-dropdown-item>
              <el-dropdown-item command="export" :icon="Download" :disabled="!bizId" divided>导出</el-dropdown-item>
              <el-dropdown-item v-if="canEdit" command="import" :icon="Upload">导入</el-dropdown-item>
              <el-dropdown-item v-if="canEdit" command="template" :icon="Document">模板</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <input ref="fileInput" type="file" accept=".xlsx" style="display: none" @change="onFilePick" />
      </div>
    </div>

    <el-empty v-if="!bizId" description="暂无业务线，请先新建业务线" />

    <template v-else>
      <!-- 统计卡片（按所选业务） -->
      <div class="stat-row">
        <div class="stat-card">
          <div class="stat-ic ic-blue"><el-icon><OfficeBuilding /></el-icon></div>
          <div class="stat-body"><div class="stat-label">业务数</div><div class="stat-val">{{ businesses.length }}</div></div>
        </div>
        <div class="stat-card">
          <div class="stat-ic ic-green"><el-icon><Connection /></el-icon></div>
          <div class="stat-body"><div class="stat-label">模型数</div><div class="stat-val">{{ businessModules.length }}</div></div>
        </div>
        <div class="stat-card">
          <div class="stat-ic ic-cyan"><el-icon><Coin /></el-icon></div>
          <div class="stat-body"><div class="stat-label">关联资产</div><div class="stat-val">{{ businessResourceTotal }}</div></div>
        </div>
        <div class="stat-card">
          <div class="stat-ic ic-orange"><el-icon><WarningFilled /></el-icon></div>
          <div class="stat-body"><div class="stat-label">异常资源</div><div class="stat-val" :class="{ danger: businessAbnormal.length }">{{ businessAbnormal.length }}</div></div>
        </div>
      </div>

      <!-- 拓扑画布 -->
      <div ref="canvasWrapRef" class="graph-card" :class="{ maximized }">
        <div class="graph-toolbar">
          <span class="g-crumb">
            <el-icon><OfficeBuilding /></el-icon>{{ curBiz?.name }}
            <template v-if="currentBusiness"><el-icon class="sep"><ArrowRight /></el-icon>{{ currentBusiness.name }}</template>
          </span>
          <el-input v-model="kw" placeholder="搜索模型..." clearable :prefix-icon="Search" class="g-search" />
          <el-select v-model="stateFilter" placeholder="状态" clearable class="g-filter">
            <el-option label="全部" value="" />
            <el-option label="正常" value="normal" />
            <el-option label="异常" value="abnormal" />
            <el-option label="离线" value="offline" />
          </el-select>
          <div class="g-view" title="切换视图">
            <span :class="{ active: viewMode === 'canvas' }" @click="viewMode = 'canvas'">画布</span>
            <span :class="{ active: viewMode === 'tree' }" @click="viewMode = 'tree'">树视图</span>
          </div>
          <span class="g-tip" v-if="canEdit && viewMode === 'canvas'">拖模型入画布建点 · 拖节点蓝点连线</span>
          <span class="g-tip" v-else-if="viewMode === 'tree'">自上而下树 · 点色块隐藏模型 · 点节点看资源</span>
          <!-- 画布=状态图例；树视图=模型图例(可点击隐藏/显示) -->
          <div v-if="viewMode === 'canvas'" class="g-legend">
            <span><i class="dot" style="background:#22c55e"></i>正常</span>
            <span><i class="dot" style="background:#f59e0b"></i>异常</span>
            <span><i class="dot" style="background:#9ca3af"></i>离线</span>
          </div>
          <div v-else class="g-legend rel-legend">
            <span
              v-for="it in treeLegend"
              :key="it.id"
              class="rel-legend-item"
              :class="{ off: hiddenTreeModels.has(it.id) }"
              @click="toggleTreeModel(it.id)"
            >
              <i class="rel-sw" :style="{ background: it.color }"></i>{{ it.name }}
            </span>
          </div>
        </div>

        <div class="graph-body">
          <div v-if="canEdit && currentBusiness && viewMode === 'canvas'" class="palette" :class="{ collapsed: !paletteOpen }">
            <template v-if="paletteOpen">
              <div class="palette-head">
                <span class="ph-title2">资源模型<span class="ph-hint">拖到画布建模型节点</span></span>
                <el-icon class="ph-toggle" title="收起" @click="paletteOpen = false"><DArrowLeft /></el-icon>
              </div>
              <el-input v-model="paletteKw" placeholder="搜索资源类型" clearable size="small" :prefix-icon="Search" class="palette-search" />
              <div class="palette-tools">
                <span @click="setAllGroups(false)">全部展开</span>
                <span @click="setAllGroups(true)">全部收起</span>
              </div>
              <el-scrollbar class="palette-body">
                <div v-for="g in paletteGroups" :key="g.id" class="pg">
                  <div class="pg-name" @click="toggleGroup(g.id)">
                    <el-icon class="pg-caret" :class="{ folded: collapsedGroups.has(g.id) }"><ArrowRight /></el-icon>
                    <span class="pg-label">{{ g.name }}</span>
                    <span class="pg-count">{{ g.models.length }}</span>
                  </div>
                  <template v-if="!collapsedGroups.has(g.id)">
                    <div
                      v-for="m in g.models"
                      :key="m.id"
                      class="pg-item"
                      draggable="true"
                      @dragstart="onPaletteDragStart(m, $event)"
                      @dragend="dragModelId = null"
                    >
                      <ModelIcon :obj-id="m.obj_id" :fallback="m.icon" :size="16" />
                      <span class="pg-label">{{ m.name }}</span>
                    </div>
                  </template>
                </div>
                <el-empty v-if="!paletteGroups.length" description="无匹配资源类型" :image-size="48" />
              </el-scrollbar>
            </template>
            <div v-else class="palette-mini" title="展开资源模型" @click="paletteOpen = true">
              <el-icon><DArrowRight /></el-icon>
              <span class="pm-label">资源模型</span>
            </div>
          </div>

          <div
            v-show="viewMode === 'canvas'"
            ref="canvasRef"
            class="canvas"
            :class="{ grabbing: dragging }"
            @mousedown="onCanvasDown"
            @wheel="onWheel"
            @dragover.prevent
            @drop.prevent="onCanvasDrop"
          >
            <el-empty v-if="!businesses.length" :description="canEdit ? '该业务线暂无业务，请在右上角「业务」菜单新建' : '该业务线暂无业务'" />
            <el-empty v-else-if="!currentBusiness" description="请选择一个业务" />
            <template v-else>
              <div v-if="!nodes.length" class="canvas-hint">{{ canEdit ? '从左侧把资源模型拖到这里，开始搭建拓扑' : '该业务暂无模型' }}</div>
              <div class="stage" :style="stageStyle">
                <svg class="edges" :width="contentW" :height="contentH">
                  <defs>
                    <marker id="topo-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                      <path d="M0,0 L10,5 L0,10 z" fill="#a7b8d0" />
                    </marker>
                  </defs>
                  <!-- 已建连线 -->
                  <g v-for="e in edgeGeoms" :key="'e' + e.id" class="edge-g">
                    <path :d="e.d" class="edge-line" marker-end="url(#topo-arrow)" />
                    <path :d="e.d" class="edge-hit" @click="removeEdge(e.id)" />
                    <g v-if="canEdit" class="edge-del" :transform="`translate(${e.mx},${e.my})`" @click="removeEdge(e.id)">
                      <circle r="9" />
                      <path d="M-3.5,-3.5 L3.5,3.5 M3.5,-3.5 L-3.5,3.5" />
                    </g>
                  </g>
                  <!-- 正在拖拽的新连线 -->
                  <line v-if="drawing" :x1="drawing.sx" :y1="drawing.sy" :x2="drawing.x" :y2="drawing.y" class="edge-temp" marker-end="url(#topo-arrow)" />
                </svg>

                <!-- 业务锚点节点（画布根，关联其下资源实例；可拖动摆放，位置持久化） -->
                <div
                  v-if="bizNode"
                  class="topo-card card-biz"
                  :class="{ dragging: bizDrag }"
                  :style="{ left: bizNode.x + 'px', top: bizNode.y + 'px', width: BIZ_W + 'px', height: BIZ_H + 'px' }"
                  @mousedown.stop="onBizDown($event)"
                  title="业务（拖动可摆放位置）"
                >
                  <span class="st-bar" :style="{ background: stateColor(businessState) }"></span>
                  <div class="card-ic biz-ic"><el-icon><OfficeBuilding /></el-icon></div>
                  <div class="card-main">
                    <div class="card-name">{{ bizNode.name }}</div>
                    <div class="card-sub">{{ businessModules.length }} 模型 · {{ businessResourceTotal }} 资源</div>
                  </div>
                  <span
                    v-if="canEdit"
                    class="conn-handle"
                    title="拖到资源模型以创建业务连线"
                    @mousedown.stop.prevent="onHandleDown(bizNode, $event)"
                  ></span>
                </div>

                <!-- 模型节点 -->
                <div
                  v-for="n in nodes"
                  :key="'n' + n.id"
                  class="topo-card card-mod"
                  :class="{ dim: nodeDim(n.name, moduleStateOf(n.raw)), dragging: nodeDrag && nodeDrag.id === n.id, 'conn-target': drawing && hoverNodeId === n.id && drawing.from !== n.id }"
                  :style="{ left: n.x + 'px', top: n.y + 'px', width: NODE_W + 'px', height: NODE_H + 'px' }"
                  @mousedown.stop="onNodeDown(n, $event)"
                  @mouseenter="hoverNodeId = n.id"
                  @mouseleave="hoverNodeId = null"
                >
                  <span class="st-bar" :style="{ background: stateColor(moduleStateOf(n.raw)) }"></span>
                  <span v-if="nodeAlertCount(n.id)" class="node-alert-badge" :style="{ background: nodeAlertColor(n.id), boxShadow: `0 1px 3px ${nodeAlertColor(n.id)}66` }" :title="`${nodeAlertCount(n.id)} 条告警`">{{ nodeAlertCount(n.id) > 99 ? '99+' : nodeAlertCount(n.id) }}</span>
                  <div class="card-ic mod-ic">
                    <ModelIcon :obj-id="modelMeta[n.model_id]?.obj_id" :fallback="modelMeta[n.model_id]?.icon" :size="16" />
                  </div>
                  <div class="card-main">
                    <div class="card-name">{{ n.name }}</div>
                    <div class="card-sub">{{ moduleCountText(n.raw) }}</div>
                  </div>
                  <div v-if="canEdit" class="card-ops node-ops">
                    <el-button link size="small" @click.stop="openEditModule(n.raw)">编辑</el-button>
                    <el-button link size="small" type="danger" @click.stop="delModule(n.raw)">删</el-button>
                  </div>
                  <!-- 连线手柄 -->
                  <span
                    v-if="canEdit"
                    class="conn-handle"
                    title="拖我连到另一个模型"
                    @mousedown.stop.prevent="onHandleDown(n, $event)"
                  ></span>
                </div>
              </div>
            </template>

            <div class="zoom-ctrl">
              <button @click="zoomIn" title="放大"><el-icon><Plus /></el-icon></button>
              <button @click="zoomOut" title="缩小"><el-icon><Minus /></el-icon></button>
              <button @click="fit" title="适应屏幕"><el-icon><Aim /></el-icon></button>
              <button @click="toggleFull" :title="maximized ? '退出全屏' : '全屏'">
                <el-icon><Close v-if="maximized" /><FullScreen v-else /></el-icon>
              </button>
            </div>
          </div>

          <!-- 树视图：只读、自上而下折叠树（业务→模型，按连线层级），与关联拓扑同一套节点样式 -->
          <div v-show="viewMode === 'tree'" class="tree-view">
            <el-empty v-if="!businessModules.length" description="该业务暂无模型，切到「画布」拖入资源模型" :image-size="70" />
            <div v-show="businessModules.length" ref="treeEl" class="tree-canvas"></div>
          </div>
        </div>
      </div>

      <!-- 底部信息区 -->
      <div class="info-row">
        <div class="info-card">
          <div class="info-head"><span><el-icon><Histogram /></el-icon> 业务详情</span></div>
          <el-tabs v-model="detailTab" class="info-tabs">
            <el-tab-pane label="基本信息" name="base">
              <div class="kv"><span class="k">业务线</span><span class="v">{{ curBiz?.name || '-' }}</span></div>
              <div class="kv"><span class="k">业务</span><span class="v">{{ currentBusiness?.name || '-' }}</span></div>
              <div class="kv"><span class="k">负责人</span><span class="v">{{ curBiz?.owner || '-' }}</span></div>
              <div class="kv"><span class="k">模型 / 连线</span><span class="v">{{ businessModules.length }} / {{ edges.length }}</span></div>
              <div class="kv"><span class="k">关联资源</span><span class="v">{{ businessResourceTotal }}</span></div>
              <div class="kv"><span class="k">状态</span><span class="v"><span class="st-pill" :style="pillStyle(businessState)">{{ stateText(businessState) }}</span></span></div>
            </el-tab-pane>
            <el-tab-pane label="关联资源" name="res">
              <el-scrollbar max-height="210px">
                <div v-for="m in businessModules" :key="m.id" class="res-line" @click="openModuleDrawer(m)">
                  <ModelIcon :obj-id="modelMeta[m.model_id]?.obj_id" :fallback="modelMeta[m.model_id]?.icon" :size="16" />
                  <span class="res-name">{{ m.name }}</span>
                  <span class="res-model">{{ m.model_name }}</span>
                  <span class="res-cnt">{{ m.host_count }}</span>
                  <span class="st-dot" :style="{ background: stateColor(moduleStateOf(m)) }"></span>
                </div>
                <el-empty v-if="!businessModules.length" description="暂无模型" :image-size="54" />
              </el-scrollbar>
            </el-tab-pane>
          </el-tabs>
        </div>

        <div class="info-card">
          <div class="info-head"><span><el-icon><PieChart /></el-icon> 关联资源统计</span></div>
          <div v-show="businessGroupDist.length" class="donut-wrap"><div ref="donutRef" class="donut"></div></div>
          <el-empty v-if="!businessGroupDist.length" description="暂无关联资源" :image-size="60" />
        </div>

        <div class="info-card">
          <div class="info-head">
            <span><el-icon><Bell /></el-icon> 告警与异常</span>
            <span class="ab-count" v-if="businessAlerts.length + businessAbnormal.length">{{ businessAlerts.length + businessAbnormal.length }}</span>
          </div>
          <el-scrollbar max-height="230px">
            <!-- 云监控告警（在触发） -->
            <div class="al-sub-head" v-if="businessAlerts.length">告警 · {{ businessAlerts.length }}</div>
            <div v-for="a in businessAlerts" :key="'al' + a.id" class="alert-line" @click="a.res_inst_id && goDetail(a.res_inst_id)">
              <span class="al-ic" :style="{ background: alertLevel(a.level).c + '1f', color: alertLevel(a.level).c }">
                <el-icon><WarningFilled /></el-icon>
              </span>
              <div class="al-body">
                <div class="al-title">{{ a.rule_name || '告警' }}</div>
                <div class="al-sub">{{ [alertLevel(a.level).t, a.model, a.label].filter(Boolean).join(' · ') }}</div>
              </div>
              <el-icon class="al-arrow"><ArrowRight /></el-icon>
            </div>
            <!-- 资源状态异常/离线 -->
            <div class="al-sub-head" v-if="businessAlerts.length && businessAbnormal.length">状态异常 · {{ businessAbnormal.length }}</div>
            <div v-for="r in businessAbnormal" :key="r.id" class="alert-line" @click="goDetail(r.id)">
              <span class="al-ic" :class="r.state === 'offline' ? 'al-grey' : 'al-orange'">
                <el-icon><component :is="r.state === 'offline' ? 'CircleClose' : 'WarningFilled'" /></el-icon>
              </span>
              <div class="al-body">
                <div class="al-title">{{ r.model }} · {{ r.label || ('#' + r.id) }}</div>
                <div class="al-sub">状态：{{ r.status }}</div>
              </div>
              <el-icon class="al-arrow"><ArrowRight /></el-icon>
            </div>
            <el-empty v-if="!businessAlerts.length && !businessAbnormal.length" description="暂无告警或异常" :image-size="60" />
          </el-scrollbar>
        </div>
      </div>
    </template>

    <!-- 模块资源管理抽屉 -->
    <el-drawer v-model="drawer" :title="drawerTitle" size="56%">
      <template v-if="selectedModule">
        <div class="mod-head">
          <span class="mod-name">
            模型：{{ selectedModule.name }}
            <el-tag v-if="selectedModule.model_name" size="small">{{ selectedModule.model_name }}</el-tag>
          </span>
          <div v-if="canEdit">
            <el-button type="primary" size="small" :icon="Plus" @click="openAssign">分配资源</el-button>
            <el-button size="small" :disabled="!moduleSelected.length" @click="openTransfer">转移</el-button>
            <el-button type="warning" size="small" :disabled="!moduleSelected.length" @click="doRecycle">回收</el-button>
          </div>
        </div>
        <el-table :data="moduleHostList" @selection-change="moduleSelected = $event">
          <el-table-column type="selection" width="45" />
          <el-table-column v-for="a in resColumns" :key="a.id" :label="a.name" show-overflow-tooltip>
            <template #default="{ row }">{{ displayValue(a, row.data?.[a.property_id]) }}</template>
          </el-table-column>
          <el-table-column v-if="!resColumns.length" label="资源">
            <template #default="{ row }">{{ resLabel(row) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="160" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="openHostRel(row)">关联拓扑</el-button>
              <el-button link type="primary" @click="goDetail(row.id)">详情</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-if="!moduleHostList.length" description="该模型暂无资源，点击「分配资源」添加" :image-size="70" />
      </template>
    </el-drawer>

    <!-- 新建/编辑业务线 -->
    <el-dialog v-model="bizDialog" :title="bizForm.id ? '编辑业务线' : '新建业务线'" width="440px">
      <el-form :model="bizForm" label-width="80px">
        <el-form-item label="名称" required><el-input v-model="bizForm.name" placeholder="业务线名称" /></el-form-item>
        <el-form-item label="负责人"><el-input v-model="bizForm.owner" placeholder="负责人（选填）" /></el-form-item>
        <el-form-item label="联系方式"><el-input v-model="bizForm.contact" placeholder="电话 / 邮箱 / 企微等（选填）" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="bizDialog = false">取消</el-button>
        <el-button type="primary" @click="submitBiz">确定</el-button>
      </template>
    </el-dialog>

    <!-- 编辑模型节点 -->
    <el-dialog v-model="editModuleDialog" title="编辑模型节点" width="440px">
      <el-form label-width="90px">
        <el-form-item label="资源类型"><el-tag>{{ editModuleForm.modelName || '-' }}</el-tag></el-form-item>
        <el-form-item label="节点名"><el-input v-model="editModuleForm.name" placeholder="请输入节点名称" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editModuleDialog = false">取消</el-button>
        <el-button type="primary" @click="saveEditModule">保存</el-button>
      </template>
    </el-dialog>

    <!-- 分配资源 -->
    <el-dialog v-model="assignDialog" :title="`分配资源 - ${selectedModule?.model_name || ''}`" width="600px">
      <el-input v-model="assignKeyword" placeholder="搜索资源（按展示字段过滤）" clearable :prefix-icon="Search" style="margin-bottom: 12px" />
      <el-table :data="filteredIdleList" border max-height="360" @selection-change="idleSelected = $event">
        <el-table-column type="selection" width="45" />
        <el-table-column v-for="a in resColumns" :key="a.id" :label="a.name" show-overflow-tooltip>
          <template #default="{ row }">{{ displayValue(a, row.data?.[a.property_id]) }}</template>
        </el-table-column>
        <el-table-column v-if="!resColumns.length" label="资源">
          <template #default="{ row }">{{ resLabel(row) }}</template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!idleList.length" description="资源池暂无该类型的空闲资源（请先在资源实例中创建对应实例）" />
      <el-empty v-else-if="!filteredIdleList.length" description="没有匹配搜索的空闲资源" :image-size="60" />
      <template #footer>
        <el-button @click="assignDialog = false">取消</el-button>
        <el-button type="primary" @click="doAssign">分配</el-button>
      </template>
    </el-dialog>

    <!-- 转移 -->
    <el-dialog v-model="transferDialog" title="转移资源到模型" width="440px">
      <el-select v-model="transferTarget" placeholder="选择同类型的目标模型" style="width: 100%">
        <el-option v-for="m in transferTargets" :key="m.id" :label="transferLabel(m)" :value="m.id" />
      </el-select>
      <el-empty v-if="!transferTargets.length" description="没有同类型的其它模型可转移" />
      <template #footer>
        <el-button @click="transferDialog = false">取消</el-button>
        <el-button type="primary" @click="doTransfer">转移</el-button>
      </template>
    </el-dialog>

    <!-- 资源关联关系与拓扑 -->
    <el-dialog v-model="relDialog" :title="`关联关系与拓扑 - ${relLabel}`" width="720px">
      <RelationPanel v-if="relDialog" :instance-id="relHostId" :self-label="relLabel" :self-model-name="relModelName" />
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import * as echarts from 'echarts'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Plus, Minus, Delete, Edit, OfficeBuilding, Coin, Download, Upload, Document,
  Search, ArrowDown, ArrowRight, Refresh, FullScreen, Connection, WarningFilled, Aim,
  Histogram, PieChart, Bell, DArrowLeft, DArrowRight, Close, MoreFilled,
} from '@element-plus/icons-vue'
import * as tapi from '@/api/topo'
import * as mapi from '@/api/model'
import { useUserStore } from '@/store/user'
import RelationPanel from '@/components/RelationPanel.vue'
import ModelIcon from '@/components/ModelIcon.vue'
import { nodeSymbol, catColor, DANGER } from '@/utils/topoSymbol'

const userStore = useUserStore()
const canEdit = computed(() => userStore.hasFunc('topo_manage'))
const router = useRouter()

const NODE_W = 104, NODE_H = 50
const BIZ_W = 152, BIZ_H = 54 // 业务锚点卡片尺寸
const bizNode = ref(null) // 画布上的业务锚点 {id,name,x,y}
const bizDrag = ref(null) // 业务锚点拖动态 {sx,sy,ox,oy,moved}

const businessList = ref([])
const bizId = ref(null)
const businessId = ref(null)
const treeData = ref([])
const summary = ref({})
const edges = ref([]) // 当前业务的连线
const nodes = ref([]) // 当前业务的模型节点（可拖动，含 x/y）
const selectedModule = ref(null)
const moduleHostList = ref([])
const moduleSelected = ref([])
const allModelsList = ref([])
const groupsList = ref([])
const resAttrs = ref([])
const detailTab = ref('base')
const drawer = ref(false)

const modelMeta = computed(() => Object.fromEntries(allModelsList.value.map((m) => [m.id, m])))
const groupNameMap = computed(() => Object.fromEntries(groupsList.value.map((g) => [g.id, g.name])))
const resColumns = computed(() => resAttrs.value.slice(0, 4))
const drawerTitle = computed(() => (selectedModule.value ? `资源管理 - ${selectedModule.value.name}` : '资源管理'))

const businesses = computed(() => treeData.value[0]?.children || [])
const curBiz = computed(() => businessList.value.find((b) => b.id === bizId.value) || null)
const currentBusiness = computed(() => businesses.value.find((b) => b.id === businessId.value) || null)
const businessModules = computed(() => currentBusiness.value?.children || [])
const businessModuleIds = computed(() => new Set(businessModules.value.map((m) => m.id)))
const businessAbnormal = computed(() => (summary.value.abnormal || []).filter((r) => businessModuleIds.value.has(r.module_id)))
const businessAlerts = computed(() => (summary.value.alerts || []).filter((a) => businessModuleIds.value.has(a.module_id)))
const LEVEL_RANK = { critical: 3, warning: 2, info: 1, ok: 0 }
// 每个模块：告警数 + 最高级别
const alertByModule = computed(() => {
  const m = {}
  for (const a of summary.value.alerts || []) {
    const e = m[a.module_id] || { count: 0, rank: -1, level: 'info' }
    e.count++
    const r = LEVEL_RANK[a.level] ?? 0
    if (r >= e.rank) { e.rank = r; e.level = a.level }
    m[a.module_id] = e
  }
  return m
})
function nodeAlertCount(id) { return alertByModule.value[id]?.count || 0 }
function nodeAlertColor(id) { return alertLevel(alertByModule.value[id]?.level).c }
const ALERT_LEVEL = { critical: { t: '紧急', c: '#dc2626' }, warning: { t: '警告', c: '#f59e0b' }, info: { t: '提示', c: '#3b82f6' }, ok: { t: '恢复', c: '#22c55e' } }
function alertLevel(l) { return ALERT_LEVEL[l] || { t: l, c: '#9ca3af' } }
const businessResourceTotal = computed(() =>
  businessModules.value.reduce((a, m) => a + (summary.value.modules?.[m.id]?.total ?? m.host_count ?? 0), 0),
)
const businessGroupDist = computed(() => {
  const map = {}
  for (const m of businessModules.value) {
    const g = groupNameMap.value[modelMeta.value[m.model_id]?.group_id] || '其他'
    const total = summary.value.modules?.[m.id]?.total ?? m.host_count ?? 0
    if (total > 0) map[g] = (map[g] || 0) + total
  }
  return Object.entries(map).map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count)
})

function optionName(a, id) {
  const o = (a.options || []).find((x) => x.id === id)
  return o ? o.name : id
}
function displayValue(a, val) {
  if (val === undefined || val === null || val === '') return '-'
  if (a.type === 'bool') return val ? '是' : '否'
  if (a.type === 'enum') return optionName(a, val)
  if (a.type === 'enummulti') return Array.isArray(val) ? val.map((v) => optionName(a, v)).join('，') : '-'
  return val
}

const assignableModels = computed(() =>
  allModelsList.value.filter((m) => !['biz', 'set', 'module'].includes(m.obj_id)),
)
const assignableModelGroups = computed(() => {
  const byGroup = {}
  for (const m of assignableModels.value) (byGroup[m.group_id] = byGroup[m.group_id] || []).push(m)
  const groups = [...groupsList.value]
    .sort((a, b) => (a.index || 0) - (b.index || 0))
    .map((g) => ({ id: g.id, name: g.name, models: byGroup[g.id] || [] }))
    .filter((g) => g.models.length)
  const known = new Set(groupsList.value.map((g) => g.id))
  const others = assignableModels.value.filter((m) => !known.has(m.group_id))
  if (others.length) groups.push({ id: 0, name: '其它', models: others })
  return groups
})

// 当前业务线下所有模型节点（转移目标用）
const allModules = computed(() => {
  const mods = []
  for (const bus of businesses.value) for (const m of bus.children || []) mods.push({ ...m, _busName: bus.name })
  return mods
})
const transferTargets = computed(() =>
  allModules.value.filter((m) => selectedModule.value && m.id !== selectedModule.value.id && m.model_id === selectedModule.value.model_id),
)
function transferLabel(m) { return `${m._busName} / ${m.name}` }

function resLabel(row) {
  const parts = []
  for (const a of resAttrs.value) {
    const v = row.data?.[a.property_id]
    if (v !== undefined && v !== null && v !== '') {
      parts.push(displayValue(a, v))
      if (parts.length >= 3) break
    }
  }
  return parts.join(' / ') || `#${row.id}`
}

// —— 状态 ——
const STATE_TEXT = { normal: '正常', abnormal: '异常', offline: '离线', empty: '空闲' }
const STATE_COLOR = { normal: '#22c55e', abnormal: '#f59e0b', offline: '#9ca3af', empty: '#c0c4cc' }
function stateText(s) { return STATE_TEXT[s] || '正常' }
function stateColor(s) { return STATE_COLOR[s] || '#22c55e' }
function pillStyle(s) { const c = stateColor(s); return { color: c, background: c + '1f' } }
function moduleStateOf(m) {
  const s = summary.value.modules?.[m.id]
  if (!s || !s.total) return 'empty'
  if (s.abnormal > 0) return 'abnormal'
  if (s.offline > 0) return 'offline'
  return 'normal'
}
const businessState = computed(() => {
  let st = 'normal', any = false
  for (const m of businessModules.value) {
    const ms = moduleStateOf(m)
    if (ms === 'empty') continue
    any = true
    if (ms === 'abnormal') return 'abnormal'
    if (ms === 'offline') st = 'offline'
  }
  return any ? st : 'normal'
})
function moduleCountText(m) {
  const s = summary.value.modules?.[m.id]
  const total = s ? s.total : m.host_count
  return `${total || 0} 个实例`
}

// —— 画布：节点 + 连线 ——
function buildNodes() {
  nodes.value = businessModules.value.map((m) => ({
    id: m.id, name: m.name, model_id: m.model_id, model_name: m.model_name,
    host_count: m.host_count, x: m.pos_x || 0, y: m.pos_y || 0, raw: m,
  }))
  // 业务锚点：有持久化坐标用之，从未摆放(0,0)则自动置于各模型上方居中
  const b = currentBusiness.value
  if (b) {
    let x = b.pos_x || 0, y = b.pos_y || 0
    if (!x && !y) { const p = autoBizPos(); x = p.x; y = p.y }
    bizNode.value = { id: b.id, name: b.name, x, y }
  } else {
    bizNode.value = null
  }
}
// 未摆放过的业务锚点：居中于各模型上方
function autoBizPos() {
  if (!nodes.value.length) return { x: 60, y: 24 }
  let minX = Infinity, maxX = -Infinity, minY = Infinity
  for (const n of nodes.value) {
    minX = Math.min(minX, n.x); maxX = Math.max(maxX, n.x + NODE_W); minY = Math.min(minY, n.y)
  }
  return { x: Math.round((minX + maxX) / 2 - BIZ_W / 2), y: Math.round(minY - 96) }
}
const nodeById = computed(() => Object.fromEntries(nodes.value.map((n) => [n.id, n])))
const contentW = computed(() => {
  let mx = nodes.value.reduce((m, n) => Math.max(m, n.x + NODE_W), 700)
  if (bizNode.value) mx = Math.max(mx, bizNode.value.x + BIZ_W)
  return Math.round(mx + 80)
})
const contentH = computed(() => {
  let my = nodes.value.reduce((m, n) => Math.max(m, n.y + NODE_H), 400)
  if (bizNode.value) my = Math.max(my, bizNode.value.y + BIZ_H)
  return Math.round(my + 80)
})

function nodeSize(n) { return n.id === bizNode.value?.id ? [BIZ_W, BIZ_H] : [NODE_W, NODE_H] }
function nodeCenter(n) {
  const [width, height] = nodeSize(n)
  return { x: n.x + width / 2, y: n.y + height / 2 }
}
// 连线端点落在节点矩形边界上（指向对端中心）
function boundaryPoint(n, toward) {
  const c = nodeCenter(n)
  const dx = toward.x - c.x, dy = toward.y - c.y
  if (dx === 0 && dy === 0) return c
  const [width, height] = nodeSize(n)
  const sx = (width / 2) / Math.abs(dx || 1e-6)
  const sy = (height / 2) / Math.abs(dy || 1e-6)
  const s = Math.min(sx, sy)
  return { x: c.x + dx * s, y: c.y + dy * s }
}
const edgeGeoms = computed(() => {
  const out = []
  for (const e of edges.value) {
    const s = e.src_module_id === bizNode.value?.id ? bizNode.value : nodeById.value[e.src_module_id]
    const d = nodeById.value[e.dst_module_id]
    if (!s || !d) continue
    const p1 = boundaryPoint(s, nodeCenter(d))
    const p2 = boundaryPoint(d, nodeCenter(s))
    const off = Math.max(22, Math.min(90, Math.abs(p2.x - p1.x) * 0.5))
    const path = `M${p1.x},${p1.y} C${p1.x + off},${p1.y} ${p2.x - off},${p2.y} ${p2.x},${p2.y}`
    out.push({ id: e.id, d: path, mx: (p1.x + p2.x) / 2, my: (p1.y + p2.y) / 2 })
  }
  return out
})
// —— 缩放 / 平移 ——
const canvasRef = ref()
const canvasWrapRef = ref()
const zoom = ref(1), panX = ref(0), panY = ref(16)
const dragging = ref(false) // 画布平移中
// will-change 只在拖动/平移时开启：常驻会让缩放图层按原尺寸栅格化再拉伸→文字模糊(hover 才重绘变清晰)
const stageStyle = computed(() => ({
  transform: `translate(${panX.value}px, ${panY.value}px) scale(${zoom.value})`,
  willChange: dragging.value || nodeDrag.value ? 'transform' : 'auto',
}))
function clampZoom(z) { return Math.min(2, Math.max(0.4, z)) }
// 围绕画布内 (cx,cy) 点缩放，保持该点不动（否则围绕原点缩放内容会跑出视野，像没反应）
function zoomAt(newZoom, cx, cy) {
  newZoom = clampZoom(newZoom)
  const z0 = zoom.value
  if (newZoom === z0) return
  const sx = (cx - panX.value) / z0, sy = (cy - panY.value) / z0
  zoom.value = newZoom
  panX.value = cx - sx * newZoom
  panY.value = cy - sy * newZoom
}
function canvasCenter() {
  const el = canvasRef.value
  return el ? { x: el.clientWidth / 2, y: el.clientHeight / 2 } : { x: 0, y: 0 }
}
function zoomIn() { const c = canvasCenter(); zoomAt(zoom.value * 1.2, c.x, c.y) }
function zoomOut() { const c = canvasCenter(); zoomAt(zoom.value / 1.2, c.x, c.y) }
function fit() {
  const el = canvasRef.value
  if (!el || !nodes.value.length) { zoom.value = 1; panX.value = 24; panY.value = 20; return }
  // 支持任意坐标（含负值）：按节点实际包围盒居中，任何位置的节点都能回到视野
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
  for (const n of nodes.value) {
    minX = Math.min(minX, n.x); minY = Math.min(minY, n.y)
    maxX = Math.max(maxX, n.x + NODE_W); maxY = Math.max(maxY, n.y + NODE_H)
  }
  if (bizNode.value) {
    minX = Math.min(minX, bizNode.value.x); minY = Math.min(minY, bizNode.value.y)
    maxX = Math.max(maxX, bizNode.value.x + BIZ_W); maxY = Math.max(maxY, bizNode.value.y + BIZ_H)
  }
  const cw = Math.max(1, maxX - minX), ch = Math.max(1, maxY - minY)
  const vw = el.clientWidth - 60, vh = el.clientHeight - 60
  const z = Math.max(0.4, Math.min(vw / cw, vh / ch, 1.4))
  zoom.value = z
  panX.value = Math.round((el.clientWidth - cw * z) / 2 - minX * z)
  panY.value = Math.round((el.clientHeight - ch * z) / 2 - minY * z)
}
function clientToStage(cx, cy) {
  const r = canvasRef.value.getBoundingClientRect()
  return { x: (cx - r.left - panX.value) / zoom.value, y: (cy - r.top - panY.value) / zoom.value }
}

// 画布平移（点空白拖动）
let panStart = null
function onCanvasDown(e) {
  if (e.target.closest('.topo-card') || e.target.closest('.zoom-ctrl') || e.target.closest('.edge-g')) return
  dragging.value = true
  panStart = { sx: e.clientX, sy: e.clientY, ox: panX.value, oy: panY.value }
}
function onWheel(e) {
  e.preventDefault()
  const r = canvasRef.value.getBoundingClientRect()
  zoomAt(zoom.value * (e.deltaY < 0 ? 1.1 : 0.9), e.clientX - r.left, e.clientY - r.top)
}

// 全屏/最大化：用 CSS 把拓扑卡片铺满视口（不依赖 Fullscreen API，内嵌/预览环境也必定生效）
const maximized = ref(false)
function toggleFull() {
  maximized.value = !maximized.value
  nextTick(() => { fit(); donut && donut.resize(); resizeTree() })
}
// ESC：优先取消正在拉的连线，否则退出最大化
function onKeydown(e) {
  if (e.key !== 'Escape') return
  if (drawing.value) { drawing.value = null; return }
  if (maximized.value) { maximized.value = false; nextTick(() => { fit(); donut && donut.resize(); resizeTree() }) }
}

// 节点拖动 / 连线拖拽
const nodeDrag = ref(null) // {id, sx, sy, ox, oy, moved}
const drawing = ref(null) // {from, sx, sy, x, y}
const hoverNodeId = ref(null)
function onNodeDown(n, e) {
  if (e.button !== 0) return
  if (e.target.closest('.node-ops') || e.target.closest('.conn-handle')) return
  nodeDrag.value = { id: n.id, sx: e.clientX, sy: e.clientY, ox: n.x, oy: n.y, moved: false }
}
function onHandleDown(n, e) {
  if (e.button !== 0) return
  const c = nodeCenter(n)
  drawing.value = { from: n.id, sx: c.x, sy: c.y, x: c.x, y: c.y }
}
function onBizDown(e) {
  if (e.button !== 0 || !bizNode.value) return
  bizDrag.value = { sx: e.clientX, sy: e.clientY, ox: bizNode.value.x, oy: bizNode.value.y, moved: false }
}
function onWinMouseMove(e) {
  if (bizDrag.value && bizNode.value) {
    const dx = (e.clientX - bizDrag.value.sx) / zoom.value
    const dy = (e.clientY - bizDrag.value.sy) / zoom.value
    if (Math.abs(e.clientX - bizDrag.value.sx) + Math.abs(e.clientY - bizDrag.value.sy) > 3) bizDrag.value.moved = true
    bizNode.value = { ...bizNode.value, x: Math.round(bizDrag.value.ox + dx), y: Math.round(bizDrag.value.oy + dy) }
    return
  }
  if (dragging.value && panStart) {
    panX.value = panStart.ox + (e.clientX - panStart.sx)
    panY.value = panStart.oy + (e.clientY - panStart.sy)
    return
  }
  if (nodeDrag.value) {
    const dx = (e.clientX - nodeDrag.value.sx) / zoom.value
    const dy = (e.clientY - nodeDrag.value.sy) / zoom.value
    if (Math.abs(e.clientX - nodeDrag.value.sx) + Math.abs(e.clientY - nodeDrag.value.sy) > 3) nodeDrag.value.moved = true
    const n = nodeById.value[nodeDrag.value.id]
    if (n) { n.x = Math.round(nodeDrag.value.ox + dx); n.y = Math.round(nodeDrag.value.oy + dy) }
    return
  }
  if (drawing.value) {
    const p = clientToStage(e.clientX, e.clientY)
    drawing.value.x = p.x; drawing.value.y = p.y
  }
}
async function onWinMouseUp() {
  if (bizDrag.value) {
    const bd = bizDrag.value
    bizDrag.value = null
    if (bd.moved && bizNode.value) {
      try { await tapi.updateModulePosition(bizNode.value.id, bizNode.value.x, bizNode.value.y) } catch { /* 忽略 */ }
      if (currentBusiness.value) { currentBusiness.value.pos_x = bizNode.value.x; currentBusiness.value.pos_y = bizNode.value.y }
    }
    return
  }
  if (dragging.value) { dragging.value = false; panStart = null }
  if (nodeDrag.value) {
    const n = nodeById.value[nodeDrag.value.id]
    if (nodeDrag.value.moved && n) {
      try { await tapi.updateModulePosition(n.id, n.x, n.y) } catch { /* 忽略 */ }
      // 同步回 tree 原始节点，刷新时不丢位置
      if (n.raw) { n.raw.pos_x = n.x; n.raw.pos_y = n.y }
    } else if (n) {
      openModuleDrawer(n.raw) // 未拖动=点击，打开资源管理
    }
    nodeDrag.value = null
  }
  if (drawing.value) {
    const from = drawing.value.from, to = hoverNodeId.value
    drawing.value = null
    if (to && to !== from) await addEdge(from, to)
  }
}

// —— 搜索 / 过滤 ——
const kw = ref(''), stateFilter = ref('')
function nodeDim(name, state) {
  if (kw.value && !String(name).toLowerCase().includes(kw.value.trim().toLowerCase())) return true
  if (stateFilter.value && state !== stateFilter.value) return true
  return false
}

// —— 模型面板 ——
const paletteKw = ref('')
const paletteOpen = ref(true)
const collapsedGroups = ref(new Set())
const dragModelId = ref(null)
const paletteGroups = computed(() => {
  const k = paletteKw.value.trim().toLowerCase()
  if (!k) return assignableModelGroups.value
  return assignableModelGroups.value
    .map((g) => ({ ...g, models: g.models.filter((m) => m.name.toLowerCase().includes(k) || (m.obj_id || '').toLowerCase().includes(k)) }))
    .filter((g) => g.models.length)
})
function toggleGroup(id) {
  const s = new Set(collapsedGroups.value)
  s.has(id) ? s.delete(id) : s.add(id)
  collapsedGroups.value = s
}
function setAllGroups(collapsed) {
  collapsedGroups.value = collapsed ? new Set(paletteGroups.value.map((g) => g.id)) : new Set()
}
function onPaletteDragStart(m, e) {
  dragModelId.value = m.id
  if (e.dataTransfer) { e.dataTransfer.setData('text/plain', String(m.id)); e.dataTransfer.effectAllowed = 'copy' }
}
async function onCanvasDrop(e) {
  const mid = dragModelId.value
  dragModelId.value = null
  if (!canEdit.value || !mid || !currentBusiness.value) return
  const p = clientToStage(e.clientX, e.clientY)
  // 以光标为节点左上角附近落点（整块落在光标右下，靠近左/上缘也不会被裁）
  await tapi.createModule(currentBusiness.value.id, mid, '', Math.round(p.x - 18), Math.round(p.y - 18))
  await refreshTree()
  buildNodes()
  ElMessage.success(`已添加「${modelMeta.value[mid]?.name || '资源'}」节点`)
}

// —— 连线增删 ——
async function loadEdges() {
  if (!currentBusiness.value) { edges.value = []; return }
  try { edges.value = await tapi.listEdges(currentBusiness.value.id) } catch { edges.value = [] }
}
async function addEdge(src, dst) {
  if (!canEdit.value || !currentBusiness.value) return
  try {
    await tapi.createEdge(currentBusiness.value.id, src, dst)
    await loadEdges()
  } catch (e) { /* 校验错误已由拦截器提示 */ }
}
async function removeEdge(id) {
  if (!canEdit.value) return
  await tapi.deleteEdge(id)
  await loadEdges()
}

// —— 环形图 ——
const donutRef = ref()
let donut = null
const PALETTE = ['#3b82f6', '#22c55e', '#f59e0b', '#a855f7', '#94a3b8', '#06b6d4', '#ec4899', '#14b8a6', '#f97316', '#64748b']
function renderDonut() {
  const dist = businessGroupDist.value
  if (!donutRef.value || !dist.length) return
  if (donut && donut.getDom() !== donutRef.value) { donut.dispose(); donut = null }
  if (!donut) donut = echarts.init(donutRef.value)
  const total = dist.reduce((a, b) => a + b.count, 0)
  donut.setOption({
    tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
    legend: {
      orient: 'vertical', right: 4, top: 'center', icon: 'circle', itemWidth: 8, itemHeight: 8, itemGap: 10,
      textStyle: { fontSize: 12, color: '#5b6b7f' },
      formatter: (name) => {
        const it = dist.find((d) => d.name === name)
        const pct = total ? Math.round((it.count / total) * 100) : 0
        return `${name}  ${pct}%`
      },
    },
    series: [{
      type: 'pie', radius: ['60%', '82%'], center: ['32%', '50%'], avoidLabelOverlap: true,
      label: { show: false }, labelLine: { show: false },
      data: dist.map((d, i) => ({ name: d.name, value: d.count, itemStyle: { color: PALETTE[i % PALETTE.length] } })),
    }],
  })
  nextTick(() => donut && donut.resize())
}

// —— 树视图（只读折叠树）——
// 与画布互为切换：保留画布编辑能力，另提供一个自上而下、按连线层级的折叠树，
// 节点样式与关联拓扑(RelationPanel)完全一致（同一套 nodeSymbol / 配色 / 异常红标）。
const viewMode = ref('canvas') // 'canvas' | 'tree'
const treeEl = ref()
let tree = null
const hiddenTreeModels = ref(new Set()) // 图例隐藏的模型类型 model_id
// 图例：每个模型类型一个「色块 + 名称」，颜色与树节点一致，可点击隐藏
const treeLegend = computed(() => {
  const seen = new Map()
  for (const m of businessModules.value) {
    if (seen.has(m.model_id)) continue
    const meta = modelMeta.value[m.model_id] || {}
    seen.set(m.model_id, { id: m.model_id, name: m.model_name || meta.name || '模型', color: catColor(meta.obj_id || m.model_name) })
  }
  return [...seen.values()]
})
function toggleTreeModel(id) {
  const s = new Set(hiddenTreeModels.value)
  s.has(id) ? s.delete(id) : s.add(id)
  hiddenTreeModels.value = s
  renderTree()
}
// 状态排序权重：异常 > 离线 > 其它（同层节点异常优先靠前）
function stateRank(st) { return st === 'abnormal' ? 2 : st === 'offline' ? 1 : 0 }
// 单个模型节点 → ECharts tree 节点
function treeModuleNode(m, kids) {
  const meta = modelMeta.value[m.model_id] || {}
  const st = moduleStateOf(m)
  const abn = st === 'abnormal'
  const color = catColor(meta.obj_id || m.model_name || m.name)
  const total = summary.value.modules?.[m.id]?.total ?? m.host_count ?? 0
  return {
    name: m.name,
    symbol: nodeSymbol(meta.obj_id, meta.icon, color, abn),
    symbolSize: kids.length ? 40 : 34,
    label: { color: abn ? DANGER : st === 'offline' ? '#94a3b8' : '#334155', fontWeight: abn ? 600 : 500 },
    _module: m,
    _state: st,
    _total: total,
    _alerts: nodeAlertCount(m.id),
    _modelName: m.model_name,
    children: kids,
  }
}
// 按连线(src→dst)把模型组织成层级：无上游(入度0)的作顶层，沿连线向下展开；
// 隐藏的模型类型 / 被搜索过滤掉的节点先剔除；环用 visited 兜底防止无限递归。
function buildTreeChildren() {
  const mods = businessModules.value.filter(
    (m) => !hiddenTreeModels.value.has(m.model_id) && !nodeDim(m.name, moduleStateOf(m)),
  )
  const byId = new Map(mods.map((m) => [m.id, m]))
  const childrenOf = new Map()
  const businessLinks = new Set()
  const indeg = new Map(mods.map((m) => [m.id, 0]))
  for (const e of edges.value) {
    if (!byId.has(e.dst_module_id)) continue
    if (e.src_module_id === currentBusiness.value?.id) {
      businessLinks.add(e.dst_module_id)
      continue
    }
    if (!byId.has(e.src_module_id)) continue
    if (!childrenOf.has(e.src_module_id)) childrenOf.set(e.src_module_id, [])
    childrenOf.get(e.src_module_id).push(e.dst_module_id)
    indeg.set(e.dst_module_id, (indeg.get(e.dst_module_id) || 0) + 1)
  }
  const visited = new Set()
  const build = (mid) => {
    visited.add(mid)
    const kids = (childrenOf.get(mid) || [])
      .filter((c) => !visited.has(c) && !businessLinks.has(c))
      .map((c) => byId.get(c))
      .sort(order)
      .map((c) => build(c.id))
    return treeModuleNode(byId.get(mid), kids)
  }
  const order = (a, b) =>
    stateRank(moduleStateOf(b)) - stateRank(moduleStateOf(a)) || String(a.name).localeCompare(String(b.name))
  const roots = mods.filter((m) => businessLinks.has(m.id) || (indeg.get(m.id) || 0) === 0).sort(order)
  const children = []
  const appendRoot = (id) => {
    const node = build(id)
    if (!businessLinks.has(id)) node.lineStyle = { opacity: 0 }
    children.push(node)
  }
  for (const m of roots) if (!visited.has(m.id)) appendRoot(m.id)
  for (const m of mods) if (!visited.has(m.id)) appendRoot(m.id) // 环中节点兜底
  return children
}
function treeTooltip(p) {
  const d = p && p.data
  if (!d) return ''
  if (!d._module) return `<b>${d.name}</b>`
  const stTxt = { normal: '正常', abnormal: '异常', offline: '离线', empty: '空闲' }[d._state] || '正常'
  const stColor = d._state === 'abnormal' ? DANGER : d._state === 'offline' ? '#94a3b8' : '#22c55e'
  const lines = [`<b>${d.name}</b>`]
  if (d._modelName) lines.push(`类型：${d._modelName}`)
  lines.push(`实例：${d._total}`)
  lines.push(`状态：<span style="color:${stColor}">● ${stTxt}</span>`)
  if (d._alerts) lines.push(`<span style="color:${DANGER}">告警 ${d._alerts} 条</span>`)
  return lines.join('<br/>')
}
function renderTree() {
  if (viewMode.value !== 'tree' || !treeEl.value) return
  if (!businessModules.value.length) { if (tree) tree.clear(); return }
  if (!tree) tree = echarts.init(treeEl.value)
  const root = {
    name: currentBusiness.value?.name || '业务',
    symbol: nodeSymbol('biz', '', '#2f66f0', false),
    symbolSize: 52,
    label: { color: '#1e293b', fontWeight: 700 },
    children: buildTreeChildren(),
  }
  tree.setOption(
    {
      tooltip: { trigger: 'item', formatter: treeTooltip, confine: true, borderColor: '#e2e8f0' },
      series: [
        {
          type: 'tree',
          orient: 'TB',
          roam: false, // 始终自动适配容器，避免「进去就遮挡、要手动缩小才看全」
          top: '10%',
          left: '8%',
          right: '8%',
          bottom: '14%',
          symbolSize: 40,
          initialTreeDepth: 2, // 默认展开到：业务 → 顶层模型 → 其直接下游（更深层折叠）
          expandAndCollapse: true,
          animationDurationUpdate: 450,
          edgeShape: 'curve',
          lineStyle: { color: '#cbd5e1', width: 1.4 },
          label: {
            position: 'top', verticalAlign: 'bottom', align: 'center', fontSize: 12,
            color: '#475569', distance: 6, width: 110, overflow: 'truncate', ellipsis: '…',
            formatter: (p) => p.data.name,
          },
          leaves: { label: { position: 'bottom', verticalAlign: 'top', align: 'center' } },
          // 真实重叠检测：只隐藏实际糊在一起的标签（随折叠/展开自动生效），hover 时再内联显示
          labelLayout: { hideOverlap: true },
          // hover 高亮时强制显示该节点标签（即便被 hideOverlap 隐藏）
          emphasis: { focus: 'relative', label: { show: true, overflow: 'break', width: 150 } },
          data: [root],
        },
      ],
    },
    true,
  )
  tree.off('click')
  tree.on('click', (p) => { if (p.data && p.data._module) openModuleDrawer(p.data._module) }) // 点节点 → 资源管理抽屉
  const doFit = () => tree && tree.resize()
  nextTick(doFit); requestAnimationFrame(doFit); setTimeout(doFit, 180)
}
function resizeTree() { tree && tree.resize() }
// 切到树视图时渲染并按容器真实尺寸自适配；切回画布时重算画布
watch(viewMode, (v) => {
  if (v === 'tree') nextTick(renderTree)
  else nextTick(fit)
})
// 树视图下数据/搜索/过滤变化即重画（画布模式下不动，省开销）
watch([businessModules, edges, () => summary.value, kw, stateFilter], () => {
  if (viewMode.value === 'tree') nextTick(renderTree)
})

// —— 数据加载 ——
async function loadBusiness() { businessList.value = await tapi.listBusiness() }
async function refreshTree() {
  if (!bizId.value) { treeData.value = []; summary.value = {}; return }
  const [tree, sum] = await Promise.all([tapi.getTree(bizId.value), tapi.getSummary(bizId.value)])
  treeData.value = [tree]
  summary.value = sum || {}
  if (!businesses.value.find((b) => b.id === businessId.value)) {
    businessId.value = businesses.value[0]?.id || null
  }
}
async function afterBusinessData() {
  hiddenTreeModels.value = new Set() // 换业务时重置树视图图例隐藏
  buildNodes()
  await loadEdges()
  await nextTick()
  renderDonut()
  fit()
  if (viewMode.value === 'tree') renderTree()
}
async function refreshAll() {
  await loadBusiness()
  await refreshTree()
  await afterBusinessData()
}
async function onBizChange() {
  selectedModule.value = null
  moduleHostList.value = []
  drawer.value = false
  businessId.value = null
  await refreshTree()
  await afterBusinessData()
}
async function onBusinessChange() {
  drawer.value = false
  await afterBusinessData()
}
onMounted(async () => {
  ;[allModelsList.value, groupsList.value] = await Promise.all([mapi.listModels(), mapi.listGroups()])
  await loadBusiness()
  if (businessList.value.length) {
    bizId.value = businessList.value[0].id
    await refreshTree()
    await afterBusinessData()
  }
  window.addEventListener('mousemove', onWinMouseMove)
  window.addEventListener('mouseup', onWinMouseUp)
  window.addEventListener('resize', onResize)
  window.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  window.removeEventListener('mousemove', onWinMouseMove)
  window.removeEventListener('mouseup', onWinMouseUp)
  window.removeEventListener('resize', onResize)
  window.removeEventListener('keydown', onKeydown)
  donut && donut.dispose()
  tree && tree.dispose()
})
function onResize() { donut && donut.resize(); resizeTree() }
watch(businessGroupDist, () => nextTick(renderDonut))

// —— 模块抽屉 ——
async function openModuleDrawer(m) {
  selectedModule.value = m
  moduleSelected.value = []
  const all = m.model_id ? await mapi.listAttributes(m.model_id) : []
  resAttrs.value = all.filter((a) => a.enabled !== false).sort((x, y) => (x.index || 0) - (y.index || 0))
  moduleHostList.value = await tapi.moduleHosts(m.id)
  drawer.value = true
}

// —— 业务线 CRUD ——
const bizDialog = ref(false)
const bizForm = ref({ id: null, name: '', owner: '', contact: '' })
const fileInput = ref(null)
function newBiz() { bizForm.value = { id: null, name: '', owner: '', contact: '' }; bizDialog.value = true }
function editBiz() {
  const b = curBiz.value
  if (!b) return
  bizForm.value = { id: b.id, name: b.name, owner: b.owner || '', contact: b.contact || '' }
  bizDialog.value = true
}
async function submitBiz() {
  const f = bizForm.value
  if (!f.name.trim()) { ElMessage.warning('名称不能为空'); return }
  const payload = { name: f.name.trim(), owner: f.owner.trim(), contact: f.contact.trim() }
  if (f.id) {
    await tapi.updateBiz(f.id, payload); await loadBusiness(); ElMessage.success('已保存')
  } else {
    const biz = await tapi.createBiz(payload); await loadBusiness(); bizId.value = biz.id; await onBizChange(); ElMessage.success('已创建')
  }
  bizDialog.value = false
}
async function delBiz() {
  await ElMessageBox.confirm('确定删除该业务线及其全部业务/模型/连线？资源将退回资源池。', '提示', { type: 'warning' })
  await tapi.deleteBiz(bizId.value); await loadBusiness(); bizId.value = businessList.value[0]?.id || null; await onBizChange(); ElMessage.success('已删除')
}
function onBizCmd(cmd) {
  if (cmd === 'edit') editBiz()
  else if (cmd === 'delete') delBiz()
  else if (cmd === 'export') doExport()
  else if (cmd === 'template') doTemplate()
  else if (cmd === 'import') fileInput.value && fileInput.value.click()
}

// —— 业务 CRUD ——
function onBusinessCmd(cmd) {
  if (cmd === 'new') newBusiness()
  else if (cmd === 'edit') renameBusiness()
  else if (cmd === 'delete') delBusiness()
}
async function newBusiness() {
  if (!bizId.value) { ElMessage.warning('请先选择业务线'); return }
  const { value } = await ElMessageBox.prompt('请输入业务名称', '新建业务', { inputPattern: /.+/, inputErrorMessage: '名称不能为空' })
  const n = await tapi.createBusiness(bizId.value, value)
  await refreshTree(); businessId.value = n.id; await afterBusinessData(); ElMessage.success('已创建')
}
async function renameBusiness() {
  if (!currentBusiness.value) return
  const { value } = await ElMessageBox.prompt('请输入业务名称', '重命名业务', { inputPattern: /.+/, inputErrorMessage: '名称不能为空', inputValue: currentBusiness.value.name })
  await tapi.updateBusiness(currentBusiness.value.id, value); await refreshTree(); ElMessage.success('已保存')
}
async function delBusiness() {
  if (!currentBusiness.value) return
  await ElMessageBox.confirm(`确定删除业务「${currentBusiness.value.name}」及其下模型/连线？资源将退回资源池。`, '提示', { type: 'warning' })
  await tapi.deleteBusiness(currentBusiness.value.id); businessId.value = null; await refreshTree(); await afterBusinessData(); ElMessage.success('已删除')
}

// 导入 / 导出
function onFilePick(e) {
  const file = e.target.files && e.target.files[0]
  if (file) doImport({ file })
  e.target.value = ''
}
async function doExport() {
  if (!bizId.value) { ElMessage.warning('请先选择业务线'); return }
  const blob = await tapi.exportTopo(bizId.value)
  const name = curBiz.value?.name || '业务拓扑'
  const url = window.URL.createObjectURL(blob)
  const a = document.createElement('a'); a.href = url; a.download = `${name}_拓扑_${Date.now()}.xlsx`; a.click()
  window.URL.revokeObjectURL(url)
}
async function doTemplate() {
  const blob = await tapi.templateTopo()
  const url = window.URL.createObjectURL(blob)
  const a = document.createElement('a'); a.href = url; a.download = '业务拓扑导入模板.xlsx'; a.click()
  window.URL.revokeObjectURL(url)
}
async function doImport({ file }) {
  const fd = new FormData(); fd.append('file', file)
  const report = await tapi.importTopo(fd)
  await refreshAll()
  if (report.failed) {
    ElMessageBox.alert(`成功 ${report.success} 条，失败 ${report.failed} 条：\n` + (report.errors || []).join('\n'), '导入结果', { type: 'warning' })
  } else {
    ElMessage.success(`导入完成，成功 ${report.success} 条`)
  }
}

// —— 模型节点编辑 / 删除 ——
const editModuleDialog = ref(false)
const editModuleForm = ref({ id: null, name: '', modelName: '' })
function openEditModule(data) {
  editModuleForm.value = { id: data.id, name: data.name, modelName: data.model_name }
  editModuleDialog.value = true
}
async function saveEditModule() {
  const name = (editModuleForm.value.name || '').trim()
  if (!name) { ElMessage.warning('名称不能为空'); return }
  await tapi.updateModule(editModuleForm.value.id, name)
  if (selectedModule.value && selectedModule.value.id === editModuleForm.value.id) selectedModule.value = { ...selectedModule.value, name }
  editModuleDialog.value = false
  await refreshTree(); buildNodes(); ElMessage.success('已保存')
}
async function delModule(data) {
  await ElMessageBox.confirm(`确定删除模型节点「${data.name}」？其下资源将退回资源池，相关连线一并删除。`, '提示', { type: 'warning' })
  await tapi.deleteModule(data.id)
  if (selectedModule.value && selectedModule.value.id === data.id) { selectedModule.value = null; moduleHostList.value = []; drawer.value = false }
  await refreshTree(); buildNodes(); await loadEdges(); ElMessage.success('已删除')
}

// —— 分配 / 转移 / 回收 ——
const assignDialog = ref(false)
const idleList = ref([])
const idleSelected = ref([])
const assignKeyword = ref('')
const filteredIdleList = computed(() => {
  const k = assignKeyword.value.trim().toLowerCase()
  if (!k) return idleList.value
  return idleList.value.filter((row) => {
    const cols = resColumns.value
    const text = cols.length ? cols.map((a) => displayValue(a, row.data?.[a.property_id])).join(' ') : resLabel(row)
    return text.toLowerCase().includes(k)
  })
})
async function openAssign() {
  const res = await tapi.idleHosts({ model_id: selectedModule.value.model_id, page: 1, page_size: 200 })
  idleList.value = res.list || []
  idleSelected.value = []
  assignKeyword.value = ''
  assignDialog.value = true
}
async function doAssign() {
  if (!idleSelected.value.length) { ElMessage.warning('请选择资源'); return }
  await tapi.assignHosts(selectedModule.value.id, idleSelected.value.map((h) => h.id))
  ElMessage.success('分配成功'); assignDialog.value = false
  await openModuleDrawer(selectedModule.value); await refreshTree(); buildNodes()
}
const transferDialog = ref(false)
const transferTarget = ref(null)
function openTransfer() { transferTarget.value = null; transferDialog.value = true }
async function doTransfer() {
  if (!transferTarget.value) { ElMessage.warning('请选择目标模型'); return }
  await tapi.transferHosts(transferTarget.value, moduleSelected.value.map((h) => h.id))
  ElMessage.success('转移成功'); transferDialog.value = false
  await openModuleDrawer(selectedModule.value); await refreshTree(); buildNodes()
}
async function doRecycle() {
  await ElMessageBox.confirm('确定将选中资源移回资源池？', '提示', { type: 'warning' })
  await tapi.recycleHosts(moduleSelected.value.map((h) => h.id))
  ElMessage.success('已回收')
  await openModuleDrawer(selectedModule.value); await refreshTree(); buildNodes()
}

// —— 资源关联关系与拓扑 ——
const relDialog = ref(false)
const relHostId = ref(null)
const relLabel = ref('')
const relModelName = ref('资源')
function openHostRel(row) {
  relHostId.value = row.id
  relLabel.value = resLabel(row)
  relModelName.value = selectedModule.value?.model_name || '资源'
  relDialog.value = true
}
function goDetail(id) { router.push(`/instances/${id}`) }
</script>

<style scoped>
.topo-page { display: flex; flex-direction: column; gap: 12px; }
/* 去标题后的纯工具栏：左=选择器，右=操作 */
.topo-toolbar { margin: 0 !important; min-height: 0; align-items: center; }
.tb-left { display: flex; align-items: center; gap: 8px; }
.tb-label { font-size: 13px; color: var(--text-secondary); font-weight: 500; }
.tb-sep { color: var(--text-muted); font-size: 13px; }
.tb-more { padding: 6px; color: var(--text-secondary); }
.tb-more:hover { color: var(--brand); }
.tb-right { display: flex; align-items: center; gap: 10px; margin-left: 0; }
.readonly-hint { color: var(--text-secondary); font-size: 13px; align-self: center; }

/* 统计卡片 */
.stat-row { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; }
.stat-card { display: flex; align-items: center; gap: 14px; background: var(--card-bg); border: 1px solid var(--card-border); border-radius: var(--radius-lg); box-shadow: var(--shadow-card); padding: 18px 20px; }
.stat-ic { width: 48px; height: 48px; border-radius: 12px; display: flex; align-items: center; justify-content: center; font-size: 24px; flex-shrink: 0; }
.ic-blue { background: #eaf1ff; color: #3b82f6; }
.ic-green { background: #e8f8ee; color: #22c55e; }
.ic-cyan { background: #e6f6fb; color: #0ea5e9; }
.ic-orange { background: #fff2e6; color: #f59e0b; }
.stat-label { font-size: 13px; color: var(--text-secondary); margin-bottom: 4px; }
.stat-val { font-size: 26px; font-weight: 700; color: var(--text-primary); line-height: 1; }
.stat-val.danger { color: #f59e0b; }

/* 画布卡片 */
.graph-card { background: var(--card-bg); border: 1px solid var(--card-border); border-radius: var(--radius-lg); box-shadow: var(--shadow-card); overflow: hidden; }
.graph-card.maximized { position: fixed; inset: 0; z-index: 2000; border-radius: 0; display: flex; flex-direction: column; }
.graph-card.maximized .graph-body { flex: 1; height: auto; min-height: 0; }
.graph-toolbar { display: flex; align-items: center; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--border-light); }
.g-crumb { display: inline-flex; align-items: center; gap: 4px; font-size: 13px; font-weight: 600; color: var(--text-regular); }
.g-crumb .el-icon { color: var(--brand); }
.g-crumb .sep { color: var(--text-muted); margin: 0 2px; }
.g-search { width: 190px; margin-left: 8px; }
.g-filter { width: 110px; }
.g-tip { font-size: 12px; color: var(--text-muted); }
.g-legend { margin-left: auto; display: flex; gap: 16px; font-size: 13px; color: var(--text-secondary); }
.g-legend span { display: inline-flex; align-items: center; gap: 6px; }
.g-legend .dot { width: 8px; height: 8px; border-radius: 50%; display: inline-block; }
/* 视图切换：画布 / 树视图 */
.g-view { display: inline-flex; border: 1px solid var(--border-light); border-radius: 8px; overflow: hidden; flex-shrink: 0; }
.g-view span { padding: 4px 12px; font-size: 13px; color: var(--text-secondary); cursor: pointer; user-select: none; background: var(--card-bg); transition: background 0.12s, color 0.12s; }
.g-view span + span { border-left: 1px solid var(--border-light); }
.g-view span:hover { color: var(--brand); }
.g-view span.active { background: #2468f2; color: #fff; }
/* 树视图模型图例（复用关联拓扑风格，可点击隐藏/显示） */
.g-legend.rel-legend { flex-wrap: wrap; justify-content: flex-end; gap: 6px 14px; max-width: 48%; }
.rel-legend-item { display: inline-flex; align-items: center; font-size: 13px; color: var(--text-secondary); cursor: pointer; user-select: none; }
.rel-legend-item.off { opacity: 0.4; text-decoration: line-through; }
.rel-sw { width: 10px; height: 10px; border-radius: 3px; margin-right: 5px; flex-shrink: 0; }
/* 树视图画布 */
.tree-view { position: relative; flex: 1; min-width: 0; height: 100%; overflow: hidden; background: linear-gradient(180deg, #fbfcfe, #f6f8fb); }
.tree-view :deep(.el-empty) { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; }
.tree-canvas { width: 100%; height: 100%; }

/* 模型面板 + 画布并排：行高固定，面板/画布都填满，面板内部滚动（展开再多也不撑高行） */
.graph-body { display: flex; align-items: stretch; height: calc(100vh - 355px); min-height: 380px; }
.palette { width: 208px; flex-shrink: 0; min-height: 0; border-right: 1px solid var(--border-light); display: flex; flex-direction: column; background: #fbfcfe; transition: width 0.18s; }
.palette.collapsed { width: 38px; }
.palette-head { display: flex; align-items: flex-start; justify-content: space-between; padding: 12px 10px 6px 14px; }
.ph-title2 { font-size: 13px; font-weight: 700; color: var(--text-primary); }
.ph-hint { display: block; font-size: 11px; font-weight: 400; color: var(--text-muted); margin-top: 2px; }
.ph-toggle { cursor: pointer; color: var(--text-muted); font-size: 15px; flex-shrink: 0; }
.ph-toggle:hover { color: var(--brand); }
.palette-search { padding: 0 12px 6px; }
.palette-tools { display: flex; gap: 12px; padding: 0 14px 6px; font-size: 11px; color: var(--brand); }
.palette-tools span { cursor: pointer; }
.palette-tools span:hover { text-decoration: underline; }
.palette-body { flex: 1; min-height: 0; padding: 0 8px 10px; }
.pg { margin-bottom: 6px; }
.pg-name { display: flex; align-items: center; gap: 4px; font-size: 11px; color: var(--text-secondary); padding: 6px 6px 4px; font-weight: 600; cursor: pointer; user-select: none; }
.pg-name:hover { color: var(--text-regular); }
.pg-caret { font-size: 11px; transition: transform 0.15s; transform: rotate(90deg); }
.pg-caret.folded { transform: rotate(0deg); }
.pg-count { margin-left: auto; font-weight: 400; color: var(--text-muted); }
.pg-item { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-radius: 8px; cursor: grab; font-size: 13px; color: var(--text-regular); border: 1px solid transparent; user-select: none; }
.pg-item:hover { background: #eef4ff; border-color: #dbe6fb; }
.pg-item:active { cursor: grabbing; }
.pg-label { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.palette-mini { flex: 1; display: flex; flex-direction: column; align-items: center; gap: 8px; padding-top: 12px; cursor: pointer; color: var(--text-secondary); }
.palette-mini:hover { color: var(--brand); }
.pm-label { writing-mode: vertical-rl; font-size: 12px; letter-spacing: 2px; }

.canvas {
  position: relative; flex: 1; min-width: 0; height: 100%; overflow: hidden;
  background: radial-gradient(circle, #e7ecf3 1px, transparent 1px) 0 0 / 22px 22px, linear-gradient(180deg, #fbfcfe, #f6f8fb);
  cursor: grab;
}
.canvas.grabbing { cursor: grabbing; }
.canvas :deep(.el-empty) { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; }
.canvas-hint { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; color: var(--text-muted); font-size: 14px; pointer-events: none; }
.stage { position: absolute; top: 0; left: 0; transform-origin: 0 0; }
.edges { position: absolute; top: 0; left: 0; overflow: visible; }
.edge-line { stroke: #a7b8d0; stroke-width: 1.6; fill: none; stroke-linecap: round; transition: stroke 0.15s, stroke-width 0.15s; }
.edge-hit { stroke: transparent; stroke-width: 16; fill: none; cursor: pointer; }
.edge-temp { stroke: #2468f2; stroke-width: 2; stroke-dasharray: 5 4; fill: none; stroke-linecap: round; }
.edge-del { display: none; cursor: pointer; }
.edge-del circle { fill: #fff; stroke: #e3e9f2; }
.edge-del path { stroke: #ef4444; stroke-width: 1.6; }
.edge-g:hover .edge-del { display: block; }
.edge-g:hover .edge-line { stroke: #2468f2; stroke-width: 2.2; }

/* 节点卡片 */
.topo-card { position: absolute; display: flex; align-items: center; gap: 7px; background: var(--card-bg); border: 1px solid #e5eaf2; border-radius: 10px; padding: 0 9px 0 11px; box-shadow: 0 1px 3px rgba(31, 45, 61, 0.08), 0 2px 10px rgba(31, 45, 61, 0.05); transition: box-shadow 0.15s, border-color 0.15s, transform 0.12s, opacity 0.2s; cursor: grab; }
.topo-card.dragging { cursor: grabbing; box-shadow: 0 10px 24px rgba(36, 104, 242, 0.22); z-index: 5; }
.topo-card.dim { opacity: 0.26; }
.topo-card.conn-target { border-color: #2468f2; box-shadow: 0 0 0 3px rgba(36, 104, 242, 0.2); }
.st-bar { position: absolute; left: 0; top: 0; bottom: 0; width: 3px; border-radius: 10px 0 0 10px; }
.topo-card .card-ic { width: 28px; height: 28px; border-radius: 7px; display: flex; align-items: center; justify-content: center; font-size: 16px; flex-shrink: 0; }
.mod-ic { background: #f1f5fb; color: #5b6b7f; }
/* 业务锚点卡片 */
.card-biz { background: linear-gradient(135deg, #eef4ff, #ffffff); border-color: #c3d6fb; cursor: grab; }
.card-biz.dragging { cursor: grabbing; box-shadow: 0 10px 24px rgba(36, 104, 242, 0.22); z-index: 5; }
.card-biz:hover { border-color: #93b4fb; box-shadow: 0 4px 16px rgba(36, 104, 242, 0.16); transform: translateY(-1px); }
.card-biz .card-name { font-weight: 700; }
.biz-ic { background: #2468f2; color: #fff; }
.card-main { flex: 1; min-width: 0; }
.card-name { font-size: 12.5px; font-weight: 600; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; line-height: 1.25; }
.card-sub { font-size: 10.5px; color: var(--text-secondary); margin-top: 1px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.st-dot { width: 9px; height: 9px; border-radius: 50%; flex-shrink: 0; }
.card-mod:hover { border-color: #93b4fb; box-shadow: 0 4px 16px rgba(36, 104, 242, 0.16); transform: translateY(-1px); }
.card-ops { position: absolute; top: -12px; right: 8px; display: none; gap: 2px; background: var(--card-bg); border: 1px solid #e3e9f2; border-radius: 8px; padding: 1px 4px; box-shadow: 0 2px 8px rgba(31, 45, 61, 0.12); }
.topo-card:hover .card-ops { display: flex; }
.card-ops .el-button { padding: 2px 4px; height: auto; }
.node-alert-badge { position: absolute; top: -7px; right: -7px; min-width: 17px; height: 17px; padding: 0 4px; border-radius: 9px; background: #dc2626; color: #fff; font-size: 11px; font-weight: 700; line-height: 17px; text-align: center; border: 2px solid #fff; box-shadow: 0 1px 3px rgba(220, 38, 38, 0.4); z-index: 3; }
.conn-handle { position: absolute; right: -6px; top: 50%; width: 12px; height: 12px; margin-top: -6px; border-radius: 50%; background: #2468f2; border: 2px solid #fff; cursor: crosshair; display: none; box-shadow: 0 1px 3px rgba(0,0,0,0.2); }
.topo-card:hover .conn-handle { display: block; }

/* 缩放控制：连体药丸 */
.zoom-ctrl { position: absolute; top: 14px; right: 14px; display: flex; flex-direction: column; background: var(--card-bg); border: 1px solid #e5eaf2; border-radius: 10px; box-shadow: 0 2px 10px rgba(31, 45, 61, 0.1); overflow: hidden; }
.zoom-ctrl button { width: 34px; height: 32px; border: none; border-bottom: 1px solid #eef1f6; background: var(--card-bg); display: flex; align-items: center; justify-content: center; cursor: pointer; color: #5b6b7f; font-size: 15px; transition: background 0.12s, color 0.12s; }
.zoom-ctrl button:last-child { border-bottom: none; }
.zoom-ctrl button:hover { background: #f0f5ff; color: #2468f2; }

/* 底部信息区 */
.info-row { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 14px; }
.info-card { background: var(--card-bg); border: 1px solid var(--card-border); border-radius: var(--radius-lg); box-shadow: var(--shadow-card); padding: 16px 18px; min-height: 290px; }
.info-head { display: flex; align-items: center; gap: 6px; font-size: 15px; font-weight: 700; color: var(--text-primary); margin-bottom: 10px; }
.info-head > span:first-child { display: inline-flex; align-items: center; gap: 6px; }
.info-head .el-icon { color: #2468f2; }
.ab-count { margin-left: auto; background: #fff2e6; color: #f59e0b; font-size: 12px; padding: 1px 8px; border-radius: 10px; }
.info-tabs :deep(.el-tabs__header) { margin-bottom: 12px; }
.kv { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px dashed var(--border-light); font-size: 13px; }
.kv:last-child { border-bottom: none; }
.kv .k { color: var(--text-secondary); }
.kv .v { color: var(--text-primary); font-weight: 500; }
.res-line { display: flex; align-items: center; gap: 8px; padding: 8px 6px; border-radius: 8px; cursor: pointer; font-size: 13px; }
.res-line:hover { background: #f5f8fe; }
.res-name { font-weight: 500; color: var(--text-primary); }
.res-model { color: var(--text-secondary); font-size: 12px; }
.res-cnt { margin-left: auto; color: var(--text-regular); font-variant-numeric: tabular-nums; }
.donut-wrap { height: 220px; }
.donut { width: 100%; height: 100%; }
.al-sub-head { font-size: 11px; font-weight: 600; color: var(--text-muted); padding: 8px 6px 4px; letter-spacing: 0.5px; }
.alert-line { display: flex; align-items: center; gap: 10px; padding: 10px 6px; border-radius: 8px; cursor: pointer; border-bottom: 1px solid var(--border-light); }
.alert-line:last-child { border-bottom: none; }
.alert-line:hover { background: #f5f8fe; }
.al-ic { width: 26px; height: 26px; border-radius: 7px; display: flex; align-items: center; justify-content: center; flex-shrink: 0; font-size: 15px; }
.al-orange { background: #fff2e6; color: #f59e0b; }
.al-grey { background: #eef0f3; color: #9ca3af; }
.al-body { flex: 1; min-width: 0; }
.al-title { font-size: 13px; font-weight: 500; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.al-sub { font-size: 12px; color: var(--text-secondary); margin-top: 2px; }
.al-arrow { color: var(--text-muted); flex-shrink: 0; }

/* 抽屉内管理 */
.mod-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; padding-bottom: 12px; border-bottom: 1px solid var(--border-light); }
.mod-name { font-size: 16px; font-weight: 700; color: var(--text-primary); display: flex; align-items: center; gap: 8px; }

/* —— 深色模式：画布/侧边面板/浮层由浅转深 —— */
html.dark .palette { background: #141a24; }
html.dark .canvas {
  background: radial-gradient(circle, #2a323f 1px, transparent 1px) 0 0 / 22px 22px,
    linear-gradient(180deg, #0f141c, #121926);
}
html.dark .tree-view { background: linear-gradient(180deg, #0f141c, #121926); }
html.dark .g-view span { background: #141a24; }
html.dark .g-view span.active { background: #2468f2; color: #fff; }
html.dark .topo-card { border-color: #2c3342; box-shadow: 0 1px 3px rgba(0, 0, 0, 0.4); }
html.dark .card-biz { background: linear-gradient(135deg, #1b2740, #141a24); border-color: #2d3a52; }
html.dark .mod-ic { background: #222b38; color: #9aa6b8; }
html.dark .pg-item:hover { background: #1c2636; border-color: #2d3a52; }
html.dark .zoom-ctrl, html.dark .card-ops { border-color: #2c3342; }
html.dark .zoom-ctrl button { border-bottom-color: #242b38; color: #9aa6b8; }
html.dark .zoom-ctrl button:hover { background: #1c2636; color: var(--brand); }
html.dark .res-line:hover, html.dark .alert-line:hover { background: #1c2636; }
html.dark .node-alert-badge, html.dark .conn-handle { border-color: #1a212e; }
html.dark .al-grey { background: #262d3a; color: #9ca3af; }
</style>
