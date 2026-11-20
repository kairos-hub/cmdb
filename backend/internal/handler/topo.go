package handler

import (
	"errors"
	"io"
	"strconv"

	"github.com/cmdb/cmdb-backend/internal/model"
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// TopoHandler 业务拓扑相关接口。
type TopoHandler struct {
	svc *service.TopoService
}

func NewTopoHandler(svc *service.TopoService) *TopoHandler {
	return &TopoHandler{svc: svc}
}

// parsePaging 从 query 读取分页参数。
func parsePaging(c *gin.Context) (int, int) {
	page, pageSize := 1, 20
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := c.Query("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			pageSize = n
		}
	}
	return page, pageSize
}

func (h *TopoHandler) fail(c *gin.Context, err error) {
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

// ---------------- 业务 ----------------

func (h *TopoHandler) ListBusiness(c *gin.Context) {
	list, err := h.svc.ListBusiness()
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, list)
}

func (h *TopoHandler) CreateBiz(c *gin.Context) {
	var req struct {
		Name    string `json:"name" binding:"required"`
		Owner   string `json:"owner"`
		Contact string `json:"contact"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	n, err := h.svc.CreateBiz(req.Name, req.Owner, req.Contact)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, n)
}

func (h *TopoHandler) UpdateBiz(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	// owner/contact 用指针区分「未传=不改」与「传空串=清空」
	var req struct {
		Name    string  `json:"name" binding:"required"`
		Owner   *string `json:"owner"`
		Contact *string `json:"contact"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	n, err := h.svc.UpdateBiz(id, req.Name, req.Owner, req.Contact)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, n)
}

func (h *TopoHandler) DeleteBiz(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteBiz(id); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *TopoHandler) Tree(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	tree, err := h.svc.Tree(id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, tree)
}

// ---------------- 业务 ----------------

func (h *TopoHandler) CreateBusiness(c *gin.Context) {
	var req struct {
		BizID uint   `json:"biz_id" binding:"required"`
		Name  string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	n, err := h.svc.CreateBusiness(req.BizID, req.Name)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, n)
}

func (h *TopoHandler) UpdateBusiness(c *gin.Context) {
	h.rename(c, model.TopoBusiness)
}

func (h *TopoHandler) DeleteBusiness(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteBusiness(id); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ---------------- 集群 ----------------

func (h *TopoHandler) CreateSet(c *gin.Context) {
	var req struct {
		BusinessID uint   `json:"business_id" binding:"required"`
		Name       string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	n, err := h.svc.CreateSet(req.BusinessID, req.Name)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, n)
}

func (h *TopoHandler) UpdateSet(c *gin.Context) {
	h.rename(c, model.TopoSet)
}

func (h *TopoHandler) DeleteSet(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteSet(id); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ---------------- 模块 ----------------

func (h *TopoHandler) CreateModule(c *gin.Context) {
	var req struct {
		BusinessID uint   `json:"business_id" binding:"required"`
		ModelID    uint   `json:"model_id" binding:"required"`
		Name       string `json:"name"`
		PosX       int    `json:"pos_x"`
		PosY       int    `json:"pos_y"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	n, err := h.svc.CreateModule(req.BusinessID, req.ModelID, req.Name, req.PosX, req.PosY)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, n)
}

func (h *TopoHandler) UpdateModulePosition(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		PosX int `json:"pos_x"`
		PosY int `json:"pos_y"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.UpdateModulePosition(id, req.PosX, req.PosY); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ---------------- 连线 ----------------

func (h *TopoHandler) ListEdges(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	edges, err := h.svc.ListEdges(id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, edges)
}

func (h *TopoHandler) CreateEdge(c *gin.Context) {
	var req struct {
		BusinessID  uint `json:"business_id" binding:"required"`
		SrcModuleID uint `json:"src_module_id" binding:"required"`
		DstModuleID uint `json:"dst_module_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	e, err := h.svc.CreateEdge(req.BusinessID, req.SrcModuleID, req.DstModuleID)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, e)
}

func (h *TopoHandler) DeleteEdge(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteEdge(id); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *TopoHandler) UpdateModule(c *gin.Context) {
	h.rename(c, model.TopoModule)
}

func (h *TopoHandler) DeleteModule(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteModule(id); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *TopoHandler) rename(c *gin.Context, nodeType string) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	n, err := h.svc.UpdateNodeName(id, nodeType, req.Name)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, n)
}

// ---------------- 主机归属 ----------------

func (h *TopoHandler) IdleHosts(c *gin.Context) {
	modelID, _ := strconv.ParseUint(c.Query("model_id"), 10, 64)
	page, pageSize := parsePaging(c)
	res, err := h.svc.IdleInstances(uint(modelID), page, pageSize)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, res)
}

func (h *TopoHandler) Summary(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	sum, err := h.svc.BizSummary(id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, sum)
}

func (h *TopoHandler) ModuleHosts(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	hosts, err := h.svc.ModuleHosts(id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, hosts)
}

func (h *TopoHandler) Assign(c *gin.Context) {
	var req struct {
		ModuleID uint   `json:"module_id" binding:"required"`
		HostIDs  []uint `json:"host_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.AssignHosts(req.ModuleID, req.HostIDs); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *TopoHandler) Transfer(c *gin.Context) {
	var req struct {
		TargetModuleID uint   `json:"target_module_id" binding:"required"`
		HostIDs        []uint `json:"host_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.TransferHosts(req.TargetModuleID, req.HostIDs); err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *TopoHandler) Recycle(c *gin.Context) {
	var req struct {
		HostIDs []uint `json:"host_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	n, err := h.svc.RecycleHosts(req.HostIDs)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, gin.H{"recycled": n})
}


// ExportTopo 导出业务拓扑全貌为 xlsx。
func (h *TopoHandler) ExportTopo(c *gin.Context) {
	var bizID uint
	if v := c.Query("biz_id"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			bizID = uint(n)
		}
	}
	data, err := h.svc.ExportTopo(bizID)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="topology.xlsx"`)
	c.Data(200, xlsxContentType, data)
}

// ImportTopo 从 xlsx 导入拓扑结构与资源归属。
func (h *TopoHandler) ImportTopo(c *gin.Context) {
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
	report, err := h.svc.ImportTopo(buf)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, report)
}

// TemplateTopo 下载拓扑导入模板。
func (h *TopoHandler) TemplateTopo(c *gin.Context) {
	data, err := h.svc.TemplateTopo()
	if err != nil {
		mapServiceError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="topology_template.xlsx"`)
	c.Data(200, xlsxContentType, data)
}
