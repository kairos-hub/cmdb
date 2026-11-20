package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// tcPeriodFor 把回看秒数映射为腾讯云 GetMonitorData 支持的周期（10/60/300/3600/86400）。
func tcPeriodFor(lookback int) int {
	switch {
	case lookback <= 3600: // 1h
		return 60
	case lookback <= 24*3600: // 6h / 24h
		return 300
	default: // 7d
		return 3600
	}
}

// tencentHostMetrics 用腾讯云监控(QCE/CVM) 取 CVM 的 CPU/内存/磁盘/网络。
//   - CPU/网络为基础指标；内存/磁盘使用率依赖云监控组件，未装则曲线为空。
//   - 流量指标单位为 Mbps，这里 ×1e6 统一为 bit/s。
func (s *MetricsService) tencentHostMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	instID := firstField(data, "resource_id", "instance_id", "bk_cloud_inst_id")
	if instID == "" || !strings.HasPrefix(instID, "ins-") {
		res.Message = "该主机无腾讯云实例 ID（resource_id），无法关联云监控"
		return res, nil
	}
	region := firstField(data, "bk_zone", "region", "bk_region")
	if region == "" {
		region = firstTencentAlertRegion(ds.Region)
	}
	res.Target = instID
	res.Step = tcPeriodFor(lookback)

	now := time.Now()
	start := now.Add(-time.Duration(lookback) * time.Second).Format(time.RFC3339)
	end := now.Format(time.RFC3339)

	metrics := []struct {
		key, name, unit, metric string
		mul                     float64 // 单位换算系数（流量 Mbps→bps）
	}{
		{"cpu", "CPU 使用率", "%", "CPUUsage", 1},
		{"mem", "内存使用率", "%", "MemUsage", 1},
		{"disk", "磁盘使用率", "%", "CvmDiskUsage", 1},
		{"net_in", "网络入流量", "bps", "LanIntraffic", 1e6},
		{"net_out", "网络出流量", "bps", "LanOuttraffic", 1e6},
	}
	for _, m := range metrics {
		pts, err := tcMonitor(ds, region, instID, m.metric, start, end, res.Step, m.mul)
		if err != nil || len(pts) == 0 {
			continue
		}
		res.Series = append(res.Series, MetricSeries{Key: m.key, Name: m.name, Unit: m.unit, Points: pts})
	}

	// 「查看完整监控」深链到 CVM 控制台实例监控页（腾讯云控制台用数字 rid 标识地域）
	if rid := tcRegionID(region); rid != "" {
		res.DetailURL = fmt.Sprintf("https://console.cloud.tencent.com/cvm/instance/detail?rid=%s&id=%s&tab=monitor", rid, instID)
	} else {
		res.DetailURL = "https://console.cloud.tencent.com/cvm/instance/index?id=" + instID
	}

	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该主机在云监控中暂无数据（或未安装云监控组件）"
	}
	return res, nil
}

// tcRegionID 腾讯云地域名 → 控制台数字 rid（深链用）。未知返回空。
func tcRegionID(region string) string {
	m := map[string]string{
		"ap-guangzhou": "1", "ap-shanghai": "4", "ap-hongkong": "5", "ap-beijing": "8",
		"ap-singapore": "9", "ap-chengdu": "16", "ap-chongqing": "19", "ap-shenzhen-fsi": "11",
		"ap-shanghai-fsi": "7", "ap-beijing-fsi": "6", "ap-nanjing": "33", "ap-seoul": "18",
		"ap-tokyo": "25", "ap-mumbai": "21", "ap-bangkok": "23", "ap-jakarta": "72",
		"na-siliconvalley": "15", "na-ashburn": "22", "eu-frankfurt": "17", "sa-saopaulo": "74",
	}
	return m[region]
}

// tcMonitor 调用 monitor GetMonitorData 取单实例单指标时序，返回 [ts秒,val] 点集（按 mul 换算）。
func tcMonitor(ds *model.DataSource, region, instID, metric, start, end string, period int, mul float64) ([][2]float64, error) {
	payload := map[string]interface{}{
		"Namespace":  "QCE/CVM",
		"MetricName": metric,
		"Period":     period,
		"StartTime":  start,
		"EndTime":    end,
		"Instances": []map[string]interface{}{
			{"Dimensions": []map[string]string{{"Name": "InstanceId", "Value": instID}}},
		},
	}
	body, err := tcAPI("monitor", "GetMonitorData", "2018-07-24", region, ds.Username, ds.Password, payload)
	if err != nil {
		return nil, err
	}
	var out struct {
		Response struct {
			DataPoints []struct {
				Timestamps []float64 `json:"Timestamps"`
				Values     []float64 `json:"Values"`
			} `json:"DataPoints"`
			Error *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Response.Error != nil || len(out.Response.DataPoints) == 0 {
		return [][2]float64{}, nil
	}
	dp := out.Response.DataPoints[0]
	n := len(dp.Timestamps)
	if len(dp.Values) < n {
		n = len(dp.Values)
	}
	pts := make([][2]float64, 0, n)
	for i := 0; i < n; i++ {
		pts = append(pts, [2]float64{dp.Timestamps[i], dp.Values[i] * mul})
	}
	return pts, nil
}
