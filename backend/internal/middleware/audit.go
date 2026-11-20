package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/cmdb/cmdb-backend/internal/model"
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Audit 记录写操作（POST/PUT/DELETE/PATCH）到审计日志，并解析出操作对象、动作、
// 所属业务、实例、描述等业务语义字段。
func Audit(db *gorm.DB, insts *service.InstanceService) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == http.MethodOptions || method == http.MethodHead {
			c.Next()
			return
		}

		parts := strings.Split(strings.Trim(c.Request.URL.Path, "/"), "/") // ["api", "<seg1>", ...]

		// 资源查询审计（只读但留痕）：实例分页查询 POST /instances/search、全局搜索 GET /search
		if isQueryEndpoint(method, parts) {
			auditQuery(c, db, insts, parts)
			return
		}

		// 其余只读请求不计入审计：GET，以及走 POST 的导入模板下载
		if method == http.MethodGet {
			c.Next()
			return
		}
		if seg(parts, 1) == "instances" && seg(parts, 2) == "template" {
			c.Next()
			return
		}

		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// 删除/修改前先捕获展示信息（删除后已查不到）
		var preObject, preInstance, preBiz string
		if insts != nil && (method == http.MethodPut || method == http.MethodDelete) {
			// 资源实例：/api/instances/:id
			if seg(parts, 1) == "instances" && isNumeric(seg(parts, 2)) {
				id, _ := strconv.ParseUint(seg(parts, 2), 10, 64)
				preObject, preInstance, preBiz = insts.AuditTarget(uint(id))
			}
			// 业务：/api/topo/business/:id（业务即 biz 实例，id 为实例 id）
			if seg(parts, 1) == "topo" && seg(parts, 2) == "business" && isNumeric(seg(parts, 3)) {
				id, _ := strconv.ParseUint(seg(parts, 3), 10, 64)
				_, preInstance, _ = insts.AuditTarget(uint(id))
			}
		}
		// 批量删除：删除前先解析所删实例的模型名（删除后查不到），作为审计对象
		if insts != nil && method == http.MethodPost &&
			seg(parts, 1) == "instances" && seg(parts, 2) == "batch-delete" {
			if obj := insts.AuditBatchObject(batchIDs(bodyBytes)); obj != "" {
				preObject = obj
			}
		}

		c.Next()

		// handler 显式要求跳过审计（如实例更新但内容未变化）
		if c.GetBool(CtxAuditSkip) {
			return
		}

		object, actionText, instance, biz := deriveSemantics(method, parts, bodyBytes, insts)
		if preObject != "" {
			object = preObject
		}
		if preInstance != "" {
			instance = preInstance
		}
		if preBiz != "" {
			biz = preBiz
		}
		// handler 覆盖动作名（如实例更新 → 更新）
		if v := c.GetString(CtxAuditAction); v != "" {
			actionText = v
		}
		// handler 覆盖操作对象 / 目标（如字段编辑 → 「模型「主机」字段」/ 字段名）
		if v := c.GetString(CtxAuditObject); v != "" {
			object = v
		}
		if v := c.GetString(CtxAuditInstance); v != "" {
			instance = v
		}
		summary := buildSummary(actionText, object, instance)

		// 请求体脱敏与合法性处理
		detail := bodyBytes
		switch {
		case len(detail) == 0 || !json.Valid(detail):
			detail = []byte(`{}`)
		case strings.Contains(strings.ToLower(string(bodyBytes)), "password"):
			detail = []byte(`{"_":"[含敏感信息已隐藏]"}`)
		case len(detail) > 2000:
			detail = []byte(`{"_":"[请求体过大已省略]"}`)
		}

		// handler 提供的变更明细（更新内容）：操作描述只记录变更内容
		if raw, ok := c.Get(CtxAuditChanges); ok {
			if changes, ok2 := raw.([]string); ok2 && len(changes) > 0 {
				summary = strings.Join(changes, "，")
				if b, err := json.Marshal(map[string][]string{"changes": changes}); err == nil {
					detail = b
				}
			}
		}

		db.Create(&model.AuditLog{
			Username:     c.GetString(CtxUsername),
			Action:       method,
			ActionText:   actionText,
			Object:       object,
			Instance:     instance,
			BizName:      biz,
			Summary:      summary,
			ResourceType: seg(parts, 1),
			ResourcePath: c.Request.URL.Path,
			Detail:       datatypes.JSON(detail),
			Status:       c.Writer.Status(),
			IP:           c.ClientIP(),
		})
	}
}

// isQueryEndpoint 判定是否为需留痕的只读端点：实例分页查询 / 全局搜索 / 导出。
func isQueryEndpoint(method string, parts []string) bool {
	if method == http.MethodPost && seg(parts, 1) == "instances" {
		switch seg(parts, 2) {
		case "search", "export":
			return true
		}
	}
	if method == http.MethodGet && seg(parts, 1) == "search" {
		return true
	}
	// 导出全部资源 GET /export/all
	if method == http.MethodGet && seg(parts, 1) == "export" && seg(parts, 2) == "all" {
		return true
	}
	return false
}

// auditQuery 记录资源查询/导出审计。仅记录成功请求；查询仅在带有搜索条件时才记录
// （无任何关键词/筛选的纯列表浏览不计入），导出一律记录。
func auditQuery(c *gin.Context, db *gorm.DB, insts *service.InstanceService, parts []string) {
	method := c.Request.Method
	var bodyBytes []byte
	if method == http.MethodPost && c.Request.Body != nil {
		bodyBytes, _ = io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	c.Next()

	if c.Writer.Status() != http.StatusOK {
		return
	}

	var actionText, object, instance, summary string
	detail := []byte(`{}`)

	switch {
	case seg(parts, 1) == "export": // 导出全部资源 GET /api/export/all
		actionText, object = "导出", "全部资源"
		summary = "导出全部资源"

	case seg(parts, 1) == "search": // 全局搜索 GET /api/search
		kw, mode := c.Query("keyword"), c.Query("mode")
		if strings.TrimSpace(kw) == "" { // 未搜索不统计
			return
		}
		actionText, object, instance = "查询", "全部资源", kw
		summary = "全局搜索「" + kw + "」"
		if b, err := json.Marshal(map[string]string{"keyword": kw, "mode": mode}); err == nil {
			detail = b
		}

	case seg(parts, 2) == "export": // 导出 POST /instances/export
		var req struct {
			ModelID    uint              `json:"model_id"`
			Keyword    string            `json:"keyword"`
			Filters    map[string]string `json:"filters"`
			Conditions []json.RawMessage `json:"conditions"`
			IDs        []uint            `json:"ids"`
		}
		_ = json.Unmarshal(bodyBytes, &req)
		actionText, object = "导出", modelObject(insts, req.ModelID)
		summary = "导出" + object
		var extras []string
		if len(req.IDs) > 0 {
			extras = append(extras, "勾选 "+strconv.Itoa(len(req.IDs))+" 项")
		}
		if req.Keyword != "" {
			extras = append(extras, "关键词: "+req.Keyword)
		}
		if n := len(req.Filters) + len(req.Conditions); n > 0 {
			extras = append(extras, "筛选 "+strconv.Itoa(n)+" 项")
		}
		if len(extras) > 0 {
			summary += "（" + strings.Join(extras, "，") + "）"
		}
		if len(bodyBytes) > 0 && json.Valid(bodyBytes) && len(bodyBytes) <= 2000 {
			detail = bodyBytes
		}

	default: // 实例分页查询 POST /instances/search
		var req struct {
			ModelID    uint              `json:"model_id"`
			Keyword    string            `json:"keyword"`
			Filters    map[string]string `json:"filters"`
			Conditions []json.RawMessage `json:"conditions"`
			Assigned   string            `json:"assigned"`
			Source     string            `json:"source"`
		}
		_ = json.Unmarshal(bodyBytes, &req)
		var extras []string
		if req.Keyword != "" {
			extras = append(extras, "关键词: "+req.Keyword)
		}
		if n := len(req.Filters) + len(req.Conditions); n > 0 {
			extras = append(extras, "筛选 "+strconv.Itoa(n)+" 项")
		}
		switch req.Assigned {
		case "assigned":
			extras = append(extras, "已归属")
		case "unassigned":
			extras = append(extras, "未归属")
		}
		switch req.Source {
		case "auto":
			extras = append(extras, "来源:数据源")
		case "manual":
			extras = append(extras, "来源:用户创建")
		}
		if len(extras) == 0 { // 未搜索（无关键词/筛选）不统计
			return
		}
		actionText, object, instance = "查询", modelObject(insts, req.ModelID), req.Keyword
		summary = "查询" + object + "（" + strings.Join(extras, "，") + "）"
		if len(bodyBytes) > 0 && json.Valid(bodyBytes) && len(bodyBytes) <= 2000 {
			detail = bodyBytes
		}
	}

	db.Create(&model.AuditLog{
		Username:     c.GetString(CtxUsername),
		Action:       method,
		ActionText:   actionText,
		Object:       object,
		Instance:     instance,
		Summary:      summary,
		ResourceType: seg(parts, 1),
		ResourcePath: c.Request.URL.Path,
		Detail:       datatypes.JSON(detail),
		Status:       c.Writer.Status(),
		IP:           c.ClientIP(),
	})
}

// modelObject 由模型 id 取「<模型名>实例」，取不到用「资源实例」。
func modelObject(insts *service.InstanceService, modelID uint) string {
	obj := ""
	if insts != nil {
		obj, _ = insts.AuditTargetFromData(modelID, nil)
	}
	if obj == "" {
		obj = "资源"
	}
	return obj + "实例"
}

// deriveSemantics 根据请求方法、路径与请求体推断操作对象/动作/实例/所属业务。
func deriveSemantics(method string, parts []string, body []byte, insts *service.InstanceService) (object, actionText, instance, biz string) {
	s1, s2, s3 := seg(parts, 1), seg(parts, 2), seg(parts, 3)
	actionText = defaultAction(method)

	switch s1 {
	case "instances":
		object = "实例"
		switch {
		case s2 == "batch-delete":
			actionText = "批量删除"
			instance = batchLabel(body)
		case s2 == "import":
			actionText = "导入"
		case s2 == "" && method == http.MethodPost:
			// 新建实例：由 body 的 model_id + data 解析对象与实例
			if insts != nil {
				var req struct {
					ModelID uint            `json:"model_id"`
					Data    json.RawMessage `json:"data"`
				}
				_ = json.Unmarshal(body, &req)
				obj, inst := insts.AuditTargetFromData(req.ModelID, req.Data)
				if obj != "" {
					object = obj + "实例"
				}
				instance = inst
			}
		}
	case "instance-relations":
		object = "实例关联"
		if method == http.MethodPost {
			actionText = "新增关联"
		} else if method == http.MethodDelete {
			actionText = "删除关联"
		}
	case "models":
		if s3 == "attributes" || s2 == "attributes" {
			object = "字段"
		} else {
			object = "模型"
			instance = strField(body, "name")
		}
	case "model-groups":
		object = "模型分组"
		instance = strField(body, "name")
	case "attributes":
		object = "字段"
	case "model-associations":
		object = "模型关联"
	case "association-kinds":
		object = "关联类型"
	case "topo":
		switch s2 {
		case "business":
			object = "业务"
			instance = strField(body, "name")
			biz = instance
		case "sets":
			object = "集群"
			instance = strField(body, "name")
		case "modules":
			object = "模块"
			instance = strField(body, "name")
		case "assign":
			object, actionText = "资源归属", "分配资源"
		case "transfer":
			object, actionText = "资源归属", "转移资源"
		case "recycle":
			object, actionText = "资源归属", "回收资源"
		}
	case "users":
		object = "用户"
		instance = strField(body, "username")
		if s3 == "roles" {
			object, actionText = "用户角色", "分配角色"
		} else if s3 == "password" {
			actionText = "重置密码"
		}
	case "roles":
		object = "角色"
		instance = strField(body, "name")
		if s3 == "permissions" {
			object, actionText = "角色权限", "配置权限"
		}
	case "audit":
		object, actionText = "审计日志", "清理"
	}
	return
}

func defaultAction(method string) string {
	switch method {
	case http.MethodPost:
		return "新增"
	case http.MethodPut, http.MethodPatch:
		return "编辑"
	case http.MethodDelete:
		return "删除"
	default:
		return method
	}
}

func buildSummary(actionText, object, instance string) string {
	s := actionText + object
	if instance != "" {
		s += "「" + instance + "」"
	}
	return s
}

// strField 从 JSON 请求体取某字符串字段。
func strField(body []byte, key string) string {
	if len(body) == 0 {
		return ""
	}
	var m map[string]interface{}
	if json.Unmarshal(body, &m) != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// batchLabel 由批量删除请求体的 ids 生成「N 项」标签。
func batchLabel(body []byte) string {
	if ids := batchIDs(body); len(ids) > 0 {
		return strconv.Itoa(len(ids)) + " 项"
	}
	return ""
}

// batchIDs 解析批量删除请求体中的实例 id 列表。
func batchIDs(body []byte) []uint {
	var m struct {
		IDs []uint `json:"ids"`
	}
	if json.Unmarshal(body, &m) == nil {
		return m.IDs
	}
	return nil
}

func seg(parts []string, i int) string {
	if i < len(parts) {
		return parts[i]
	}
	return ""
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
