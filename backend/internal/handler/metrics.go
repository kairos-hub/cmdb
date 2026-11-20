package handler

import (
	"github.com/cmdb/cmdb-backend/internal/middleware"
	"github.com/cmdb/cmdb-backend/internal/model"
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// MetricsHandler 主机资源监控接口。
type MetricsHandler struct {
	svc  *service.MetricsService
	inst *service.InstanceService
	rbac *service.RBACService
}

func NewMetricsHandler(svc *service.MetricsService, inst *service.InstanceService, rbac *service.RBACService) *MetricsHandler {
	return &MetricsHandler{svc: svc, inst: inst, rbac: rbac}
}

// HostMetrics GET /instances/:id/metrics?range=1h
// 返回该主机近 range 时段的 CPU/内存/磁盘/网络时序，按来源自动路由监控后端。
func (h *MetricsHandler) HostMetrics(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	inst, err := h.inst.Get(id)
	if err != nil {
		response.NotFound(c, "实例不存在")
		return
	}
	if !h.rbac.HasPermission(c.GetUint(middleware.CtxUserID), inst.ModelID, model.ActionView) {
		response.Forbidden(c, "无权查看该实例")
		return
	}
	rng := c.DefaultQuery("range", "1h")
	res, err := h.svc.HostMetrics(id, rng)
	if err != nil {
		response.NotFound(c, "实例不存在")
		return
	}
	response.OK(c, res)
}
