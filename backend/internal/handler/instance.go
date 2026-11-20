package handler

import (
	"errors"
	"io"
	"strconv"

	"github.com/cmdb/cmdb-backend/internal/middleware"
	"github.com/cmdb/cmdb-backend/internal/model"
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// InstanceHandler 资源实例相关接口。
type InstanceHandler struct {
	svc  *service.InstanceService
	rbac *service.RBACService
}

func NewInstanceHandler(svc *service.InstanceService, rbac *service.RBACService) *InstanceHandler {
	return &InstanceHandler{svc: svc, rbac: rbac}
}

func (h *InstanceHandler) fail(c *gin.Context, err error) {
	var ve *service.ValidationError
	switch {
	case errors.As(err, &ve):
		response.BadRequest(c, err.Error())
	case errors.Is(err, service.ErrNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrInvalidRef):
		response.BadRequest(c, err.Error())
	default:
		response.InternalError(c, err.Error())
	}
}

// Search 分页查询实例（支持关键字与字段筛选）。
func (h *InstanceHandler) Search(c *gin.Context) {
	var req struct {
		ModelID    uint                `json:"model_id" binding:"required"`
		Page       int                 `json:"page"`
		PageSize   int                 `json:"page_size"`
		Keyword    string              `json:"keyword"`
		Filters    map[string]string   `json:"filters"`
		Conditions []service.Condition `json:"conditions"`
		Assigned   string              `json:"assigned"`
		Source     string              `json:"source"` // ""/auto/manual
		Biz        string              `json:"biz"`    // 业务线过滤：业务实例 id（空=全部）
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if !h.rbac.HasPermission(c.GetUint(middleware.CtxUserID), req.ModelID, model.ActionView) {
		response.Forbidden(c, "无权查看该模型的实例")
		return
	}
	res, err := h.svc.List(req.ModelID, req.Page, req.PageSize, req.Keyword, req.Filters, req.Conditions, req.Assigned, req.Source, req.Biz)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, res)
}

// Collect 主机信息采集上报（按内网IP upsert）。供采集脚本调用。
func (h *InstanceHandler) Collect(c *gin.Context) {
	var req struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Data == nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if !h.rbac.HasPermission(c.GetUint(middleware.CtxUserID), h.svc.HostModelID(), model.ActionCreate) {
		response.Forbidden(c, "无主机采集权限")
		return
	}
	inst, created, err := h.svc.CollectHost(req.Data, service.CreatorSync, 0)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, gin.H{"id": inst.ID, "created": created})
}

func (h *InstanceHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	inst, err := h.svc.Get(id)
	if err != nil {
		h.fail(c, err)
		return
	}
	if !h.rbac.HasPermission(c.GetUint(middleware.CtxUserID), inst.ModelID, model.ActionView) {
		response.Forbidden(c, "无权查看该实例")
		return
	}
	response.OK(c, inst)
}

func (h *InstanceHandler) Create(c *gin.Context) {
	var req struct {
		ModelID uint                   `json:"model_id" binding:"required"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if !h.rbac.HasPermission(c.GetUint(middleware.CtxUserID), req.ModelID, model.ActionCreate) {
		response.Forbidden(c, "无权创建该模型的实例")
		return
	}
	user := c.GetString(middleware.CtxUsername)
	inst, err := h.svc.Create(req.ModelID, req.Data, user)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, inst)
}

func (h *InstanceHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	existing, err := h.svc.Get(id)
	if err != nil {
		h.fail(c, err)
		return
	}
	if !h.rbac.HasPermission(c.GetUint(middleware.CtxUserID), existing.ModelID, model.ActionUpdate) {
		response.Forbidden(c, "无权修改该实例")
		return
	}
	user := c.GetString(middleware.CtxUsername)
	inst, changes, err := h.svc.Update(id, req.Data, user)
	if err != nil {
		h.fail(c, err)
		return
	}
	// 审计：无变更则不记录；有变更则记为「更新」并附带变更明细
	if len(changes) == 0 {
		c.Set(middleware.CtxAuditSkip, true)
	} else {
		c.Set(middleware.CtxAuditAction, "更新")
		c.Set(middleware.CtxAuditChanges, changes)
	}
	response.OK(c, inst)
}

func (h *InstanceHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	existing, err := h.svc.Get(id)
	if err != nil {
		h.fail(c, err)
		return
	}
	if !h.rbac.HasPermission(c.GetUint(middleware.CtxUserID), existing.ModelID, model.ActionDelete) {
		response.Forbidden(c, "无权删除该实例")
		return
	}
	if err := h.svc.Delete(id); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *InstanceHandler) BatchDelete(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if !h.rbac.IsAdmin(c.GetUint(middleware.CtxUserID)) {
		response.Forbidden(c, "批量删除需要管理员权限")
		return
	}
	n, err := h.svc.BatchDelete(req.IDs)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": n})
}

// ---------------- 实例关联 ----------------

func (h *InstanceHandler) ListRelations(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	rels, err := h.svc.ListRelations(id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, rels)
}

func (h *InstanceHandler) CreateRelation(c *gin.Context) {
	var req struct {
		AssociationID uint `json:"association_id" binding:"required"`
		SrcInstID     uint `json:"src_inst_id" binding:"required"`
		DstInstID     uint `json:"dst_inst_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if !h.canEditRelation(c, req.SrcInstID, req.DstInstID) {
		response.Forbidden(c, "无权添加该关联（需对任一端模型有编辑权限）")
		return
	}
	rel, err := h.svc.CreateRelation(req.AssociationID, req.SrcInstID, req.DstInstID)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, rel)
}

// canEditRelation 校验：对关联两端实例任一模型拥有 update 权限（超管恒真）。
func (h *InstanceHandler) canEditRelation(c *gin.Context, srcInstID, dstInstID uint) bool {
	uid := c.GetUint(middleware.CtxUserID)
	if h.rbac.IsAdmin(uid) {
		return true
	}
	for _, instID := range []uint{srcInstID, dstInstID} {
		inst, err := h.svc.Get(instID)
		if err != nil {
			continue
		}
		if h.rbac.HasPermission(uid, inst.ModelID, model.ActionUpdate) {
			return true
		}
	}
	return false
}

func (h *InstanceHandler) DeleteRelation(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	rel, err := h.svc.GetRelation(id)
	if err != nil {
		h.fail(c, err)
		return
	}
	if !h.canEditRelation(c, rel.SrcInstID, rel.DstInstID) {
		response.Forbidden(c, "无权删除该关联（需对任一端模型有编辑权限）")
		return
	}
	if err := h.svc.DeleteRelation(id); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ---------------- 导入导出 ----------------

const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// Export 导出匹配条件的实例为 xlsx。
func (h *InstanceHandler) Export(c *gin.Context) {
	var req struct {
		ModelID    uint                `json:"model_id" binding:"required"`
		Keyword    string              `json:"keyword"`
		Filters    map[string]string   `json:"filters"`
		Conditions []service.Condition `json:"conditions"`
		IDs        []uint              `json:"ids"`      // 勾选导出：非空则仅导这些实例
		Assigned   string              `json:"assigned"` // 归属筛选：assigned/unassigned（按业务线）
		Source     string              `json:"source"`   // 数据源筛选：manual/auto/数据源id
		Biz        string              `json:"biz"`      // 业务线筛选：业务实例 id
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if !h.rbac.HasPermission(c.GetUint(middleware.CtxUserID), req.ModelID, model.ActionView) {
		response.Forbidden(c, "无权导出该模型的实例")
		return
	}
	data, err := h.svc.ExportExcel(req.ModelID, req.Keyword, req.Filters, req.Conditions, req.IDs, req.Assigned, req.Source, req.Biz)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="instances.xlsx"`)
	c.Data(200, xlsxContentType, data)
}

// ExportAll 导出所有资源（每个模型一个 sheet），仅含当前用户有查看权限的模型。
func (h *InstanceHandler) ExportAll(c *gin.Context) {
	uid := c.GetUint(middleware.CtxUserID)
	canView := func(modelID uint) bool {
		return h.rbac.HasPermission(uid, modelID, model.ActionView)
	}
	// 可选按数据源过滤：source_id 省略=全部来源，0=用户创建(手动)，其它=指定数据源
	var sourceID *uint
	if sv := c.Query("source_id"); sv != "" {
		if n, err := strconv.ParseUint(sv, 10, 64); err == nil {
			u := uint(n)
			sourceID = &u
		}
	}
	data, err := h.svc.ExportAllExcel(canView, sourceID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="all_resources.xlsx"`)
	c.Data(200, xlsxContentType, data)
}

// Template 下载导入模板（仅表头）。
func (h *InstanceHandler) Template(c *gin.Context) {
	var req struct {
		ModelID uint `json:"model_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if !h.rbac.HasPermission(c.GetUint(middleware.CtxUserID), req.ModelID, model.ActionView) {
		response.Forbidden(c, "无权下载该模型的模板")
		return
	}
	data, err := h.svc.TemplateExcel(req.ModelID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="template.xlsx"`)
	c.Data(200, xlsxContentType, data)
}

// Import 上传 xlsx 批量导入实例。
func (h *InstanceHandler) Import(c *gin.Context) {
	modelID64, err := strconv.ParseUint(c.PostForm("model_id"), 10, 64)
	if err != nil || modelID64 == 0 {
		response.BadRequest(c, "缺少 model_id")
		return
	}
	modelID := uint(modelID64)
	if !h.rbac.HasPermission(c.GetUint(middleware.CtxUserID), modelID, model.ActionCreate) {
		response.Forbidden(c, "无权导入该模型的实例")
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请上传文件")
		return
	}
	file, err := fh.Open()
	if err != nil {
		response.InternalError(c, "读取文件失败")
		return
	}
	defer file.Close()
	buf, err := io.ReadAll(file)
	if err != nil {
		response.InternalError(c, "读取文件失败")
		return
	}
	report, err := h.svc.ImportExcel(modelID, buf, c.GetString(middleware.CtxUsername))
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, report)
}

// GlobalSearch 跨所有资源实例搜索（mode=exact 精确 / 否则模糊）。
func (h *InstanceHandler) GlobalSearch(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	modelID, _ := strconv.ParseUint(c.Query("model_id"), 10, strconv.IntSize)
	paged := c.Query("page") != "" || c.Query("page_size") != "" || modelID != 0
	if !paged {
		pageSize, _ = strconv.Atoi(c.Query("limit"))
		if pageSize <= 0 || pageSize > 100 {
			pageSize = 50
		}
	} else if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	permissions := h.rbac.GetUserPermissions(c.GetUint(middleware.CtxUserID))
	result, err := h.svc.GlobalSearch(c.Query("keyword"), c.Query("mode"), page, pageSize, uint(modelID), permissions, paged)
	if err != nil {
		h.fail(c, err)
		return
	}
	if paged {
		response.OK(c, result)
		return
	}
	response.OK(c, result.List)
}
