package handler

import (
	"github.com/cmdb/cmdb-backend/internal/model"
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// DataSourceHandler 外部数据源配置与同步。
type DataSourceHandler struct {
	svc *service.DataSourceService
}

func NewDataSourceHandler(svc *service.DataSourceService) *DataSourceHandler {
	return &DataSourceHandler{svc: svc}
}

func (h *DataSourceHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, list)
}

// Reorder 按给定 id 顺序重排数据源。
func (h *DataSourceHandler) Reorder(c *gin.Context) {
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

// Options 返回数据源精简列表（登录可用，供导出等选择来源）。
func (h *DataSourceHandler) Options(c *gin.Context) {
	list, err := h.svc.ListOptions()
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, list)
}

func (h *DataSourceHandler) Create(c *gin.Context) {
	var d model.DataSource
	if err := c.ShouldBindJSON(&d); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Create(&d); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, d)
}

func (h *DataSourceHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var d model.DataSource
	if err := c.ShouldBindJSON(&d); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Update(id, &d); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *DataSourceHandler) Delete(c *gin.Context) {
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

// Test 测试数据源连通性（不落库）。
func (h *DataSourceHandler) Test(c *gin.Context) {
	var d model.DataSource
	if err := c.ShouldBindJSON(&d); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	count, err := h.svc.TestConnection(&d)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"ok": true, "count": count})
}

// Sync 立即从该数据源同步主机。
func (h *DataSourceHandler) Sync(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	report, err := h.svc.Sync(id)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, report)
}

// SyncAckApps 同步某个阿里云 ACK 集群的应用服务（走该集群 ARMS Prometheus）。
func (h *DataSourceHandler) SyncAckApps(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	cluster := c.Query("cluster")
	if cluster == "" {
		response.BadRequest(c, "缺少 cluster 参数")
		return
	}
	report, err := h.svc.SyncAckApps(id, cluster)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, report)
}

