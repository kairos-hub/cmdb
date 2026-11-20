package handler

import (
	"errors"
	"strconv"

	"github.com/cmdb/cmdb-backend/internal/middleware"
	"github.com/cmdb/cmdb-backend/internal/model"
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

// ModelHandler 模型管理相关接口。
type ModelHandler struct {
	svc *service.ModelService
}

func NewModelHandler(svc *service.ModelService) *ModelHandler {
	return &ModelHandler{svc: svc}
}

// fail 按业务错误类型映射为合适的 HTTP 状态码。
func (h *ModelHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrDuplicate),
		errors.Is(err, service.ErrGroupNotEmpty),
		errors.Is(err, service.ErrReferenced):
		response.Conflict(c, err.Error())
	case errors.Is(err, service.ErrBuiltinProtected):
		response.Forbidden(c, err.Error())
	case errors.Is(err, service.ErrInvalidFieldType),
		errors.Is(err, service.ErrInvalidMapping),
		errors.Is(err, service.ErrInvalidRef):
		response.BadRequest(c, err.Error())
	default:
		response.InternalError(c, err.Error())
	}
}

func parseID(c *gin.Context, key string) (uint, bool) {
	v, err := strconv.ParseUint(c.Param(key), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的ID")
		return 0, false
	}
	return uint(v), true
}

// ---------------- 分组 ----------------

func (h *ModelHandler) ListGroups(c *gin.Context) {
	groups, err := h.svc.ListGroups()
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, groups)
}

func (h *ModelHandler) CreateGroup(c *gin.Context) {
	var req struct {
		Code  string `json:"code" binding:"required"`
		Name  string `json:"name" binding:"required"`
		Index int    `json:"index"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	g := &model.ModelGroup{Code: req.Code, Name: req.Name, Index: req.Index}
	if err := h.svc.CreateGroup(g); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, g)
}

func (h *ModelHandler) UpdateGroup(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Name  string `json:"name" binding:"required"`
		Index int    `json:"index"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	g, err := h.svc.UpdateGroup(id, req.Name, req.Index)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, g)
}

func (h *ModelHandler) DeleteGroup(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteGroup(id); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ---------------- 模型 ----------------

func (h *ModelHandler) ListModels(c *gin.Context) {
	var groupID uint
	if v := c.Query("group_id"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			groupID = uint(n)
		}
	}
	models, err := h.svc.ListModels(groupID)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, models)
}

func (h *ModelHandler) GetModel(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	m, err := h.svc.GetModel(id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, m)
}

func (h *ModelHandler) CreateModel(c *gin.Context) {
	var req struct {
		GroupID uint   `json:"group_id" binding:"required"`
		ObjID   string `json:"obj_id" binding:"required"`
		Name    string `json:"name" binding:"required"`
		Icon    string `json:"icon"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	m := &model.Model{GroupID: req.GroupID, ObjID: req.ObjID, Name: req.Name, Icon: req.Icon}
	if err := h.svc.CreateModel(m); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, m)
}

func (h *ModelHandler) UpdateModel(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Name    string `json:"name" binding:"required"`
		Icon    string `json:"icon"`
		GroupID uint   `json:"group_id"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	m, err := h.svc.UpdateModel(id, req.Name, req.Icon, req.GroupID, req.Enabled)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, m)
}

func (h *ModelHandler) DeleteModel(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteModel(id); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ReorderModels 按给定顺序重排某分组内的模型。
func (h *ModelHandler) ReorderModels(c *gin.Context) {
	groupID, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.ReorderModels(groupID, req.IDs); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ---------------- 字段 ----------------

type attributeRequest struct {
	PropertyID string         `json:"property_id"`
	Name       string         `json:"name" binding:"required"`
	Type       string         `json:"type"`
	Required   bool           `json:"required"`
	Unique     bool           `json:"unique"`
	Editable   bool           `json:"editable"`
	Enabled    *bool          `json:"enabled"`
	ListHidden bool           `json:"list_hidden"`
	Options    datatypes.JSON `json:"options"`
	Default    string         `json:"default"`
	Regex      string         `json:"regex"`
	Group      string         `json:"group"`
	Index      int            `json:"index"`
}

func (h *ModelHandler) ListAttributes(c *gin.Context) {
	modelID, ok := parseID(c, "id")
	if !ok {
		return
	}
	attrs, err := h.svc.ListAttributes(modelID)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, attrs)
}

func (h *ModelHandler) CreateAttribute(c *gin.Context) {
	modelID, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req attributeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if req.PropertyID == "" || req.Type == "" {
		response.BadRequest(c, "property_id 与 type 不能为空")
		return
	}
	a := &model.Attribute{
		ModelID:    modelID,
		PropertyID: req.PropertyID,
		Name:       req.Name,
		Type:       req.Type,
		Required:   req.Required,
		Unique:     req.Unique,
		Editable:   req.Editable,
		Enabled:    req.Enabled == nil || *req.Enabled, // 默认启用
		ListHidden: req.ListHidden,
		Options:    req.Options,
		Default:    req.Default,
		Regex:      req.Regex,
		Group:      req.Group,
		Index:      req.Index,
	}
	if err := h.svc.CreateAttribute(a); err != nil {
		h.fail(c, err)
		return
	}
	h.setFieldAudit(c, a.ModelID, fieldLabel(a))
	response.OK(c, a)
}

func (h *ModelHandler) UpdateAttribute(c *gin.Context) {
	aid, ok := parseID(c, "aid")
	if !ok {
		return
	}
	var req attributeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	in := &model.Attribute{
		Name:     req.Name,
		Type:     req.Type,
		Required: req.Required,
		Unique:   req.Unique,
		Editable: req.Editable,
		Enabled:    req.Enabled == nil || *req.Enabled, // 未传视为启用
		ListHidden: req.ListHidden,
		Options:    req.Options,
		Default:    req.Default,
		Regex:      req.Regex,
		Group:      req.Group,
		Index:      req.Index,
	}
	a, changes, err := h.svc.UpdateAttribute(aid, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	// 审计：记录所属模型、字段名与变更明细；无变更则不记录
	h.setFieldAudit(c, a.ModelID, fieldLabel(a))
	if len(changes) == 0 {
		c.Set(middleware.CtxAuditSkip, true)
	} else {
		c.Set(middleware.CtxAuditChanges, changes)
	}
	response.OK(c, a)
}

func (h *ModelHandler) DeleteAttribute(c *gin.Context) {
	aid, ok := parseID(c, "aid")
	if !ok {
		return
	}
	// 删除前捕获所属模型与字段名（删除后查不到）
	if a, e := h.svc.GetAttribute(aid); e == nil {
		h.setFieldAudit(c, a.ModelID, fieldLabel(a))
	}
	if err := h.svc.DeleteAttribute(aid); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

// setFieldAudit 为字段的增删改设置审计对象（「模型「X」字段」）与目标（字段名）。
func (h *ModelHandler) setFieldAudit(c *gin.Context, modelID uint, fieldName string) {
	if m, err := h.svc.GetModel(modelID); err == nil && m != nil {
		c.Set(middleware.CtxAuditObject, "模型「"+m.Name+"」字段")
	}
	if fieldName != "" {
		c.Set(middleware.CtxAuditInstance, fieldName)
	}
}

// fieldLabel 字段审计目标名：优先显示名，回退 property_id。
func fieldLabel(a *model.Attribute) string {
	if a == nil {
		return ""
	}
	if a.Name != "" {
		return a.Name
	}
	return a.PropertyID
}

// ReorderAttributes 按给定顺序重排模型字段。
func (h *ModelHandler) ReorderAttributes(c *gin.Context) {
	modelID, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.ReorderAttributes(modelID, req.IDs); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ---------------- 关联类型 ----------------

func (h *ModelHandler) ListKinds(c *gin.Context) {
	kinds, err := h.svc.ListKinds()
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, kinds)
}

func (h *ModelHandler) CreateKind(c *gin.Context) {
	var req struct {
		Code         string `json:"code" binding:"required"`
		Name         string `json:"name"`
		SrcToDstNote string `json:"src_to_dst_note"`
		DstToSrcNote string `json:"dst_to_src_note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	k := &model.AssociationKind{
		Code: req.Code, Name: req.Name,
		SrcToDstNote: req.SrcToDstNote, DstToSrcNote: req.DstToSrcNote,
	}
	if err := h.svc.CreateKind(k); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, k)
}

func (h *ModelHandler) DeleteKind(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteKind(id); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ---------------- 模型关联 ----------------

func (h *ModelHandler) ListAssociations(c *gin.Context) {
	assocs, err := h.svc.ListAssociations()
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, assocs)
}

func (h *ModelHandler) CreateAssociation(c *gin.Context) {
	var req struct {
		KindID     uint   `json:"kind_id" binding:"required"`
		SrcModelID uint   `json:"src_model_id" binding:"required"`
		DstModelID uint   `json:"dst_model_id" binding:"required"`
		Mapping    string `json:"mapping" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	a := &model.ModelAssociation{
		KindID: req.KindID, SrcModelID: req.SrcModelID,
		DstModelID: req.DstModelID, Mapping: req.Mapping,
	}
	if err := h.svc.CreateAssociation(a); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, a)
}

func (h *ModelHandler) DeleteAssociation(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteAssociation(id); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}
