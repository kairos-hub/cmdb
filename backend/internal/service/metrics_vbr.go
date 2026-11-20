package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// vbrMetrics 边界路由器(VBR) 监控：阿里云走 CMS acs_cen 的 VBR* 指标。
func (s *MetricsService) vbrMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	switch ds.Type {
	case model.DataSourceAliyun:
		return s.aliyunVbrMetrics(ds, data, lookback, step, res)
	default:
		res.Message = "该来源暂不支持边界路由器监控（" + ds.Type + "）"
		return res, nil
	}
}

// dsOwningCen 按 CEN 实例ID 找到同步该 CEN 的数据源（即 CEN 属主账号），用于跨账号取 VBR 指标。
func (s *MetricsService) dsOwningCen(cenID string) *model.DataSource {
	if cenID == "" {
		return nil
	}
	cenModelID := s.inst.ModelIDByObjID("cen")
	if cenModelID == 0 {
		return nil
	}
	var insts []model.Instance
	if s.db.Where("model_id = ?", cenModelID).Find(&insts).Error != nil {
		return nil
	}
	for _, it := range insts {
		if it.SourceID == 0 {
			continue
		}
		if firstField(decodeInstData(it.Data), "resource_id") == cenID {
			var ds model.DataSource
			if s.db.First(&ds, it.SourceID).Error == nil {
				return &ds
			}
		}
	}
	return nil
}

// vbrCmsAgg 查询 acs_cen VBR* 指标（维度按 vbrInstanceId，必要时加 cenId 精确匹配），
// 按时间戳聚合为单条序列（速率/流量 sum，延时/丢包 max）。
func vbrCmsAgg(ds *model.DataSource, endpoint, metric, vbrID, cenID string, startMs, endMs int64, period string, useMax bool) [][2]float64 {
	dims := fmt.Sprintf(`{"vbrInstanceId":"%s"}`, vbrID)
	if cenID != "" {
		dims = fmt.Sprintf(`{"cenId":"%s","vbrInstanceId":"%s"}`, cenID, vbrID)
	}
	extra := map[string]string{
		"Namespace":  "acs_cen",
		"MetricName": metric,
		"StartTime":  strconv.FormatInt(startMs, 10),
		"EndTime":    strconv.FormatInt(endMs, 10),
		"Period":     period,
		"Dimensions": dims,
	}
	body, err := aliRPC(endpoint, ds.Username, ds.Password, "2019-01-01", "DescribeMetricList", extra)
	if err != nil {
		return nil
	}
	var out struct {
		Datapoints string `json:"Datapoints"`
	}
	if json.Unmarshal(body, &out) != nil || strings.TrimSpace(out.Datapoints) == "" {
		return nil
	}
	var dps []map[string]interface{}
	if json.Unmarshal([]byte(out.Datapoints), &dps) != nil {
		return nil
	}
	agg := map[float64]float64{}
	for _, dp := range dps {
		ts, ok := cmsNum(dp["timestamp"])
		if !ok {
			continue
		}
		val, ok := cmsDatapointVal(dp)
		if !ok {
			continue
		}
		t := ts / 1000.0
		if useMax {
			if cur, had := agg[t]; !had || val > cur {
				agg[t] = val
			}
		} else {
			agg[t] += val
		}
	}
	pts := make([][2]float64, 0, len(agg))
	for t, v := range agg {
		pts = append(pts, [2]float64{t, v})
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i][0] < pts[j][0] })
	return pts
}

// aliyunVbrMetrics 阿里云 VBR：CMS acs_cen VBR* 指标。关联键=resource_id(vbr-xxx)，cen 字段辅助精确匹配。
func (s *MetricsService) aliyunVbrMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" || !strings.HasPrefix(instID, "vbr-") {
		res.Message = "该边界路由器无阿里云实例 ID（resource_id），无法关联云监控"
		return res, nil
	}
	cenID := firstField(data, "cen")
	region := firstField(data, "region", "bk_region", "bk_zone")
	if region == "" {
		region = parseRegions(ds.Region)[0]
	}
	res.Target = instID
	// acs_cen 的 VBR 指标归「关联 CEN 的属主账号」。VBR 可能由非 CEN 属主账号同步（跨账号专线接入），
	// 此时用本实例数据源会查不到，需改用 CEN 属主的数据源凭据。
	metDs := ds
	if owner := s.dsOwningCen(cenID); owner != nil && owner.Type == model.DataSourceAliyun {
		metDs = owner
	}
	endpoint := cmsEndpoint(region)
	endMs := time.Now().UnixMilli()
	startMs := endMs - int64(lookback)*1000
	period := fmt.Sprintf("%d", step)

	add := func(key, name, unit, label, metric string, useMax bool) {
		pts := vbrCmsAgg(metDs, endpoint, metric, instID, cenID, startMs, endMs, period, useMax)
		if len(pts) == 0 {
			return
		}
		res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: unit, Label: label, Points: pts})
	}
	add("in_rate", "流入带宽", "bps", "流入", "VBRInternetInRate", false)
	add("out_rate", "流出带宽", "bps", "流出", "VBRInternetOutRate", false)
	add("latency", "健康检查延时", "ms", "", "VBRHealthyCheckLatency", true)
	add("loss", "健康检查丢包率", "%", "", "VBRHealthyCheckLossRate", true)

	res.Cards = []MetricCard{
		{Key: "flow", Name: "带宽（流入/流出）", Unit: "bps", SeriesKeys: []string{"in_rate", "out_rate"}},
		{Key: "latency", Name: "健康检查延时", Unit: "ms", SeriesKeys: []string{"latency"}},
		{Key: "loss", Name: "健康检查丢包率", Unit: "%", SeriesKeys: []string{"loss"}},
	}
	// 云监控边界路由器监控页：category=vbr，dimension=instanceId:<vbrId>（%3A 即冒号）
	res.DetailURL = fmt.Sprintf("https://cloudmonitornext.console.aliyun.com/productMonitorChart?category=vbr&dimension=instanceId%%3A%s&region=%s", instID, region)
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该边界路由器在云监控中暂无数据"
	}
	return res, nil
}
