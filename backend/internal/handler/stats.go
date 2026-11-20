package handler

import (
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// StatsHandler 概览统计接口。
type StatsHandler struct {
	svc *service.StatsService
}

func NewStatsHandler(svc *service.StatsService) *StatsHandler {
	return &StatsHandler{svc: svc}
}

func (h *StatsHandler) Overview(c *gin.Context) {
	o, err := h.svc.Overview()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, o)
}

func (h *StatsHandler) Analysis(c *gin.Context) {
	a, err := h.svc.Analysis()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, a)
}
