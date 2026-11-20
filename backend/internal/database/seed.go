package database

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cmdb/cmdb-backend/internal/model"
	"github.com/cmdb/cmdb-backend/internal/service"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Seed 初始化内置分组、模型、字段与关联类型（幂等）。
func Seed(db *gorm.DB) error {
	// 1. 内置分组
	hostGroup := model.ModelGroup{Code: "hosts", Name: "主机资源", Index: 1, IsBuiltin: true}
	orgGroup := model.ModelGroup{Code: "organization", Name: "组织架构", Index: 2, IsBuiltin: true}
	dbGroup := model.ModelGroup{Code: "database", Name: "数据库", Index: 3, IsBuiltin: true}
	mwGroup := model.ModelGroup{Code: "middleware", Name: "中间件", Index: 4, IsBuiltin: true}
	netGroup := model.ModelGroup{Code: "network", Name: "网络设备", Index: 5, IsBuiltin: true}
	lbGroup := model.ModelGroup{Code: "loadbalancer", Name: "负载均衡", Index: 6, IsBuiltin: true}
	cdnGroup := model.ModelGroup{Code: "domain_cdn", Name: "域名CDN", Index: 7, IsBuiltin: true}
	storageGroup := model.ModelGroup{Code: "storage", Name: "存储设备", Index: 8, IsBuiltin: true}
	containerGroup := model.ModelGroup{Code: "container", Name: "容器编排", Index: 9, IsBuiltin: true}
	bigdataGroup := model.ModelGroup{Code: "bigdata", Name: "大数据", Index: 10, IsBuiltin: true}
	for _, g := range []*model.ModelGroup{&hostGroup, &orgGroup, &dbGroup, &mwGroup, &netGroup, &lbGroup, &cdnGroup, &storageGroup, &containerGroup, &bigdataGroup} {
		if err := ensureGroup(db, g); err != nil {
			return err
		}
	}

	// 2. 内置模型
	hostModel := model.Model{GroupID: hostGroup.ID, ObjID: "host", Name: "主机", Icon: "Monitor", IsBuiltin: true, Enabled: true}
	bizModel := model.Model{GroupID: orgGroup.ID, ObjID: "biz", Name: "业务", Icon: "OfficeBuilding", IsBuiltin: true, Enabled: true}
	setModel := model.Model{GroupID: orgGroup.ID, ObjID: "set", Name: "集群", Icon: "Grid", IsBuiltin: true, Enabled: true}
	moduleModel := model.Model{GroupID: orgGroup.ID, ObjID: "module", Name: "模块", Icon: "Coin", IsBuiltin: true, Enabled: true}
	// 数据库与中间件
	mysqlModel := model.Model{GroupID: dbGroup.ID, ObjID: "mysql", Name: "MySQL", Icon: "Coin", IsBuiltin: true, Enabled: true}
	redisModel := model.Model{GroupID: dbGroup.ID, ObjID: "redis", Name: "Redis", Icon: "Lightning", IsBuiltin: true, Enabled: true}
	kafkaModel := model.Model{GroupID: mwGroup.ID, ObjID: "kafka", Name: "Kafka", Icon: "Connection", IsBuiltin: true, Enabled: true}
	lindormModel := model.Model{GroupID: dbGroup.ID, ObjID: "lindorm", Name: "Lindorm", Icon: "DataAnalysis", IsBuiltin: true, Enabled: true}
	elasticModel := model.Model{GroupID: dbGroup.ID, ObjID: "elastic", Name: "Elasticsearch", Icon: "Search", IsBuiltin: true, Enabled: true}
	// 网络与负载均衡
	nlbModel := model.Model{GroupID: lbGroup.ID, ObjID: "nlb", Name: "NLB", Icon: "Share", IsBuiltin: true, Enabled: true}
	albModel := model.Model{GroupID: lbGroup.ID, ObjID: "alb", Name: "ALB", Icon: "SetUp", IsBuiltin: true, Enabled: true}
	clbModel := model.Model{GroupID: lbGroup.ID, ObjID: "clb", Name: "CLB", Icon: "Share", IsBuiltin: true, Enabled: true}
	eipModel := model.Model{GroupID: netGroup.ID, ObjID: "eip", Name: "弹性公网IP", Icon: "Position", IsBuiltin: true, Enabled: true}
	wafModel := model.Model{GroupID: cdnGroup.ID, ObjID: "waf", Name: "WAF", Icon: "Lock", IsBuiltin: true, Enabled: true}
	domainModel := model.Model{GroupID: cdnGroup.ID, ObjID: "domain", Name: "域名", Icon: "Link", IsBuiltin: true, Enabled: true}
	cdnModel := model.Model{GroupID: cdnGroup.ID, ObjID: "cdn", Name: "CDN", Icon: "Lightning", IsBuiltin: true, Enabled: true}
	vpcModel := model.Model{GroupID: netGroup.ID, ObjID: "vpc", Name: "专有网络VPC", Icon: "Grid", IsBuiltin: true, Enabled: true}
	vswitchModel := model.Model{GroupID: netGroup.ID, ObjID: "vswitch", Name: "交换机", Icon: "Operation", IsBuiltin: true, Enabled: true}
	natgwModel := model.Model{GroupID: netGroup.ID, ObjID: "natgw", Name: "NAT网关", Icon: "Switch", IsBuiltin: true, Enabled: true}
	cenModel := model.Model{GroupID: netGroup.ID, ObjID: "cen", Name: "云企业网", Icon: "Connection", IsBuiltin: true, Enabled: true}
	havipModel := model.Model{GroupID: netGroup.ID, ObjID: "havip", Name: "高可用虚拟IP", Icon: "Position", IsBuiltin: true, Enabled: true}
	expressConnectModel := model.Model{GroupID: netGroup.ID, ObjID: "express_connect", Name: "高速通道", Icon: "Link", IsBuiltin: true, Enabled: true}
	vbrModel := model.Model{GroupID: netGroup.ID, ObjID: "vbr", Name: "边界路由器", Icon: "Switch", IsBuiltin: true, Enabled: true}
	apigwModel := model.Model{GroupID: lbGroup.ID, ObjID: "apigw", Name: "API网关", Icon: "Connection", IsBuiltin: true, Enabled: true}
	ossModel := model.Model{GroupID: storageGroup.ID, ObjID: "oss", Name: "对象存储OSS", Icon: "FolderOpened", IsBuiltin: true, Enabled: true}
	k8sModel := model.Model{GroupID: containerGroup.ID, ObjID: "kubernetes", Name: "Kubernetes", Icon: "Box", IsBuiltin: true, Enabled: true}
	k8sAppModel := model.Model{GroupID: containerGroup.ID, ObjID: "k8s_app", Name: "应用服务", Icon: "Grid", IsBuiltin: true, Enabled: true}
	nacosModel := model.Model{GroupID: mwGroup.ID, ObjID: "nacos", Name: "Nacos", Icon: "Compass", IsBuiltin: true, Enabled: true}
	xxljobModel := model.Model{GroupID: mwGroup.ID, ObjID: "xxljob", Name: "XXL-JOB", Icon: "Timer", IsBuiltin: true, Enabled: true}
	minioModel := model.Model{GroupID: storageGroup.ID, ObjID: "minio", Name: "MinIO", Icon: "FolderOpened", IsBuiltin: true, Enabled: true}
	nginxModel := model.Model{GroupID: lbGroup.ID, ObjID: "nginx", Name: "Nginx", Icon: "Connection", IsBuiltin: true, Enabled: true}
	emailModel := model.Model{GroupID: mwGroup.ID, ObjID: "email", Name: "邮箱", Icon: "Message", IsBuiltin: true, Enabled: true}
	dnsRecordModel := model.Model{GroupID: cdnGroup.ID, ObjID: "dns_record", Name: "DNS解析", Icon: "List", IsBuiltin: true, Enabled: true}
	maxcomputeModel := model.Model{GroupID: bigdataGroup.ID, ObjID: "maxcompute", Name: "MaxCompute", Icon: "DataAnalysis", IsBuiltin: true, Enabled: true}
	hologresModel := model.Model{GroupID: bigdataGroup.ID, ObjID: "hologres", Name: "Hologres", Icon: "Histogram", IsBuiltin: true, Enabled: true}
	dataworksModel := model.Model{GroupID: bigdataGroup.ID, ObjID: "dataworks", Name: "DataWorks", Icon: "DataLine", IsBuiltin: true, Enabled: true}
	mseModel := model.Model{GroupID: mwGroup.ID, ObjID: "mse", Name: "ZooKeeper", Icon: "Connection", IsBuiltin: true, Enabled: true}
	for _, m := range []*model.Model{
		&hostModel, &bizModel, &setModel, &moduleModel,
		&mysqlModel, &redisModel, &kafkaModel, &lindormModel, &elasticModel, &nlbModel, &albModel, &clbModel, &eipModel,
		&wafModel, &domainModel, &cdnModel, &vpcModel, &vswitchModel, &natgwModel, &cenModel, &havipModel, &expressConnectModel, &vbrModel, &apigwModel, &ossModel, &k8sModel, &k8sAppModel,
		&nacosModel, &xxljobModel, &minioModel, &nginxModel, &emailModel, &dnsRecordModel, &maxcomputeModel, &hologresModel, &dataworksModel,
		&mseModel,
	} {
		if err := ensureModel(db, m); err != nil {
			return err
		}
	}

	// 3. 内置字段
	statusOptions := datatypes.JSON([]byte(`[{"id":"1","name":"运行中"},{"id":"2","name":"停用"}]`))
	// 服务类资源通用状态
	svcStatus := datatypes.JSON([]byte(`[{"id":"running","name":"运行中"},{"id":"stopped","name":"已停止"},{"id":"abnormal","name":"异常"},{"id":"creating","name":"创建中"}]`))
	nacosMode := datatypes.JSON([]byte(`[{"id":"standalone","name":"单机"},{"id":"cluster","name":"集群"}]`))
	redisArch := datatypes.JSON([]byte(`[{"id":"standard","name":"标准版"},{"id":"cluster","name":"集群版"},{"id":"rwsplit","name":"读写分离"}]`))
	lindormEngine := datatypes.JSON([]byte(`[{"id":"wide","name":"宽表"},{"id":"tsdb","name":"时序"},{"id":"search","name":"搜索"},{"id":"file","name":"文件"}]`))
	lindormDisk := datatypes.JSON([]byte(`[{"id":"cloud_efficiency","name":"高效云盘"},{"id":"cloud_ssd","name":"SSD云盘"},{"id":"cloud_essd","name":"ESSD云盘"},{"id":"cloud_essd_pl0","name":"ESSD PL0云盘"},{"id":"local_ssd","name":"本地SSD盘"},{"id":"capacity_cloud_storage","name":"容量型云存储"}]`))
	clbAddrType := datatypes.JSON([]byte(`[{"id":"internet","name":"公网"},{"id":"intranet","name":"私网"}]`))
	cloudProvider := datatypes.JSON([]byte(`[{"id":"aliyun","name":"阿里云"},{"id":"tencent","name":"腾讯云"},{"id":"huawei","name":"华为云"},{"id":"aws","name":"AWS"},{"id":"azure","name":"Azure"},{"id":"idc","name":"IDC"},{"id":"other","name":"其他"}]`))
	eipBilling := datatypes.JSON([]byte(`[{"id":"bandwidth","name":"按带宽"},{"id":"traffic","name":"按流量"}]`))
	eipISP := datatypes.JSON([]byte(`[{"id":"bgp","name":"BGP(多线)"},{"id":"telecom","name":"电信"},{"id":"unicom","name":"联通"},{"id":"mobile","name":"移动"}]`))
	eipBind := datatypes.JSON([]byte(`[{"id":"unbound","name":"未绑定"},{"id":"ecs","name":"云主机"},{"id":"nlb","name":"NLB"},{"id":"alb","name":"ALB"},{"id":"nat","name":"NAT网关"}]`))
	wafMode := datatypes.JSON([]byte(`[{"id":"block","name":"拦截"},{"id":"alert","name":"告警"},{"id":"watch","name":"仅观察"}]`))
	cdnType := datatypes.JSON([]byte(`[{"id":"web","name":"图片小文件"},{"id":"download","name":"大文件下载"},{"id":"video","name":"视音频点播"},{"id":"liveStream","name":"直播流媒体"}]`))
	cdnCoverage := datatypes.JSON([]byte(`[{"id":"domestic","name":"仅中国内地"},{"id":"overseas","name":"全球(不含中国内地)"},{"id":"global","name":"全球"}]`))
	onOff := datatypes.JSON([]byte(`[{"id":"on","name":"已开启"},{"id":"off","name":"未开启"}]`))
	domainRecord := datatypes.JSON([]byte(`[{"id":"A","name":"A"},{"id":"CNAME","name":"CNAME"},{"id":"AAAA","name":"AAAA"},{"id":"MX","name":"MX"},{"id":"TXT","name":"TXT"}]`))
	domainStatus := datatypes.JSON([]byte(`[{"id":"normal","name":"正常"},{"id":"expiring","name":"即将过期"},{"id":"expired","name":"已过期"},{"id":"disabled","name":"停用"}]`))
	ossStorage := datatypes.JSON([]byte(`[{"id":"standard","name":"标准"},{"id":"ia","name":"低频访问"},{"id":"archive","name":"归档"},{"id":"cold","name":"冷归档"}]`))
	ossAcl := datatypes.JSON([]byte(`[{"id":"private","name":"私有"},{"id":"public-read","name":"公共读"},{"id":"public-rw","name":"公共读写"}]`))
	yesNo := datatypes.JSON([]byte(`[{"id":"yes","name":"是"},{"id":"no","name":"否"}]`))
	mcType := datatypes.JSON([]byte(`[{"id":"managed","name":"托管项目"},{"id":"external","name":"外部项目"}]`))
	chargeType := datatypes.JSON([]byte(`[{"id":"PrePaid","name":"包年包月"},{"id":"PostPaid","name":"按量付费"}]`))
	apigEdition := datatypes.JSON([]byte(`[{"id":"Basic","name":"基础版"},{"id":"Professional","name":"专业版"}]`))
	mseEngine := datatypes.JSON([]byte(`[{"id":"nacos","name":"Nacos"},{"id":"zookeeper","name":"ZooKeeper"},{"id":"eureka","name":"Eureka"}]`))
	kafkaSpec := datatypes.JSON([]byte(`[{"id":"normal","name":"标准版"},{"id":"professional","name":"专业版"},{"id":"professionalForHighRead","name":"专业版(高读)"}]`))
	kafkaDisk := datatypes.JSON([]byte(`[{"id":"ssd","name":"SSD云盘"},{"id":"efficient","name":"高效云盘"}]`))
	// 应用服务（K8s 工作负载）枚举
	workloadType := datatypes.JSON([]byte(`[{"id":"Deployment","name":"Deployment"},{"id":"StatefulSet","name":"StatefulSet"},{"id":"DaemonSet","name":"DaemonSet"},{"id":"CronJob","name":"CronJob"},{"id":"Job","name":"Job"}]`))
	appSvcType := datatypes.JSON([]byte(`[{"id":"ClusterIP","name":"ClusterIP"},{"id":"NodePort","name":"NodePort"},{"id":"LoadBalancer","name":"LoadBalancer"},{"id":"Headless","name":"Headless(无头)"},{"id":"None","name":"无"}]`))
	appStatus := datatypes.JSON([]byte(`[{"id":"running","name":"运行中"},{"id":"degraded","name":"部分可用"},{"id":"stopped","name":"已停止"},{"id":"abnormal","name":"异常"}]`))
	// cloudAttrs 为资源模型追加「云厂商 + 云可用区」字段
	cloudAttrs := func(modelID uint, startIndex int) []model.Attribute {
		return []model.Attribute{
			{ModelID: modelID, PropertyID: "cloud_provider", Name: "云厂商", Type: model.FieldEnum, Editable: true, Enabled: true, Options: cloudProvider, Group: "default", Index: startIndex, IsBuiltin: true},
			{ModelID: modelID, PropertyID: "cloud_az", Name: "云可用区", Type: model.FieldSingleChar, Editable: true, Enabled: true, Group: "default", Index: startIndex + 1, IsBuiltin: true},
		}
	}
	attrs := []model.Attribute{
		{ModelID: hostModel.ID, PropertyID: "bk_host_innerip", Name: "内网IP", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: hostModel.ID, PropertyID: "bk_host_name", Name: "主机名", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: hostModel.ID, PropertyID: "bk_os_name", Name: "操作系统", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: hostModel.ID, PropertyID: "bk_cpu", Name: "CPU核数", Type: model.FieldInt, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: hostModel.ID, PropertyID: "bk_mem", Name: "内存(MB)", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: hostModel.ID, PropertyID: "bk_disk", Name: "磁盘(GB)", Type: model.FieldInt, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: hostModel.ID, PropertyID: "bk_host_status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: statusOptions, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: hostModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 10, IsBuiltin: true},
		{ModelID: bizModel.ID, PropertyID: "bk_biz_name", Name: "业务名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: bizModel.ID, PropertyID: "owner", Name: "负责人", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: bizModel.ID, PropertyID: "contact", Name: "联系方式", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: setModel.ID, PropertyID: "bk_set_name", Name: "集群名", Type: model.FieldSingleChar, Required: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: moduleModel.ID, PropertyID: "bk_module_name", Name: "模块名", Type: model.FieldSingleChar, Required: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},

		// MySQL
		{ModelID: mysqlModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: mysqlModel.ID, PropertyID: "version", Name: "版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: mysqlModel.ID, PropertyID: "endpoint", Name: "连接地址", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: mysqlModel.ID, PropertyID: "port", Name: "端口", Type: model.FieldInt, Editable: true, Default: "3306", Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: mysqlModel.ID, PropertyID: "arch", Name: "实例类型", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: mysqlModel.ID, PropertyID: "cpu_cores", Name: "CPU核数", Type: model.FieldInt, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 20, IsBuiltin: true},
		{ModelID: mysqlModel.ID, PropertyID: "mem_mb", Name: "内存(MB)", Type: model.FieldInt, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 21, IsBuiltin: true},
		{ModelID: mysqlModel.ID, PropertyID: "disk_gb", Name: "磁盘(GB)", Type: model.FieldInt, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 22, IsBuiltin: true},
		{ModelID: mysqlModel.ID, PropertyID: "spec", Name: "规格", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: mysqlModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: mysqlModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: mysqlModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// Redis
		{ModelID: redisModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: redisModel.ID, PropertyID: "version", Name: "版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: redisModel.ID, PropertyID: "endpoint", Name: "连接地址", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: redisModel.ID, PropertyID: "port", Name: "端口", Type: model.FieldInt, Editable: true, Default: "6379", Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: redisModel.ID, PropertyID: "arch", Name: "架构", Type: model.FieldEnum, Editable: true, Options: redisArch, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: redisModel.ID, PropertyID: "mem_gb", Name: "内存(GB)", Type: model.FieldInt, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: redisModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: redisModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: redisModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// Elasticsearch
		{ModelID: elasticModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: elasticModel.ID, PropertyID: "version", Name: "版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: elasticModel.ID, PropertyID: "endpoint", Name: "连接地址", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: elasticModel.ID, PropertyID: "port", Name: "端口", Type: model.FieldInt, Editable: true, Default: "9200", Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: elasticModel.ID, PropertyID: "spec", Name: "节点规格", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: elasticModel.ID, PropertyID: "cpu_cores", Name: "单节点CPU核数", Type: model.FieldInt, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: elasticModel.ID, PropertyID: "mem_gb", Name: "单节点内存(GB)", Type: model.FieldInt, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: elasticModel.ID, PropertyID: "node_count", Name: "节点数", Type: model.FieldInt, Editable: true, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: elasticModel.ID, PropertyID: "disk_gb", Name: "单节点磁盘(GB)", Type: model.FieldInt, Editable: true, Group: "default", Index: 9, IsBuiltin: true},
		{ModelID: elasticModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 10, IsBuiltin: true},
		{ModelID: elasticModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 11, IsBuiltin: true},
		{ModelID: elasticModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// Kafka（字段按阿里云消息队列 Kafka 版 GetInstanceList 同步数据设计）
		{ModelID: kafkaModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "version", Name: "版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "spec_type", Name: "版本类型", Type: model.FieldEnum, Editable: true, Options: kafkaSpec, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "endpoint", Name: "默认接入点", Type: model.FieldLongChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "ssl_endpoint", Name: "SSL接入点", Type: model.FieldLongChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "sasl_endpoint", Name: "SASL接入点", Type: model.FieldLongChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "traffic_spec", Name: "流量规格", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "disk_gb", Name: "磁盘(GB)", Type: model.FieldInt, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "disk_type", Name: "磁盘类型", Type: model.FieldEnum, Editable: true, Options: kafkaDisk, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "topic_limit", Name: "Topic上限", Type: model.FieldInt, Editable: true, Group: "default", Index: 9, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "msg_retain", Name: "消息保留(小时)", Type: model.FieldInt, Editable: true, Group: "default", Index: 10, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "paid_type", Name: "计费方式", Type: model.FieldEnum, Editable: true, Options: chargeType, Group: "default", Index: 11, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 12, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 13, IsBuiltin: true},
		{ModelID: kafkaModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// Lindorm（字段按阿里云 Lindorm GetLindormInstanceList/GetLindormInstance 同步数据设计）
		{ModelID: lindormModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: lindormModel.ID, PropertyID: "version", Name: "版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: lindormModel.ID, PropertyID: "engine", Name: "引擎", Type: model.FieldEnumMulti, Editable: true, Options: lindormEngine, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: lindormModel.ID, PropertyID: "spec", Name: "规格", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: lindormModel.ID, PropertyID: "storage_gb", Name: "存储(GB)", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: lindormModel.ID, PropertyID: "disk_category", Name: "磁盘类型", Type: model.FieldEnum, Editable: true, Options: lindormDisk, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: lindormModel.ID, PropertyID: "paid_type", Name: "计费方式", Type: model.FieldEnum, Editable: true, Options: chargeType, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: lindormModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: lindormModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 9, IsBuiltin: true},
		{ModelID: lindormModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// NLB 网络型负载均衡
		{ModelID: nlbModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: nlbModel.ID, PropertyID: "eip", Name: "EIP", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: nlbModel.ID, PropertyID: "dns_name", Name: "DNS名称", Type: model.FieldLongChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: nlbModel.ID, PropertyID: "addr_type", Name: "网络类型", Type: model.FieldEnum, Editable: true, Options: clbAddrType, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: nlbModel.ID, PropertyID: "bandwidth", Name: "带宽(Mbps)", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: nlbModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: nlbModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: nlbModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// ALB 应用型负载均衡
		{ModelID: albModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: albModel.ID, PropertyID: "eip", Name: "EIP", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: albModel.ID, PropertyID: "dns_name", Name: "DNS名称", Type: model.FieldLongChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: albModel.ID, PropertyID: "addr_type", Name: "网络类型", Type: model.FieldEnum, Editable: true, Options: clbAddrType, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: albModel.ID, PropertyID: "bandwidth", Name: "带宽(Mbps)", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: albModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: albModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: albModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// CEN 云企业网（全局资源，跨地域/跨 VPC 骨干网）
		{ModelID: cenModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: cenModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: cenModel.ID, PropertyID: "protection_level", Name: "服务等级", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: cenModel.ID, PropertyID: "description", Name: "描述", Type: model.FieldLongChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: cenModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 5, IsBuiltin: true},

		// 高可用虚拟IP（HaVip，VPC 内可在多台 ECS 间漂移的虚拟 IP）
		{ModelID: havipModel.ID, PropertyID: "ip_addr", Name: "虚拟IP", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: havipModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: havipModel.ID, PropertyID: "bound_instance", Name: "绑定实例", Type: model.FieldLongChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: havipModel.ID, PropertyID: "eip", Name: "绑定EIP", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: havipModel.ID, PropertyID: "vpc_id", Name: "VPC", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: havipModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: havipModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: havipModel.ID, PropertyID: "description", Name: "描述", Type: model.FieldLongChar, Editable: true, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: havipModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// 高速通道（Express Connect 物理专线）
		{ModelID: expressConnectModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: expressConnectModel.ID, PropertyID: "bandwidth", Name: "带宽(Mbps)", Type: model.FieldInt, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: expressConnectModel.ID, PropertyID: "spec", Name: "规格", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: expressConnectModel.ID, PropertyID: "line_operator", Name: "运营商", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: expressConnectModel.ID, PropertyID: "access_point", Name: "接入点", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: expressConnectModel.ID, PropertyID: "port_type", Name: "端口类型", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: expressConnectModel.ID, PropertyID: "peer_location", Name: "对端物理位置", Type: model.FieldLongChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: expressConnectModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: expressConnectModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 9, IsBuiltin: true},
		{ModelID: expressConnectModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// 边界路由器（VBR，建在物理专线上、接入 CEN 的路由端点）
		{ModelID: vbrModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: vbrModel.ID, PropertyID: "local_gateway_ip", Name: "本端网关IP", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: vbrModel.ID, PropertyID: "peer_gateway_ip", Name: "对端网关IP", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: vbrModel.ID, PropertyID: "vlan_id", Name: "VLAN ID", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: vbrModel.ID, PropertyID: "bandwidth", Name: "带宽(Mbps)", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: vbrModel.ID, PropertyID: "physical_connection", Name: "物理专线", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: vbrModel.ID, PropertyID: "cen", Name: "关联云企业网", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: vbrModel.ID, PropertyID: "access_point", Name: "接入点", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: vbrModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 9, IsBuiltin: true},
		{ModelID: vbrModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 10, IsBuiltin: true},
		{ModelID: vbrModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// CLB 传统型负载均衡（经典 SLB，四/七层）
		{ModelID: clbModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: clbModel.ID, PropertyID: "vip", Name: "VIP", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: clbModel.ID, PropertyID: "eip", Name: "公网IP", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: clbModel.ID, PropertyID: "addr_type", Name: "网络类型", Type: model.FieldEnum, Editable: true, Options: clbAddrType, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: clbModel.ID, PropertyID: "spec", Name: "规格", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: clbModel.ID, PropertyID: "bandwidth", Name: "带宽(Mbps)", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: clbModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: clbModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: clbModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// EIP 弹性公网IP
		{ModelID: eipModel.ID, PropertyID: "ip_addr", Name: "IP地址", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: eipModel.ID, PropertyID: "inst_name", Name: "实例名称", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: eipModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: eipModel.ID, PropertyID: "bandwidth", Name: "带宽(Mbps)", Type: model.FieldInt, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: eipModel.ID, PropertyID: "billing", Name: "计费方式", Type: model.FieldEnum, Editable: true, Options: eipBilling, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: eipModel.ID, PropertyID: "isp", Name: "线路类型", Type: model.FieldEnum, Editable: true, Options: eipISP, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: eipModel.ID, PropertyID: "bind_type", Name: "绑定类型", Type: model.FieldEnum, Editable: true, Options: eipBind, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: eipModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: eipModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 9, IsBuiltin: true},

		// 专有网络 VPC（vpc_id 作唯一键，VpcName 可能为空）
		{ModelID: vpcModel.ID, PropertyID: "vpc_id", Name: "VPC ID", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: vpcModel.ID, PropertyID: "vpc_name", Name: "名称", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: vpcModel.ID, PropertyID: "cidr", Name: "网段(CIDR)", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: vpcModel.ID, PropertyID: "is_default", Name: "是否默认", Type: model.FieldEnum, Editable: true, Options: yesNo, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: vpcModel.ID, PropertyID: "vswitch_count", Name: "交换机数", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: vpcModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: vpcModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 7, IsBuiltin: true},

		// 交换机 VSwitch（vswitch_id 作唯一键；属于某个 VPC）
		{ModelID: vswitchModel.ID, PropertyID: "vswitch_id", Name: "交换机ID", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: vswitchModel.ID, PropertyID: "vswitch_name", Name: "名称", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: vswitchModel.ID, PropertyID: "cidr", Name: "网段(CIDR)", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: vswitchModel.ID, PropertyID: "zone", Name: "可用区", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: vswitchModel.ID, PropertyID: "available_ip", Name: "可用IP数", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: vswitchModel.ID, PropertyID: "vpc_id", Name: "所属VPC", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: vswitchModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: vswitchModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 8, IsBuiltin: true},

		// MaxCompute 大数据计算（project_name 作唯一键）
		{ModelID: maxcomputeModel.ID, PropertyID: "project_name", Name: "项目名称", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: maxcomputeModel.ID, PropertyID: "owner", Name: "所有者", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: maxcomputeModel.ID, PropertyID: "proj_type", Name: "项目类型", Type: model.FieldEnum, Editable: true, Options: mcType, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: maxcomputeModel.ID, PropertyID: "quota", Name: "计算配额", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: maxcomputeModel.ID, PropertyID: "storage_gb", Name: "存储用量(GB)", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: maxcomputeModel.ID, PropertyID: "created_time", Name: "创建时间", Type: model.FieldDateTime, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: maxcomputeModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: maxcomputeModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 8, IsBuiltin: true},

		// Hologres 实时数仓（instance_id 作唯一键）
		{ModelID: hologresModel.ID, PropertyID: "instance_id", Name: "实例ID", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: hologresModel.ID, PropertyID: "inst_name", Name: "实例名称", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: hologresModel.ID, PropertyID: "holo_type", Name: "实例类型", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: hologresModel.ID, PropertyID: "version", Name: "版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: hologresModel.ID, PropertyID: "billing", Name: "计费方式", Type: model.FieldEnum, Editable: true, Options: chargeType, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: hologresModel.ID, PropertyID: "cpu_cores", Name: "CPU核数", Type: model.FieldInt, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: hologresModel.ID, PropertyID: "mem_gb", Name: "内存(GB)", Type: model.FieldInt, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: hologresModel.ID, PropertyID: "compute_nodes", Name: "计算节点数", Type: model.FieldInt, Editable: true, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: hologresModel.ID, PropertyID: "endpoint_vpc", Name: "VPC连接地址", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 9, IsBuiltin: true},
		{ModelID: hologresModel.ID, PropertyID: "endpoint_public", Name: "公网连接地址", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 10, IsBuiltin: true},
		{ModelID: hologresModel.ID, PropertyID: "endpoint_classic", Name: "经典网络连接地址", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 11, IsBuiltin: true},
		{ModelID: hologresModel.ID, PropertyID: "created_time", Name: "创建时间", Type: model.FieldDateTime, Editable: true, Group: "default", Index: 12, IsBuiltin: true},
		{ModelID: hologresModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 13, IsBuiltin: true},
		{ModelID: hologresModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 14, IsBuiltin: true},

		// DataWorks 工作空间（project_id 作唯一键）
		{ModelID: dataworksModel.ID, PropertyID: "project_id", Name: "工作空间ID", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: dataworksModel.ID, PropertyID: "project_name", Name: "工作空间名称", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: dataworksModel.ID, PropertyID: "identifier", Name: "工作空间标识", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: dataworksModel.ID, PropertyID: "owner", Name: "所有者", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: dataworksModel.ID, PropertyID: "is_default", Name: "是否默认", Type: model.FieldEnum, Editable: true, Options: yesNo, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: dataworksModel.ID, PropertyID: "description", Name: "描述", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: dataworksModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: dataworksModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 8, IsBuiltin: true},

		// WAF（inst_name 作防护域名唯一键；一个 WAF 实例下多个防护域名，实例ID 对各域名相同故不作唯一键）
		{ModelID: wafModel.ID, PropertyID: "inst_name", Name: "防护域名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: wafModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: wafModel.ID, PropertyID: "spec", Name: "规格", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: wafModel.ID, PropertyID: "mode", Name: "防护模式", Type: model.FieldEnum, Editable: true, Options: wafMode, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: wafModel.ID, PropertyID: "cname", Name: "接入CNAME", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: wafModel.ID, PropertyID: "source_addr", Name: "回源地址", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: wafModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: wafModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 8, IsBuiltin: true},

		// 域名
		{ModelID: domainModel.ID, PropertyID: "domain_name", Name: "域名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: domainModel.ID, PropertyID: "registrar", Name: "注册商", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: domainModel.ID, PropertyID: "dns_provider", Name: "域名持有者", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: domainModel.ID, PropertyID: "record_type", Name: "主解析类型", Type: model.FieldEnum, Editable: true, Options: domainRecord, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: domainModel.ID, PropertyID: "expire_date", Name: "到期日期", Type: model.FieldDate, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: domainModel.ID, PropertyID: "registration_date", Name: "注册日期", Type: model.FieldDate, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: domainModel.ID, PropertyID: "contact_email", Name: "联系人邮箱", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: domainModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: domainStatus, Group: "default", Index: 8, IsBuiltin: true},

		// DNS解析（域名级：一个域名一条，records 字段存该域名全部解析记录明细）
		{ModelID: dnsRecordModel.ID, PropertyID: "domain_name", Name: "域名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: dnsRecordModel.ID, PropertyID: "record_count", Name: "记录数", Type: model.FieldInt, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: dnsRecordModel.ID, PropertyID: "version", Name: "套餐版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: dnsRecordModel.ID, PropertyID: "records", Name: "解析记录", Type: model.FieldLongChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 10, IsBuiltin: true},

		// CDN
		{ModelID: cdnModel.ID, PropertyID: "domain", Name: "加速域名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: cdnModel.ID, PropertyID: "cname", Name: "CNAME", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: cdnModel.ID, PropertyID: "cdn_type", Name: "加速类型", Type: model.FieldEnum, Editable: true, Options: cdnType, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: cdnModel.ID, PropertyID: "source", Name: "源站", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: cdnModel.ID, PropertyID: "coverage", Name: "加速区域", Type: model.FieldEnum, Editable: true, Options: cdnCoverage, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: cdnModel.ID, PropertyID: "ssl", Name: "HTTPS", Type: model.FieldEnum, Editable: true, Options: onOff, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: cdnModel.ID, PropertyID: "description", Name: "描述", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: cdnModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: cdnModel.ID, PropertyID: "resource_id", Name: "资源ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 9, IsBuiltin: true},

		// NAT网关
		{ModelID: natgwModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: natgwModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: natgwModel.ID, PropertyID: "spec", Name: "规格", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: natgwModel.ID, PropertyID: "eip_count", Name: "绑定EIP数", Type: model.FieldInt, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: natgwModel.ID, PropertyID: "bandwidth", Name: "带宽(Mbps)", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: natgwModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: natgwModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 7, IsBuiltin: true},

		// 云原生API网关（阿里云 APIG）
		{ModelID: apigwModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: apigwModel.ID, PropertyID: "gateway_type", Name: "网关类型", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: apigwModel.ID, PropertyID: "edition", Name: "规格", Type: model.FieldEnum, Editable: true, Options: apigEdition, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: apigwModel.ID, PropertyID: "spec", Name: "实例规格", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: apigwModel.ID, PropertyID: "version", Name: "版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: apigwModel.ID, PropertyID: "replicas", Name: "节点数", Type: model.FieldInt, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: apigwModel.ID, PropertyID: "internet_endpoint", Name: "公网接入地址", Type: model.FieldLongChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: apigwModel.ID, PropertyID: "intranet_endpoint", Name: "内网接入地址", Type: model.FieldLongChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: apigwModel.ID, PropertyID: "paid_type", Name: "计费方式", Type: model.FieldEnum, Editable: true, Options: chargeType, Group: "default", Index: 9, IsBuiltin: true},
		{ModelID: apigwModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 10, IsBuiltin: true},
		{ModelID: apigwModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 11, IsBuiltin: true},
		{ModelID: apigwModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// 对象存储OSS
		{ModelID: ossModel.ID, PropertyID: "bucket", Name: "存储桶名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: ossModel.ID, PropertyID: "endpoint", Name: "访问域名", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: ossModel.ID, PropertyID: "storage_class", Name: "存储类型", Type: model.FieldEnum, Editable: true, Options: ossStorage, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: ossModel.ID, PropertyID: "acl", Name: "读写权限", Type: model.FieldEnum, Editable: true, Options: ossAcl, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: ossModel.ID, PropertyID: "used_gb", Name: "已用容量(GB)", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: ossModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: ossModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 7, IsBuiltin: true},

		// Kubernetes
		{ModelID: k8sModel.ID, PropertyID: "cluster_name", Name: "集群名称", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: k8sModel.ID, PropertyID: "version", Name: "版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: k8sModel.ID, PropertyID: "api_server", Name: "API Server", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: k8sModel.ID, PropertyID: "node_count", Name: "节点数", Type: model.FieldInt, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: k8sModel.ID, PropertyID: "pod_cidr", Name: "Pod网段", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: k8sModel.ID, PropertyID: "cni", Name: "网络插件", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: k8sModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: k8sModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: k8sModel.ID, PropertyID: "resource_id", Name: "集群ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},
		{ModelID: k8sModel.ID, PropertyID: "cluster_profile", Name: "集群形态", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 9, IsBuiltin: true},

		// Nacos
		{ModelID: nacosModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: nacosModel.ID, PropertyID: "version", Name: "版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: nacosModel.ID, PropertyID: "endpoint", Name: "接入地址", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: nacosModel.ID, PropertyID: "port", Name: "端口", Type: model.FieldInt, Editable: true, Default: "8848", Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: nacosModel.ID, PropertyID: "mode", Name: "部署模式", Type: model.FieldEnum, Editable: true, Options: nacosMode, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: nacosModel.ID, PropertyID: "node_count", Name: "节点数", Type: model.FieldInt, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: nacosModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: nacosModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: nacosModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// 注册配置中心（阿里云 MSE，ZooKeeper / Eureka；Nacos 归入 Nacos 模型）
		{ModelID: mseModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: mseModel.ID, PropertyID: "engine_type", Name: "引擎类型", Type: model.FieldEnum, Editable: true, Options: mseEngine, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: mseModel.ID, PropertyID: "version", Name: "版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: mseModel.ID, PropertyID: "spec", Name: "规格", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: mseModel.ID, PropertyID: "node_count", Name: "节点数", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: mseModel.ID, PropertyID: "endpoint", Name: "接入地址", Type: model.FieldLongChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: mseModel.ID, PropertyID: "port", Name: "端口", Type: model.FieldInt, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: mseModel.ID, PropertyID: "paid_type", Name: "计费方式", Type: model.FieldEnum, Editable: true, Options: chargeType, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: mseModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 9, IsBuiltin: true},
		{ModelID: mseModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 10, IsBuiltin: true},
		{ModelID: mseModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// XXL-JOB
		{ModelID: xxljobModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: xxljobModel.ID, PropertyID: "version", Name: "版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: xxljobModel.ID, PropertyID: "endpoint", Name: "调度中心地址", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: xxljobModel.ID, PropertyID: "port", Name: "端口", Type: model.FieldInt, Editable: true, Default: "8080", Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: xxljobModel.ID, PropertyID: "executor_count", Name: "执行器数", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: xxljobModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: xxljobModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 7, IsBuiltin: true},

		// MinIO
		{ModelID: minioModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: minioModel.ID, PropertyID: "endpoint", Name: "访问地址", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: minioModel.ID, PropertyID: "console", Name: "控制台地址", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: minioModel.ID, PropertyID: "version", Name: "版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: minioModel.ID, PropertyID: "used_gb", Name: "已用容量(GB)", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: minioModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: minioModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: minioModel.ID, PropertyID: "resource_id", Name: "实例ID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 23, IsBuiltin: true},

		// Nginx
		{ModelID: nginxModel.ID, PropertyID: "inst_name", Name: "实例名", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: nginxModel.ID, PropertyID: "version", Name: "版本", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: nginxModel.ID, PropertyID: "server_ip", Name: "服务器IP", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: nginxModel.ID, PropertyID: "listen_port", Name: "监听端口", Type: model.FieldInt, Editable: true, Default: "80", Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: nginxModel.ID, PropertyID: "config_path", Name: "配置路径", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: nginxModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: nginxModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 7, IsBuiltin: true},

		// 邮箱（企业邮箱 / 邮件服务）
		{ModelID: emailModel.ID, PropertyID: "inst_name", Name: "名称", Type: model.FieldSingleChar, Required: true, Unique: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: emailModel.ID, PropertyID: "mail_domain", Name: "邮箱域名", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: emailModel.ID, PropertyID: "provider", Name: "服务商", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: emailModel.ID, PropertyID: "smtp_host", Name: "SMTP服务器", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: emailModel.ID, PropertyID: "smtp_port", Name: "SMTP端口", Type: model.FieldInt, Editable: true, Default: "465", Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: emailModel.ID, PropertyID: "imap_host", Name: "IMAP服务器", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: emailModel.ID, PropertyID: "admin_email", Name: "管理员邮箱", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: emailModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: emailModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Editable: true, Options: svcStatus, Group: "default", Index: 9, IsBuiltin: true},
	}
	// 资源模型追加云厂商 / 云可用区（主机状态占 index 7，其余模型状态占 index 8）
	attrs = append(attrs, cloudAttrs(hostModel.ID, 8)...)
	attrs = append(attrs, cloudAttrs(eipModel.ID, 8)...)
	attrs = append(attrs, cloudAttrs(clbModel.ID, 8)...)
	for _, mid := range []uint{mysqlModel.ID, redisModel.ID, nlbModel.ID, albModel.ID} {
		attrs = append(attrs, cloudAttrs(mid, 9)...)
	}
	// Lindorm 状态占 index 9，云字段从 10 起
	attrs = append(attrs, cloudAttrs(lindormModel.ID, 10)...)
	// Kafka 状态占 index 13，云字段从 14 起
	attrs = append(attrs, cloudAttrs(kafkaModel.ID, 14)...)
	// Elasticsearch 状态占 index 11，云字段从 12 起
	attrs = append(attrs, cloudAttrs(elasticModel.ID, 12)...)
	// WAF 状态占 index 6，云字段从 7 起
	attrs = append(attrs, cloudAttrs(wafModel.ID, 7)...)
	// 云原生API网关 状态占 index 11，云字段从 12 起
	attrs = append(attrs, cloudAttrs(apigwModel.ID, 12)...)
	// NAT网关新增 resource_id，状态占 index 7，云字段从 8 起
	attrs = append(attrs, cloudAttrs(vpcModel.ID, 8)...)
	attrs = append(attrs, cloudAttrs(vswitchModel.ID, 9)...)
	attrs = append(attrs, cloudAttrs(maxcomputeModel.ID, 9)...)
	attrs = append(attrs, cloudAttrs(hologresModel.ID, 15)...)
	attrs = append(attrs, cloudAttrs(dataworksModel.ID, 9)...)
	attrs = append(attrs, cloudAttrs(natgwModel.ID, 8)...)
	// OSS 状态占 index 7，云字段从 8 起（域名、CDN 为全局资源不加云可用区）
	attrs = append(attrs, cloudAttrs(ossModel.ID, 8)...)
	// Kubernetes 状态占 index 8，云字段从 9 起
	attrs = append(attrs, cloudAttrs(k8sModel.ID, 9)...)
	// 应用服务（K8s 工作负载）字段
	attrs = append(attrs, []model.Attribute{
		{ModelID: k8sAppModel.ID, PropertyID: "app_name", Name: "应用名", Type: model.FieldSingleChar, Required: true, Editable: true, Group: "default", Index: 1, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "cluster_name", Name: "所属集群", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 2, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "namespace", Name: "命名空间", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 3, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "workload_type", Name: "类型", Type: model.FieldEnum, Options: workloadType, Editable: true, Group: "default", Index: 4, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "replicas_desired", Name: "期望副本", Type: model.FieldInt, Editable: true, Group: "default", Index: 5, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "replicas_available", Name: "可用副本", Type: model.FieldInt, Editable: true, Group: "default", Index: 6, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "image", Name: "镜像", Type: model.FieldLongChar, Editable: true, Group: "default", Index: 7, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "service_type", Name: "服务类型", Type: model.FieldEnum, Options: appSvcType, Editable: true, Group: "default", Index: 8, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "endpoint", Name: "访问地址", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 9, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "ingress_host", Name: "对外域名", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 10, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "owner", Name: "负责人", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 11, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "status", Name: "状态", Type: model.FieldEnum, Options: appStatus, Editable: true, Group: "default", Index: 12, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "region", Name: "地域", Type: model.FieldSingleChar, Editable: true, Group: "default", Index: 13, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "app_key", Name: "应用唯一键", Type: model.FieldSingleChar, Unique: true, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 24, IsBuiltin: true},
		{ModelID: k8sAppModel.ID, PropertyID: "resource_id", Name: "工作负载UID", Type: model.FieldSingleChar, Editable: true, Enabled: true, ListHidden: true, Group: "default", Index: 25, IsBuiltin: true},
	}...)
	attrs = append(attrs, cloudAttrs(k8sAppModel.ID, 15)...)
	// Nacos 状态占 8；XXL-JOB / MinIO 状态占 7
	attrs = append(attrs, cloudAttrs(nacosModel.ID, 9)...)
	// 注册配置中心(MSE) 状态占 index 10，云字段从 11 起
	attrs = append(attrs, cloudAttrs(mseModel.ID, 11)...)
	attrs = append(attrs, cloudAttrs(xxljobModel.ID, 8)...)
	attrs = append(attrs, cloudAttrs(minioModel.ID, 8)...)
	attrs = append(attrs, cloudAttrs(nginxModel.ID, 8)...)
	attrs = append(attrs, cloudAttrs(emailModel.ID, 10)...)
	for i := range attrs {
		if err := ensureAttr(db, &attrs[i]); err != nil {
			return err
		}
	}

	// 4. 内置关联类型
	kinds := []model.AssociationKind{
		{Code: "belong", Name: "属于", SrcToDstNote: "属于", DstToSrcNote: "包含", IsBuiltin: true},
		{Code: "run", Name: "运行", SrcToDstNote: "运行于", DstToSrcNote: "运行", IsBuiltin: true},
		{Code: "connect", Name: "连接", SrcToDstNote: "连接", DstToSrcNote: "被连接", IsBuiltin: true},
		{Code: "serve", Name: "服务", SrcToDstNote: "服务于", DstToSrcNote: "使用", IsBuiltin: true},
	}
	for i := range kinds {
		if err := ensureKind(db, &kinds[i]); err != nil {
			return err
		}
	}
	belongKind, runKind, connectKind, serveKind := kinds[0], kinds[1], kinds[2], kinds[3]

	// 5. 内置模型关联：运行于主机 / 连接 / 主机服务于业务（业务归属不走关联，统一用业务拓扑）
	assocs := []model.ModelAssociation{
		// 运行于主机
		{Code: "run_mysql_host", KindID: runKind.ID, SrcModelID: mysqlModel.ID, DstModelID: hostModel.ID, Mapping: model.MappingManyToMany},
		{Code: "run_redis_host", KindID: runKind.ID, SrcModelID: redisModel.ID, DstModelID: hostModel.ID, Mapping: model.MappingManyToMany},
		{Code: "run_kafka_host", KindID: runKind.ID, SrcModelID: kafkaModel.ID, DstModelID: hostModel.ID, Mapping: model.MappingManyToMany},
		{Code: "run_lindorm_host", KindID: runKind.ID, SrcModelID: lindormModel.ID, DstModelID: hostModel.ID, Mapping: model.MappingManyToMany},
		{Code: "run_elastic_host", KindID: runKind.ID, SrcModelID: elasticModel.ID, DstModelID: hostModel.ID, Mapping: model.MappingManyToMany},
		// 负载均衡连接后端主机
		{Code: "connect_nlb_host", KindID: connectKind.ID, SrcModelID: nlbModel.ID, DstModelID: hostModel.ID, Mapping: model.MappingManyToMany},
		{Code: "connect_alb_host", KindID: connectKind.ID, SrcModelID: albModel.ID, DstModelID: hostModel.ID, Mapping: model.MappingManyToMany},
		{Code: "connect_clb_host", KindID: connectKind.ID, SrcModelID: clbModel.ID, DstModelID: hostModel.ID, Mapping: model.MappingManyToMany},
		// 主机服务于业务（n:n）：主机拓扑归属唯一，跨业务使用通过该关联表达
		{Code: "serve_host_biz", KindID: serveKind.ID, SrcModelID: hostModel.ID, DstModelID: bizModel.ID, Mapping: model.MappingManyToMany},
		// 弹性公网IP：绑定到主机/NLB/ALB
		{Code: "connect_eip_host", KindID: connectKind.ID, SrcModelID: eipModel.ID, DstModelID: hostModel.ID, Mapping: model.MappingOneToOne},
		{Code: "connect_eip_nlb", KindID: connectKind.ID, SrcModelID: eipModel.ID, DstModelID: nlbModel.ID, Mapping: model.MappingOneToOne},
		{Code: "connect_eip_alb", KindID: connectKind.ID, SrcModelID: eipModel.ID, DstModelID: albModel.ID, Mapping: model.MappingOneToOne},
		{Code: "connect_eip_clb", KindID: connectKind.ID, SrcModelID: eipModel.ID, DstModelID: clbModel.ID, Mapping: model.MappingOneToOne},
		// 边缘/网络资源之间的连接
		{Code: "connect_cdn_domain", KindID: connectKind.ID, SrcModelID: cdnModel.ID, DstModelID: domainModel.ID, Mapping: model.MappingOneToOne},
		// DNS解析记录 属于 域名（一个域名多条解析记录）
		{Code: "belong_dnsrecord_domain", KindID: belongKind.ID, SrcModelID: dnsRecordModel.ID, DstModelID: domainModel.ID, Mapping: model.MappingManyToMany},
		{Code: "connect_waf_domain", KindID: connectKind.ID, SrcModelID: wafModel.ID, DstModelID: domainModel.ID, Mapping: model.MappingManyToMany},
		{Code: "connect_apigw_host", KindID: connectKind.ID, SrcModelID: apigwModel.ID, DstModelID: hostModel.ID, Mapping: model.MappingManyToMany},
		{Code: "connect_natgw_eip", KindID: connectKind.ID, SrcModelID: natgwModel.ID, DstModelID: eipModel.ID, Mapping: model.MappingManyToMany},
		// 交换机 属于 VPC（一个 VPC 下多个交换机）
		{Code: "belong_vswitch_vpc", KindID: belongKind.ID, SrcModelID: vswitchModel.ID, DstModelID: vpcModel.ID, Mapping: model.MappingManyToMany},

		// Kubernetes 运行于主机（节点）
		{Code: "run_k8s_host", KindID: runKind.ID, SrcModelID: k8sModel.ID, DstModelID: hostModel.ID, Mapping: model.MappingManyToMany},

		// 高可用虚拟IP(HaVip)：可在多台主机间漂移、可绑定 EIP、位于某个 VPC
		{Code: "connect_havip_host", KindID: connectKind.ID, SrcModelID: havipModel.ID, DstModelID: hostModel.ID, Mapping: model.MappingManyToMany},
		{Code: "connect_eip_havip", KindID: connectKind.ID, SrcModelID: eipModel.ID, DstModelID: havipModel.ID, Mapping: model.MappingOneToOne},
		{Code: "belong_havip_vpc", KindID: belongKind.ID, SrcModelID: havipModel.ID, DstModelID: vpcModel.ID, Mapping: model.MappingManyToMany},
		// 云企业网(CEN)：互通多个 VPC
		{Code: "connect_cen_vpc", KindID: connectKind.ID, SrcModelID: cenModel.ID, DstModelID: vpcModel.ID, Mapping: model.MappingManyToMany},
		// 边界路由器(VBR)：建在物理专线上、接入云企业网
		{Code: "belong_vbr_express_connect", KindID: belongKind.ID, SrcModelID: vbrModel.ID, DstModelID: expressConnectModel.ID, Mapping: model.MappingManyToMany},
		{Code: "connect_vbr_cen", KindID: connectKind.ID, SrcModelID: vbrModel.ID, DstModelID: cenModel.ID, Mapping: model.MappingManyToMany},

		// 应用服务（K8s 工作负载）：属于集群 / 连接中间件 / 应用间依赖
		{Code: "belong_k8sapp_kubernetes", KindID: belongKind.ID, SrcModelID: k8sAppModel.ID, DstModelID: k8sModel.ID, Mapping: model.MappingManyToMany},
		{Code: "connect_k8sapp_mysql", KindID: connectKind.ID, SrcModelID: k8sAppModel.ID, DstModelID: mysqlModel.ID, Mapping: model.MappingManyToMany},
		{Code: "connect_k8sapp_redis", KindID: connectKind.ID, SrcModelID: k8sAppModel.ID, DstModelID: redisModel.ID, Mapping: model.MappingManyToMany},
		{Code: "connect_k8sapp_kafka", KindID: connectKind.ID, SrcModelID: k8sAppModel.ID, DstModelID: kafkaModel.ID, Mapping: model.MappingManyToMany},
		{Code: "connect_k8sapp_nacos", KindID: connectKind.ID, SrcModelID: k8sAppModel.ID, DstModelID: nacosModel.ID, Mapping: model.MappingManyToMany},
		{Code: "connect_k8sapp_mse", KindID: connectKind.ID, SrcModelID: k8sAppModel.ID, DstModelID: mseModel.ID, Mapping: model.MappingManyToMany},
		{Code: "connect_k8sapp_app", KindID: connectKind.ID, SrcModelID: k8sAppModel.ID, DstModelID: k8sAppModel.ID, Mapping: model.MappingManyToMany},
	}
	for i := range assocs {
		if err := ensureAssoc(db, &assocs[i]); err != nil {
			return err
		}
	}
	assocID := map[string]uint{}
	for i := range assocs {
		assocID[assocs[i].Code] = assocs[i].ID
	}

	// 存量回填：历史模块/归属无 model_id 的，统一按主机类型（旧版拓扑只放主机）
	db.Model(&model.TopoNode{}).Where("node_type = ? AND (model_id = 0 OR model_id IS NULL)", model.TopoModule).Update("model_id", hostModel.ID)
	db.Model(&model.HostModule{}).Where("model_id = 0 OR model_id IS NULL").Update("model_id", hostModel.ID)

	// 6. 示例实例与实例关联：仅「全新空库」首次播种一次，之后永不重播。
	//    用持久化标记 demo_data_seeded 把关——此前 seedSampleData 每次启动都跑、ensureInstance
	//    「不存在就建」，导致用户删掉的演示数据(电商平台/支付中心/kafka 等)每次重启被复活。
	//    老库(已有实例)视为已初始化：只打标记、不播种，尊重用户已有的删除。
	var demoFlag model.Setting
	if db.Where("`key` = ?", model.SettingDemoSeeded).First(&demoFlag).Error != nil {
		var instCount int64
		db.Model(&model.Instance{}).Count(&instCount)
		if instCount == 0 { // 真·空库才播种演示数据
			if err := seedSampleData(db, map[string]uint{
				"host": hostModel.ID, "biz": bizModel.ID,
				"mysql": mysqlModel.ID, "redis": redisModel.ID, "kafka": kafkaModel.ID,
				"lindorm": lindormModel.ID, "elastic": elasticModel.ID, "nlb": nlbModel.ID, "alb": albModel.ID, "clb": clbModel.ID, "eip": eipModel.ID,
				"waf": wafModel.ID, "domain": domainModel.ID, "cdn": cdnModel.ID,
				"natgw": natgwModel.ID, "apigw": apigwModel.ID, "oss": ossModel.ID,
				"kubernetes": k8sModel.ID,
			}, assocID); err != nil {
				return err
			}
		}
		db.Create(&model.Setting{Key: model.SettingDemoSeeded, Value: "1"})
	}

	// 7. 所有模型补充「备注」字段（幂等，排在各模型字段最后）
	if err := ensureCommentFieldAll(db); err != nil {
		return err
	}

	// 8. 预设内置角色（幂等）：只读 + 按功能分工的管理角色
	if err := ensurePresetRoles(db); err != nil {
		return err
	}

	// 9. 「只读」角色默认拥有全模型查看权限（view 已按模型控制，否则只读角色将看不到任何资源）
	if err := ensureReadonlyViewPerm(db); err != nil {
		return err
	}

	// 9b.「只读」角色默认附带模型/拓扑/运营分析的「只读查看」功能权限，使其能查看这三个模块（仅查看，写操作仍仅管理角色）。
	//    幂等：仅当该角色 functions 为空时写入，不覆盖管理员后续调整。
	if err := ensureReadonlyViewFuncs(db); err != nil {
		return err
	}

	// 10. 默认把云厂商/云可用区/备注设为列表隐藏（仅首次初始化，不覆盖管理员后续调整）
	if err := ensureDefaultListHidden(db); err != nil {
		return err
	}

	// 11. 存量迁移：MySQL「架构」字段改为「实例类型」文本（ensureAttr 不更新已存在字段）
	db.Exec("UPDATE attribute SET name = '实例类型', type = 'singlechar' WHERE model_id = ? AND property_id = 'arch'", mysqlModel.ID)

	// 12. 所有模型的状态枚举补充「已下线」选项（失联标记用，幂等）
	if err := ensureOfflineStatusOption(db); err != nil {
		return err
	}

	// 13. 主机内网IP恢复唯一标记（唯一校验作用域为「同一数据源(source_id)内」，跨数据源可重复）
	db.Exec("UPDATE attribute SET `unique` = 1 WHERE model_id = ? AND property_id = 'bk_host_innerip'", hostModel.ID)

	// 14. 存量回填：按创建人名匹配数据源，填 instance.source_id（供改名平滑衔接）
	db.Exec("UPDATE instance i JOIN data_source d ON i.creator = d.name SET i.source_id = d.id WHERE i.source_id = 0")

	// 15. 存量迁移：resource_id 字段显示名「资源ID」统一改为「实例ID」
	db.Exec("UPDATE attribute SET name = '实例ID' WHERE property_id = 'resource_id' AND name = '资源ID'")

	// 16. 统一「云厂商」枚举为字符串 id（历史主机模型用数字 id，与新模型不一致），并回填实例数据
	if err := ensureCloudProviderOptions(db); err != nil {
		return err
	}

	// 17. 存量迁移：域名模型 dns_provider 字段显示名「解析商」改为「域名持有者」
	db.Exec("UPDATE attribute SET name = '域名持有者' WHERE property_id = 'dns_provider' AND name = '解析商'")

	// 18. DNS解析模型由「每条记录一实例」改为「每域名一实例」：改名、删旧字段、清旧实例、域名唯一
	if dnsID := dnsRecordModel.ID; dnsID != 0 {
		db.Exec("UPDATE model SET name = 'DNS解析' WHERE id = ? AND name = 'DNS解析记录'", dnsID)
		db.Exec("DELETE FROM attribute WHERE model_id = ? AND property_id IN ('rr','type','value','ttl','line','status','resource_id')", dnsID)
		// 仅清旧的「每条记录一实例」行（无新版 records 字段者）；新版「每域名一实例」行都带 records，保留。
		// 注意：此前误写成无条件 DELETE，导致每次重启都清空 DNS 实例——必须带 records 判断，否则重启即丢数据。
		db.Exec("DELETE FROM instance WHERE model_id = ? AND NOT JSON_CONTAINS_PATH(data, 'one', '$.records')", dnsID)
		db.Exec("UPDATE attribute SET `unique` = 1 WHERE model_id = ? AND property_id = 'domain_name'", dnsID)
		db.Exec("UPDATE attribute SET name = '域名' WHERE model_id = ? AND property_id = 'domain_name' AND name = '所属域名'", dnsID)
	}

	// 内置字段顺序归一化：这几段 UPDATE attribute index 原本每次启动都跑，会覆盖用户手动排序。
	// 用持久化标记 field_order_seeded 把关——只在首次（或尚未标记的存量库）归一化一次，之后不再重刷。
	fieldOrderDone := false
	{
		var foFlag model.Setting
		fieldOrderDone = db.Where("`key` = ?", model.SettingFieldOrderSeeded).First(&foFlag).Error == nil
	}

	// 19. EIP 字段顺序重排（存量字段 index 不会被 ensureAttr 更新），并确保「实例ID」列表可见
	if eipID := eipModel.ID; eipID != 0 {
		if !fieldOrderDone {
			eipOrder := map[string]int{
				"ip_addr": 1, "inst_name": 2, "resource_id": 3, "bandwidth": 4,
				"billing": 5, "isp": 6, "bind_type": 7, "region": 8, "status": 9,
				"cloud_provider": 10, "cloud_az": 11, "comment": 12,
			}
			for prop, idx := range eipOrder {
				db.Exec("UPDATE attribute SET `index` = ? WHERE model_id = ? AND property_id = ?", idx, eipID, prop)
			}
		}
		db.Exec("UPDATE attribute SET list_hidden = 0 WHERE model_id = ? AND property_id = 'resource_id'", eipID)
		// 「计费类型」与「计费方式」重复，若曾创建则删除
		db.Exec("DELETE FROM attribute WHERE model_id = ? AND property_id = 'charge_type'", eipID)
	}

	// 20. NAT网关 字段顺序重排，并把「实例ID」设为列表可见
	if natgwID := natgwModel.ID; natgwID != 0 {
		if !fieldOrderDone {
			natgwOrder := map[string]int{
				"inst_name": 1, "resource_id": 2, "spec": 3, "eip_count": 4, "bandwidth": 5,
				"region": 6, "status": 7, "cloud_provider": 8, "cloud_az": 9, "comment": 10,
			}
			for prop, idx := range natgwOrder {
				db.Exec("UPDATE attribute SET `index` = ? WHERE model_id = ? AND property_id = ?", idx, natgwID, prop)
			}
		}
		db.Exec("UPDATE attribute SET list_hidden = 0 WHERE model_id = ? AND property_id = 'resource_id'", natgwID)
	}

	// 21. Elasticsearch 字段顺序重排（新增 CPU/内存字段后对齐；存量字段 index 不会被 ensureAttr 更新）
	if esID := elasticModel.ID; esID != 0 && !fieldOrderDone {
		esOrder := map[string]int{
			"inst_name": 1, "version": 2, "endpoint": 3, "port": 4, "spec": 5,
			"cpu_cores": 6, "mem_gb": 7, "node_count": 8, "disk_gb": 9, "region": 10, "status": 11,
			"cloud_provider": 12, "cloud_az": 13, "resource_id": 14, "comment": 15,
		}
		for prop, idx := range esOrder {
			db.Exec("UPDATE attribute SET `index` = ? WHERE model_id = ? AND property_id = ?", idx, esID, prop)
		}
	}

	// 22. WAF 回源地址字段改名：source_ip(回源IP) → source_addr(回源地址)。
	// 回源可为 CNAME 域名或多值逗号拼接，含「IP」命名会误触单 IP 校验，故改名避开。
	if wafID := wafModel.ID; wafID != 0 {
		db.Exec("DELETE FROM attribute WHERE model_id = ? AND property_id = 'source_ip'", wafID)
		// 若此前已有以 source_ip 为键的实例数据，迁移到 source_addr（通常为空，幂等）
		db.Exec("UPDATE instance SET data = JSON_SET(data, '$.source_addr', JSON_EXTRACT(data, '$.source_ip')) WHERE model_id = ? AND JSON_CONTAINS_PATH(data, 'one', '$.source_ip')", wafID)
		db.Exec("UPDATE instance SET data = JSON_REMOVE(data, '$.source_ip') WHERE model_id = ? AND JSON_CONTAINS_PATH(data, 'one', '$.source_ip')", wafID)
	}

	// 23. WAF 模型字段调整：inst_name 显示名「实例名」→「防护域名」；删除与之重复的 protect_domain 列；
	// 隐藏的 resource_id(实例ID, 值=waf-cn-xxx) 设为可见列；并重排字段顺序。
	if wafID := wafModel.ID; wafID != 0 {
		db.Exec("UPDATE attribute SET name = '防护域名' WHERE model_id = ? AND property_id = 'inst_name'", wafID)
		db.Exec("DELETE FROM attribute WHERE model_id = ? AND property_id = 'protect_domain'", wafID)
		db.Exec("UPDATE instance SET data = JSON_REMOVE(data, '$.protect_domain') WHERE model_id = ? AND JSON_CONTAINS_PATH(data, 'one', '$.protect_domain')", wafID)
		db.Exec("UPDATE attribute SET list_hidden = 0 WHERE model_id = ? AND property_id = 'resource_id'", wafID)
		if !fieldOrderDone {
			wafOrder := map[string]int{
				"inst_name": 1, "resource_id": 2, "spec": 3, "mode": 4, "cname": 5,
				"source_addr": 6, "region": 7, "status": 8, "comment": 9,
			}
			for prop, idx := range wafOrder {
				db.Exec("UPDATE attribute SET `index` = ? WHERE model_id = ? AND property_id = ?", idx, wafID, prop)
			}
		}
	}
	// 字段顺序归一化只跑一次：打标记，之后重启/同步不再重刷，尊重用户手动排序
	if !fieldOrderDone {
		db.Create(&model.Setting{Key: model.SettingFieldOrderSeeded, Value: "1"})
	}

	// 24. VPC 网段字段显示名「IPv4网段」→「网段(CIDR)」：名含「IP」会误触单 IP 校验，而值是 CIDR(192.168.0.0/16)。
	if vpcID := vpcModel.ID; vpcID != 0 {
		db.Exec("UPDATE attribute SET name = '网段(CIDR)' WHERE model_id = ? AND property_id = 'cidr' AND name = 'IPv4网段'", vpcID)
	}

	// 25. CDN 模型按阿里云同步数据重构字段：删除旧 origin/biz_type，新字段(cname/cdn_type/source/coverage/ssl/icp/resource_id)由 ensureAttr 新建。
	if cdnID := cdnModel.ID; cdnID != 0 {
		db.Exec("DELETE FROM attribute WHERE model_id = ? AND property_id IN ('origin','biz_type')", cdnID)
		// 旧 origin 数据平滑迁到新 source（若有），再清理旧键
		db.Exec("UPDATE instance SET data = JSON_SET(data, '$.source', JSON_EXTRACT(data, '$.origin')) WHERE model_id = ? AND JSON_CONTAINS_PATH(data, 'one', '$.origin') AND NOT JSON_CONTAINS_PATH(data, 'one', '$.source')", cdnID)
		db.Exec("UPDATE instance SET data = JSON_REMOVE(data, '$.origin', '$.biz_type') WHERE model_id = ? AND (JSON_CONTAINS_PATH(data, 'one', '$.origin') OR JSON_CONTAINS_PATH(data, 'one', '$.biz_type'))", cdnID)
		// domain_id 字段名含「domain/域名」会误触域名格式校验（值是数字ID），改名为 resource_id/资源ID
		db.Exec("DELETE FROM attribute WHERE model_id = ? AND property_id = 'domain_id'", cdnID)
		db.Exec("UPDATE instance SET data = JSON_SET(data, '$.resource_id', JSON_EXTRACT(data, '$.domain_id')) WHERE model_id = ? AND JSON_CONTAINS_PATH(data, 'one', '$.domain_id')", cdnID)
		db.Exec("UPDATE instance SET data = JSON_REMOVE(data, '$.domain_id') WHERE model_id = ? AND JSON_CONTAINS_PATH(data, 'one', '$.domain_id')", cdnID)
		// 加速类型枚举名称对齐阿里云官方业务类型（web=图片小文件 等；存量字段 options 不会被 ensureAttr 更新）
		db.Exec(`UPDATE attribute SET options = ? WHERE model_id = ? AND property_id = 'cdn_type'`,
			`[{"id":"web","name":"图片小文件"},{"id":"download","name":"大文件下载"},{"id":"video","name":"视音频点播"},{"id":"liveStream","name":"直播流媒体"}]`, cdnID)
		// icp(备案号) 实为域名的自由文本 Description，并非真备案号字段，改名为 description/描述
		db.Exec("DELETE FROM attribute WHERE model_id = ? AND property_id = 'icp'", cdnID)
		db.Exec("UPDATE instance SET data = JSON_SET(data, '$.description', JSON_EXTRACT(data, '$.icp')) WHERE model_id = ? AND JSON_CONTAINS_PATH(data, 'one', '$.icp')", cdnID)
		db.Exec("UPDATE instance SET data = JSON_REMOVE(data, '$.icp') WHERE model_id = ? AND JSON_CONTAINS_PATH(data, 'one', '$.icp')", cdnID)
		cdnOrder := map[string]int{
			"domain": 1, "cname": 2, "cdn_type": 3, "source": 4, "coverage": 5,
			"ssl": 6, "description": 7, "status": 8, "resource_id": 9, "comment": 10,
		}
		if !fieldOrderDone {
			for prop, idx := range cdnOrder {
				db.Exec("UPDATE attribute SET `index` = ? WHERE model_id = ? AND property_id = ?", idx, cdnID, prop)
			}
		}
	}

	// 26. 拆分「网络与负载均衡」分组为 网络 / 负载均衡与网关 / 域名与CDN（存量分组名、模型归属 ensure 不更新）。
	db.Exec("UPDATE model_group SET name = '网络设备', `index` = 4 WHERE code = 'network'")
	db.Exec("UPDATE model_group SET `index` = 5 WHERE code = 'loadbalancer'")
	db.Exec("UPDATE model_group SET `index` = 6 WHERE code = 'domain_cdn'")
	db.Exec("UPDATE model_group SET `index` = 7 WHERE code = 'storage'")
	db.Exec("UPDATE model_group SET `index` = 8 WHERE code = 'container'")
	if lbGroup.ID != 0 {
		db.Exec("UPDATE model SET group_id = ? WHERE obj_id IN ('nlb','alb','clb','apigw','nginx')", lbGroup.ID)
	}
	if cdnGroup.ID != 0 {
		db.Exec("UPDATE model SET group_id = ? WHERE obj_id IN ('domain','dns_record','cdn','waf')", cdnGroup.ID)
	}
	if netGroup.ID != 0 {
		db.Exec("UPDATE model SET group_id = ? WHERE obj_id IN ('vpc','vswitch','eip','natgw')", netGroup.ID)
	}

	// 27. MaxCompute 新增「创建时间」后重排字段顺序（存量字段 index 不会被 ensureAttr 更新）。
	if mcID := maxcomputeModel.ID; mcID != 0 {
		mcOrder := map[string]int{
			"project_name": 1, "owner": 2, "proj_type": 3, "quota": 4, "storage_gb": 5,
			"created_time": 6, "region": 7, "status": 8, "cloud_provider": 9, "cloud_az": 10, "comment": 11,
		}
		if !fieldOrderDone {
			for prop, idx := range mcOrder {
				db.Exec("UPDATE attribute SET `index` = ? WHERE model_id = ? AND property_id = ?", idx, mcID, prop)
			}
		}
	}

	// 28. Hologres 新增计算资源(cpu/mem/节点)与三种连接地址字段，删除旧单一 endpoint 字段并重排。
	if holoID := hologresModel.ID; holoID != 0 {
		db.Exec("DELETE FROM attribute WHERE model_id = ? AND property_id = 'endpoint'", holoID)
		db.Exec("UPDATE instance SET data = JSON_REMOVE(data, '$.endpoint') WHERE model_id = ? AND JSON_CONTAINS_PATH(data, 'one', '$.endpoint')", holoID)
		holoOrder := map[string]int{
			"instance_id": 1, "inst_name": 2, "holo_type": 3, "version": 4, "billing": 5,
			"cpu_cores": 6, "mem_gb": 7, "compute_nodes": 8, "endpoint_vpc": 9, "endpoint_public": 10,
			"endpoint_classic": 11, "created_time": 12, "region": 13, "status": 14,
			"cloud_provider": 15, "cloud_az": 16, "comment": 17,
		}
		if !fieldOrderDone {
			for prop, idx := range holoOrder {
				db.Exec("UPDATE attribute SET `index` = ? WHERE model_id = ? AND property_id = ?", idx, holoID, prop)
			}
		}
	}

	// 29. 清理悬空实例关系：源或目标实例已不存在的 instance_relation 行。
	// 成因：历史迁移里用裸 DELETE 删实例（如步骤 18 清旧版 DNS解析实例）绕过了关系级联，
	// 残留的关系行会在对端详情页的关联拓扑里显示成「#<已删ID>」的幽灵节点。幂等：删完即无。
	db.Exec(`DELETE r FROM instance_relation r
	  LEFT JOIN instance si ON si.id = r.src_inst_id
	  LEFT JOIN instance di ON di.id = r.dst_inst_id
	  WHERE si.id IS NULL OR di.id IS NULL`)

	// 30. Kafka 模型字段按阿里云同步数据重构：删除无同步来源的 port/broker_count，
	// 新增 spec_type/traffic_spec/disk_type/topic_limit/msg_retain/paid_type/resource_id（由 ensureAttr 新建），并重排 index。
	if kafkaID := kafkaModel.ID; kafkaID != 0 {
		db.Exec("DELETE FROM attribute WHERE model_id = ? AND property_id IN ('port','broker_count')", kafkaID)
		db.Exec("UPDATE instance SET data = JSON_REMOVE(data, '$.port', '$.broker_count') WHERE model_id = ? AND (JSON_CONTAINS_PATH(data, 'one', '$.port') OR JSON_CONTAINS_PATH(data, 'one', '$.broker_count'))", kafkaID)
		if !fieldOrderDone {
			kafkaOrder := map[string]int{
				"inst_name": 1, "resource_id": 2, "version": 3, "spec_type": 4, "endpoint": 5, "ssl_endpoint": 6,
				"traffic_spec": 7, "disk_gb": 8, "disk_type": 9, "topic_limit": 10, "msg_retain": 11,
				"paid_type": 12, "region": 13, "status": 14, "cloud_provider": 15, "cloud_az": 16,
			}
			for prop, idx := range kafkaOrder {
				db.Exec("UPDATE attribute SET `index` = ? WHERE model_id = ? AND property_id = ?", idx, kafkaID, prop)
			}
		}
		// 接入点改用域名并支持多类型：旧「接入点(singlechar)」→「默认接入点(longchar)」，
		// 新增「SSL接入点」由 ensureAttr 建好。存量行 name/type 不会被 ensureAttr 更新，这里显式对齐。
		db.Exec("UPDATE attribute SET name = '默认接入点', type = ? WHERE model_id = ? AND property_id = 'endpoint'", model.FieldLongChar, kafkaID)
	}

	// NLB 模型字段调整：删除 protocol/listen_port/spec（监听级/无同步来源），新增 dns_name/addr_type（由 ensureAttr 新建）。
	if nlbID := nlbModel.ID; nlbID != 0 {
		db.Exec("DELETE FROM attribute WHERE model_id = ? AND property_id IN ?", nlbID, []string{"protocol", "listen_port", "spec", "vip"})
		db.Exec("UPDATE instance SET data = JSON_REMOVE(data, '$.protocol', '$.listen_port', '$.spec', '$.vip') WHERE model_id = ? AND (JSON_CONTAINS_PATH(data, 'one', '$.protocol', '$.listen_port', '$.spec', '$.vip'))", nlbID)
	}

	// ALB 模型字段调整：删除 endpoint/protocol/listen_port/spec/vip，新增 eip(多可用区EIP,逗号分隔)/dns_name/addr_type/resource_id。
	if albID := albModel.ID; albID != 0 {
		db.Exec("DELETE FROM attribute WHERE model_id = ? AND property_id IN ?", albID, []string{"endpoint", "protocol", "listen_port", "spec", "vip"})
		db.Exec("UPDATE instance SET data = JSON_REMOVE(data, '$.endpoint', '$.protocol', '$.listen_port', '$.spec', '$.vip') WHERE model_id = ? AND (JSON_CONTAINS_PATH(data, 'one', '$.endpoint', '$.protocol', '$.listen_port', '$.spec', '$.vip'))", albID)
	}

	// 31. Lindorm 模型字段按阿里云同步数据重构：删除无同步来源的 endpoint(连接地址，Lindorm 连接地址按引擎分离、接口不返回)，
	// 新增 disk_category/paid_type/resource_id（由 ensureAttr 新建），并重排 index。
	if lindormID := lindormModel.ID; lindormID != 0 {
		db.Exec("DELETE FROM attribute WHERE model_id = ? AND property_id = 'endpoint'", lindormID)
		db.Exec("UPDATE instance SET data = JSON_REMOVE(data, '$.endpoint') WHERE model_id = ? AND JSON_CONTAINS_PATH(data, 'one', '$.endpoint')", lindormID)
		if !fieldOrderDone {
			lindormOrder := map[string]int{
				"inst_name": 1, "resource_id": 2, "version": 3, "engine": 4, "spec": 5, "storage_gb": 6,
				"disk_category": 7, "paid_type": 8, "region": 9, "status": 10,
				"cloud_provider": 11, "cloud_az": 12,
			}
			for prop, idx := range lindormOrder {
				db.Exec("UPDATE attribute SET `index` = ? WHERE model_id = ? AND property_id = ?", idx, lindormID, prop)
			}
		}
	}

	// 32. API网关模型由「传统 API 网关(CloudAPI)」字段改造为「云原生API网关(APIG)」字段：
	// 删除无同步来源的旧字段 endpoint/group_count/api_count，新增字段由 ensureAttr 建好，并重排 index。
	if apigwID := apigwModel.ID; apigwID != 0 {
		db.Exec("DELETE FROM attribute WHERE model_id = ? AND property_id IN ('endpoint','group_count','api_count')", apigwID)
		db.Exec("UPDATE instance SET data = JSON_REMOVE(data, '$.endpoint', '$.group_count', '$.api_count') WHERE model_id = ? AND (JSON_CONTAINS_PATH(data, 'one', '$.endpoint') OR JSON_CONTAINS_PATH(data, 'one', '$.group_count') OR JSON_CONTAINS_PATH(data, 'one', '$.api_count'))", apigwID)
		apigwOrder := map[string]int{
			"inst_name": 1, "gateway_type": 2, "edition": 3, "spec": 4, "version": 5,
			"replicas": 6, "internet_endpoint": 7, "intranet_endpoint": 8, "paid_type": 9,
			"region": 10, "status": 11, "cloud_provider": 12, "cloud_az": 13, "resource_id": 23,
		}
		if !fieldOrderDone {
			for prop, idx := range apigwOrder {
				db.Exec("UPDATE attribute SET `index` = ? WHERE model_id = ? AND property_id = ?", idx, apigwID, prop)
			}
		}
	}

	// 33. 注册配置中心(mse) 模型更名为 ZooKeeper（Nacos 已拆分到 Nacos 模型，mse 模型仅承载 ZooKeeper/Eureka）。
	db.Exec("UPDATE model SET name = 'ZooKeeper' WHERE obj_id = 'mse' AND name = '注册配置中心'")

	// 34. 「数据库与中间件」拆分为「数据库」+「中间件」：原 middleware 组改名「中间件」，
	// 新建「数据库」组(ensureGroup 已建)，把数据库类模型移入「数据库」组，并重排各组顺序。
	db.Exec("UPDATE model_group SET name = '中间件', `index` = 4 WHERE code = 'middleware'")
	if dbGroup.ID != 0 {
		db.Exec("UPDATE model SET group_id = ? WHERE obj_id IN ('mysql','redis','lindorm','elastic')", dbGroup.ID)
	}
	if mwGroup.ID != 0 {
		db.Exec("UPDATE model SET group_id = ? WHERE obj_id IN ('kafka','nacos','xxljob','email','mse')", mwGroup.ID)
	}
	groupOrder := map[string]int{
		"database": 3, "middleware": 4, "network": 5, "loadbalancer": 6,
		"domain_cdn": 7, "storage": 8, "container": 9, "bigdata": 10,
	}
	for code, idx := range groupOrder {
		db.Exec("UPDATE model_group SET `index` = ? WHERE code = ?", idx, code)
	}
	// 35. 分组名称统一为 2–4 字（缩短带「与」的长名，视觉更整齐）。
	groupRename := map[string]string{
		"loadbalancer": "负载均衡", "domain_cdn": "域名CDN", "container": "容器编排",
		"network": "网络设备", "storage": "存储设备",
	}
	for code, name := range groupRename {
		db.Exec("UPDATE model_group SET name = ? WHERE code = ?", name, code)
	}

	// 36. 主机状态枚举 id 1/2 的英文名改为中文（仅当仍是英文 Running/Down 时才改，不覆盖用户后续自定义）。
	if hostModel.ID != 0 {
		var a model.Attribute
		if db.Where("model_id = ? AND property_id = ?", hostModel.ID, "bk_host_status").First(&a).Error == nil {
			var opts []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			if json.Unmarshal(a.Options, &opts) == nil {
				changed := false
				for i := range opts {
					if opts[i].ID == "1" && opts[i].Name == "Running" {
						opts[i].Name = "运行中"
						changed = true
					}
					if opts[i].ID == "2" && opts[i].Name == "Down" {
						opts[i].Name = "宕机"
						changed = true
					}
				}
				if changed {
					if b, e := json.Marshal(opts); e == nil {
						db.Model(&model.Attribute{}).Where("id = ?", a.ID).Update("options", datatypes.JSON(b))
					}
				}
			}
		}
	}

	// 37. 业务拓扑加「业务」层（业务线→业务→集群→模块）。历史集群直接挂在业务线下
	//     (topo_node.parent_id == biz_id)，为每个这样的业务线建一个「默认业务」节点，
	//     把其下集群改挂到该业务下。幂等：只处理 parent_id==biz_id 的集群(旧结构)，
	//     迁移后它们 parent 指向业务节点，再跑即跳过；默认业务按名复用不重复建。
	{
		var orphanSets []model.TopoNode
		db.Where("node_type = ? AND parent_id = biz_id", model.TopoSet).Find(&orphanSets)
		byBiz := map[uint][]uint{}
		for _, st := range orphanSets {
			byBiz[st.BizID] = append(byBiz[st.BizID], st.ID)
		}
		for bizID, setIDs := range byBiz {
			var bus model.TopoNode
			if db.Where("biz_id = ? AND node_type = ? AND name = ?", bizID, model.TopoBusiness, "默认业务").First(&bus).Error != nil {
				bus = model.TopoNode{NodeType: model.TopoBusiness, Name: "默认业务", BizID: bizID, ParentID: bizID}
				if err := db.Create(&bus).Error; err != nil {
					return err
				}
			}
			if err := db.Model(&model.TopoNode{}).Where("id IN ?", setIDs).Update("parent_id", bus.ID).Error; err != nil {
				return err
			}
		}
	}

	// 38. 新增「业务线」字段到所有资源模型（枚举，选项=业务线列表；手填、与拓扑解耦）。
	//     幂等：仅对无 biz_line 字段的资源模型补建(排除 biz/set/module 组织模型)，字段排末尾；
	//     选项每次刷新为当前业务线；按拓扑归属(host_module.biz_id)回填初始值(仅当前为空的实例)。
	{
		var resModels []model.Model
		db.Where("obj_id NOT IN ?", []string{"biz", "set", "module"}).Find(&resModels)
		for _, m := range resModels {
			var cnt int64
			db.Model(&model.Attribute{}).Where("model_id = ? AND property_id = ?", m.ID, "biz_line").Count(&cnt)
			if cnt > 0 {
				continue
			}
			var maxIdx int
			db.Model(&model.Attribute{}).Where("model_id = ?", m.ID).Select("COALESCE(MAX(`index`),0)").Scan(&maxIdx)
			db.Create(&model.Attribute{
				ModelID: m.ID, PropertyID: "biz_line", Name: "业务线",
				Type: model.FieldEnum, Editable: true, Enabled: true,
				Group: "default", Index: maxIdx + 1, IsBuiltin: true,
			})
		}
		// 刷新业务线选项为当前业务线
		_ = service.SyncBizLineOptions(db)
		// 按拓扑归属回填初始值（仅当前 biz_line 为空/不存在的实例）
		db.Exec(`UPDATE instance i JOIN host_module hm ON hm.host_id = i.id
		         SET i.data = JSON_SET(i.data, '$.biz_line', CAST(hm.biz_id AS CHAR))
		         WHERE JSON_EXTRACT(i.data, '$.biz_line') IS NULL
		            OR JSON_UNQUOTE(JSON_EXTRACT(i.data, '$.biz_line')) = ''`)
	}

	// 39. 去掉「集群」层，拓扑改为 业务线→业务→模块（自由画布）。历史集群(node_type=set)的父是业务节点
	//     (step37 之后)，把集群下的模块改挂到集群的父业务上，再删除集群节点；给无坐标的模块分配网格初始位置。
	//     幂等：迁移后已无 set 节点，再跑跳过；坐标仅对 (0,0) 的模块赋值(赋值后首个为 60,60 ≠ 0 不再触发)。
	{
		var sets []model.TopoNode
		db.Where("node_type = ?", model.TopoSet).Find(&sets)
		for _, set := range sets {
			db.Model(&model.TopoNode{}).
				Where("parent_id = ? AND node_type = ?", set.ID, model.TopoModule).
				Update("parent_id", set.ParentID) // set.ParentID = 所属业务节点
		}
		if len(sets) > 0 {
			ids := make([]uint, len(sets))
			for i, st := range sets {
				ids[i] = st.ID
			}
			db.Where("id IN ?", ids).Delete(&model.TopoNode{})
		}
		// 给坐标为 0 的模块按业务分组排网格初始位置
		var businesses []model.TopoNode
		db.Where("node_type = ?", model.TopoBusiness).Find(&businesses)
		for _, b := range businesses {
			var mods []model.TopoNode
			db.Where("parent_id = ? AND node_type = ?", b.ID, model.TopoModule).Order("id asc").Find(&mods)
			for i, m := range mods {
				if m.PosX == 0 && m.PosY == 0 {
					db.Model(&model.TopoNode{}).Where("id = ?", m.ID).Updates(map[string]interface{}{
						"pos_x": 60 + (i%4)*200,
						"pos_y": 60 + (i/4)*130,
					})
				}
			}
		}
	}

	return nil
}

// ensureCloudProviderOptions 把所有模型的「云厂商」枚举统一为规范的字符串 id，
// 并把历史实例数据里的旧数字 id（1=阿里云…）回填为对应字符串 id。幂等。
func ensureCloudProviderOptions(db *gorm.DB) error {
	opts := `[{"id":"aliyun","name":"阿里云"},{"id":"tencent","name":"腾讯云"},{"id":"huawei","name":"华为云"},{"id":"aws","name":"AWS"},{"id":"azure","name":"Azure"},{"id":"idc","name":"IDC"},{"id":"other","name":"其他"}]`
	if err := db.Exec("UPDATE attribute SET options = ? WHERE property_id = 'cloud_provider'", opts).Error; err != nil {
		return err
	}
	num2str := map[string]string{"1": "aliyun", "2": "tencent", "3": "huawei", "4": "aws", "5": "azure", "6": "idc", "7": "other"}
	for num, str := range num2str {
		db.Exec("UPDATE instance SET data = JSON_SET(data, '$.cloud_provider', ?) WHERE JSON_UNQUOTE(JSON_EXTRACT(data, '$.cloud_provider')) = ?", str, num)
	}
	return nil
}

// OfflineStatusID 失联/下线状态的统一选项 id。
const OfflineStatusID = model.OfflineStatusID

// ensureOfflineStatusOption 为所有状态枚举字段(property_id=status/bk_host_status)追加「已下线」选项。
func ensureOfflineStatusOption(db *gorm.DB) error {
	var attrs []model.Attribute
	if err := db.Where("type = ? AND property_id IN ?", model.FieldEnum, []string{"status", "bk_host_status"}).Find(&attrs).Error; err != nil {
		return err
	}
	type opt struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	for i := range attrs {
		var opts []opt
		_ = json.Unmarshal(attrs[i].Options, &opts)
		// 去重：名为「已下线」的选项只保留一个(id=offline)，其余（如历史 id=3）剔除并把数据迁移到 offline
		kept := make([]opt, 0, len(opts))
		dupIDs := []string{}
		offlineSeen := false
		for _, o := range opts {
			if o.Name == "已下线" || o.ID == OfflineStatusID {
				if o.ID == OfflineStatusID {
					offlineSeen = true
				} else {
					dupIDs = append(dupIDs, o.ID) // 重复的旧「已下线」
				}
				continue
			}
			kept = append(kept, o)
		}
		kept = append(kept, opt{ID: OfflineStatusID, Name: "已下线"}) // 统一一个已下线
		_ = offlineSeen
		// 迁移使用旧 id 的实例 → offline，避免数据变「未知」
		path := "$." + attrs[i].PropertyID
		for _, dup := range dupIDs {
			db.Model(&model.Instance{}).
				Where("model_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, ?)) = ?", attrs[i].ModelID, path, dup).
				UpdateColumn("data", gorm.Expr("JSON_SET(data, ?, ?)", path, OfflineStatusID))
		}
		b, _ := json.Marshal(kept)
		db.Model(&model.Attribute{}).Where("id = ?", attrs[i].ID).Update("options", datatypes.JSON(b))
	}
	return nil
}

// ensureDefaultListHidden 首次把 cloud_provider/cloud_az/comment 设为列表隐藏。
// 启发式：若这三类字段中已有任意一个处于隐藏状态，视为已初始化过，不再强制覆盖。
func ensureDefaultListHidden(db *gorm.DB) error {
	props := []string{"cloud_provider", "cloud_az", "comment"}
	var cnt int64
	if err := db.Model(&model.Attribute{}).
		Where("property_id IN ? AND list_hidden = ?", props, true).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return nil
	}
	return db.Exec("UPDATE attribute SET list_hidden = 1 WHERE property_id IN ?", props).Error
}

// ensureReadonlyViewPerm 确保「只读」角色有一条全模型（model_id=0）的 view 数据权限。
// 幂等：已存在针对 model_id=0 的权限行则不改动（尊重管理员后续调整）。
func ensureReadonlyViewPerm(db *gorm.DB) error {
	var role model.Role
	if err := db.Where("code = ?", "readonly").First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	var cnt int64
	if err := db.Model(&model.RolePermission{}).
		Where("role_id = ? AND model_id = ?", role.ID, 0).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return nil
	}
	actions, _ := json.Marshal([]string{model.ActionView})
	return db.Create(&model.RolePermission{
		RoleID:  role.ID,
		ModelID: 0,
		Actions: datatypes.JSON(actions),
	}).Error
}

// ensureReadonlyViewFuncs 给「只读」角色附带模型/拓扑/运营分析的只读查看功能权限。
// 幂等：仅当该角色 functions 为空（未设置过）时写入，避免覆盖管理员后续调整。
func ensureReadonlyViewFuncs(db *gorm.DB) error {
	var role model.Role
	if err := db.Where("code = ?", "readonly").First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	var fns []string
	_ = json.Unmarshal(role.Functions, &fns)
	if len(fns) > 0 {
		return nil // 已设置过，尊重现状
	}
	v, _ := json.Marshal([]string{model.FuncModelView, model.FuncTopoView, model.FuncStatsView})
	return db.Model(&model.Role{}).Where("id = ?", role.ID).Update("functions", datatypes.JSON(v)).Error
}

// ensurePresetRoles 幂等创建一组内置角色。每个角色只带对应功能权限、无数据写权限
// （资源实例的查询对所有登录用户开放）。已存在的按 code 跳过，不覆盖管理员后续调整。
func ensurePresetRoles(db *gorm.DB) error {
	presets := []struct {
		Code, Name, Desc string
		Functions        []string
	}{
		{"readonly", "只读", "仅可查看首页与资源实例（查询/搜索/详情），其它权限需管理员授权", []string{}},
		{"model_admin", "模型管理员", "可管理模型分组、模型、字段与关联", []string{model.FuncModelManage}},
		{"topo_admin", "业务拓扑管理员", "可管理业务、集群、模块及资源分配", []string{model.FuncTopoManage}},
		{"ops_analyst", "运营分析员", "可查看运营分析看板", []string{model.FuncStatsView}},
		{"user_admin", "用户管理员", "可管理用户、角色与权限分配", []string{model.FuncUserManage}},
		{"audit_viewer", "审计员", "可查看操作审计日志", []string{model.FuncAuditView}},
	}
	for _, p := range presets {
		var cnt int64
		if err := db.Model(&model.Role{}).Where("code = ?", p.Code).Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			continue
		}
		fns, _ := json.Marshal(p.Functions)
		if err := db.Create(&model.Role{
			Code:        p.Code,
			Name:        p.Name,
			Description: p.Desc,
			IsAdmin:     false,
			IsBuiltin:   true,
			Functions:   datatypes.JSON(fns),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ensureCommentFieldAll 为所有模型确保存在一个「备注」字段（property_id=comment）。
// 幂等：已存在的模型跳过；新建的放在该模型字段的最后。
func ensureCommentFieldAll(db *gorm.DB) error {
	var models []model.Model
	if err := db.Find(&models).Error; err != nil {
		return err
	}
	for _, m := range models {
		var cnt int64
		if err := db.Model(&model.Attribute{}).
			Where("model_id = ? AND property_id = ?", m.ID, "comment").Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			continue
		}
		var maxIdx int
		db.Model(&model.Attribute{}).Where("model_id = ?", m.ID).
			Select("COALESCE(MAX(`index`), 0)").Scan(&maxIdx)
		a := &model.Attribute{
			ModelID:    m.ID,
			PropertyID: "comment",
			Name:       "备注",
			Type:       model.FieldLongChar,
			Editable:   true,
			Enabled:    true,
			Group:      "default",
			Index:      maxIdx + 1,
		}
		if err := ensureAttr(db, a); err != nil {
			return err
		}
	}
	return nil
}

// seedSampleData 写入一组示例实例并按模型关联建立实例关系（幂等）。
func seedSampleData(db *gorm.DB, mid map[string]uint, assocID map[string]uint) error {
	// 业务
	bizEC, err := ensureInstance(db, mid["biz"], "bk_biz_name", map[string]interface{}{"bk_biz_name": "电商平台"})
	if err != nil {
		return err
	}
	bizPay, err := ensureInstance(db, mid["biz"], "bk_biz_name", map[string]interface{}{"bk_biz_name": "支付中心"})
	if err != nil {
		return err
	}

	// 主机
	hostData := []map[string]interface{}{
		{"bk_host_innerip": "10.0.1.11", "bk_host_name": "web-01", "bk_os_name": "CentOS 7.9", "bk_cpu": 8, "bk_mem": 16384, "bk_disk": 500, "bk_host_status": "1"},
		{"bk_host_innerip": "10.0.1.12", "bk_host_name": "web-02", "bk_os_name": "CentOS 7.9", "bk_cpu": 8, "bk_mem": 16384, "bk_disk": 500, "bk_host_status": "1"},
		{"bk_host_innerip": "10.0.1.21", "bk_host_name": "db-01", "bk_os_name": "Ubuntu 22.04", "bk_cpu": 16, "bk_mem": 32768, "bk_disk": 1000, "bk_host_status": "1"},
		{"bk_host_innerip": "10.0.1.22", "bk_host_name": "db-02", "bk_os_name": "Ubuntu 22.04", "bk_cpu": 16, "bk_mem": 32768, "bk_disk": 1000, "bk_host_status": "1"},
	}
	hostIDs := make([]uint, len(hostData))
	for i, d := range hostData {
		hostIDs[i], err = ensureInstance(db, mid["host"], "bk_host_innerip", d)
		if err != nil {
			return err
		}
	}
	hWeb01, hWeb02, hDB01, hDB02 := hostIDs[0], hostIDs[1], hostIDs[2], hostIDs[3]

	// 中间件 / 数据库
	mysqlID, err := ensureInstance(db, mid["mysql"], "inst_name", map[string]interface{}{
		"inst_name": "mysql-order", "version": "8.0", "endpoint": "10.0.1.21:3306", "port": 3306,
		"arch": "ha", "spec": "8C32G", "region": "cn-hangzhou", "status": "running",
	})
	if err != nil {
		return err
	}
	redisID, err := ensureInstance(db, mid["redis"], "inst_name", map[string]interface{}{
		"inst_name": "redis-cache", "version": "6.2", "endpoint": "10.0.1.22:6379", "port": 6379,
		"arch": "cluster", "mem_gb": 16, "region": "cn-hangzhou", "status": "running",
	})
	if err != nil {
		return err
	}
	kafkaID, err := ensureInstance(db, mid["kafka"], "inst_name", map[string]interface{}{
		"inst_name": "kafka-log", "version": "3.6", "endpoint": "10.0.1.21:9092", "port": 9092,
		"broker_count": 3, "disk_gb": 1000, "region": "cn-hangzhou", "status": "running",
	})
	if err != nil {
		return err
	}
	lindormID, err := ensureInstance(db, mid["lindorm"], "inst_name", map[string]interface{}{
		"inst_name": "lindorm-metric", "version": "3.6", "endpoint": "ld-xxxx.lindorm.rds.aliyuncs.com",
		"engine": []string{"wide", "tsdb"}, "spec": "4C16G", "storage_gb": 2000, "region": "cn-hangzhou", "status": "running",
	})
	if err != nil {
		return err
	}

	// 负载均衡
	nlbID, err := ensureInstance(db, mid["nlb"], "inst_name", map[string]interface{}{
		"inst_name": "nlb-gateway", "vip": "10.0.1.100", "protocol": "tcp", "listen_port": 443,
		"bandwidth": 1000, "spec": "slb.s3.large", "region": "cn-hangzhou", "status": "running",
	})
	if err != nil {
		return err
	}
	albID, err := ensureInstance(db, mid["alb"], "inst_name", map[string]interface{}{
		"inst_name": "alb-web", "endpoint": "web.example.com", "protocol": "https", "listen_port": 443,
		"bandwidth": 1000, "spec": "alb.s2.small", "region": "cn-hangzhou", "status": "running",
	})
	if err != nil {
		return err
	}

	// 弹性公网IP
	eipID, err := ensureInstance(db, mid["eip"], "ip_addr", map[string]interface{}{
		"ip_addr": "47.98.0.1", "bandwidth": 100, "billing": "bandwidth", "isp": "bgp",
		"bind_type": "ecs", "region": "cn-hangzhou", "status": "running",
	})
	if err != nil {
		return err
	}

	// WAF / 域名 / CDN / NAT网关 / API网关 / OSS
	wafID, err := ensureInstance(db, mid["waf"], "inst_name", map[string]interface{}{
		"inst_name": "www.example.com", "spec": "高级版", "resource_id": "waf-cn-example",
		"mode": "block", "region": "中国大陆", "status": "running",
	})
	if err != nil {
		return err
	}
	domainID, err := ensureInstance(db, mid["domain"], "domain_name", map[string]interface{}{
		"domain_name": "example.com", "registrar": "阿里云", "dns_provider": "阿里云",
		"record_type": "A", "expire_date": "2027-10-14", "status": "normal",
	})
	if err != nil {
		return err
	}
	cdnID, err := ensureInstance(db, mid["cdn"], "domain", map[string]interface{}{
		"domain": "cdn.example.com", "cname": "cdn.example.com.w.kunlunaq.com", "cdn_type": "web",
		"source": "47.98.0.1", "coverage": "domestic", "ssl": "on", "status": "running",
	})
	if err != nil {
		return err
	}
	natgwID, err := ensureInstance(db, mid["natgw"], "inst_name", map[string]interface{}{
		"inst_name": "nat-prod", "spec": "nat.small", "eip_count": 1, "bandwidth": 500,
		"region": "cn-hangzhou", "status": "running",
	})
	if err != nil {
		return err
	}
	apigwID, err := ensureInstance(db, mid["apigw"], "inst_name", map[string]interface{}{
		"inst_name": "apigw-open", "endpoint": "api.example.com", "group_count": 3, "api_count": 42,
		"region": "cn-hangzhou", "status": "running",
	})
	if err != nil {
		return err
	}
	if _, err = ensureInstance(db, mid["oss"], "bucket", map[string]interface{}{
		"bucket": "static-assets", "endpoint": "static-assets.oss-cn-hangzhou.aliyuncs.com",
		"storage_class": "standard", "acl": "public-read", "used_gb": 320,
		"region": "cn-hangzhou", "status": "running",
	}); err != nil {
		return err
	}

	// Kubernetes 集群
	k8sID, err := ensureInstance(db, mid["kubernetes"], "cluster_name", map[string]interface{}{
		"cluster_name": "prod-k8s", "version": "1.28", "api_server": "https://10.0.1.10:6443",
		"node_count": 2, "pod_cidr": "10.244.0.0/16", "cni": "Calico",
		"region": "cn-hangzhou", "status": "running",
	})
	if err != nil {
		return err
	}

	// 实例关联
	rels := []struct {
		code     string
		src, dst uint
	}{
		// 运行于主机
		{"run_mysql_host", mysqlID, hDB01},
		{"run_redis_host", redisID, hDB02},
		{"run_kafka_host", kafkaID, hDB01},
		{"run_lindorm_host", lindormID, hDB01},
		// 负载均衡连接后端主机
		{"connect_nlb_host", nlbID, hWeb01},
		{"connect_alb_host", albID, hWeb01},
		{"connect_alb_host", albID, hWeb02},
		// 主机跨业务使用：web-01 归属电商平台，同时服务于支付中心
		{"serve_host_biz", hWeb01, bizPay},
		// 弹性公网IP：绑定 web-01
		{"connect_eip_host", eipID, hWeb01},
		// 边缘/网络资源连接
		{"connect_cdn_domain", cdnID, domainID},
		{"connect_waf_domain", wafID, domainID},
		{"connect_apigw_host", apigwID, hWeb01},
		{"connect_natgw_eip", natgwID, eipID},
		// K8s 集群运行于主机节点
		{"run_k8s_host", k8sID, hWeb01},
		{"run_k8s_host", k8sID, hWeb02},
	}
	for _, r := range rels {
		aid := assocID[r.code]
		if aid == 0 || r.src == 0 || r.dst == 0 {
			continue
		}
		if err := ensureRelation(db, aid, r.src, r.dst); err != nil {
			return err
		}
	}

	// 拓扑示例：电商平台 → 公共服务集群 → web 模块（主机类型），分配 web-01 / web-02
	if bizEC != 0 {
		setID, err := ensureTopoNode(db, bizEC, bizEC, model.TopoSet, "公共服务集群", 0)
		if err != nil {
			return err
		}
		modID, err := ensureTopoNode(db, bizEC, setID, model.TopoModule, "web", mid["host"])
		if err != nil {
			return err
		}
		for _, hid := range []uint{hWeb01, hWeb02} {
			if hid == 0 {
				continue
			}
			if err := ensureHostModule(db, hid, modID, mid["host"], bizEC); err != nil {
				return err
			}
		}
	}

	// 注：示例不自动把资源分配到业务——业务归属（业务线）应由使用者在业务拓扑中显式分配。
	return nil
}

// ensureInstance 按「模型 + 唯一字段值」判重，不存在则创建，返回实例 ID。
func ensureInstance(db *gorm.DB, modelID uint, uniqProp string, data map[string]interface{}) (uint, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return 0, err
	}
	uniqVal := fmt.Sprintf("%v", data[uniqProp])
	var existing model.Instance
	err = db.Where("model_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, ?)) = ?", modelID, "$."+uniqProp, uniqVal).
		First(&existing).Error
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	inst := model.Instance{ModelID: modelID, Data: datatypes.JSON(raw), Creator: "system", Updater: "system"}
	if err := db.Create(&inst).Error; err != nil {
		return 0, err
	}
	return inst.ID, nil
}

// ensureRelation 按 (关联, 源, 目标) 判重建立实例关系。
func ensureRelation(db *gorm.DB, assocID, src, dst uint) error {
	var existing model.InstanceRelation
	err := db.Where("association_id = ? AND src_inst_id = ? AND dst_inst_id = ?", assocID, src, dst).
		First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.Create(&model.InstanceRelation{AssociationID: assocID, SrcInstID: src, DstInstID: dst}).Error
}

// ensureTopoNode 按 (业务, 类型, 名称) 判重创建拓扑节点。modelID 仅模块使用。
func ensureTopoNode(db *gorm.DB, bizID, parentID uint, nodeType, name string, modelID uint) (uint, error) {
	var existing model.TopoNode
	err := db.Where("biz_id = ? AND node_type = ? AND name = ?", bizID, nodeType, name).First(&existing).Error
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	n := model.TopoNode{BizID: bizID, ParentID: parentID, NodeType: nodeType, Name: name, ModelID: modelID}
	if err := db.Create(&n).Error; err != nil {
		return 0, err
	}
	return n.ID, nil
}

// ensureHostModule 将资源实例分配到模块（未分配时才处理，host_id 唯一）。
func ensureHostModule(db *gorm.DB, hostID, moduleID, modelID, bizID uint) error {
	var cnt int64
	db.Model(&model.HostModule{}).Where("host_id = ?", hostID).Count(&cnt)
	if cnt > 0 {
		return nil
	}
	return db.Create(&model.HostModule{HostID: hostID, ModuleID: moduleID, ModelID: modelID, BizID: bizID}).Error
}

func ensureAssoc(db *gorm.DB, a *model.ModelAssociation) error {
	var existing model.ModelAssociation
	err := db.Where("code = ?", a.Code).First(&existing).Error
	if err == nil {
		*a = existing
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.Create(a).Error
}

func ensureGroup(db *gorm.DB, g *model.ModelGroup) error {
	var existing model.ModelGroup
	err := db.Where("code = ?", g.Code).First(&existing).Error
	if err == nil {
		*g = existing
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.Create(g).Error
}

func ensureModel(db *gorm.DB, m *model.Model) error {
	var existing model.Model
	err := db.Where("obj_id = ?", m.ObjID).First(&existing).Error
	if err == nil {
		*m = existing
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.Create(m).Error
}

func ensureAttr(db *gorm.DB, a *model.Attribute) error {
	var existing model.Attribute
	err := db.Where("model_id = ? AND property_id = ?", a.ModelID, a.PropertyID).First(&existing).Error
	if err == nil {
		*a = existing
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.Create(a).Error
}

func ensureKind(db *gorm.DB, k *model.AssociationKind) error {
	var existing model.AssociationKind
	err := db.Where("code = ?", k.Code).First(&existing).Error
	if err == nil {
		*k = existing
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.Create(k).Error
}
