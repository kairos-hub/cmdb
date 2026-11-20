package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cmdb/cmdb-backend/internal/model"
	"github.com/xuri/excelize/v2"
)

// 导入导出列（固定顺序）
var topoHeader = []string{"业务线", "业务", "模块", "资源类型", "匹配字段", "资源标识"}

// TopoImportReport 导入结果。
type TopoImportReport struct {
	Success int      `json:"success"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors"`
}

// matchAttr 选取用于定位实例的字段：优先第一个启用的唯一字段，否则第一个启用字段。
func (s *TopoService) matchAttr(modelID uint) *model.Attribute {
	var attrs []model.Attribute
	s.db.Where("model_id = ? AND enabled = ?", modelID, true).Order("`index` asc, id asc").Find(&attrs)
	for _, a := range attrs {
		if a.Unique {
			return &a
		}
	}
	if len(attrs) > 0 {
		return &attrs[0]
	}
	return nil
}

// ExportTopo 导出业务拓扑（业务/集群/模块/资源类型/已分配资源）为 xlsx。
// bizID=0 导出全部业务，否则仅导出该业务。
func (s *TopoService) ExportTopo(bizID uint) ([]byte, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	for i, h := range topoHeader {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}

	row := 2
	write := func(vals []string) {
		for i, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			_ = f.SetCellValue(sheet, cell, v)
		}
		row++
	}

	modelNameCache := map[uint]string{}
	attrCache := map[uint]*model.Attribute{}
	modelName := func(id uint) string {
		if n, ok := modelNameCache[id]; ok {
			return n
		}
		var m model.Model
		s.db.First(&m, id)
		modelNameCache[id] = m.Name
		return m.Name
	}
	matchOf := func(id uint) *model.Attribute {
		if a, ok := attrCache[id]; ok {
			return a
		}
		a := s.matchAttr(id)
		attrCache[id] = a
		return a
	}

	bizList, err := s.ListBusiness()
	if err != nil {
		return nil, err
	}
	for _, biz := range bizList {
		if bizID != 0 && biz.ID != bizID {
			continue
		}
		var businesses []model.TopoNode
		s.db.Where("biz_id = ? AND node_type = ?", biz.ID, model.TopoBusiness).Order("id asc").Find(&businesses)
		if len(businesses) == 0 {
			write([]string{biz.Name, "", "", "", "", "", ""})
			continue
		}
		for _, bus := range businesses {
			var mods []model.TopoNode
			s.db.Where("parent_id = ? AND node_type = ?", bus.ID, model.TopoModule).Order("id asc").Find(&mods)
			if len(mods) == 0 {
				write([]string{biz.Name, bus.Name, "", "", "", ""})
				continue
			}
			for _, mod := range mods {
				mid := s.moduleModelID(&mod)
				mName := modelName(mid)
				ma := matchOf(mid)
				matchField := ""
				if ma != nil {
					matchField = ma.Name
				}
				var hostIDs []uint
				s.db.Model(&model.HostModule{}).Where("module_id = ?", mod.ID).Pluck("host_id", &hostIDs)
				if len(hostIDs) == 0 {
					write([]string{biz.Name, bus.Name, mod.Name, mName, matchField, ""})
					continue
				}
				var insts []model.Instance
				s.db.Where("id IN ?", hostIDs).Find(&insts)
				for _, inst := range insts {
					ident := ""
					if ma != nil {
						var dm map[string]interface{}
						_ = json.Unmarshal(inst.Data, &dm)
						ident = displayValue(*ma, dm[ma.PropertyID])
					}
					write([]string{biz.Name, bus.Name, mod.Name, mName, matchField, ident})
				}
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// TemplateTopo 生成导入模板（表头 + 一行示例）。
func (s *TopoService) TemplateTopo() ([]byte, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	for i, h := range topoHeader {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	example := []string{"示例业务线", "示例业务", "web模块", "主机", "内网IP", "10.0.0.1"}
	for i, v := range example {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheet, cell, v)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ImportTopo 从 xlsx 导入拓扑结构与资源归属（幂等补建业务/集群/模块，按匹配字段分配资源）。
func (s *TopoService) ImportTopo(data []byte) (*TopoImportReport, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, newValidationErr("无法解析 Excel 文件")
	}
	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		return nil, newValidationErr("读取内容失败")
	}
	report := &TopoImportReport{Errors: []string{}}
	if len(rows) <= 1 {
		return report, nil
	}

	bizCache := map[string]uint{} // bizName -> bizID
	busCache := map[string]uint{} // bizID|busName -> businessID
	modCache := map[string]uint{} // businessID|modName -> moduleID
	modModel := map[uint]uint{}   // moduleID -> modelID

	cell := func(r []string, i int) string {
		if i < len(r) {
			return strings.TrimSpace(r[i])
		}
		return ""
	}

	for idx, r := range rows {
		if idx == 0 {
			continue // 表头
		}
		bizName := cell(r, 0)
		busName := cell(r, 1)
		modName := cell(r, 2)
		resType := cell(r, 3)
		matchField := cell(r, 4)
		matchValue := cell(r, 5)
		rowNo := idx + 1

		if bizName == "" {
			continue
		}

		// 业务线
		bizID, err := s.ensureBiz(bizName, bizCache)
		if err != nil {
			s.failRow(report, rowNo, err)
			continue
		}
		if busName == "" {
			continue // 仅业务线一行
		}
		// 业务
		busID, err := s.ensureBusiness(bizID, busName, busCache)
		if err != nil {
			s.failRow(report, rowNo, err)
			continue
		}
		if modName == "" {
			continue // 仅到业务一行
		}
		// 模块（需资源类型）
		if resType == "" {
			s.failRow(report, rowNo, newValidationErr("模块「%s」缺少资源类型", modName))
			continue
		}
		modID, modelID, err := s.ensureModule(busID, modName, resType, modCache, modModel)
		if err != nil {
			s.failRow(report, rowNo, err)
			continue
		}
		// 资源归属（可选）
		if matchValue == "" {
			report.Success++
			continue
		}
		if err := s.assignByMatch(modID, modelID, bizID, matchField, matchValue); err != nil {
			s.failRow(report, rowNo, err)
			continue
		}
		report.Success++
	}
	return report, nil
}

func (s *TopoService) failRow(r *TopoImportReport, rowNo int, err error) {
	r.Failed++
	r.Errors = append(r.Errors, fmt.Sprintf("第%d行：%s", rowNo, err.Error()))
}

func (s *TopoService) ensureBiz(name string, cache map[string]uint) (uint, error) {
	if id, ok := cache[name]; ok {
		return id, nil
	}
	var inst model.Instance
	err := s.db.Where("model_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, '$.bk_biz_name')) = ?", s.bizModelID(), name).First(&inst).Error
	if err == nil {
		cache[name] = inst.ID
		return inst.ID, nil
	}
	b, e := s.CreateBiz(name, "", "")
	if e != nil {
		return 0, e
	}
	cache[name] = b.ID
	return b.ID, nil
}

func (s *TopoService) ensureBusiness(bizID uint, name string, cache map[string]uint) (uint, error) {
	key := fmt.Sprintf("%d|%s", bizID, name)
	if id, ok := cache[key]; ok {
		return id, nil
	}
	var node model.TopoNode
	err := s.db.Where("biz_id = ? AND parent_id = ? AND node_type = ? AND name = ?", bizID, bizID, model.TopoBusiness, name).First(&node).Error
	if err == nil {
		cache[key] = node.ID
		return node.ID, nil
	}
	n, e := s.CreateBusiness(bizID, name)
	if e != nil {
		return 0, e
	}
	cache[key] = n.ID
	return n.ID, nil
}

func (s *TopoService) ensureModule(businessID uint, name, resType string, cache map[string]uint, modModel map[uint]uint) (uint, uint, error) {
	key := fmt.Sprintf("%d|%s", businessID, name)
	if id, ok := cache[key]; ok {
		return id, modModel[id], nil
	}
	var m model.Model
	if err := s.db.Where("name = ?", resType).First(&m).Error; err != nil {
		return 0, 0, newValidationErr("资源类型「%s」不存在", resType)
	}
	var node model.TopoNode
	err := s.db.Where("parent_id = ? AND node_type = ? AND name = ?", businessID, model.TopoModule, name).First(&node).Error
	if err == nil {
		cache[key] = node.ID
		modModel[node.ID] = s.moduleModelID(&node)
		return node.ID, modModel[node.ID], nil
	}
	n, e := s.CreateModule(businessID, m.ID, name, 0, 0)
	if e != nil {
		return 0, 0, e
	}
	cache[key] = n.ID
	modModel[n.ID] = m.ID
	return n.ID, m.ID, nil
}

// assignByMatch 按匹配字段找到实例并分配到模块（已在他处则转移）。
func (s *TopoService) assignByMatch(moduleID, modelID, bizID uint, matchField, matchValue string) error {
	// 解析匹配字段的 property_id
	prop := ""
	if matchField != "" {
		var a model.Attribute
		if err := s.db.Where("model_id = ? AND name = ?", modelID, matchField).First(&a).Error; err == nil {
			prop = a.PropertyID
		}
	}
	if prop == "" {
		if a := s.matchAttr(modelID); a != nil {
			prop = a.PropertyID
		}
	}
	if prop == "" || !safePropRe.MatchString(prop) {
		return newValidationErr("模型无可用匹配字段")
	}
	var inst model.Instance
	err := s.db.Where("model_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, ?)) = ?", modelID, "$."+prop, matchValue).First(&inst).Error
	if err != nil {
		return newValidationErr("未找到资源 %s=%s", matchField, matchValue)
	}
	// 已分配的资源不覆盖：在目标模块则幂等成功，在其他模块则导入失败
	var hm model.HostModule
	if s.db.Where("host_id = ?", inst.ID).First(&hm).Error == nil {
		if hm.ModuleID == moduleID {
			return nil // 已在目标模块，无需变更
		}
		return newValidationErr("资源 %s=%s 已被分配到其他业务，未覆盖", matchField, matchValue)
	}
	return s.db.Create(&model.HostModule{HostID: inst.ID, ModuleID: moduleID, ModelID: modelID, BizID: bizID}).Error
}
