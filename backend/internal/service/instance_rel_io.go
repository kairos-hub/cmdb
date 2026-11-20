package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// relHeader 导入导出中「关联关系」列名。
const relHeader = "关联关系"

// uniqueMark 导入模板中唯一字段表头的可见标识后缀；导入解析表头时会忽略它。
const uniqueMark = "（唯一）"

// 关联单元格每行格式：对端模型|对端标识|关联码|方向(out/in)，多条用换行分隔。
// 仅包含真实关联，拓扑派生关联（derived）不导出、不参与导入。

// matchField 选取用于定位实例的字段：优先第一个启用的唯一字段，否则第一个启用字段。
func (s *InstanceService) matchField(modelID uint) *model.Attribute {
	var attrs []model.Attribute
	s.db.Where("model_id = ? AND enabled = ?", modelID, true).Order("`index` asc, id asc").Find(&attrs)
	for i := range attrs {
		if attrs[i].Unique {
			return &attrs[i]
		}
	}
	if len(attrs) > 0 {
		return &attrs[0]
	}
	return nil
}

// instMatchValue 返回实例匹配字段的原始值（用于导出标识，供导入回匹配）。
func (s *InstanceService) instMatchValue(instID, modelID uint) string {
	a := s.matchField(modelID)
	if a == nil {
		return ""
	}
	var inst model.Instance
	if s.db.First(&inst, instID).Error != nil {
		return ""
	}
	var m map[string]interface{}
	_ = json.Unmarshal(inst.Data, &m)
	if v, ok := m[a.PropertyID]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func (s *InstanceService) assocCode(id uint) string {
	var a model.ModelAssociation
	if s.db.First(&a, id).Error != nil {
		return ""
	}
	return a.Code
}

// relationsCell 生成某实例的「关联关系」导出单元格（排除拓扑派生关联）。
func (s *InstanceService) relationsCell(instID uint) string {
	views, err := s.ListRelations(instID)
	if err != nil {
		return ""
	}
	lines := make([]string, 0, len(views))
	for _, v := range views {
		if v.Derived {
			continue
		}
		code := s.assocCode(v.AssociationID)
		if code == "" {
			continue
		}
		ident := s.instMatchValue(v.PeerInstID, v.PeerModelID)
		lines = append(lines, fmt.Sprintf("%s|%s|%s|%s", v.PeerModelName, ident, code, v.Direction))
	}
	return strings.Join(lines, "\n")
}

// importRelations 解析关联单元格并为实例补建关联（幂等；逐条错误计入报告）。
func (s *InstanceService) importRelations(instID, instModelID uint, cell string, report *ImportReport, rowNo int) {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return
	}
	for _, line := range strings.Split(cell, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			s.relFail(report, rowNo, "关联格式应为 对端模型|对端标识|关联码|方向")
			continue
		}
		peerModelName := strings.TrimSpace(parts[0])
		peerIdent := strings.TrimSpace(parts[1])
		code := strings.TrimSpace(parts[2])
		dir := strings.TrimSpace(parts[3])

		var assoc model.ModelAssociation
		if s.db.Where("code = ?", code).First(&assoc).Error != nil {
			s.relFail(report, rowNo, fmt.Sprintf("关联类型 %s 不存在", code))
			continue
		}
		var pm model.Model
		if s.db.Where("name = ?", peerModelName).First(&pm).Error != nil {
			s.relFail(report, rowNo, fmt.Sprintf("对端模型 %s 不存在", peerModelName))
			continue
		}
		pa := s.matchField(pm.ID)
		if pa == nil || !safePropRe.MatchString(pa.PropertyID) {
			s.relFail(report, rowNo, fmt.Sprintf("对端模型 %s 无可用匹配字段", peerModelName))
			continue
		}
		var peer model.Instance
		if s.db.Where("model_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, ?)) = ?", pm.ID, "$."+pa.PropertyID, peerIdent).First(&peer).Error != nil {
			s.relFail(report, rowNo, fmt.Sprintf("未找到对端资源 %s=%s", peerModelName, peerIdent))
			continue
		}
		src, dst := instID, peer.ID
		if dir == "in" {
			src, dst = peer.ID, instID
		}
		var cnt int64
		s.db.Model(&model.InstanceRelation{}).
			Where("association_id = ? AND src_inst_id = ? AND dst_inst_id = ?", assoc.ID, src, dst).Count(&cnt)
		if cnt > 0 {
			continue // 已存在
		}
		if err := s.db.Create(&model.InstanceRelation{AssociationID: assoc.ID, SrcInstID: src, DstInstID: dst}).Error; err != nil {
			s.relFail(report, rowNo, err.Error())
		}
	}
}

func (s *InstanceService) relFail(report *ImportReport, rowNo int, msg string) {
	report.Failed++
	if len(report.Errors) < 100 {
		report.Errors = append(report.Errors, fmt.Sprintf("第%d行关联: %s", rowNo, msg))
	}
}
