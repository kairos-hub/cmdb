package handler

import (
	"io"
	"strconv"

	"github.com/cmdb/cmdb-backend/internal/model"
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// AlertSourceHandler 告警源配置、告警拉取/回调、告警列表与清理。
type AlertSourceHandler struct {
	svc *service.AlertService
}

func NewAlertSourceHandler(svc *service.AlertService) *AlertSourceHandler {
	return &AlertSourceHandler{svc: svc}
}

func (h *AlertSourceHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, list)
}

// Options 告警源精简列表（登录可用，供告警事件页筛选，不含凭证）。
func (h *AlertSourceHandler) Options(c *gin.Context) {
	list, err := h.svc.ListOptions()
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, list)
}

// ProductDist 告警产品分布（登录可用，供总览扇形图）。
func (h *AlertSourceHandler) ProductDist(c *gin.Context) {
	list, err := h.svc.ProductDist()
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, list)
}

func (h *AlertSourceHandler) Create(c *gin.Context) {
	var a model.AlertSource
	if err := c.ShouldBindJSON(&a); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Create(&a); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, a)
}

func (h *AlertSourceHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var a model.AlertSource
	if err := c.ShouldBindJSON(&a); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Update(id, &a); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *AlertSourceHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(id); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// Reorder 按给定 id 顺序重排告警源。
func (h *AlertSourceHandler) Reorder(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Reorder(req.IDs); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"ok": true})
}

// Test 测试云告警源凭证连通性（不落库）。
func (h *AlertSourceHandler) Test(c *gin.Context) {
	var a model.AlertSource
	if err := c.ShouldBindJSON(&a); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.TestConnection(&a); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"ok": true})
}

// Sync 手动拉取某告警源的告警（阿里云/腾讯云）。
func (h *AlertSourceHandler) Sync(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	n, err := h.svc.SyncAlerts(id)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"count": n})
}

// ListAlerts 告警列表（级别/状态/告警源/关键词/时间范围筛选）。
func (h *AlertSourceHandler) ListAlerts(c *gin.Context) {
	p := service.AlertListParams{
		Level:   c.Query("level"),
		Status:  c.Query("status"),
		Keyword: c.Query("keyword"),
		Sort:    c.Query("sort"),
	}
	if v, e := strconv.Atoi(c.Query("source")); e == nil {
		p.SourceID = uint(v)
	}
	if v, e := strconv.Atoi(c.Query("inst")); e == nil {
		p.ResInstID = uint(v)
	}
	if v, e := strconv.Atoi(c.Query("days")); e == nil {
		p.Days = v
	}
	if v, e := strconv.Atoi(c.Query("page")); e == nil {
		p.Page = v
	}
	if v, e := strconv.Atoi(c.Query("page_size")); e == nil {
		p.PageSize = v
	}
	res, err := h.svc.ListAlerts(p)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, res)
}

// AlertWebhook 接收任意平台的告警回调（无需登录，token 校验）。
func (h *AlertSourceHandler) AlertWebhook(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	token := c.Query("token")
	if token == "" {
		token = c.GetHeader("X-Webhook-Token")
	}
	body, _ := io.ReadAll(c.Request.Body)
	n, err := h.svc.IngestAlertWebhook(id, token, body)
	if err != nil {
		c.JSON(400, gin.H{"ok": false, "msg": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "count": n})
}

// CleanupAlerts 清理告警（days<=0 清空，受 source 限定；否则删 N 天前）。
func (h *AlertSourceHandler) CleanupAlerts(c *gin.Context) {
	days := 0
	if v, e := strconv.Atoi(c.Query("days")); e == nil {
		days = v
	}
	var sourceID uint
	if v, e := strconv.Atoi(c.Query("source")); e == nil && v > 0 {
		sourceID = uint(v)
	}
	n, err := h.svc.CleanupAlerts(days, sourceID)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": n})
}
