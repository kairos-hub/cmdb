package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// kafkaMetrics Kafka 实例监控：阿里云走 CMS acs_kafka。
func (s *MetricsService) kafkaMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	switch ds.Type {
	case model.DataSourceAliyun:
		return s.aliyunKafkaMetrics(ds, data, lookback, step, res)
	default:
		res.Message = "该来源暂不支持 Kafka 监控（" + ds.Type + "）"
		return res, nil
	}
}

// aliyunKafkaMetrics 阿里云 Kafka：CMS acs_kafka。关联键=resource_id(alikafka_xxx)。
func (s *MetricsService) aliyunKafkaMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" || !strings.HasPrefix(instID, "alikafka") {
		res.Message = "该 Kafka 无阿里云实例 ID（resource_id），无法关联云监控"
		return res, nil
	}
	region := firstField(data, "region", "bk_zone", "bk_region")
	if region == "" {
		region = parseRegions(ds.Region)[0]
	}
	res.Target = instID
	endpoint := cmsEndpoint(region)
	endMs := time.Now().UnixMilli()
	startMs := endMs - int64(lookback)*1000
	period := fmt.Sprintf("%d", step)

	// 兼容新老架构：优先老实例指标名(instance_*)，V3 实例补 V3 名，各取有数据的（空的后续剪卡）
	metrics := []struct {
		key, name, unit, label, metric string
	}{
		{"cpu", "CPU 使用率", "%", "", "InstanceCpuUsageV2"},
		{"disk", "磁盘使用率", "%", "", "instance_disk_capacity"},
		{"msg_in", "生产流量", "Bps", "生产", "instance_message_input"},
		{"msg_out", "消费流量", "Bps", "消费", "instance_message_output"},
		{"msg_in", "生产流量", "Bps", "生产", "InstanceMessageInputV3"},
		{"msg_out", "消费流量", "Bps", "消费", "InstanceMessageOutputV3"},
		{"num_in", "消息生产速率", "/s", "", "instance_message_num_input"},
		{"num_in", "消息生产速率", "/s", "", "InstanceMessageNumInputV3"},
		{"conn", "连接数", "", "", "InstanceTotalConnectionV3"},
		{"part", "分区使用率", "%", "", "PartitionInstanceRatioV3"},
	}
	seen := map[string]bool{}
	for _, m := range metrics {
		if seen[m.key] {
			continue // 同一卡已取到数据（老名优先），跳过备用名
		}
		groups, err := cmsMetric(ds, endpoint, "acs_kafka", instID, m.metric, startMs, endMs, period, "")
		if err != nil || len(groups) == 0 {
			continue
		}
		res.Series = append(res.Series, MetricSeries{Key: m.key, Name: m.name, Unit: m.unit, Label: m.label, Points: groups[0].points})
		seen[m.key] = true
	}

	res.Cards = []MetricCard{
		{Key: "cpu", Name: "CPU 使用率", Unit: "%", SeriesKeys: []string{"cpu"}},
		{Key: "disk", Name: "磁盘使用率", Unit: "%", SeriesKeys: []string{"disk"}},
		{Key: "flow", Name: "流量（生产/消费）", Unit: "Bps", SeriesKeys: []string{"msg_in", "msg_out"}},
		{Key: "num_in", Name: "消息生产速率", Unit: "/s", SeriesKeys: []string{"num_in"}},
		{Key: "conn", Name: "连接数", Unit: "", SeriesKeys: []string{"conn"}},
		{Key: "part", Name: "分区使用率", Unit: "%", SeriesKeys: []string{"part"}},
	}
	res.DetailURL = fmt.Sprintf("https://kafka.console.aliyun.com/%s/instances/%s/cloud-monitor", region, instID)
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 Kafka 在云监控中暂无数据"
	}
	return res, nil
}
