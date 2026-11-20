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

// cmsEndpoint 返回云监控(CMS)地域化 endpoint。
func cmsEndpoint(region string) string {
	if region == "" {
		region = "cn-shanghai"
	}
	return "https://metrics." + region + ".aliyuncs.com"
}

// aliyunHostMetrics 用云监控(acs_ecs_dashboard) 取 ECS 的 CPU/内存/磁盘/网络。
//   - CPU/网络为基础指标，无需云监控插件；
//   - 内存/磁盘使用率依赖云监控 agent，未装则对应曲线为空（静默跳过）。
func (s *MetricsService) aliyunHostMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	instID := firstField(data, "resource_id", "instance_id", "bk_cloud_inst_id")
	if instID == "" || !strings.HasPrefix(instID, "i-") {
		res.Message = "该主机无阿里云实例 ID（resource_id），无法关联云监控"
		return res, nil
	}
	region := firstField(data, "bk_zone", "region", "bk_region")
	if region == "" {
		region = parseRegions(ds.Region)[0]
	}
	res.Target = instID
	endpoint := cmsEndpoint(region)
	endMs := time.Now().UnixMilli()
	startMs := endMs - int64(lookback)*1000
	period := strconv.Itoa(step)

	// 单线指标：CPU / 网络进出（阿里云网络速率单位为 bit/s，与前端口径一致）
	singles := []struct {
		key, name, unit, metric string
	}{
		{"cpu", "CPU 使用率", "%", "CPUUtilization"},
		{"mem", "内存使用率", "%", "memory_usedutilization"},
		{"net_in", "网络入流量", "bps", "IntranetInRate"},
		{"net_out", "网络出流量", "bps", "IntranetOutRate"},
	}
	for _, m := range singles {
		groups, err := cmsMetric(ds, endpoint, "acs_ecs_dashboard", instID, m.metric, startMs, endMs, period, "")
		if err != nil || len(groups) == 0 {
			continue
		}
		res.Series = append(res.Series, MetricSeries{Key: m.key, Name: m.name, Unit: m.unit, Points: groups[0].points})
	}

	// 磁盘使用率：按挂载点多条线（diskname=挂载点，如 /；依赖 agent）
	if disks, err := cmsMetric(ds, endpoint, "acs_ecs_dashboard", instID, "diskusage_utilization", startMs, endMs, period, "diskname"); err == nil {
		for _, d := range disks {
			res.Series = append(res.Series, MetricSeries{Key: "disk", Name: "磁盘使用率", Unit: "%", Label: cleanDiskLabel(d.label), Points: d.points})
		}
	}

	// 「查看完整监控」深链到 ECS 控制台实例监控页
	res.DetailURL = fmt.Sprintf("https://ecs.console.aliyun.com/#/server/%s/monitor?regionId=%s", instID, region)

	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该主机在云监控中暂无数据（或未安装云监控插件）"
	}
	return res, nil
}

// cmsMetric 调用 DescribeMetricList 取单个指标，按 groupBy 维度（如 device）拆分多条线；
// groupBy 为空时返回单条线。取 Average 统计值，timestamp 为毫秒。
func cmsMetric(ds *model.DataSource, endpoint, namespace, instID, metric string, startMs, endMs int64, period, groupBy string) ([]labeledSeries, error) {
	extra := map[string]string{
		"Namespace":  namespace,
		"MetricName": metric,
		"StartTime":  strconv.FormatInt(startMs, 10),
		"EndTime":    strconv.FormatInt(endMs, 10),
		"Period":     period,
		"Dimensions": fmt.Sprintf(`{"instanceId":"%s"}`, instID),
	}
	body, err := aliRPC(endpoint, ds.Username, ds.Password, "2019-01-01", "DescribeMetricList", extra)
	if err != nil {
		return nil, err
	}
	var out struct {
		Code       string `json:"Code"`
		Datapoints string `json:"Datapoints"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.Datapoints) == "" {
		return []labeledSeries{}, nil
	}
	var dps []map[string]interface{}
	if err := json.Unmarshal([]byte(out.Datapoints), &dps); err != nil {
		return nil, err
	}
	// 按 groupBy 聚合点集
	byGroup := map[string][][2]float64{}
	for _, dp := range dps {
		ts, ok := cmsNum(dp["timestamp"])
		if !ok {
			continue
		}
		val, ok := cmsDatapointVal(dp)
		if !ok {
			continue
		}
		label := ""
		if groupBy != "" {
			label = fmt.Sprint(dp[groupBy])
		}
		byGroup[label] = append(byGroup[label], [2]float64{ts / 1000.0, val}) // ms→s
	}
	list := make([]labeledSeries, 0, len(byGroup))
	for lbl, pts := range byGroup {
		sort.Slice(pts, func(i, j int) bool { return pts[i][0] < pts[j][0] })
		list = append(list, labeledSeries{label: lbl, points: pts})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].label < list[j].label })
	return list, nil
}

// cleanDiskLabel 清洗挂载点标签：云监控常把同盘多个挂载点用逗号拼接，取首个并标注其余数量。
func cleanDiskLabel(lbl string) string {
	lbl = strings.TrimSpace(lbl)
	if lbl == "" {
		return "disk"
	}
	if i := strings.Index(lbl, ","); i >= 0 {
		first := strings.TrimSpace(lbl[:i])
		more := strings.Count(lbl, ",") + 1 - 1
		return fmt.Sprintf("%s 等%d项", first, more)
	}
	return lbl
}

// cmsDatapointVal 从云监控数据点取值：优先 Average，依次回退 Value/Maximum/Minimum（部分指标只报某一种）。
func cmsDatapointVal(dp map[string]interface{}) (float64, bool) {
	for _, k := range []string{"Average", "Value", "Maximum", "Minimum"} {
		if v, ok := cmsNum(dp[k]); ok {
			return v, true
		}
	}
	return 0, false
}

// cmsNum 把 JSON 数值（float64/字符串）转 float。
func cmsNum(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// firstField 返回 data 中首个非空字段值（字符串）。
func firstField(data map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := data[k]; ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}
