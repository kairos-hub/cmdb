package router

import (
	"github.com/cmdb/cmdb-backend/internal/auth"
	"github.com/cmdb/cmdb-backend/internal/handler"
	"github.com/cmdb/cmdb-backend/internal/middleware"
	"github.com/cmdb/cmdb-backend/internal/model"
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// New 构建 HTTP 路由。
//
// 权限分层：
//   - authorized：所有登录用户可访问（读操作 + 实例数据写操作，实例写在 handler 内按对象级权限校验）
//   - admin：仅超级管理员（模型/拓扑等配置类写操作、用户/角色/审计管理）
func New(db *gorm.DB, userService *service.UserService, rbacService *service.RBACService, tokenManager *auth.TokenManager, startBackgroundJobs bool) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	modelService := service.NewModelService(db)
	instanceService := service.NewInstanceService(db)
	topoService := service.NewTopoService(db)
	auditService := service.NewAuditService(db)
	statsService := service.NewStatsService(db)
	dataSourceService := service.NewDataSourceService(db, instanceService)
	alertService := service.NewAlertService(db, instanceService)
	integrationService := service.NewIntegrationService(db)
	metricsService := service.NewMetricsService(db, instanceService)
	settingService := service.NewSettingService(db)

	if startBackgroundJobs {
		dataSourceService.StartScheduler()                           // 资源同步定时调度
		alertService.StartScheduler()                                // 告警拉取定时调度（与资源同步独立）
		service.StartOfflineCleanup(settingService, instanceService) // 下线资源清理调度
		service.StartAuditCleanup(settingService, auditService)      // 审计日志定时清理调度
		service.StartAlertCleanup(settingService, alertService)      // 告警定时清理调度（默认保留90天、每天凌晨3点）
		alertService.ReconcileAllAlerts()                            // 启动时做一次告警恢复对账（清理历史卡在 firing 的）
	}

	healthHandler := handler.NewHealthHandler(db)
	authHandler := handler.NewAuthHandler(userService, tokenManager)
	modelHandler := handler.NewModelHandler(modelService)
	instanceHandler := handler.NewInstanceHandler(instanceService, rbacService)
	topoHandler := handler.NewTopoHandler(topoService)
	userHandler := handler.NewUserHandler(userService, rbacService)
	roleHandler := handler.NewRoleHandler(rbacService)
	auditHandler := handler.NewAuditHandler(auditService)
	statsHandler := handler.NewStatsHandler(statsService)
	dataSourceHandler := handler.NewDataSourceHandler(dataSourceService)
	alertSourceHandler := handler.NewAlertSourceHandler(alertService)
	integrationHandler := handler.NewIntegrationHandler(integrationService)
	metricsHandler := handler.NewMetricsHandler(metricsService, instanceService, rbacService)
	settingHandler := handler.NewSettingHandler(settingService)

	api := r.Group("/api")
	{
		api.GET("/health", healthHandler.Check)
		api.POST("/auth/login", authHandler.Login)
		api.POST("/alerts/webhook/:id", alertSourceHandler.AlertWebhook) // 通用告警回调（无需登录，token 校验）

		// 登录用户可访问；写操作记录审计
		authorized := api.Group("")
		authorized.Use(middleware.Auth(tokenManager))
		authorized.Use(middleware.Audit(db, instanceService))
		{
			authorized.GET("/auth/me", authHandler.Me)
			authorized.GET("/auth/permissions", userHandler.CurrentPermissions)
			authorized.GET("/stats/overview", statsHandler.Overview)
			authorized.GET("/stats/analysis", middleware.FuncRequired(rbacService, model.FuncStatsView), statsHandler.Analysis)
			authorized.GET("/search", instanceHandler.GlobalSearch)

			// ---- 只读：模型元数据 ----
			authorized.GET("/model-groups", modelHandler.ListGroups)
			authorized.GET("/models", modelHandler.ListModels)
			authorized.GET("/models/:id", modelHandler.GetModel)
			authorized.GET("/models/:id/attributes", modelHandler.ListAttributes)
			authorized.GET("/association-kinds", modelHandler.ListKinds)
			authorized.GET("/model-associations", modelHandler.ListAssociations)

			// ---- 资源实例：读 + 写（写在 handler 内按对象级权限校验）----
			authorized.POST("/instances/search", instanceHandler.Search)
			authorized.POST("/instances/export", instanceHandler.Export)
			authorized.GET("/export/all", instanceHandler.ExportAll)
			authorized.GET("/data-sources/options", dataSourceHandler.Options)
			authorized.GET("/alerts", alertSourceHandler.ListAlerts)             // 告警列表：登录可见
			authorized.GET("/alert-sources/options", alertSourceHandler.Options) // 告警源精简项：登录可见（筛选用）
			authorized.GET("/alerts/products", alertSourceHandler.ProductDist)   // 告警产品分布：登录可见（总览扇形图）
			authorized.GET("/integrations", integrationHandler.List)             // 系统集成入口：登录可见
			authorized.POST("/instances/template", instanceHandler.Template)
			authorized.POST("/instances/batch-delete", instanceHandler.BatchDelete)
			authorized.POST("/instances/import", instanceHandler.Import)
			authorized.POST("/instances/collect", instanceHandler.Collect)
			authorized.POST("/instances", instanceHandler.Create)
			authorized.GET("/instances/:id", instanceHandler.Get)
			authorized.PUT("/instances/:id", instanceHandler.Update)
			authorized.DELETE("/instances/:id", instanceHandler.Delete)
			authorized.GET("/instances/:id/metrics", metricsHandler.HostMetrics) // 主机资源监控时序
			authorized.GET("/instances/:id/relations", instanceHandler.ListRelations)
			authorized.POST("/instance-relations", instanceHandler.CreateRelation)
			authorized.DELETE("/instance-relations/:id", instanceHandler.DeleteRelation)

			// ---- 只读：业务拓扑 ----
			authorized.GET("/topo/business", topoHandler.ListBusiness)
			authorized.GET("/topo/business/:id/tree", topoHandler.Tree)
			authorized.GET("/topo/business/:id/summary", topoHandler.Summary)
			authorized.GET("/topo/businesses/:id/edges", topoHandler.ListEdges)
			authorized.GET("/topo/modules/:id/hosts", topoHandler.ModuleHosts)
			authorized.GET("/topo/idle-hosts", topoHandler.IdleHosts)
			authorized.GET("/topo/export", topoHandler.ExportTopo)
			authorized.GET("/topo/template", topoHandler.TemplateTopo)

			// 功能权限：模型管理
			modelMgmt := authorized.Group("")
			modelMgmt.Use(middleware.FuncRequired(rbacService, model.FuncModelManage))
			{
				modelMgmt.POST("/model-groups", modelHandler.CreateGroup)
				modelMgmt.PUT("/model-groups/:id", modelHandler.UpdateGroup)
				modelMgmt.DELETE("/model-groups/:id", modelHandler.DeleteGroup)
				modelMgmt.POST("/models", modelHandler.CreateModel)
				modelMgmt.PUT("/model-groups/:id/models/reorder", modelHandler.ReorderModels)
				modelMgmt.PUT("/models/:id", modelHandler.UpdateModel)
				modelMgmt.DELETE("/models/:id", modelHandler.DeleteModel)
				modelMgmt.POST("/models/:id/attributes", modelHandler.CreateAttribute)
				modelMgmt.PUT("/models/:id/attributes/reorder", modelHandler.ReorderAttributes)
				modelMgmt.PUT("/attributes/:aid", modelHandler.UpdateAttribute)
				modelMgmt.DELETE("/attributes/:aid", modelHandler.DeleteAttribute)
				modelMgmt.POST("/association-kinds", modelHandler.CreateKind)
				modelMgmt.DELETE("/association-kinds/:id", modelHandler.DeleteKind)
				modelMgmt.POST("/model-associations", modelHandler.CreateAssociation)
				modelMgmt.DELETE("/model-associations/:id", modelHandler.DeleteAssociation)
			}

			// 功能权限：业务拓扑管理
			topoMgmt := authorized.Group("")
			topoMgmt.Use(middleware.FuncRequired(rbacService, model.FuncTopoManage))
			{
				topoMgmt.POST("/topo/business", topoHandler.CreateBiz)
				topoMgmt.PUT("/topo/business/:id", topoHandler.UpdateBiz)
				topoMgmt.DELETE("/topo/business/:id", topoHandler.DeleteBiz)
				topoMgmt.POST("/topo/businesses", topoHandler.CreateBusiness)
				topoMgmt.PUT("/topo/businesses/:id", topoHandler.UpdateBusiness)
				topoMgmt.DELETE("/topo/businesses/:id", topoHandler.DeleteBusiness)
				topoMgmt.POST("/topo/sets", topoHandler.CreateSet)
				topoMgmt.PUT("/topo/sets/:id", topoHandler.UpdateSet)
				topoMgmt.DELETE("/topo/sets/:id", topoHandler.DeleteSet)
				topoMgmt.POST("/topo/modules", topoHandler.CreateModule)
				topoMgmt.PUT("/topo/modules/:id", topoHandler.UpdateModule)
				topoMgmt.PUT("/topo/modules/:id/position", topoHandler.UpdateModulePosition)
				topoMgmt.DELETE("/topo/modules/:id", topoHandler.DeleteModule)
				topoMgmt.POST("/topo/edges", topoHandler.CreateEdge)
				topoMgmt.DELETE("/topo/edges/:id", topoHandler.DeleteEdge)
				topoMgmt.POST("/topo/assign", topoHandler.Assign)
				topoMgmt.POST("/topo/transfer", topoHandler.Transfer)
				topoMgmt.POST("/topo/recycle", topoHandler.Recycle)
				topoMgmt.POST("/topo/import", topoHandler.ImportTopo)
			}

			// 功能权限：用户与角色-只读（查看列表/角色/权限/设置；编辑者因蕴含关系自动通过）
			userView := authorized.Group("")
			userView.Use(middleware.FuncRequired(rbacService, model.FuncUserView))
			{
				userView.GET("/users", userHandler.List)
				userView.GET("/users/:id/roles", userHandler.GetRoles)
				userView.GET("/roles", roleHandler.List)
				userView.GET("/roles/:id/permissions", roleHandler.GetPermissions)
				userView.GET("/settings", settingHandler.Get)
			}

			// 功能权限：用户与角色-编辑
			userMgmt := authorized.Group("")
			userMgmt.Use(middleware.FuncRequired(rbacService, model.FuncUserManage))
			{
				userMgmt.POST("/users", userHandler.Create)
				userMgmt.PUT("/users/:id", userHandler.Update)
				userMgmt.DELETE("/users/:id", userHandler.Delete)
				userMgmt.PUT("/users/:id/password", userHandler.ChangePassword)
				userMgmt.PUT("/users/:id/roles", userHandler.SetRoles)

				userMgmt.POST("/roles", roleHandler.Create)
				userMgmt.PUT("/roles/:id", roleHandler.Update)
				userMgmt.DELETE("/roles/:id", roleHandler.Delete)
				userMgmt.PUT("/roles/:id/permissions", roleHandler.SetPermissions)

				userMgmt.PUT("/settings", settingHandler.Update)
			}

			// 功能权限：审计日志
			auditView := authorized.Group("")
			auditView.Use(middleware.FuncRequired(rbacService, model.FuncAuditView))
			{
				auditView.GET("/audit", auditHandler.List)
				auditView.DELETE("/audit", auditHandler.Cleanup)
			}

			// 功能权限：数据源-只读（查看配置列表；编辑者因蕴含关系自动通过）
			dsView := authorized.Group("")
			dsView.Use(middleware.FuncRequired(rbacService, model.FuncDataSourceView))
			{
				dsView.GET("/data-sources", dataSourceHandler.List)
				dsView.GET("/alert-sources", alertSourceHandler.List)
			}

			// 功能权限：数据源-编辑（新建/测试/修改/删除/同步）
			dsMgmt := authorized.Group("")
			dsMgmt.Use(middleware.FuncRequired(rbacService, model.FuncDataSource))
			{
				dsMgmt.POST("/data-sources", dataSourceHandler.Create)
				dsMgmt.POST("/data-sources/test", dataSourceHandler.Test)
				dsMgmt.PUT("/data-sources/:id", dataSourceHandler.Update)
				dsMgmt.DELETE("/data-sources/:id", dataSourceHandler.Delete)
				dsMgmt.POST("/data-sources/:id/sync", dataSourceHandler.Sync)
				dsMgmt.POST("/data-sources/:id/sync-ack-apps", dataSourceHandler.SyncAckApps)
				dsMgmt.PUT("/data-sources/reorder", dataSourceHandler.Reorder)

				// 告警源（与数据源解耦；复用数据源编辑权限）
				dsMgmt.POST("/alert-sources", alertSourceHandler.Create)
				dsMgmt.POST("/alert-sources/test", alertSourceHandler.Test)
				dsMgmt.PUT("/alert-sources/reorder", alertSourceHandler.Reorder)
				dsMgmt.PUT("/alert-sources/:id", alertSourceHandler.Update)
				dsMgmt.DELETE("/alert-sources/:id", alertSourceHandler.Delete)
				dsMgmt.POST("/alert-sources/:id/sync", alertSourceHandler.Sync)
				dsMgmt.DELETE("/alerts", alertSourceHandler.CleanupAlerts)

				// 系统集成入口（复用数据源编辑权限）
				dsMgmt.POST("/integrations", integrationHandler.Create)
				dsMgmt.PUT("/integrations/reorder", integrationHandler.Reorder)
				dsMgmt.PUT("/integrations/:id", integrationHandler.Update)
				dsMgmt.DELETE("/integrations/:id", integrationHandler.Delete)
			}
		}
	}

	return r
}
