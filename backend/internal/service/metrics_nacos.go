package service

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// mseClusterIDRe 从 Nacos 接入点（如 mse-8009e590-nacos-ans.mse.aliyuncs.com）提取 ClusterId（mse-8009e590）。
var mseClusterIDRe = regexp.MustCompile(`mse-[0-9a-z]+`)

// nacosMetrics 阿里云 MSE Nacos 监控。
// 说明：MSE 注册配置中心（Nacos）在云监控中为「应用分组型」监控，CloudMonitor DescribeMetricList
// 不暴露实例级时序（acs_mse 指标元数据为空），因此无法内嵌曲线；提供「完整监控」深链到 MSE 控制台监控页。
func (s *MetricsService) nacosMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	if ds.Type != model.DataSourceAliyun {
		res.Message = "该来源暂不支持 Nacos 监控（" + ds.Type + "）"
		return res, nil
	}
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" {
		res.Message = "该 Nacos 无阿里云实例 ID（resource_id），无法关联监控"
		return res, nil
	}
	region := firstField(data, "region", "bk_region", "bk_zone")
	if region == "" {
		region = parseRegions(ds.Region)[0]
	}
	res.Target = instID
	res.Supported = true
	res.Message = "阿里云 MSE Nacos 的监控数据未开放 API 对接，请点击右上角「完整监控」前往阿里云 MSE 控制台查看"

	// 「完整监控」深链到 MSE 控制台监控页。ClusterId 从接入点提取，VersionCode 由版本推导。
	q := url.Values{}
	q.Set("InstanceId", instID)
	q.Set("ClusterType", "Nacos-Ans")
	q.Set("region", region)
	if cid := mseClusterIDRe.FindString(firstField(data, "endpoint")); cid != "" {
		q.Set("ClusterId", cid)
	}
	if name := firstField(data, "inst_name"); name != "" {
		q.Set("ClusterName", name)
	}
	if ver := firstField(data, "version"); ver != "" {
		q.Set("AppVersion", ver)
		q.Set("VersionCode", "NACOS_"+strings.ReplaceAll(ver, ".", "_"))
	}
	res.DetailURL = "https://mse.console.aliyun.com/#/Instance/Observe/Monitor?" + q.Encode()
	return res, nil
}
