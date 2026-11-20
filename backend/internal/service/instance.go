package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ValidationError 表示实例数据校验失败（映射为 HTTP 400）。
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func newValidationErr(format string, a ...interface{}) error {
	return &ValidationError{Msg: fmt.Sprintf(format, a...)}
}

var safePropRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// ipPropRe 识别 IP 类字段（按 property_id 命名，含内网/公网/外网/VIP 等）。
var ipPropRe = regexp.MustCompile(`(^|_)ip$|innerip|outerip|ipaddr|ip_addr|public_?ip|pub_?ip|wan_?ip|(^|_)vip$|(^|_)eip$`)

// isIPField 判断字段是否为 IP 类（文本字段 + 命名像 IP，或字段名含「IP」）。
func isIPField(a model.Attribute) bool {
	if a.Type != model.FieldSingleChar {
		return false
	}
	return ipPropRe.MatchString(a.PropertyID) || strings.Contains(a.Name, "IP")
}

// domainRe 校验域名（支持多级与 *. 泛域名，不含协议/路径/端口）。
var domainRe = regexp.MustCompile(`^(\*\.)?([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// isDomainField 判断字段是否为域名类（文本字段 + 命名含 domain 或字段名含「域名」）。
// 排除「持有者/所有者/负责人/联系人」等——其值是人或组织名、而非域名本身，不做域名校验。
func isDomainField(a model.Attribute) bool {
	if a.Type != model.FieldSingleChar {
		return false
	}
	name, prop := a.Name, strings.ToLower(a.PropertyID)
	// 邮箱模型「邮箱域名」为手工配置字段，值可能是邮箱地址/服务器主机/内网域名等，不做严格域名校验
	if prop == "mail_domain" {
		return false
	}
	for _, kw := range []string{"持有者", "持有人", "所有者", "负责人", "联系人"} {
		if strings.Contains(name, kw) {
			return false
		}
	}
	for _, kw := range []string{"owner", "holder", "registrant", "contact"} {
		if strings.Contains(prop, kw) {
			return false
		}
	}
	return strings.Contains(prop, "domain") || strings.Contains(name, "域名")
}

// emailRe 校验邮箱格式。
var emailRe = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// isEmailField 判断字段是否为邮箱类（文本字段 + 命名含 email/mail 或字段名含「邮箱」/「邮件」）。
func isEmailField(a model.Attribute) bool {
	if a.Type != model.FieldSingleChar {
		return false
	}
	// 「邮箱域名」是域名而非邮箱地址，且 property 含 mail、名含邮箱会误触，排除
	if a.PropertyID == "mail_domain" {
		return false
	}
	return strings.Contains(a.PropertyID, "email") || strings.Contains(a.PropertyID, "mail") ||
		strings.Contains(a.Name, "邮箱") || strings.Contains(a.Name, "邮件")
}

// InstanceService 实例管理业务逻辑。
type InstanceService struct {
	db *gorm.DB
}

func NewInstanceService(db *gorm.DB) *InstanceService {
	return &InstanceService{db: db}
}

func (s *InstanceService) exists(dst interface{}, id uint) bool {
	var cnt int64
	s.db.Model(dst).Where("id = ?", id).Count(&cnt)
	return cnt > 0
}

// ListResult 分页列表结果。
type ListResult struct {
	List     []model.Instance `json:"list"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

func (s *InstanceService) loadAttributes(modelID uint) ([]model.Attribute, error) {
	var attrs []model.Attribute
	err := s.db.Where("model_id = ?", modelID).Find(&attrs).Error
	return attrs, err
}

// validate 按模型字段定义校验并规范化数据（仅保留已定义字段）。
func (s *InstanceService) validate(modelID uint, data map[string]interface{}, excludeID uint, sourceID uint) (datatypes.JSON, error) {
	attrs, err := s.loadAttributes(modelID)
	if err != nil {
		return nil, err
	}
	normalized := map[string]interface{}{}
	for _, a := range attrs {
		val, has := data[a.PropertyID]
		// 停用字段不强制必填/唯一，但已有数据仍保留
		if a.Enabled && a.Required && (!has || isEmptyVal(val)) {
			return nil, newValidationErr("字段「%s」为必填项", a.Name)
		}
		if !has || val == nil {
			continue
		}
		if err := validateType(a, val); err != nil {
			return nil, err
		}
		// IP 类字段格式校验（IPv4/IPv6）
		if a.Enabled && isIPField(a) {
			sv, _ := val.(string)
			// 允许逗号分隔的多个 IP（如 ALB 多可用区 EIP），逐段校验
			for _, part := range strings.Split(sv, ",") {
				p := strings.TrimSpace(part)
				if p != "" && net.ParseIP(p) == nil {
					return nil, newValidationErr("字段「%s」不是合法的 IP 地址（IPv4/IPv6）", a.Name)
				}
			}
		}
		// 域名字段格式校验
		if a.Enabled && isDomainField(a) {
			sv, _ := val.(string)
			if !domainRe.MatchString(strings.TrimSpace(sv)) {
				return nil, newValidationErr("字段「%s」不是合法的域名", a.Name)
			}
		}
		// 邮箱字段格式校验
		if a.Enabled && isEmailField(a) {
			sv, _ := val.(string)
			if !emailRe.MatchString(strings.TrimSpace(sv)) {
				return nil, newValidationErr("字段「%s」不是合法的邮箱", a.Name)
			}
		}
		if a.Enabled && a.Unique {
			ok, err := s.checkUnique(modelID, a.PropertyID, val, excludeID, sourceID)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, newValidationErr("字段「%s」的值已存在", a.Name)
			}
		}
		normalized[a.PropertyID] = val
	}
	b, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(b), nil
}

func isEmptyVal(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []interface{}:
		return len(x) == 0
	}
	return false
}

func validateType(a model.Attribute, val interface{}) error {
	switch a.Type {
	case model.FieldInt, model.FieldFloat:
		if _, ok := val.(float64); !ok {
			return newValidationErr("字段「%s」应为数字", a.Name)
		}
	case model.FieldBool:
		if _, ok := val.(bool); !ok {
			return newValidationErr("字段「%s」应为布尔值", a.Name)
		}
	case model.FieldEnum:
		sv, ok := val.(string)
		if !ok || !contains(optionIDs(a.Options), sv) {
			return newValidationErr("字段「%s」的值不在可选范围内", a.Name)
		}
	case model.FieldEnumMulti:
		arr, ok := val.([]interface{})
		if !ok {
			return newValidationErr("字段「%s」应为数组", a.Name)
		}
		ids := optionIDs(a.Options)
		for _, item := range arr {
			sv, ok := item.(string)
			if !ok || !contains(ids, sv) {
				return newValidationErr("字段「%s」含非法选项", a.Name)
			}
		}
	}
	return nil
}

func optionIDs(opts datatypes.JSON) []string {
	var list []struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(opts, &list)
	ids := make([]string, 0, len(list))
	for _, o := range list {
		ids = append(ids, o.ID)
	}
	return ids
}

func contains(arr []string, v string) bool {
	for _, x := range arr {
		if x == v {
			return true
		}
	}
	return false
}

// checkUnique 唯一性校验，作用域限定在同一来源(source_id)内：同数据源不可重复，跨数据源可重复。
func (s *InstanceService) checkUnique(modelID uint, prop string, val interface{}, excludeID uint, sourceID uint) (bool, error) {
	var cnt int64
	q := s.db.Model(&model.Instance{}).
		Where("model_id = ? AND source_id = ?", modelID, sourceID).
		Where("JSON_UNQUOTE(JSON_EXTRACT(data, ?)) = ?", "$."+prop, fmt.Sprintf("%v", val))
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	if err := q.Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt == 0, nil
}

// List 分页查询实例，支持字段精确筛选与全字段关键字模糊。
// Condition 高级筛选条件（带操作符）。
type Condition struct {
	Field  string   `json:"field"`
	Op     string   `json:"op"`     // eq/ne/contains/gt/lt/between/daterange/in
	Value  string   `json:"value"`  // 单值 / 区间起
	Value2 string   `json:"value2"` // 区间止
	Values []string `json:"values"` // in 多值
}

// applyQuery 把关键字、等值 filters 与高级 conditions 应用到查询（AND 组合）。
func (s *InstanceService) applyQuery(q *gorm.DB, keyword string, filters map[string]string, conditions []Condition) *gorm.DB {
	for k, v := range filters {
		if !safePropRe.MatchString(k) || v == "" {
			continue
		}
		q = q.Where("JSON_UNQUOTE(JSON_EXTRACT(data, ?)) = ?", "$."+k, v)
	}
	for _, c := range conditions {
		q = applyCondition(q, c)
	}
	if keyword != "" {
		q = q.Where("JSON_SEARCH(data, 'one', ?) IS NOT NULL", "%"+keyword+"%")
	}
	return q
}

// applyCondition 按操作符构建单条件。非法字段或缺值时原样返回。
func applyCondition(q *gorm.DB, c Condition) *gorm.DB {
	if !safePropRe.MatchString(c.Field) {
		return q
	}
	path := "$." + c.Field
	str := "JSON_UNQUOTE(JSON_EXTRACT(data, ?))"
	num := "CAST(JSON_UNQUOTE(JSON_EXTRACT(data, ?)) AS DECIMAL(30,6))"
	switch c.Op {
	case "", "eq":
		if c.Value == "" {
			return q
		}
		return q.Where(str+" = ?", path, c.Value)
	case "ne":
		if c.Value == "" {
			return q
		}
		return q.Where(str+" <> ?", path, c.Value)
	case "contains":
		if c.Value == "" {
			return q
		}
		return q.Where(str+" LIKE ?", path, "%"+c.Value+"%")
	case "gt":
		if c.Value == "" {
			return q
		}
		return q.Where(num+" > ?", path, c.Value)
	case "lt":
		if c.Value == "" {
			return q
		}
		return q.Where(num+" < ?", path, c.Value)
	case "between":
		if c.Value == "" || c.Value2 == "" {
			return q
		}
		return q.Where(num+" BETWEEN ? AND ?", path, c.Value, c.Value2)
	case "daterange":
		if c.Value == "" || c.Value2 == "" {
			return q
		}
		return q.Where(str+" >= ? AND "+str+" <= ?", path, c.Value, path, c.Value2)
	case "in":
		if len(c.Values) == 0 {
			return q
		}
		return q.Where(str+" IN ?", path, c.Values)
	}
	return q
}

func (s *InstanceService) List(modelID uint, page, pageSize int, keyword string, filters map[string]string, conditions []Condition, assigned, source, biz string) (*ListResult, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 20
	}
	q := s.db.Model(&model.Instance{}).Where("model_id = ?", modelID)
	q = s.applyQuery(q, keyword, filters, conditions)
	q = s.applyAssigned(q, assigned)
	q = s.applySourceBiz(q, source, biz)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	// 排序：主机按内网IP升序（INET6_ATON 保证数值序，兼容 IPv4/IPv6；非法/空值 NULL 排最前）；
	// 其它模型默认按 id 倒序（最新创建在前）。
	order := "id desc"
	if modelID == s.hostModelID() {
		order = "INET6_ATON(JSON_UNQUOTE(JSON_EXTRACT(data, '$.bk_host_innerip'))) asc, id asc"
	}
	var list []model.Instance
	if err := q.Order(order).Limit(pageSize).Offset((page - 1) * pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	s.fillBizLine(modelID, list)
	s.fillSourceName(list)
	return &ListResult{List: list, Total: total, Page: page, PageSize: pageSize}, nil
}

// applyAssigned 按「业务线归属」过滤（以手填 data.biz_line 为准，与拓扑 host_module 解耦），供列表与导出共用：
//   - assigned:   已填业务线（非空且非 null/0）
//   - unassigned: 未填业务线（空/null/0）
func (s *InstanceService) applyAssigned(q *gorm.DB, assigned string) *gorm.DB {
	bizExpr := "JSON_UNQUOTE(JSON_EXTRACT(data, '$.biz_line'))"
	switch assigned {
	case "assigned":
		q = q.Where("(" + bizExpr + " IS NOT NULL AND " + bizExpr + " NOT IN ('', 'null', '0'))")
	case "unassigned":
		q = q.Where("(" + bizExpr + " IS NULL OR " + bizExpr + " IN ('', 'null', '0'))")
	}
	return q
}

// applySourceBiz 叠加「数据源」与「业务线」筛选，供列表与导出共用，保证导出与所见一致。
//   - source: manual=手动(source_id=0)，auto=全部自动同步(source_id>0)，数字=指定数据源 id
//   - biz:    业务线实例 id，按实例「业务线」字段值(data.biz_line，枚举存业务线id)过滤；空或 0 表示不限
func (s *InstanceService) applySourceBiz(q *gorm.DB, source, biz string) *gorm.DB {
	if bizID, err := strconv.ParseUint(biz, 10, 64); err == nil && bizID > 0 {
		q = q.Where("JSON_UNQUOTE(JSON_EXTRACT(data, '$.biz_line')) = ?", biz)
	}
	switch source {
	case "auto":
		q = q.Where("source_id > 0")
	case "manual":
		q = q.Where("source_id = 0")
	default:
		if sid, err := strconv.ParseUint(source, 10, 64); err == nil && sid > 0 {
			q = q.Where("source_id = ?", sid)
		}
	}
	return q
}

// fillSourceName 按 source_id 关联数据源，标注来源名与类型（改名后仍准确）。
func (s *InstanceService) fillSourceName(list []model.Instance) {
	if len(list) == 0 {
		return
	}
	var dss []model.DataSource
	s.db.Select("id", "name", "type").Find(&dss)
	byID := map[uint]model.DataSource{}
	for _, d := range dss {
		byID[d.ID] = d
	}
	for i := range list {
		if d, ok := byID[list[i].SourceID]; ok {
			list[i].SourceName = d.Name
			list[i].SourceType = d.Type
		}
	}
}

func (s *InstanceService) Get(id uint) (*model.Instance, error) {
	var inst model.Instance
	if err := s.db.First(&inst, id).Error; err != nil {
		return nil, ErrNotFound
	}
	list := []model.Instance{inst}
	s.fillBizLine(inst.ModelID, list)
	return &list[0], nil
}

func (s *InstanceService) hostModelID() uint {
	var m model.Model
	s.db.Where("obj_id = ?", "host").First(&m)
	return m.ID
}

func (s *InstanceService) bizModelID() uint {
	var m model.Model
	s.db.Where("obj_id = ?", "biz").First(&m)
	return m.ID
}

// fillBizLine 填充实例业务线（派生，只读）：主机取拓扑归属，其余模型取「属于业务」关联。
func (s *InstanceService) fillBizLine(modelID uint, insts []model.Instance) {
	if len(insts) == 0 {
		return
	}
	// 业务线统一来自业务拓扑归属（host_module，含主机与各类资源）
	s.fillHostBiz(insts)
}

// fillHostBiz 为主机实例填充业务线（派生自拓扑归属 HostModule → 业务实例名）。
func (s *InstanceService) fillHostBiz(insts []model.Instance) {
	if len(insts) == 0 {
		return
	}
	hostIDs := make([]uint, 0, len(insts))
	for i := range insts {
		hostIDs = append(hostIDs, insts[i].ID)
	}
	var hms []model.HostModule
	s.db.Where("host_id IN ?", hostIDs).Find(&hms)
	if len(hms) == 0 {
		return
	}
	hostBiz := map[uint]uint{}
	bizSet := map[uint]bool{}
	for _, h := range hms {
		hostBiz[h.HostID] = h.BizID
		bizSet[h.BizID] = true
	}
	bizIDs := make([]uint, 0, len(bizSet))
	for id := range bizSet {
		bizIDs = append(bizIDs, id)
	}
	var bizInsts []model.Instance
	s.db.Where("id IN ?", bizIDs).Find(&bizInsts)
	nameMap := map[uint]string{}
	for _, b := range bizInsts {
		nameMap[b.ID] = bizName(b.Data)
	}
	for i := range insts {
		if bid, ok := hostBiz[insts[i].ID]; ok {
			insts[i].BizLine = nameMap[bid]
		}
	}
}

// AuditTarget 返回某实例的审计展示信息：所属模型名（操作对象）、实例摘要、所属业务。
// 用于在删除/修改前捕获展示信息。实例不存在时返回空串。
func (s *InstanceService) AuditTarget(instID uint) (object, instance, biz string) {
	var inst model.Instance
	if err := s.db.First(&inst, instID).Error; err != nil {
		return
	}
	object = s.modelName(inst.ModelID)
	if attrs, err := s.attributesSorted(inst.ModelID); err == nil {
		instance = summarize(inst.Data, attrs)
	}
	list := []model.Instance{inst}
	s.fillHostBiz(list)
	biz = list[0].BizLine
	return
}

// AuditBatchObject 返回这批实例所属的模型名（用于批量删除审计显示）；
// 跨多个模型时在名称后加「等」。需在删除前调用（删除后查不到）。
func (s *InstanceService) AuditBatchObject(ids []uint) string {
	if len(ids) == 0 {
		return ""
	}
	var mids []uint
	if err := s.db.Model(&model.Instance{}).Where("id IN ?", ids).
		Distinct().Pluck("model_id", &mids).Error; err != nil || len(mids) == 0 {
		return ""
	}
	name := s.modelName(mids[0])
	if len(mids) > 1 {
		name += "等"
	}
	return name
}

// AuditTargetFromData 由模型 id 与实例数据 JSON 生成审计展示信息（用于新建时，实例尚无 id）。
func (s *InstanceService) AuditTargetFromData(modelID uint, dataJSON []byte) (object, instance string) {
	object = s.modelName(modelID)
	if attrs, err := s.attributesSorted(modelID); err == nil {
		instance = summarize(dataJSON, attrs)
	}
	return
}

func (s *InstanceService) Create(modelID uint, data map[string]interface{}, user string) (*model.Instance, error) {
	if !s.exists(&model.Model{}, modelID) {
		return nil, ErrInvalidRef
	}
	jsonData, err := s.validate(modelID, data, 0, 0) // 手动创建 source_id=0
	if err != nil {
		return nil, err
	}
	inst := &model.Instance{ModelID: modelID, Data: jsonData, Creator: user, Updater: user}
	if err := s.db.Create(inst).Error; err != nil {
		return nil, err
	}
	if modelID == s.bizModelID() {
		_ = SyncBizLineOptions(s.db) // 业务线新增，刷新各模型业务线字段下拉选项
	}
	return inst, nil
}

// HostModelID 导出主机模型 id（供采集等处做权限校验）。
func (s *InstanceService) HostModelID() uint { return s.hostModelID() }

// CollectHost 按内网IP对主机实例做 upsert（sourceID=来源数据源 id，0=采集脚本/手动）。
func (s *InstanceService) CollectHost(data map[string]interface{}, user string, sourceID uint) (*model.Instance, bool, error) {
	hostID := s.hostModelID()
	if hostID == 0 {
		return nil, false, newValidationErr("主机模型不存在")
	}
	return s.UpsertInstance(hostID, "bk_host_innerip", data, user, sourceID)
}

// CollectCloudHost 面向云厂商主机（如阿里云 ECS）的 upsert：以云实例ID(resource_id)为唯一标识，
// 内网IP变更也能命中同一台，不会重复建。迁移兼容见 CollectCloudInstance。
func (s *InstanceService) CollectCloudHost(data map[string]interface{}, user string, sourceID uint) (*model.Instance, bool, error) {
	hostID := s.hostModelID()
	if hostID == 0 {
		return nil, false, newValidationErr("主机模型不存在")
	}
	return s.CollectCloudInstance(hostID, "bk_host_innerip", data, user, sourceID)
}

// CollectCloudInstance 面向云厂商资源（阿里云 ECS/RDS 等）的通用 upsert：以云实例ID(resource_id)
// 为唯一标识，实例名/IP 变更也能命中同一条、不会重复建。迁移兼容：按 resource_id 找不到时，回退按
// 同源 fallbackProp（主机=内网IP、RDS=实例名）认领一条尚无 resource_id 的历史记录并补写 resource_id。
// resource_id 为空时退回按 fallbackProp 匹配（原规则）。
func (s *InstanceService) CollectCloudInstance(modelID uint, fallbackProp string, data map[string]interface{}, user string, sourceID uint) (*model.Instance, bool, error) {
	if modelID == 0 {
		return nil, false, newValidationErr("模型不存在")
	}
	if !safePropRe.MatchString(fallbackProp) {
		return nil, false, newValidationErr("非法匹配字段")
	}
	rid, _ := data["resource_id"].(string)
	if strings.TrimSpace(rid) == "" {
		// 无实例ID，按原规则（fallbackProp + 数据源）处理
		return s.UpsertInstance(modelID, fallbackProp, data, user, sourceID)
	}
	// 1) 优先按 (模型, resource_id, 数据源) 定位
	var inst model.Instance
	err := s.db.Where("model_id = ? AND source_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, '$.resource_id')) = ?", modelID, sourceID, rid).
		First(&inst).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 2) 迁移兼容：按同源 fallbackProp 认领一条尚无 resource_id 的历史记录
		if fv, _ := data[fallbackProp].(string); strings.TrimSpace(fv) != "" {
			e := s.db.Where("model_id = ? AND source_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, ?)) = ?", modelID, sourceID, "$."+fallbackProp, fv).
				Where("JSON_EXTRACT(data, '$.resource_id') IS NULL OR JSON_UNQUOTE(JSON_EXTRACT(data, '$.resource_id')) = ''").
				First(&inst).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return nil, false, e
			}
			if e != nil { // 仍找不到 → 新建
				return s.createHost(modelID, data, user, sourceID)
			}
		} else {
			return s.createHost(modelID, data, user, sourceID)
		}
	}
	// 命中（resource_id 或 fallbackProp 认领）→ 合并字段更新
	var m map[string]interface{}
	if json.Unmarshal(inst.Data, &m) != nil || m == nil {
		m = map[string]interface{}{}
	}
	for k, v := range data {
		m[k] = v
	}
	jsonData, e := s.validate(modelID, m, inst.ID, sourceID)
	if e != nil {
		return nil, false, e
	}
	inst.Data = jsonData
	inst.Updater = user
	inst.Creator = user // 同源：创建人随数据源名更新（支持改名平滑衔接）
	if e := s.db.Save(&inst).Error; e != nil {
		return nil, false, e
	}
	return &inst, false, nil
}

// createHost 校验并新建一条实例（供 CollectCloudInstance 使用）。
func (s *InstanceService) createHost(modelID uint, data map[string]interface{}, user string, sourceID uint) (*model.Instance, bool, error) {
	jsonData, e := s.validate(modelID, data, 0, sourceID)
	if e != nil {
		return nil, false, e
	}
	ni := &model.Instance{ModelID: modelID, Data: jsonData, Creator: user, Updater: user, SourceID: sourceID}
	if e := s.db.Create(ni).Error; e != nil {
		return nil, false, e
	}
	return ni, true, nil
}

// CleanupOfflineInstances 删除「已下线」且超过 days 天未更新的实例（级联清理归属/关联）。
func (s *InstanceService) CleanupOfflineInstances(days int) (int, error) {
	if days <= 0 {
		days = 7
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	var insts []model.Instance
	err := s.db.Where("updated_at < ?", cutoff).
		Where("JSON_UNQUOTE(JSON_EXTRACT(data, '$.status')) = ? OR JSON_UNQUOTE(JSON_EXTRACT(data, '$.bk_host_status')) = ?",
			model.OfflineStatusID, model.OfflineStatusID).
		Find(&insts).Error
	if err != nil {
		return 0, err
	}
	n := 0
	for _, inst := range insts {
		if inst.ModelID == s.bizModelID() {
			continue
		}
		if s.Delete(inst.ID) == nil {
			n++
		}
	}
	return n, nil
}

// statusProp 返回模型的状态枚举字段 property_id（status/bk_host_status），无则空。
func (s *InstanceService) statusProp(modelID uint) string {
	var a model.Attribute
	if s.db.Where("model_id = ? AND type = ? AND property_id IN ?", modelID, model.FieldEnum, []string{"status", "bk_host_status"}).First(&a).Error == nil {
		return a.PropertyID
	}
	return ""
}

// MarkOffline 将某数据源同步的、本次未出现(seen 之外)的实例状态置为「已下线」。
// 仅限 creator=该数据源、指定模型；不删除、不动归属。返回标记数。
func (s *InstanceService) MarkOffline(modelID uint, sourceID uint, seen []uint) (int64, error) {
	prop := s.statusProp(modelID)
	if prop == "" || sourceID == 0 {
		return 0, nil
	}
	path := "$." + prop
	q := s.db.Model(&model.Instance{}).
		Where("model_id = ? AND source_id = ?", modelID, sourceID).
		Where("JSON_UNQUOTE(JSON_EXTRACT(data, ?)) <> ?", path, model.OfflineStatusID)
	if len(seen) > 0 {
		q = q.Where("id NOT IN ?", seen)
	}
	r := q.UpdateColumn("data", gorm.Expr("JSON_SET(data, ?, ?)", path, model.OfflineStatusID))
	return r.RowsAffected, r.Error
}

// ModelIDByObjID 按 obj_id 取模型 id（找不到返回 0）。
func (s *InstanceService) ModelIDByObjID(objID string) uint {
	var m model.Model
	if s.db.Where("obj_id = ?", objID).First(&m).Error != nil {
		return 0
	}
	return m.ID
}

// FindInstanceIDByProp 在指定模型内按某字段值定位实例（有 source_id 则限定数据源作用域），找不到返回 0。
func (s *InstanceService) FindInstanceIDByProp(modelID uint, prop, value string, sourceID uint) uint {
	if modelID == 0 || strings.TrimSpace(value) == "" || !safePropRe.MatchString(prop) {
		return 0
	}
	var inst model.Instance
	q := s.db.Model(&model.Instance{}).
		Where("model_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, ?)) = ?", modelID, "$."+prop, value)
	if sourceID > 0 {
		q = q.Where("source_id = ?", sourceID)
	}
	if q.Select("id").First(&inst).Error != nil {
		return 0
	}
	return inst.ID
}

// AssocIDByCode 按关联编码取模型关联 id（找不到返回 0）。
func (s *InstanceService) AssocIDByCode(code string) uint {
	var a model.ModelAssociation
	if s.db.Where("code = ?", code).First(&a).Error != nil {
		return 0
	}
	return a.ID
}

// EnsureRelation 幂等创建实例关联（已存在则跳过）；任一 id 为 0 时静默跳过。
func (s *InstanceService) EnsureRelation(assocID, src, dst uint) error {
	if assocID == 0 || src == 0 || dst == 0 {
		return nil
	}
	var existing model.InstanceRelation
	if s.db.Where("association_id = ? AND src_inst_id = ? AND dst_inst_id = ?", assocID, src, dst).First(&existing).Error == nil {
		return nil
	}
	return s.db.Create(&model.InstanceRelation{AssociationID: assocID, SrcInstID: src, DstInstID: dst}).Error
}

// UpsertInstance 按某唯一字段对指定模型做 upsert：存在则合并字段更新（保留手工字段），不存在则新建。
// 创建人跟随：原创建人非真实登录用户时随 user 更新，人工创建者保留。
func (s *InstanceService) UpsertInstance(modelID uint, matchProp string, data map[string]interface{}, user string, sourceID uint) (*model.Instance, bool, error) {
	if modelID == 0 {
		return nil, false, newValidationErr("模型不存在")
	}
	if !safePropRe.MatchString(matchProp) {
		return nil, false, newValidationErr("非法匹配字段")
	}
	val, _ := data[matchProp].(string)
	if strings.TrimSpace(val) == "" {
		return nil, false, newValidationErr("缺少匹配字段 %s", matchProp)
	}
	// 唯一作用域 = (模型, 匹配字段, 数据源)：有 source_id 按它定位（改名不影响）；否则按 creator
	var inst model.Instance
	q := s.db.Where("model_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, ?)) = ?", modelID, "$."+matchProp, val)
	if sourceID > 0 {
		q = q.Where("source_id = ?", sourceID)
	} else {
		q = q.Where("creator = ? AND source_id = 0", user)
	}
	err := q.First(&inst).Error
	if err == nil {
		var m map[string]interface{}
		if json.Unmarshal(inst.Data, &m) != nil || m == nil {
			m = map[string]interface{}{}
		}
		for k, v := range data {
			m[k] = v
		}
		jsonData, e := s.validate(modelID, m, inst.ID, sourceID)
		if e != nil {
			return nil, false, e
		}
		inst.Data = jsonData
		inst.Updater = user
		// 同源：创建人随当前数据源名更新（支持数据源改名平滑衔接）
		if sourceID > 0 {
			inst.Creator = user
		}
		if e := s.db.Save(&inst).Error; e != nil {
			return nil, false, e
		}
		return &inst, false, nil
	}
	jsonData, e := s.validate(modelID, data, 0, sourceID)
	if e != nil {
		return nil, false, e
	}
	ni := &model.Instance{ModelID: modelID, Data: jsonData, Creator: user, Updater: user, SourceID: sourceID}
	if e := s.db.Create(ni).Error; e != nil {
		return nil, false, e
	}
	return ni, true, nil
}

// Update 更新实例。返回变更明细（"字段名: 旧 → 新"）；无任何变更时不写库，changes 为空。
func (s *InstanceService) Update(id uint, data map[string]interface{}, user string) (*model.Instance, []string, error) {
	var inst model.Instance
	if err := s.db.First(&inst, id).Error; err != nil {
		return nil, nil, ErrNotFound
	}
	jsonData, err := s.validate(inst.ModelID, data, id, inst.SourceID)
	if err != nil {
		return nil, nil, err
	}
	changes := s.diffData(inst.ModelID, inst.Data, jsonData)
	if len(changes) == 0 {
		return &inst, nil, nil // 无变更，不写库
	}
	inst.Data = jsonData
	inst.Updater = user
	if err := s.db.Save(&inst).Error; err != nil {
		return nil, nil, err
	}
	if inst.ModelID == s.bizModelID() {
		_ = SyncBizLineOptions(s.db) // 业务线改名等，刷新各模型业务线字段下拉选项
	}
	return &inst, changes, nil
}

// diffData 比较新旧数据，返回按字段的可读变更（仅模型已定义字段）。
func (s *InstanceService) diffData(modelID uint, oldRaw, newRaw []byte) []string {
	var oldMap, newMap map[string]interface{}
	_ = json.Unmarshal(oldRaw, &oldMap)
	_ = json.Unmarshal(newRaw, &newMap)
	attrs, _ := s.attributesSorted(modelID)
	changes := []string{}
	for _, a := range attrs {
		ov, nv := oldMap[a.PropertyID], newMap[a.PropertyID]
		if fmt.Sprintf("%v", ov) == fmt.Sprintf("%v", nv) {
			continue
		}
		changes = append(changes, fmt.Sprintf("%s: %s → %s", a.Name, displayValue(a, ov), displayValue(a, nv)))
	}
	return changes
}

func (s *InstanceService) Delete(id uint) error {
	var inst model.Instance
	if err := s.db.First(&inst, id).Error; err != nil {
		return ErrNotFound
	}
	if inst.ModelID == s.bizModelID() {
		return newValidationErr("业务请在「业务拓扑」中删除")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("src_inst_id = ? OR dst_inst_id = ?", id, id).Delete(&model.InstanceRelation{}).Error; err != nil {
			return err
		}
		// 清理业务拓扑归属，避免拓扑计数残留
		if err := tx.Where("host_id = ?", id).Delete(&model.HostModule{}).Error; err != nil {
			return err
		}
		return tx.Delete(&inst).Error
	})
}

func (s *InstanceService) BatchDelete(ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var bizCnt int64
	s.db.Model(&model.Instance{}).Where("id IN ? AND model_id = ?", ids, s.bizModelID()).Count(&bizCnt)
	if bizCnt > 0 {
		return 0, newValidationErr("选中项包含业务，业务请在「业务拓扑」中删除")
	}
	var deleted int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("src_inst_id IN ? OR dst_inst_id IN ?", ids, ids).Delete(&model.InstanceRelation{}).Error; err != nil {
			return err
		}
		// 清理业务拓扑归属，避免拓扑计数残留
		if err := tx.Where("host_id IN ?", ids).Delete(&model.HostModule{}).Error; err != nil {
			return err
		}
		res := tx.Where("id IN ?", ids).Delete(&model.Instance{})
		if res.Error != nil {
			return res.Error
		}
		deleted = res.RowsAffected
		return nil
	})
	return deleted, err
}

// ---------------- 实例关联 ----------------

func (s *InstanceService) CreateRelation(assocID, src, dst uint) (*model.InstanceRelation, error) {
	if !s.exists(&model.ModelAssociation{}, assocID) {
		return nil, ErrInvalidRef
	}
	if !s.exists(&model.Instance{}, src) || !s.exists(&model.Instance{}, dst) {
		return nil, ErrInvalidRef
	}
	// 去重：同一关联类型下同一对 (src,dst) 不可重复关联
	var dup int64
	s.db.Model(&model.InstanceRelation{}).
		Where("association_id = ? AND src_inst_id = ? AND dst_inst_id = ?", assocID, src, dst).
		Count(&dup)
	if dup > 0 {
		return nil, newValidationErr("该关联已存在，请勿重复关联")
	}
	rel := &model.InstanceRelation{AssociationID: assocID, SrcInstID: src, DstInstID: dst}
	if err := s.db.Create(rel).Error; err != nil {
		return nil, err
	}
	return rel, nil
}

func (s *InstanceService) DeleteRelation(id uint) error {
	var rel model.InstanceRelation
	if err := s.db.First(&rel, id).Error; err != nil {
		return ErrNotFound
	}
	return s.db.Delete(&rel).Error
}

// GetRelation 按 id 读取单条实例关联（用于权限校验等）。
func (s *InstanceService) GetRelation(id uint) (*model.InstanceRelation, error) {
	var rel model.InstanceRelation
	if err := s.db.First(&rel, id).Error; err != nil {
		return nil, ErrNotFound
	}
	return &rel, nil
}

// RelationView 面向某实例视角的一条关联（含关系语义与对端实例展示）。
type RelationView struct {
	ID            uint   `json:"id"`
	AssociationID uint   `json:"association_id"`
	Direction     string `json:"direction"` // out=当前实例为源, in=当前实例为目标
	KindName      string `json:"kind_name"`
	Note          string `json:"note"` // 从当前实例看向对端的关系描述
	PeerInstID    uint   `json:"peer_inst_id"`
	PeerModelID   uint   `json:"peer_model_id"`
	PeerModelName string `json:"peer_model_name"`
	PeerDisplay   string `json:"peer_display"`
	PeerStatus    string `json:"peer_status"`   // 对端实例状态名（运行中/异常/已下线…），无则空
	PeerAbnormal  bool   `json:"peer_abnormal"` // 对端是否异常（status=abnormal/offline）
	Derived       bool   `json:"derived"`       // true=拓扑派生的只读关联（不可删除）
}

// peerStatusInfo 从实例 data 读取 status，返回状态显示名与是否异常。
// 异常口径：非运行即异常——复用全局 classifyState（running/1/normal 为正常，其余为异常/离线）；无 status 不标。
func peerStatusInfo(data datatypes.JSON) (string, bool) {
	var m map[string]interface{}
	if json.Unmarshal(data, &m) != nil || m == nil {
		return "", false
	}
	id := optStr(m["status"])
	if id == "" {
		return "", false
	}
	return statusLabel(id), classifyState(id) != "normal"
}

// statusLabel 常见标准化状态 id → 中文名（自定义模型若用标准 id 亦可覆盖）。
func statusLabel(id string) string {
	switch id {
	case "offline":
		return "已下线"
	case "abnormal":
		return "异常"
	case "stopped":
		return "已停止"
	case "creating":
		return "创建中"
	case "running", "1", "normal":
		return "运行中"
	case "2", "disabled":
		return "停用"
	case "expired":
		return "已过期"
	case "expiring":
		return "即将过期"
	}
	return id
}

func uintKeys(m map[uint]bool) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (s *InstanceService) ListRelations(instID uint) ([]RelationView, error) {
	var rels []model.InstanceRelation
	if err := s.db.Where("src_inst_id = ? OR dst_inst_id = ?", instID, instID).Order("id asc").Find(&rels).Error; err != nil {
		return nil, err
	}
	views := make([]RelationView, 0, len(rels))
	if len(rels) == 0 {
		// 无显式关联，仍可能有拓扑派生关联
		s.appendTopoRelations(instID, &views)
		return views, nil
	}

	assocIDs := map[uint]bool{}
	peerIDs := map[uint]bool{}
	for _, r := range rels {
		assocIDs[r.AssociationID] = true
		peer := r.DstInstID
		if r.SrcInstID != instID {
			peer = r.SrcInstID
		}
		peerIDs[peer] = true
	}

	var assocs []model.ModelAssociation
	s.db.Where("id IN ?", uintKeys(assocIDs)).Find(&assocs)
	am := map[uint]model.ModelAssociation{}
	kindIDs := map[uint]bool{}
	for _, a := range assocs {
		am[a.ID] = a
		kindIDs[a.KindID] = true
	}
	var kinds []model.AssociationKind
	s.db.Where("id IN ?", uintKeys(kindIDs)).Find(&kinds)
	km := map[uint]model.AssociationKind{}
	for _, k := range kinds {
		km[k.ID] = k
	}

	var peers []model.Instance
	s.db.Where("id IN ?", uintKeys(peerIDs)).Find(&peers)
	pm := map[uint]model.Instance{}
	modelIDs := map[uint]bool{}
	for _, p := range peers {
		pm[p.ID] = p
		modelIDs[p.ModelID] = true
	}
	var models []model.Model
	s.db.Where("id IN ?", uintKeys(modelIDs)).Find(&models)
	mm := map[uint]model.Model{}
	for _, m := range models {
		mm[m.ID] = m
	}
	var attrs []model.Attribute
	s.db.Where("model_id IN ? AND enabled = ?", uintKeys(modelIDs), true).Order("`index` asc, id asc").Find(&attrs)
	attrMap := map[uint][]model.Attribute{}
	for _, at := range attrs {
		attrMap[at.ModelID] = append(attrMap[at.ModelID], at)
	}

	for _, r := range rels {
		assoc := am[r.AssociationID]
		kind := km[assoc.KindID]
		peerID := r.DstInstID
		dir := "out"
		note := kind.SrcToDstNote
		if r.SrcInstID != instID {
			peerID = r.SrcInstID
			dir = "in"
			note = kind.DstToSrcNote
		}
		peer := pm[peerID]
		pst, pab := peerStatusInfo(peer.Data)
		views = append(views, RelationView{
			ID:            r.ID,
			AssociationID: r.AssociationID,
			Direction:     dir,
			KindName:      kind.Name,
			Note:          note,
			PeerInstID:    peerID,
			PeerModelID:   peer.ModelID,
			PeerModelName: mm[peer.ModelID].Name,
			PeerDisplay:   summarize(peer.Data, attrMap[peer.ModelID]),
			PeerStatus:    pst,
			PeerAbnormal:  pab,
		})
	}
	s.appendTopoRelations(instID, &views)
	return views, nil
}

// appendTopoRelations 注入业务线派生的只读关联（业务线来自资源的 biz_line 字段，与拓扑归属解耦）：
//   - 业务线实例：追加 biz_line 指向它的全部资源（下辖资源）
//   - 其他实例：追加其 biz_line 所指的业务线（所属业务线）
func (s *InstanceService) appendTopoRelations(instID uint, views *[]RelationView) {
	var inst model.Instance
	if s.db.First(&inst, instID).Error != nil {
		return
	}
	bizModelID := s.bizModelID()

	if inst.ModelID == bizModelID {
		// 下辖资源：biz_line 字段指向本业务线的实例
		var ress []model.Instance
		s.db.Where("JSON_UNQUOTE(JSON_EXTRACT(data, '$.biz_line')) = ?", strconv.FormatUint(uint64(instID), 10)).Find(&ress)
		attrCache := map[uint][]model.Attribute{}
		for _, r := range ress {
			if _, ok := attrCache[r.ModelID]; !ok {
				attrCache[r.ModelID], _ = s.attributesSorted(r.ModelID)
			}
			dpst, dpab := peerStatusInfo(r.Data)
			*views = append(*views, RelationView{
				Direction: "out", KindName: "业务线", Note: "下辖资源",
				PeerInstID: r.ID, PeerModelID: r.ModelID, PeerModelName: s.modelName(r.ModelID),
				PeerDisplay: summarize(r.Data, attrCache[r.ModelID]), Derived: true,
				PeerStatus: dpst, PeerAbnormal: dpab,
			})
		}
		return
	}

	// 其他实例：所属业务线（读 biz_line 字段，值为业务线实例 id）
	var data map[string]interface{}
	_ = json.Unmarshal(inst.Data, &data)
	bl := optStr(data["biz_line"])
	if bl == "" {
		return
	}
	bizID, err := strconv.ParseUint(bl, 10, 64)
	if err != nil || bizID == 0 {
		return
	}
	var biz model.Instance
	if s.db.Where("id = ? AND model_id = ?", bizID, bizModelID).First(&biz).Error != nil {
		return
	}
	*views = append(*views, RelationView{
		// 所属业务线：资源「属于」业务线，箭头由资源指向业务线（out=self 为源）
		Direction: "out", KindName: "业务线", Note: "所属业务线",
		PeerInstID: biz.ID, PeerModelID: bizModelID, PeerModelName: s.modelName(bizModelID),
		PeerDisplay: bizName(biz.Data), Derived: true,
	})
}

func (s *InstanceService) modelName(id uint) string {
	var m model.Model
	s.db.Select("id, name").First(&m, id)
	return m.Name
}
