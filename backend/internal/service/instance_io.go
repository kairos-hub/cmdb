package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cmdb/cmdb-backend/internal/model"
	"github.com/xuri/excelize/v2"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SearchHit 全局搜索命中项。
type SearchHit struct {
	InstanceID uint   `json:"instance_id"`
	ModelID    uint   `json:"model_id"`
	ModelName  string `json:"model_name"`
	ModelObjID string `json:"model_obj_id"`
	Icon       string `json:"icon"`
	Display    string `json:"display"`
}

type SearchCategory struct {
	ModelID uint   `json:"model_id"`
	Name    string `json:"name"`
	Icon    string `json:"icon"`
	Count   int64  `json:"count"`
}

type GlobalSearchResult struct {
	List       []SearchHit      `json:"list"`
	Total      int64            `json:"total"`
	Categories []SearchCategory `json:"categories"`
	Page       int              `json:"page"`
}

// GlobalSearch 在用户可查看的模型中检索。分类计数涵盖全部命中，列表按所选模型分页。
func (s *InstanceService) GlobalSearch(keyword, mode string, page, pageSize int, modelID uint, permissions UserPermissions, paged bool) (*GlobalSearchResult, error) {
	result := &GlobalSearchResult{List: []SearchHit{}, Categories: []SearchCategory{}, Page: 1}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return result, nil
	}

	q := s.db.Model(&model.Instance{})
	if !permissions.IsAdmin && !contains(permissions.Permissions[0], model.ActionView) {
		allowed := make([]uint, 0, len(permissions.Permissions))
		for id, actions := range permissions.Permissions {
			if contains(actions, model.ActionView) {
				allowed = append(allowed, id)
			}
		}
		if len(allowed) == 0 {
			return result, nil
		}
		q = q.Where("model_id IN ?", allowed)
	}
	if mode == "exact" {
		q = q.Where("JSON_SEARCH(data, 'one', ?) IS NOT NULL", keyword)
	} else {
		q = q.Where("CAST(data AS CHAR) LIKE ?", "%"+keyword+"%")
	}

	models := map[uint]model.Model{}
	if paged {
		if err := q.Session(&gorm.Session{}).Select("model_id, COUNT(*) AS count").
			Group("model_id").Order("model_id").Scan(&result.Categories).Error; err != nil {
			return nil, err
		}
		categoryIDs := make([]uint, 0, len(result.Categories))
		for _, category := range result.Categories {
			categoryIDs = append(categoryIDs, category.ModelID)
			if modelID == 0 || category.ModelID == modelID {
				result.Total += category.Count
			}
		}
		if err := s.loadSearchModels(categoryIDs, models); err != nil {
			return nil, err
		}
		for i := range result.Categories {
			m := models[result.Categories[i].ModelID]
			result.Categories[i].Name = m.Name
			result.Categories[i].Icon = m.Icon
		}
		if result.Total == 0 {
			return result, nil
		}
		lastPage := int((result.Total-1)/int64(pageSize)) + 1
		result.Page = min(max(1, page), lastPage)
	}
	if modelID != 0 {
		q = q.Where("model_id = ?", modelID)
	}

	var insts []model.Instance
	if err := q.Order("id desc").Limit(pageSize).Offset((result.Page - 1) * pageSize).Find(&insts).Error; err != nil {
		return nil, err
	}
	if len(insts) == 0 {
		return result, nil
	}
	seen := map[uint]bool{}
	ids := make([]uint, 0)
	for _, instance := range insts {
		if !seen[instance.ModelID] {
			seen[instance.ModelID] = true
			ids = append(ids, instance.ModelID)
		}
	}
	if !paged {
		if err := s.loadSearchModels(ids, models); err != nil {
			return nil, err
		}
	}
	var attrs []model.Attribute
	if err := s.db.Where("model_id IN ? AND enabled = ?", ids, true).Order("`index` asc, id asc").Find(&attrs).Error; err != nil {
		return nil, err
	}
	am := map[uint][]model.Attribute{}
	for _, a := range attrs {
		am[a.ModelID] = append(am[a.ModelID], a)
	}

	for _, inst := range insts {
		m := models[inst.ModelID]
		result.List = append(result.List, SearchHit{
			InstanceID: inst.ID,
			ModelID:    inst.ModelID,
			ModelName:  m.Name,
			ModelObjID: m.ObjID,
			Icon:       m.Icon,
			Display:    summarize(inst.Data, am[inst.ModelID]),
		})
	}
	return result, nil
}

func (s *InstanceService) loadSearchModels(ids []uint, byID map[uint]model.Model) error {
	if len(ids) == 0 {
		return nil
	}
	var models []model.Model
	if err := s.db.Where("id IN ?", ids).Find(&models).Error; err != nil {
		return err
	}
	for _, m := range models {
		byID[m.ID] = m
	}
	return nil
}

// summarize 生成实例内容摘要：按属性定义顺序取前若干个有值字段，用字段名+可读值展示。
func summarize(raw []byte, attrs []model.Attribute) string {
	var data map[string]interface{}
	if json.Unmarshal(raw, &data) != nil {
		return ""
	}
	parts := make([]string, 0, 5)
	for _, a := range attrs {
		v, ok := data[a.PropertyID]
		if !ok || v == nil {
			continue
		}
		text := displayValue(a, v)
		if text == "" {
			continue
		}
		parts = append(parts, a.Name+"="+text)
		if len(parts) >= 5 {
			break
		}
	}
	out := strings.Join(parts, " / ")
	if r := []rune(out); len(r) > 120 {
		out = string(r[:120]) + "..."
	}
	return out
}

// ImportReport 导入结果报告。
type ImportReport struct {
	Success int      `json:"success"` // 成功行数 = 新增 + 更新
	Created int      `json:"created"` // 新增条数
	Updated int      `json:"updated"` // 命中唯一键/主标识而更新的条数
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors"`
}

type enumOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func parseOptions(raw []byte) []enumOption {
	var opts []enumOption
	_ = json.Unmarshal(raw, &opts)
	return opts
}

func enumName(attr model.Attribute, id string) string {
	for _, o := range parseOptions(attr.Options) {
		if o.ID == id {
			return o.Name
		}
	}
	return id
}

func enumID(attr model.Attribute, nameOrID string) (string, bool) {
	for _, o := range parseOptions(attr.Options) {
		if o.Name == nameOrID || o.ID == nameOrID {
			return o.ID, true
		}
	}
	return "", false
}

// displayValue 将存储值转为可读文本（用于导出）。
func displayValue(attr model.Attribute, val interface{}) string {
	if val == nil {
		return ""
	}
	switch attr.Type {
	case model.FieldBool:
		if b, ok := val.(bool); ok {
			if b {
				return "是"
			}
			return "否"
		}
	case model.FieldEnum:
		if sv, ok := val.(string); ok {
			return enumName(attr, sv)
		}
	case model.FieldEnumMulti:
		if arr, ok := val.([]interface{}); ok {
			names := make([]string, 0, len(arr))
			for _, it := range arr {
				if sv, ok := it.(string); ok {
					names = append(names, enumName(attr, sv))
				}
			}
			return strings.Join(names, ",")
		}
	}
	switch x := val.(type) {
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		return x
	default:
		return fmt.Sprintf("%v", x)
	}
}

// parseValue 将单元格文本按字段类型转为存储值（用于导入）。返回 (值, 是否有值, 错误)。
func parseValue(attr model.Attribute, raw string) (interface{}, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false, nil
	}
	switch attr.Type {
	case model.FieldInt, model.FieldFloat:
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, false, fmt.Errorf("字段「%s」应为数字", attr.Name)
		}
		return n, true, nil
	case model.FieldBool:
		return raw == "是" || strings.EqualFold(raw, "true") || raw == "1", true, nil
	case model.FieldEnum:
		id, ok := enumID(attr, raw)
		if !ok {
			return nil, false, fmt.Errorf("字段「%s」的值「%s」不在可选范围内", attr.Name, raw)
		}
		return id, true, nil
	case model.FieldEnumMulti:
		ids := []interface{}{}
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			id, ok := enumID(attr, p)
			if !ok {
				return nil, false, fmt.Errorf("字段「%s」含非法选项「%s」", attr.Name, p)
			}
			ids = append(ids, id)
		}
		return ids, true, nil
	default:
		return raw, true, nil
	}
}

func (s *InstanceService) attributesSorted(modelID uint) ([]model.Attribute, error) {
	var attrs []model.Attribute
	err := s.db.Where("model_id = ? AND enabled = ?", modelID, true).Order("`index` asc, id asc").Find(&attrs).Error
	return attrs, err
}

// queryAll 返回符合条件的全部实例（不分页，用于导出）。
func (s *InstanceService) queryAll(modelID uint, keyword string, filters map[string]string, conditions []Condition, ids []uint, assigned, source, biz string) ([]model.Instance, error) {
	q := s.db.Model(&model.Instance{}).Where("model_id = ?", modelID)
	q = s.applyQuery(q, keyword, filters, conditions)
	q = s.applyAssigned(q, assigned)
	q = s.applySourceBiz(q, source, biz)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	var list []model.Instance
	err := q.Order("id desc").Find(&list).Error
	return list, err
}

// fillSheet 把表头 + 实例行写入指定 sheet；rels 与 insts 对齐（可为 nil），最后追加「关联关系」列。
func fillSheet(f *excelize.File, sheet string, attrs []model.Attribute, insts []model.Instance, rels []string) {
	for i, a := range attrs {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, a.Name)
	}
	relCol := len(attrs) + 1
	relCell, _ := excelize.CoordinatesToCellName(relCol, 1)
	_ = f.SetCellValue(sheet, relCell, relHeader)

	for r, inst := range insts {
		var data map[string]interface{}
		_ = json.Unmarshal(inst.Data, &data)
		for i, a := range attrs {
			cell, _ := excelize.CoordinatesToCellName(i+1, r+2)
			_ = f.SetCellValue(sheet, cell, displayValue(a, data[a.PropertyID]))
		}
		if r < len(rels) && rels[r] != "" {
			cell, _ := excelize.CoordinatesToCellName(relCol, r+2)
			_ = f.SetCellValue(sheet, cell, rels[r])
		}
	}
}

// buildExcel 生成单模型实例表格。rels 与 insts 对齐（可为 nil），最后追加「关联关系」列。
func buildExcel(attrs []model.Attribute, insts []model.Instance, rels []string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	fillSheet(f, f.GetSheetName(0), attrs, insts, rels)
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// sheetNameRe 匹配 Excel sheet 名中的非法字符。
var sheetNameRe = regexp.MustCompile(`[\[\]:*?/\\]`)

// sanitizeSheetName 规范 sheet 名：替换非法字符、截断到 31 字符、去重。
func sanitizeSheetName(name string, used map[string]int) string {
	s := sheetNameRe.ReplaceAllString(strings.TrimSpace(name), "_")
	if s == "" {
		s = "Sheet"
	}
	r := []rune(s)
	if len(r) > 31 {
		s = string(r[:31])
	}
	base := s
	for {
		if _, ok := used[s]; !ok {
			used[s] = 1
			return s
		}
		used[base]++
		suffix := fmt.Sprintf("(%d)", used[base])
		br := []rune(base)
		if len(br)+len(suffix) > 31 {
			br = br[:31-len(suffix)]
		}
		s = string(br) + suffix
	}
}

// ExportAllExcel 导出所有资源模型的实例为一个多 sheet 工作簿（每个模型一个 sheet）。
// canView 为按模型的查看权限判定（nil 表示不限制）；sourceID 非 nil 时仅导出该数据源（0=用户创建）的实例。
// 仅导出启用、有实例、可查看的资源模型，跳过组织模型。
func (s *InstanceService) ExportAllExcel(canView func(uint) bool, sourceID *uint) ([]byte, error) {
	var models []model.Model
	if err := s.db.Where("enabled = ?", true).
		Where("obj_id NOT IN ?", []string{"biz", "set", "module"}).
		Order("`index` asc, id asc").Find(&models).Error; err != nil {
		return nil, err
	}
	f := excelize.NewFile()
	defer f.Close()
	defaultSheet := f.GetSheetName(0)
	used := map[string]int{}
	added := 0
	for _, m := range models {
		if canView != nil && !canView(m.ID) {
			continue
		}
		attrs, err := s.attributesSorted(m.ID)
		if err != nil {
			continue
		}
		q := s.db.Where("model_id = ?", m.ID)
		if sourceID != nil {
			q = q.Where("source_id = ?", *sourceID)
		}
		var insts []model.Instance
		if err := q.Order("id desc").Find(&insts).Error; err != nil || len(insts) == 0 {
			continue
		}
		rels := make([]string, len(insts))
		for i, inst := range insts {
			rels[i] = s.relationsCell(inst.ID)
		}
		sheet := sanitizeSheetName(m.Name, used)
		idx, err := f.NewSheet(sheet)
		if err != nil {
			continue
		}
		fillSheet(f, sheet, attrs, insts, rels)
		if added == 0 {
			f.SetActiveSheet(idx)
		}
		added++
	}
	if added > 0 {
		_ = f.DeleteSheet(defaultSheet)
	} else {
		// 无任何可导出数据：留一个提示 sheet
		_ = f.SetCellValue(defaultSheet, "A1", "无可导出的资源数据")
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ExportExcel 导出匹配条件的实例为 xlsx（含数据源/业务线筛选，与列表所见一致）。
func (s *InstanceService) ExportExcel(modelID uint, keyword string, filters map[string]string, conditions []Condition, ids []uint, assigned, source, biz string) ([]byte, error) {
	attrs, err := s.attributesSorted(modelID)
	if err != nil {
		return nil, err
	}
	insts, err := s.queryAll(modelID, keyword, filters, conditions, ids, assigned, source, biz)
	if err != nil {
		return nil, err
	}
	rels := make([]string, len(insts))
	for i, inst := range insts {
		rels[i] = s.relationsCell(inst.ID)
	}
	return buildExcel(attrs, insts, rels)
}

var fieldTypeCN = map[string]string{
	"singlechar": "单行文本", "longchar": "长文本", "int": "整数", "float": "浮点数",
	"bool": "布尔(是/否)", "enum": "枚举单选", "enummulti": "枚举多选",
	"date": "日期", "datetime": "日期时间", "user": "用户", "list": "列表",
}

// optionNames 解析枚举字段的选项名列表。
func optionNames(a model.Attribute) []string {
	var opts []struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(a.Options, &opts)
	names := make([]string, 0, len(opts))
	for _, o := range opts {
		names = append(names, o.Name)
	}
	return names
}

// sampleValue 按字段类型生成样例值。
func sampleValue(a model.Attribute) string {
	switch a.Type {
	case "bool":
		return "是"
	case "int":
		return "1"
	case "float":
		return "1.0"
	case "date":
		return "2026-01-01"
	case "datetime":
		return "2026-01-01 12:00:00"
	case "enum":
		if ns := optionNames(a); len(ns) > 0 {
			return ns[0]
		}
		return "选项名"
	case "enummulti":
		if ns := optionNames(a); len(ns) > 0 {
			return ns[0]
		}
		return "选项名"
	case "longchar":
		return "示例" + a.Name
	default:
		return "示例" + a.Name
	}
}

// fieldComment 生成字段表头批注（类型/必填/唯一/可选值/填写提示）。
func fieldComment(a model.Attribute) string {
	lines := []string{"类型：" + fieldTypeCN[a.Type]}
	if a.Required {
		lines = append(lines, "必填：是")
	} else {
		lines = append(lines, "必填：否")
	}
	if a.Unique {
		lines = append(lines, "唯一：是（可作为导入/关联的匹配字段）")
	}
	switch a.Type {
	case "enum", "enummulti":
		if ns := optionNames(a); len(ns) > 0 {
			lines = append(lines, "可选值："+strings.Join(ns, " / "))
		}
		if a.Type == "enummulti" {
			lines = append(lines, "多选用英文逗号分隔，填选项名称")
		} else {
			lines = append(lines, "填选项名称")
		}
	case "bool":
		lines = append(lines, "填 是 或 否")
	case "date":
		lines = append(lines, "格式 YYYY-MM-DD")
	case "datetime":
		lines = append(lines, "格式 YYYY-MM-DD HH:mm:ss")
	}
	return strings.Join(lines, "\n")
}

// sampleRelation 给出关联关系列的样例（尽量用该模型真实的关联）。
func (s *InstanceService) sampleRelation(modelID uint) string {
	var assoc model.ModelAssociation
	if s.db.Where("src_model_id = ? OR dst_model_id = ?", modelID, modelID).First(&assoc).Error == nil {
		peerID := assoc.DstModelID
		dir := "out"
		if assoc.DstModelID == modelID {
			peerID, dir = assoc.SrcModelID, "in"
		}
		var pm model.Model
		s.db.First(&pm, peerID)
		ident := "对端唯一值"
		if a := s.matchField(peerID); a != nil {
			ident = "对端" + a.Name
		}
		return fmt.Sprintf("%s|%s|%s|%s", pm.Name, ident, assoc.Code, dir)
	}
	return "对端模型|对端标识|关联码|out"
}

// TemplateExcel 生成导入模板：表头 + 字段中文批注 + 一行样例（含关联关系样例）。
func (s *InstanceService) TemplateExcel(modelID uint) ([]byte, error) {
	attrs, err := s.attributesSorted(modelID)
	if err != nil {
		return nil, err
	}
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)

	addComment := func(cell, text string) {
		_ = f.AddComment(sheet, excelize.Comment{
			Cell:   cell,
			Author: "CMDB",
			Text:   text,
			Width:  260,
			Height: 120,
		})
	}

	// 唯一字段表头高亮样式（加粗 + 浅黄底），直观标识导入匹配键
	uniqStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#FFF3CD"}, Pattern: 1},
	})
	// 表头 + 批注 + 样例
	for i, a := range attrs {
		head, _ := excelize.CoordinatesToCellName(i+1, 1)
		name := a.Name
		if a.Enabled && a.Unique {
			name += uniqueMark // 表头追加「（唯一）」标识，导入时自动忽略该后缀
		}
		_ = f.SetCellValue(sheet, head, name)
		if a.Enabled && a.Unique {
			_ = f.SetCellStyle(sheet, head, head, uniqStyle)
		}
		addComment(head, fieldComment(a))
		row2, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheet, row2, sampleValue(a))
	}
	// 关联关系列
	relC := len(attrs) + 1
	relHead, _ := excelize.CoordinatesToCellName(relC, 1)
	_ = f.SetCellValue(sheet, relHead, relHeader)
	addComment(relHead, "资源间关联关系（选填）\n每条一行，格式：对端模型|对端标识|关联码|方向\n方向 out=本资源为源，in=本资源为目标\n对端标识填对端资源的唯一字段值\n多条用换行分隔\n拓扑归属（业务/集群/模块）不在此维护")
	relRow2, _ := excelize.CoordinatesToCellName(relC, 2)
	_ = f.SetCellValue(sheet, relRow2, s.sampleRelation(modelID))

	// 第二个 sheet：关联关系说明（中英文映射 + 填写示例）
	s.buildRelRefSheet(f, modelID)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildRelRefSheet 生成「关联关系说明」sheet：列出与该模型相关的关联，含关系名称、
// 关联码、对端模型、方向与填写示例，供用户参照填写「关联关系」列。
func (s *InstanceService) buildRelRefSheet(f *excelize.File, modelID uint) {
	name := "关联关系说明"
	if _, err := f.NewSheet(name); err != nil {
		return
	}
	headers := []string{"关系名称", "关联码", "对端模型", "方向(填 in/out)", "填写示例"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(name, cell, h)
	}

	var assocs []model.ModelAssociation
	s.db.Where("src_model_id = ? OR dst_model_id = ?", modelID, modelID).Find(&assocs)

	modelName := func(id uint) string {
		var m model.Model
		s.db.First(&m, id)
		return m.Name
	}
	kindCache := map[uint]model.AssociationKind{}
	kindOf := func(id uint) model.AssociationKind {
		if k, ok := kindCache[id]; ok {
			return k
		}
		var k model.AssociationKind
		s.db.First(&k, id)
		kindCache[id] = k
		return k
	}
	peerIdentHint := func(peerID uint) string {
		if a := s.matchField(peerID); a != nil {
			return "对端" + a.Name
		}
		return "对端标识"
	}

	row := 2
	for _, a := range assocs {
		k := kindOf(a.KindID)
		if a.SrcModelID == modelID {
			note := k.SrcToDstNote
			if note == "" {
				note = k.Name
			}
			writeWithHint(f, name, &row, note, a.Code, modelName(a.DstModelID), "out", peerIdentHint(a.DstModelID))
		}
		if a.DstModelID == modelID {
			note := k.DstToSrcNote
			if note == "" {
				note = k.Name
			}
			writeWithHint(f, name, &row, note, a.Code, modelName(a.SrcModelID), "in", peerIdentHint(a.SrcModelID))
		}
	}
	if row == 2 {
		cell, _ := excelize.CoordinatesToCellName(1, 2)
		_ = f.SetCellValue(name, cell, "该资源类型暂无可用关联类型")
	}
}

func writeWithHint(f *excelize.File, sheet string, row *int, note, code, peer, dir, identHint string) {
	vals := []string{note, code, peer, dir, fmt.Sprintf("%s|<%s>|%s|%s", peer, identHint, code, dir)}
	for i, v := range vals {
		cell, _ := excelize.CoordinatesToCellName(i+1, *row)
		_ = f.SetCellValue(sheet, cell, v)
	}
	*row++
}

// ImportExcel 解析 xlsx 并逐行创建实例，返回结果报告。
func (s *InstanceService) ImportExcel(modelID uint, data []byte, user string) (*ImportReport, error) {
	attrs, err := s.attributesSorted(modelID)
	if err != nil {
		return nil, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, newValidationErr("无法解析 Excel 文件")
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, newValidationErr("文件没有数据行")
	}

	nameToAttr := map[string]model.Attribute{}
	for _, a := range attrs {
		nameToAttr[a.Name] = a
	}
	header := rows[0]
	colAttr := make([]*model.Attribute, len(header))
	relCol := -1
	for i, h := range header {
		hh := strings.TrimSpace(h)
		if hh == relHeader {
			relCol = i
			continue
		}
		// 兼容模板里唯一字段表头带的「（唯一）」标识，匹配属性时忽略该后缀
		hh = strings.TrimSuffix(hh, uniqueMark)
		hh = strings.TrimSuffix(hh, "(唯一)")
		hh = strings.TrimSpace(hh)
		if a, ok := nameToAttr[hh]; ok {
			aa := a
			colAttr[i] = &aa
		}
	}

	// 导入 upsert 的匹配键：优先启用的唯一字段；没有则取主标识（首个启用字段）
	matchKeys := importMatchKeys(attrs)

	report := &ImportReport{Errors: []string{}}
	for ri := 1; ri < len(rows); ri++ {
		row := rows[ri]
		if isBlankRow(row) {
			continue
		}
		rowData := map[string]interface{}{}
		var rowErr error
		for ci, cell := range row {
			if ci >= len(colAttr) || colAttr[ci] == nil {
				continue
			}
			v, ok, err := parseValue(*colAttr[ci], cell)
			if err != nil {
				rowErr = err
				break
			}
			if ok {
				rowData[colAttr[ci].PropertyID] = v
			}
		}
		var saved *model.Instance
		if rowErr == nil {
			// 命中唯一键/主标识则更新（合并已有字段），否则新增
			if existing := s.findByImportKeys(modelID, matchKeys, rowData); existing != nil {
				merged := mergeInstanceData(existing.Data, rowData)
				saved, _, rowErr = s.Update(existing.ID, merged, user)
				if rowErr == nil {
					report.Updated++
				}
			} else {
				saved, rowErr = s.Create(modelID, rowData, user)
				if rowErr == nil {
					report.Created++
				}
			}
		}
		if rowErr != nil {
			report.Failed++
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, fmt.Sprintf("第%d行: %s", ri+1, rowErr.Error()))
			}
			continue
		}
		report.Success++
		// 导入关联关系（若有该列）
		if relCol >= 0 && relCol < len(row) && saved != nil {
			s.importRelations(saved.ID, modelID, row[relCol], report, ri+1)
		}
	}
	return report, nil
}

// importMatchKeys 返回导入 upsert 的匹配键：优先所有启用的唯一字段；都没有则取主标识（首个启用字段）。
func importMatchKeys(attrs []model.Attribute) []model.Attribute {
	var uniq []model.Attribute
	for _, a := range attrs {
		if a.Enabled && a.Unique {
			uniq = append(uniq, a)
		}
	}
	if len(uniq) > 0 {
		return uniq
	}
	for _, a := range attrs {
		if a.Enabled {
			return []model.Attribute{a}
		}
	}
	return nil
}

// findByImportKeys 按匹配键在同模型下查已有实例：逐个键用行内非空值匹配，命中任一即返回。
func (s *InstanceService) findByImportKeys(modelID uint, keys []model.Attribute, rowData map[string]interface{}) *model.Instance {
	for _, a := range keys {
		v, ok := rowData[a.PropertyID]
		if !ok {
			continue
		}
		sv := strings.TrimSpace(fmt.Sprint(v))
		if sv == "" {
			continue
		}
		var inst model.Instance
		if s.db.Where("model_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, ?)) = ?", modelID, "$."+a.PropertyID, sv).
			Order("id asc").First(&inst).Error == nil {
			return &inst
		}
	}
	return nil
}

// mergeInstanceData 把导入行覆盖到已有数据之上（已有字段保留，行内字段更新），供 upsert 更新用。
func mergeInstanceData(existing datatypes.JSON, row map[string]interface{}) map[string]interface{} {
	m := map[string]interface{}{}
	_ = json.Unmarshal(existing, &m)
	for k, v := range row {
		m[k] = v
	}
	return m
}

func isBlankRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
