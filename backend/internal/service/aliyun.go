package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

func aliNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CreatorAliyun 阿里云同步来源标识。
const CreatorAliyun = "aliyun"

// aliPercentEncode 按阿里云 RPC 签名规范编码。
func aliPercentEncode(s string) string {
	e := url.QueryEscape(s)
	e = strings.ReplaceAll(e, "+", "%20")
	e = strings.ReplaceAll(e, "*", "%2A")
	e = strings.ReplaceAll(e, "%7E", "~")
	return e
}

// aliRPC 发起一次阿里云 RPC 风格 OpenAPI 调用（GET + HMAC-SHA1 签名），返回响应体。
func aliRPC(endpoint, ak, sk, version, action string, extra map[string]string) ([]byte, error) {
	params := map[string]string{
		"Format":           "JSON",
		"Version":          version,
		"Action":           action,
		"AccessKeyId":      ak,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   aliNonce(),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	for k, v := range extra {
		params[k] = v
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, aliPercentEncode(k)+"="+aliPercentEncode(params[k]))
	}
	canonical := strings.Join(parts, "&")
	stringToSign := "GET&" + aliPercentEncode("/") + "&" + aliPercentEncode(canonical)
	mac := hmac.New(sha1.New, []byte(sk+"&"))
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	reqURL := endpoint + "/?" + canonical + "&Signature=" + aliPercentEncode(sig)
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		// 阿里云错误体含 Code/Message
		var e struct{ Code, Message string }
		_ = json.Unmarshal(body, &e)
		if e.Message != "" {
			return nil, newValidationErr("阿里云接口错误：%s", e.Message)
		}
		return nil, newValidationErr("阿里云接口返回 HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// aliRPCPost 与 aliRPC 同为 RPC V1(HMAC-SHA1) 签名，但用 POST（参数放 form body）。
// 部分 RPC 接口（如 DataWorks ListProjects）只支持 POST。
func aliRPCPost(endpoint, ak, sk, version, action string, extra map[string]string) ([]byte, error) {
	params := map[string]string{
		"Format":           "JSON",
		"Version":          version,
		"Action":           action,
		"AccessKeyId":      ak,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   aliNonce(),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	for k, v := range extra {
		params[k] = v
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, aliPercentEncode(k)+"="+aliPercentEncode(params[k]))
	}
	canonical := strings.Join(parts, "&")
	stringToSign := "POST&" + aliPercentEncode("/") + "&" + aliPercentEncode(canonical)
	mac := hmac.New(sha1.New, []byte(sk+"&"))
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	form := canonical + "&Signature=" + aliPercentEncode(sig)
	req, err := http.NewRequest("POST", endpoint+"/", bytes.NewReader([]byte(form)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		var e struct{ Code, Message string }
		_ = json.Unmarshal(body, &e)
		if e.Message != "" {
			return nil, newValidationErr("阿里云接口错误：%s", e.Message)
		}
		return nil, newValidationErr("阿里云接口返回 HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// aliRPCv3 阿里云 V3 签名(ACS3-HMAC-SHA256) 调用，用于只支持新签名的产品（如 ALB）。
// 参数放 JSON body（POST），返回响应体。
func aliRPCv3(endpoint, ak, sk, version, action string, params map[string]string) ([]byte, error) {
	host := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	body := []byte("{}")
	if len(params) > 0 {
		body, _ = json.Marshal(params)
	}
	sum := sha256.Sum256(body)
	hashedPayload := hex.EncodeToString(sum[:])
	date := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	nonce := aliNonce()
	headers := map[string]string{
		"host":                  host,
		"x-acs-action":          action,
		"x-acs-version":         version,
		"x-acs-date":            date,
		"x-acs-signature-nonce": nonce,
		"x-acs-content-sha256":  hashedPayload,
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var canon strings.Builder
	signedHeaders := make([]string, 0, len(keys))
	for _, k := range keys {
		canon.WriteString(k + ":" + strings.TrimSpace(headers[k]) + "\n")
		signedHeaders = append(signedHeaders, k)
	}
	signed := strings.Join(signedHeaders, ";")
	// 规范请求：方法\nURI\n查询串\n规范头\n签名头\n荷载哈希（RPC 的 URI 为 /，查询串空）
	canonicalRequest := "POST\n/\n\n" + canon.String() + "\n" + signed + "\n" + hashedPayload
	crSum := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "ACS3-HMAC-SHA256\n" + hex.EncodeToString(crSum[:])
	mac := hmac.New(sha256.New, []byte(sk))
	mac.Write([]byte(stringToSign))
	sig := hex.EncodeToString(mac.Sum(nil))
	auth := "ACS3-HMAC-SHA256 Credential=" + ak + ",SignedHeaders=" + signed + ",Signature=" + sig

	req, err := http.NewRequest("POST", endpoint+"/", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		var e struct{ Code, Message string }
		_ = json.Unmarshal(rb, &e)
		if e.Message != "" {
			return nil, newValidationErr("阿里云接口错误：%s", e.Message)
		}
		return nil, newValidationErr("阿里云接口返回 HTTP %d", resp.StatusCode)
	}
	return rb, nil
}

// ecsEndpoint 返回 ECS 地域化 endpoint。
func ecsEndpoint(region string) string {
	if region == "" {
		region = "cn-shanghai"
	}
	return "https://ecs." + region + ".aliyuncs.com"
}

// parseRegions 解析地域配置：逗号分隔多地域；空则默认 cn-shanghai。
func parseRegions(s string) []string {
	out := []string{}
	for _, r := range strings.Split(s, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		out = append(out, "cn-shanghai")
	}
	return out
}

type aliEcsInstance struct {
	InstanceID     string `json:"InstanceId"`
	InstanceName   string `json:"InstanceName"`
	Status         string `json:"Status"`
	Cpu            int    `json:"Cpu"`
	Memory         int    `json:"Memory"`
	OSName         string `json:"OSName"`
	RegionID       string `json:"RegionId"`
	ZoneID         string `json:"ZoneId"`
	VpcAttributes  struct {
		PrivateIPAddress struct {
			IPAddress []string `json:"IpAddress"`
		} `json:"PrivateIpAddress"`
	} `json:"VpcAttributes"`
	InnerIPAddress struct {
		IPAddress []string `json:"IpAddress"`
	} `json:"InnerIpAddress"`
}

type aliEcsResp struct {
	TotalCount int `json:"TotalCount"`
	PageNumber int `json:"PageNumber"`
	PageSize   int `json:"PageSize"`
	Instances  struct {
		Instance []aliEcsInstance `json:"Instance"`
	} `json:"Instances"`
}

// aliTestConnection 用 ECS DescribeInstances(PageSize=1) 验证 AK/SK（取第一个地域）。
func (s *DataSourceService) aliTestConnection(ds *model.DataSource) (int, error) {
	region := parseRegions(ds.Region)[0]
	body, err := aliRPC(ecsEndpoint(region), ds.Username, ds.Password, "2014-05-26", "DescribeInstances",
		map[string]string{"RegionId": region, "PageSize": "1", "PageNumber": "1"})
	if err != nil {
		return 0, err
	}
	var r aliEcsResp
	if err := json.Unmarshal(body, &r); err != nil {
		return 0, newValidationErr("解析阿里云响应失败")
	}
	return r.TotalCount, nil
}

// aliDiskMap 拉取某地域全部云盘，按 InstanceId 汇总容量(GB)。
func (s *DataSourceService) aliDiskMap(ds *model.DataSource, region string) map[string]int {
	out := map[string]int{}
	page := 1
	for {
		body, err := aliRPC(ecsEndpoint(region), ds.Username, ds.Password, "2014-05-26", "DescribeDisks",
			map[string]string{"RegionId": region, "PageSize": "100", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			break
		}
		var r struct {
			TotalCount int `json:"TotalCount"`
			PageSize   int `json:"PageSize"`
			Disks      struct {
				Disk []struct {
					InstanceID string `json:"InstanceId"`
					Size       int    `json:"Size"`
				} `json:"Disk"`
			} `json:"Disks"`
		}
		if json.Unmarshal(body, &r) != nil {
			break
		}
		for _, d := range r.Disks.Disk {
			if d.InstanceID != "" {
				out[d.InstanceID] += d.Size
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.Disks.Disk) == 0 {
			break
		}
		page++
	}
	return out
}

// SyncAliyun 从阿里云 ECS 同步主机（支持多地域，逗号分隔）。
func (s *DataSourceService) SyncAliyun(ds *model.DataSource) (*SyncReport, error) {
	if ds.Username == "" || ds.Password == "" {
		return nil, newValidationErr("缺少 AccessKey / AccessSecret")
	}
	report := &SyncReport{Errors: []string{}, skippedModels: map[string]bool{}}
	seenHost := []uint{}
	seenMysql := []uint{}
	seenRedis := []uint{}
	seenKafka := []uint{}
	seenLindorm := []uint{}
	seenApig := []uint{}
	seenMse := []uint{}
	seenNacos := []uint{}
	seenEip := []uint{}
	seenClb := []uint{}
	seenNlb := []uint{}
	seenAlb := []uint{}
	seenNatgw := []uint{}
	seenHavip := []uint{}
	seenEc := []uint{}
	seenVbr := []uint{}
	seenES := []uint{}
	seenVpc := []uint{}
	seenVsw := []uint{}
	seenMc := []uint{}
	seenHolo := []uint{}
	seenDw := []uint{}
	for _, region := range parseRegions(ds.Region) {
		if err := s.syncAliyunRegion(ds, region, report, &seenHost, &seenMysql, &seenRedis, &seenKafka, &seenLindorm, &seenApig, &seenMse, &seenNacos, &seenEip, &seenClb, &seenNlb, &seenAlb, &seenNatgw, &seenHavip, &seenEc, &seenVbr, &seenES, &seenVpc, &seenVsw, &seenMc, &seenHolo, &seenDw); err != nil {
			return nil, err
		}
	}
	// 域名为全局资源（不分地域），单独同步一次
	seenDomain := []uint{}
	if err := s.syncAliyunDomain(ds, report, &seenDomain); err != nil {
		return nil, err
	}
	// DNS 解析记录（云解析 Alidns，全局），单独同步一次
	seenDns := []uint{}
	if err := s.syncAliyunDnsRecords(ds, report, &seenDns); err != nil {
		return nil, err
	}
	// 对象存储 OSS（全局资源），单独同步一次
	seenOss := []uint{}
	if err := s.syncAliyunOss(ds, report, &seenOss); err != nil {
		return nil, err
	}
	// WAF 防护域名（全局资源），单独同步一次
	seenWaf := []uint{}
	if err := s.syncAliyunWaf(ds, report, &seenWaf); err != nil {
		return nil, err
	}
	// CDN 加速域名（全局资源），单独同步一次
	seenCdn := []uint{}
	if err := s.syncAliyunCdn(ds, report, &seenCdn); err != nil {
		return nil, err
	}
	// 容器服务 ACK 集群（全局，一次返回全部地域），单独同步一次
	seenK8s := []uint{}
	if err := s.syncAliyunK8s(ds, report, &seenK8s); err != nil {
		return nil, err
	}
	// 云企业网 CEN（全局资源，跨地域/跨 VPC），单独同步一次
	seenCen := []uint{}
	if err := s.syncAliyunCen(ds, report, &seenCen); err != nil {
		return nil, err
	}
	// 失联标记：本次未出现的（该数据源同步的）实例置「已下线」。
	// 有实例级失败(Failed>0)时整体跳过；某产品本次因错误被整体跳过(skippedModels)时也不标记其下线，
	// 避免因无权限/未开通/接口错误把实际在线的资源误判为下线。
	if report.Failed == 0 {
		markOffline := func(objID string, seen []uint) {
			if report.skippedModels[objID] {
				return // 该产品本次被跳过（未读到数据），不做下线标记
			}
			mid := s.inst.ModelIDByObjID(objID)
			if objID == "host" {
				mid = s.inst.HostModelID()
			}
			if mid != 0 {
				_, _ = s.inst.MarkOffline(mid, ds.ID, seen)
			}
		}
		markOffline("host", seenHost)
		markOffline("mysql", seenMysql)
		markOffline("redis", seenRedis)
		markOffline("kafka", seenKafka)
		markOffline("lindorm", seenLindorm)
		markOffline("apigw", seenApig)
		markOffline("mse", seenMse)
		markOffline("nacos", seenNacos)
		markOffline("eip", seenEip)
		markOffline("nlb", seenNlb)
		markOffline("alb", seenAlb)
		markOffline("clb", seenClb)
		markOffline("natgw", seenNatgw)
		markOffline("havip", seenHavip)
		markOffline("express_connect", seenEc)
		markOffline("vbr", seenVbr)
		markOffline("elastic", seenES)
		markOffline("vpc", seenVpc)
		markOffline("vswitch", seenVsw)
		markOffline("maxcompute", seenMc)
		markOffline("hologres", seenHolo)
		markOffline("dataworks", seenDw)
		markOffline("domain", seenDomain)
		markOffline("dns_record", seenDns)
		markOffline("oss", seenOss)
		markOffline("waf", seenWaf)
		markOffline("cdn", seenCdn)
		markOffline("kubernetes", seenK8s)
		markOffline("cen", seenCen)
	}
	// 附加：各 ACK 集群的应用服务（走 ARMS Prometheus / kube-state-metrics），best-effort，不影响主体
	s.syncAckAppsAll(ds, report)
	return report, nil
}

// syncAckAppsAll 遍历该阿里云源下所有 ACK 集群(kubernetes 实例)，各走其 ARMS Prometheus 同步应用服务。
// 每集群独立 best-effort：未接入 ARMS 或查不到工作负载则跳过；下线标记按集群隔离(见 syncK8sApps)。
func (s *DataSourceService) syncAckAppsAll(ds *model.DataSource, report *SyncReport) {
	k8sModelID := s.inst.ModelIDByObjID("kubernetes")
	if k8sModelID == 0 {
		return
	}
	var insts []model.Instance
	s.db.Where("model_id = ? AND source_id = ?", k8sModelID, ds.ID).Find(&insts)
	for _, inst := range insts {
		var d map[string]interface{}
		if json.Unmarshal(inst.Data, &d) != nil {
			continue
		}
		rid, _ := d["resource_id"].(string)
		cluster, _ := d["cluster_name"].(string)
		region, _ := d["region"].(string)
		if strings.TrimSpace(rid) == "" || strings.TrimSpace(cluster) == "" {
			continue
		}
		base := armsPromBaseURL(ds, rid, region)
		if base == "" {
			continue // 集群未接入 ARMS Prometheus：跳过
		}
		s.syncK8sApps(base, "", "", "", cluster, ds.Name, ds.ID, report)
	}
}

// rdsEndpoint 返回 RDS 地域化 endpoint。
func rdsEndpoint(region string) string {
	if region == "" {
		region = "cn-shanghai"
	}
	return "https://rds." + region + ".aliyuncs.com"
}

// rdsCategoryName 阿里云 RDS 实例类型(Category) → 中文。
func rdsCategoryName(c string) string {
	switch c {
	case "Basic":
		return "基础版"
	case "HighAvailability":
		return "高可用版"
	case "Cluster":
		return "集群版"
	case "AlwaysOn":
		return "集群版(AlwaysOn)"
	case "Finance":
		return "三节点企业版"
	case "serverless_basic", "serverless_standard", "serverless_ha":
		return "Serverless"
	}
	return c
}

// aliRdsAttr 查单个 RDS 实例的实例类型与规格明细（Category/CPU/内存MB/磁盘GB/端口）。
func (s *DataSourceService) aliRdsAttr(ds *model.DataSource, region, dbInstanceID string) (category string, cpu, memMB, diskGB, port int) {
	body, err := aliRPC(rdsEndpoint(region), ds.Username, ds.Password, "2014-08-15", "DescribeDBInstanceAttribute",
		map[string]string{"RegionId": region, "DBInstanceId": dbInstanceID})
	if err != nil {
		return
	}
	var r struct {
		Items struct {
			DBInstanceAttribute []struct {
				Category          string      `json:"Category"`
				DBInstanceCPU     string      `json:"DBInstanceCPU"`
				DBInstanceMemory  json.Number `json:"DBInstanceMemory"`
				DBInstanceStorage json.Number `json:"DBInstanceStorage"`
				Port              string      `json:"Port"`
			} `json:"DBInstanceAttribute"`
		} `json:"Items"`
	}
	if json.Unmarshal(body, &r) != nil || len(r.Items.DBInstanceAttribute) == 0 {
		return
	}
	a := r.Items.DBInstanceAttribute[0]
	category = a.Category
	if n, e := strconv.Atoi(strings.TrimSpace(a.DBInstanceCPU)); e == nil {
		cpu = n
	}
	if n, e := a.DBInstanceMemory.Int64(); e == nil {
		memMB = int(n)
	}
	if n, e := a.DBInstanceStorage.Int64(); e == nil {
		diskGB = int(n)
	}
	if n, e := strconv.Atoi(strings.TrimSpace(a.Port)); e == nil {
		port = n
	}
	return
}

// syncAliyunRDS 同步单地域 RDS 实例到 MySQL 模型。
func (s *DataSourceService) syncAliyunRDS(ds *model.DataSource, region string, report *SyncReport, seenMysql *[]uint) error {
	mysqlID := s.inst.ModelIDByObjID("mysql")
	if mysqlID == 0 {
		return nil // 无 MySQL 模型则跳过
	}
	page := 1
	for {
		body, err := aliRPC(rdsEndpoint(region), ds.Username, ds.Password, "2014-08-15", "DescribeDBInstances",
			map[string]string{"RegionId": region, "PageSize": "100", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			report.skipProduct("mysql", "RDS(MySQL) 同步跳过("+region+")："+err.Error())
			return nil
		}
		var r struct {
			TotalRecordCount int `json:"TotalRecordCount"`
			PageRecordCount  int `json:"PageRecordCount"`
			Items            struct {
				DBInstance []struct {
					DBInstanceID     string `json:"DBInstanceId"`
					Description      string `json:"DBInstanceDescription"`
					Engine           string `json:"Engine"`
					EngineVersion    string `json:"EngineVersion"`
					Status           string `json:"DBInstanceStatus"`
					Class            string `json:"DBInstanceClass"`
					ConnectionString string `json:"ConnectionString"`
					RegionID         string `json:"RegionId"`
					ZoneID           string `json:"ZoneId"`
				} `json:"DBInstance"`
			} `json:"Items"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 RDS 响应失败")
		}
		for _, db := range r.Items.DBInstance {
			name := firstNonEmpty(db.Description, db.DBInstanceID)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"inst_name":      name,
				"version":        strings.TrimSpace(db.Engine + " " + db.EngineVersion),
				"spec":           db.Class,
				"cloud_provider": "aliyun",
			}
			if db.DBInstanceID != "" {
				data["resource_id"] = db.DBInstanceID // 阿里云 RDS 实例ID
			}
			if db.ConnectionString != "" {
				data["endpoint"] = db.ConnectionString
			}
			if db.RegionID != "" {
				data["region"] = db.RegionID
			}
			if db.ZoneID != "" {
				data["cloud_az"] = db.ZoneID
			}
			switch db.Status {
			case "Running":
				data["status"] = "running"
			case "Creating":
				data["status"] = "creating"
			default:
				data["status"] = "abnormal"
			}
			// 实例类型 + 规格明细（CPU/内存/磁盘）需逐实例查 DescribeDBInstanceAttribute
			if cat, cpu, mem, disk, port := s.aliRdsAttr(ds, region, db.DBInstanceID); cat != "" || cpu > 0 || port > 0 {
				if cat != "" {
					data["arch"] = rdsCategoryName(cat)
				}
				if cpu > 0 {
					data["cpu_cores"] = float64(cpu)
				}
				if mem > 0 {
					data["mem_mb"] = float64(mem)
				}
				if disk > 0 {
					data["disk_gb"] = float64(disk)
				}
				if port > 0 {
					data["port"] = float64(port)
				}
			}
			inst, created, e := s.inst.CollectCloudInstance(mysqlID, "inst_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenMysql = append(*seenMysql, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.PageRecordCount == 0 || page*100 >= r.TotalRecordCount || len(r.Items.DBInstance) == 0 {
			break
		}
		page++
	}
	return nil
}

// syncAliyunRegion 同步单个地域的 ECS + RDS + Redis + EIP + CLB + NAT + ES + VPC + 交换机 + MaxCompute。
func (s *DataSourceService) syncAliyunRegion(ds *model.DataSource, region string, report *SyncReport, seenHost, seenMysql, seenRedis, seenKafka, seenLindorm, seenApig, seenMse, seenNacos, seenEip, seenClb, seenNlb, seenAlb, seenNatgw, seenHavip, seenEc, seenVbr, seenES, seenVpc, seenVsw, seenMc, seenHolo, seenDw *[]uint) error {
	diskByInst := s.aliDiskMap(ds, region)
	page := 1
	for {
		body, err := aliRPC(ecsEndpoint(region), ds.Username, ds.Password, "2014-05-26", "DescribeInstances",
			map[string]string{"RegionId": region, "PageSize": "100", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			report.skipProduct("host", "ECS(主机) 同步跳过("+region+")："+err.Error())
			break
		}
		var r aliEcsResp
		if err := json.Unmarshal(body, &r); err != nil {
			report.skipProduct("host", "ECS(主机) 响应解析失败("+region+")")
			break
		}
		for _, inst := range r.Instances.Instance {
			ip := firstIP(inst.VpcAttributes.PrivateIPAddress.IPAddress, inst.InnerIPAddress.IPAddress)
			if ip == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"bk_host_innerip": ip,
				"bk_host_name":    firstNonEmpty(inst.InstanceName, inst.InstanceID),
				"cloud_provider":  "aliyun",
			}
			if inst.InstanceID != "" {
				data["resource_id"] = inst.InstanceID // 阿里云 ECS InstanceId
			}
			if inst.OSName != "" {
				data["bk_os_name"] = inst.OSName
			}
			if inst.Cpu > 0 {
				data["bk_cpu"] = float64(inst.Cpu)
			}
			if inst.Memory > 0 {
				data["bk_mem"] = float64(inst.Memory)
			}
			if d := diskByInst[inst.InstanceID]; d > 0 {
				data["bk_disk"] = float64(d)
			}
			if inst.RegionID != "" {
				data["bk_zone"] = inst.RegionID
			}
			if inst.ZoneID != "" {
				data["cloud_az"] = inst.ZoneID
			}
			if inst.Status == "Running" {
				data["bk_host_status"] = "1"
			} else {
				data["bk_host_status"] = "2"
			}
			inst, created, e := s.inst.CollectCloudHost(data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, ip+": "+e.Error())
				}
				continue
			}
			*seenHost = append(*seenHost, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.Instances.Instance) == 0 {
			break
		}
		page++
	}
	// 同地域 RDS → MySQL 模型
	if err := s.syncAliyunRDS(ds, region, report, seenMysql); err != nil {
		return err
	}
	// 同地域 Redis(KVStore) → Redis 模型
	if err := s.syncAliyunRedis(ds, region, report, seenRedis); err != nil {
		return err
	}
	// 同地域 消息队列 Kafka 版(alikafka) → Kafka 模型
	if err := s.syncAliyunKafka(ds, region, report, seenKafka); err != nil {
		return err
	}
	// 同地域 云原生多模数据库 Lindorm(hitsdb) → Lindorm 模型
	if err := s.syncAliyunLindorm(ds, region, report, seenLindorm); err != nil {
		return err
	}
	// 同地域 云原生API网关(APIG) → API网关 模型
	if err := s.syncAliyunApig(ds, region, report, seenApig); err != nil {
		return err
	}
	// 同地域 微服务引擎(MSE) 注册配置中心 → ZooKeeper/Eureka 入注册配置中心模型，Nacos 入 Nacos 模型
	if err := s.syncAliyunMse(ds, region, report, seenMse, seenNacos); err != nil {
		return err
	}
	// 同地域 弹性公网IP(EIP) → 弹性公网IP 模型
	if err := s.syncAliyunEip(ds, region, report, seenEip); err != nil {
		return err
	}
	// 同地域 高可用虚拟IP(HaVip) → 高可用虚拟IP 模型
	if err := s.syncAliyunHavip(ds, region, report, seenHavip); err != nil {
		return err
	}
	// 同地域 高速通道物理专线 → 高速通道 模型
	if err := s.syncAliyunExpressConnect(ds, region, report, seenEc); err != nil {
		return err
	}
	// 同地域 边界路由器(VBR) → 边界路由器 模型
	if err := s.syncAliyunVbr(ds, region, report, seenVbr); err != nil {
		return err
	}
	// 同地域 传统型负载均衡 CLB(经典SLB) → CLB 模型
	if err := s.syncAliyunNlb(ds, region, report, seenNlb); err != nil {
		return err
	}
	if err := s.syncAliyunAlb(ds, region, report, seenAlb); err != nil {
		return err
	}
	if err := s.syncAliyunClb(ds, region, report, seenClb); err != nil {
		return err
	}
	// 同地域 NAT 网关 → NAT网关 模型
	if err := s.syncAliyunNatgw(ds, region, report, seenNatgw); err != nil {
		return err
	}
	// 同地域 专有网络 VPC → VPC 模型
	if err := s.syncAliyunVpc(ds, region, report, seenVpc); err != nil {
		return err
	}
	// 同地域 交换机 VSwitch → 交换机 模型（并建立「属于 VPC」实例关联）
	if err := s.syncAliyunVswitch(ds, region, report, seenVsw); err != nil {
		return err
	}
	// 同地域 MaxCompute 项目 → MaxCompute 模型
	if err := s.syncAliyunMaxCompute(ds, region, report, seenMc); err != nil {
		return err
	}
	// 同地域 Hologres 实例 → Hologres 模型
	if err := s.syncAliyunHologres(ds, region, report, seenHolo); err != nil {
		return err
	}
	// 同地域 DataWorks 工作空间 → DataWorks 模型
	if err := s.syncAliyunDataWorks(ds, region, report, seenDw); err != nil {
		return err
	}
	// 同地域 Elasticsearch → Elasticsearch 模型
	return s.syncAliyunElastic(ds, region, report, seenES)
}

// redisEndpoint 返回阿里云云数据库 Redis(KVStore) 地域化 endpoint。
func redisEndpoint(region string) string {
	if region == "" {
		region = "cn-shanghai"
	}
	return "https://r-kvstore." + region + ".aliyuncs.com"
}

// redisStatusID 阿里云 KVStore 实例状态 → 内置状态枚举 id。
// 口径：运行=Normal；创建=Creating/Starting；已停止=停机/到期保留/释放等（Stopping/Stopped/Inactive/Released）；
// 异常=真正的故障（Error/Unavailable）；变配/升级/维护等瞬时态实例仍在服务，归运行中避免误报。
func redisStatusID(s string) string {
	switch s {
	case "Normal":
		return "running"
	case "Creating", "Starting":
		return "creating"
	case "Stopping", "Stopped", "Inactive", "Released":
		return "stopped"
	case "Error", "Unavailable":
		return "abnormal"
	case "Changing", "Flushing", "Transforming", "Migrating", "BackupRecovering",
		"MinorVersionUpgrading", "MajorVersionUpgrading", "NetworkModifying", "SSLModifying",
		"ModifyingParams", "VpcTransforming", "HybridCloudSyncing":
		return "running" // 瞬时维护/变更态，实例仍对外服务，不误标异常
	default:
		return "abnormal"
	}
}

// syncAliyunRedis 同步单地域云数据库 Redis(KVStore) 实例到 Redis 模型。
func (s *DataSourceService) syncAliyunRedis(ds *model.DataSource, region string, report *SyncReport, seenRedis *[]uint) error {
	redisID := s.inst.ModelIDByObjID("redis")
	if redisID == 0 {
		return nil // 无 Redis 模型则跳过
	}
	page := 1
	for {
		body, err := aliRPC(redisEndpoint(region), ds.Username, ds.Password, "2015-01-01", "DescribeInstances",
			map[string]string{"RegionId": region, "PageSize": "100", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			report.skipProduct("redis", "Redis 同步跳过("+region+")："+err.Error())
			return nil
		}
		var r struct {
			TotalCount int `json:"TotalCount"`
			PageSize   int `json:"PageSize"`
			Instances  struct {
				KVStoreInstance []struct {
					InstanceID       string      `json:"InstanceId"`
					InstanceName     string      `json:"InstanceName"`
					InstanceStatus   string      `json:"InstanceStatus"`
					InstanceClass    string      `json:"InstanceClass"`
					Capacity         json.Number `json:"Capacity"` // MB
					ConnectionDomain string      `json:"ConnectionDomain"`
					Port             json.Number `json:"Port"`
					EngineVersion    string      `json:"EngineVersion"`
					ArchitectureType string      `json:"ArchitectureType"`
					RegionID         string      `json:"RegionId"`
					ZoneID           string      `json:"ZoneId"`
				} `json:"KVStoreInstance"`
			} `json:"Instances"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 Redis 响应失败")
		}
		for _, kv := range r.Instances.KVStoreInstance {
			name := firstNonEmpty(kv.InstanceName, kv.InstanceID)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"inst_name":      name,
				"cloud_provider": "aliyun",
				"status":         redisStatusID(kv.InstanceStatus),
			}
			if kv.InstanceID != "" {
				data["resource_id"] = kv.InstanceID // 阿里云 Redis 实例ID
			}
			if kv.EngineVersion != "" {
				data["version"] = kv.EngineVersion
			}
			if kv.ConnectionDomain != "" {
				data["endpoint"] = kv.ConnectionDomain
			}
			if n, e := kv.Port.Int64(); e == nil && n > 0 {
				data["port"] = float64(n)
			}
			if n, e := kv.Capacity.Int64(); e == nil && n > 0 {
				data["mem_gb"] = float64((n + 1023) / 1024) // MB → GB(向上取整，避免 <1GB 记成 0)
			}
			switch kv.ArchitectureType {
			case "standard", "cluster", "rwsplit":
				data["arch"] = kv.ArchitectureType
			}
			if kv.RegionID != "" {
				data["region"] = kv.RegionID
			}
			if kv.ZoneID != "" {
				data["cloud_az"] = kv.ZoneID
			}
			inst, created, e := s.inst.CollectCloudInstance(redisID, "inst_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenRedis = append(*seenRedis, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.Instances.KVStoreInstance) == 0 {
			break
		}
		page++
	}
	return nil
}

// alikafkaEndpoint 返回阿里云消息队列 Kafka 版地域化 endpoint。
func alikafkaEndpoint(region string) string {
	if region == "" {
		region = "cn-shanghai"
	}
	return "https://alikafka." + region + ".aliyuncs.com"
}

// kafkaStatusID 阿里云 Kafka 实例 ServiceStatus → 内置服务状态枚举 id。
// 0待部署/1部署中→creating，5服务中→running，10已过期/15已释放→stopped，其余→abnormal。
func kafkaStatusID(code int) string {
	switch code {
	case 5:
		return "running"
	case 0, 1:
		return "creating"
	case 10, 15:
		return "stopped"
	default:
		return "abnormal"
	}
}

// kafkaDiskTypeID 阿里云 Kafka 磁盘类型(DiskType) → 枚举 id：0 高效云盘，1 SSD。
func kafkaDiskTypeID(code int) string {
	switch code {
	case 0:
		return "efficient"
	case 1:
		return "ssd"
	}
	return ""
}

// kafkaPaidTypeID 阿里云 Kafka 付费类型(PaidType) → 枚举 id。
// 阿里云官方取值：0=包年包月(预付费)、1=按量付费(后付费)、3=Serverless按量付费、4=Confluent系列包年包月。
func kafkaPaidTypeID(code int) string {
	switch code {
	case 0, 4:
		return "PrePaid" // 包年包月
	default:
		return "PostPaid" // 1/3 按量付费
	}
}

// kafkaSpecTypeName 阿里云 Kafka 规格类型(SpecType) → 枚举 id（与同步值一致，用于下拉显示）。
func kafkaSpecTypeID(s string) string {
	switch s {
	case "normal", "professional", "professionalForHighRead":
		return s
	}
	return ""
}

// syncAliyunKafka 同步单地域消息队列 Kafka 版实例到 Kafka 模型。
// GetInstanceList 一次返回该地域全部实例（不分页）。
func (s *DataSourceService) syncAliyunKafka(ds *model.DataSource, region string, report *SyncReport, seenKafka *[]uint) error {
	kafkaID := s.inst.ModelIDByObjID("kafka")
	if kafkaID == 0 {
		return nil // 无 Kafka 模型则跳过
	}
	body, err := aliRPC(alikafkaEndpoint(region), ds.Username, ds.Password, "2019-09-16", "GetInstanceList",
		map[string]string{"RegionId": region})
	if err != nil {
		report.skipProduct("kafka", "Kafka 同步跳过("+region+")："+err.Error())
		return nil
	}
	var r struct {
		InstanceList struct {
			InstanceVO []struct {
				InstanceID               string `json:"InstanceId"`
				Name                     string `json:"Name"`
				ServiceStatus            int    `json:"ServiceStatus"`
				SpecType                 string `json:"SpecType"`
				EndPoint                 string `json:"EndPoint"`
				DomainEndpoint           string `json:"DomainEndpoint"`
				SslDomainEndpoint        string `json:"SslDomainEndpoint"`
				SslEndPoint              string `json:"SslEndPoint"`
				SaslDomainEndpoint       string `json:"SaslDomainEndpoint"`
				SaslEndPoint             string `json:"SaslEndPoint"`
				DiskType                 int    `json:"DiskType"`
				DiskSize                 int    `json:"DiskSize"`
				IoMax                    int    `json:"IoMax"`
				IoMaxSpec                string `json:"IoMaxSpec"`
				TopicNumLimit            int    `json:"TopicNumLimit"`
				MsgRetain                int    `json:"MsgRetain"`
				PaidType                 int    `json:"PaidType"`
				RegionID                 string `json:"RegionId"`
				ZoneID                   string `json:"ZoneId"`
				UpgradeServiceDetailInfo struct {
					Current2OpenSourceVersion string `json:"Current2OpenSourceVersion"`
				} `json:"UpgradeServiceDetailInfo"`
			} `json:"InstanceVO"`
		} `json:"InstanceList"`
	}
	if json.Unmarshal(body, &r) != nil {
		return newValidationErr("解析阿里云 Kafka 响应失败")
	}
	for _, kf := range r.InstanceList.InstanceVO {
		name := firstNonEmpty(kf.Name, kf.InstanceID)
		if name == "" {
			report.Skipped++
			continue
		}
		report.Discovered++
		data := map[string]interface{}{
			"inst_name":      name,
			"cloud_provider": "aliyun",
			"status":         kafkaStatusID(kf.ServiceStatus),
		}
		if kf.InstanceID != "" {
			data["resource_id"] = kf.InstanceID // 阿里云 Kafka 实例ID
		}
		if v := kf.UpgradeServiceDetailInfo.Current2OpenSourceVersion; v != "" {
			data["version"] = v
		}
		if id := kafkaSpecTypeID(kf.SpecType); id != "" {
			data["spec_type"] = id
		}
		// 默认接入点优先用域名(DomainEndpoint，VPC 内网域名:9092)，回退 IP(EndPoint)
		if ep := firstNonEmpty(kf.DomainEndpoint, kf.EndPoint); ep != "" {
			data["endpoint"] = ep
		}
		// SSL 接入点同样优先域名(SslDomainEndpoint:9093)，回退 IP(SslEndPoint)
		if ep := firstNonEmpty(kf.SslDomainEndpoint, kf.SslEndPoint); ep != "" {
			data["ssl_endpoint"] = ep
		}
		// SASL 接入点(9094)：优先域名(SaslDomainEndpoint)，回退 IP(SaslEndPoint)；仅启用 SASL 的实例有
		if ep := firstNonEmpty(kf.SaslDomainEndpoint, kf.SaslEndPoint); ep != "" {
			data["sasl_endpoint"] = ep
		}
		// 流量规格优先取规格字符串(IoMaxSpec)，否则回退峰值流量 MB/s
		if kf.IoMaxSpec != "" {
			data["traffic_spec"] = kf.IoMaxSpec
		} else if kf.IoMax > 0 {
			data["traffic_spec"] = strconv.Itoa(kf.IoMax) + " MB/s"
		}
		if kf.DiskSize > 0 {
			data["disk_gb"] = float64(kf.DiskSize)
		}
		if dt := kafkaDiskTypeID(kf.DiskType); dt != "" {
			data["disk_type"] = dt
		}
		if kf.TopicNumLimit > 0 {
			data["topic_limit"] = float64(kf.TopicNumLimit)
		}
		if kf.MsgRetain > 0 {
			data["msg_retain"] = float64(kf.MsgRetain)
		}
		data["paid_type"] = kafkaPaidTypeID(kf.PaidType)
		if kf.RegionID != "" {
			data["region"] = kf.RegionID
		}
		if kf.ZoneID != "" {
			data["cloud_az"] = kf.ZoneID
		}
		inst, created, e := s.inst.CollectCloudInstance(kafkaID, "inst_name", data, ds.Name, ds.ID)
		if e != nil {
			report.Failed++
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, name+": "+e.Error())
			}
			continue
		}
		*seenKafka = append(*seenKafka, inst.ID)
		if created {
			report.Created++
		} else {
			report.Updated++
		}
	}
	return nil
}

// lindormEndpoint 返回阿里云云原生多模数据库 Lindorm(hitsdb) 地域化 endpoint。
func lindormEndpoint(region string) string {
	if region == "" {
		region = "cn-hangzhou"
	}
	return "https://hitsdb." + region + ".aliyuncs.com"
}

// lindormStatusID 阿里云 Lindorm 实例状态(InstanceStatus) → 内置服务状态枚举 id。
// ACTIVATION 为正常运行；含 CREATING/GUARD_CREATING 等视为创建中；
// DELETED/RELEASED/LOCKED/EXPIRED 等视为已停止；其余（变配/升级等瞬时态）按运行中处理，避免误报异常。
func lindormStatusID(s string) string {
	u := strings.ToUpper(s)
	switch {
	case u == "ACTIVATION":
		return "running"
	case strings.Contains(u, "CREATING"):
		return "creating"
	case strings.Contains(u, "DELET"), strings.Contains(u, "RELEAS"),
		strings.Contains(u, "LOCK"), strings.Contains(u, "EXPIRE"):
		return "stopped"
	case u == "":
		return "abnormal"
	default:
		return "running"
	}
}

// lindormEngines 阿里云 Lindorm 引擎类型(EngineType) 位掩码 → 多选枚举 id 列表。
// 位定义：1=文件引擎(file)、2=宽表引擎(wide)、4=时序引擎(tsdb)、8=搜索引擎(search)。
// 例：EngineType=6=2+4=宽表+时序。返回 []interface{} 以匹配 EnumMulti 校验。
func lindormEngines(code int) []interface{} {
	out := []interface{}{}
	if code&1 != 0 {
		out = append(out, "file")
	}
	if code&2 != 0 {
		out = append(out, "wide")
	}
	if code&4 != 0 {
		out = append(out, "tsdb")
	}
	if code&8 != 0 {
		out = append(out, "search")
	}
	return out
}

// lindormPaidTypeID 阿里云 Lindorm 付费类型(PayType) → 计费方式枚举 id。
func lindormPaidTypeID(s string) string {
	if strings.EqualFold(s, "PREPAY") {
		return "PrePaid"
	}
	return "PostPaid"
}

// syncAliyunLindorm 同步单地域云原生多模数据库 Lindorm 实例到 Lindorm 模型。
// 列表接口 GetLindormInstanceList(POST, 分页) 提供基础信息；版本/规格/磁盘类型需逐实例调
// GetLindormInstance 详情补全（连接地址按引擎分离，列表/详情均不返回，故不同步 endpoint）。
func (s *DataSourceService) syncAliyunLindorm(ds *model.DataSource, region string, report *SyncReport, seenLindorm *[]uint) error {
	lindormID := s.inst.ModelIDByObjID("lindorm")
	if lindormID == 0 {
		return nil // 无 Lindorm 模型则跳过
	}
	page := 1
	for {
		body, err := aliRPCPost(lindormEndpoint(region), ds.Username, ds.Password, "2020-06-15", "GetLindormInstanceList",
			map[string]string{"RegionId": region, "PageNumber": strconv.Itoa(page), "PageSize": "50"})
		if err != nil {
			report.skipProduct("lindorm", "Lindorm 同步跳过("+region+")："+err.Error())
			return nil
		}
		var r struct {
			Total        int `json:"Total"`
			InstanceList []struct {
				InstanceID      string `json:"InstanceId"`
				InstanceAlias   string `json:"InstanceAlias"`
				InstanceStatus  string `json:"InstanceStatus"`
				EngineType      int    `json:"EngineType"`
				InstanceStorage int    `json:"InstanceStorage"`
				PayType         string `json:"PayType"`
				RegionID        string `json:"RegionId"`
				ZoneID          string `json:"ZoneId"`
			} `json:"InstanceList"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 Lindorm 响应失败")
		}
		if len(r.InstanceList) == 0 {
			break
		}
		for _, li := range r.InstanceList {
			name := firstNonEmpty(li.InstanceAlias, li.InstanceID)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"inst_name":      name,
				"cloud_provider": "aliyun",
				"status":         lindormStatusID(li.InstanceStatus),
				"paid_type":      lindormPaidTypeID(li.PayType),
			}
			if li.InstanceID != "" {
				data["resource_id"] = li.InstanceID
			}
			// 引擎先用列表接口的 EngineType 位掩码兜底，下面详情拿到更可靠的 EngineList 再覆盖
			if eng := lindormEngines(li.EngineType); len(eng) > 0 {
				data["engine"] = eng
			}
			if li.InstanceStorage > 0 {
				data["storage_gb"] = float64(li.InstanceStorage)
			}
			if li.RegionID != "" {
				data["region"] = li.RegionID
			}
			if li.ZoneID != "" {
				data["cloud_az"] = li.ZoneID
			}
			// 详情补全：版本/规格/磁盘类型 + 以 EngineList 为准的引擎类型
			if ver, spec, disk, engines := s.lindormDetail(ds, region, li.InstanceID); ver != "" || spec != "" || disk != "" || len(engines) > 0 {
				if ver != "" {
					data["version"] = ver
				}
				if spec != "" {
					data["spec"] = spec
				}
				if disk != "" {
					data["disk_category"] = disk
				}
				if len(engines) > 0 {
					data["engine"] = engines // 覆盖位掩码的误判（如宽表被记成时序）
				}
			}
			inst, created, e := s.inst.CollectCloudInstance(lindormID, "inst_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenLindorm = append(*seenLindorm, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if len(r.InstanceList) < 50 {
			break
		}
		page++
	}
	return nil
}

// lindormEngineName 阿里云 Lindorm 详情 EngineList[].Engine 名称 → 引擎枚举 id。
// 注意：阿里云里 "lindorm" 即「宽表引擎(LindormTable)」。EngineType 位掩码易误判，应以此为准。
func lindormEngineName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "lindorm", "table", "wide":
		return "wide"
	case "tsdb", "ts":
		return "tsdb"
	case "search":
		return "search"
	case "dfs", "file", "filesystem", "lindormdfs":
		return "file"
	}
	return ""
}

// lindormDetail 调 GetLindormInstance 取引擎列表(版本/规格/引擎类型)及磁盘类型，失败返回空（不致命）。
// engines 以详情里的 EngineList[].Engine 名称为准（比列表接口的 EngineType 位掩码可靠）。
func (s *DataSourceService) lindormDetail(ds *model.DataSource, region, instanceID string) (version, spec, disk string, engines []interface{}) {
	if instanceID == "" {
		return "", "", "", nil
	}
	body, err := aliRPCPost(lindormEndpoint(region), ds.Username, ds.Password, "2020-06-15", "GetLindormInstance",
		map[string]string{"RegionId": region, "InstanceId": instanceID})
	if err != nil {
		return "", "", "", nil
	}
	var d struct {
		DiskCategory string `json:"DiskCategory"`
		EngineList   []struct {
			Version       string `json:"Version"`
			Specification string `json:"Specification"`
			Engine        string `json:"Engine"`
		} `json:"EngineList"`
	}
	if json.Unmarshal(body, &d) != nil {
		return "", "", "", nil
	}
	seen := map[string]bool{}
	for _, e := range d.EngineList {
		if id := lindormEngineName(e.Engine); id != "" && !seen[id] {
			seen[id] = true
			engines = append(engines, id)
		}
	}
	if len(d.EngineList) > 0 {
		version = d.EngineList[0].Version
		spec = d.EngineList[0].Specification
	}
	return version, spec, d.DiskCategory, engines
}

// vpcEndpoint 返回阿里云专有网络 VPC 地域化 endpoint（弹性公网IP 属 VPC 产品）。
func vpcEndpoint(region string) string {
	if region == "" {
		region = "cn-shanghai"
	}
	return "https://vpc." + region + ".aliyuncs.com"
}

// eipBillingID 阿里云 EIP 计费方式(InternetChargeType) → 内置枚举 id。
func eipBillingID(c string) string {
	switch c {
	case "PayByBandwidth":
		return "bandwidth"
	case "PayByTraffic":
		return "traffic"
	}
	return ""
}

// eipISPID 阿里云 EIP 线路类型(ISP) → 内置枚举 id。
func eipISPID(isp string) string {
	switch isp {
	case "BGP", "BGP_PRO":
		return "bgp"
	case "ChinaTelecom":
		return "telecom"
	case "ChinaUnicom":
		return "unicom"
	case "ChinaMobile":
		return "mobile"
	}
	return ""
}

// eipBindID 阿里云 EIP 绑定实例类型(InstanceType) → 内置枚举 id；未绑定返回 unbound。
func eipBindID(status, instType string) string {
	if status == "Available" || instType == "" {
		return "unbound"
	}
	switch instType {
	case "EcsInstance", "NetworkInterface", "HaVip":
		return "ecs"
	case "SlbInstance":
		return "nlb"
	case "Nat":
		return "nat"
	}
	return ""
}

// eipStatusID 阿里云 EIP 状态(Status) → 内置服务状态枚举 id。
func eipStatusID(s string) string {
	switch s {
	case "InUse", "Available", "Associating", "Unassociating":
		return "running"
	case "Allocating":
		return "creating"
	}
	return "abnormal"
}

// syncAliyunEip 同步单地域弹性公网IP(EIP) 到 弹性公网IP 模型（VPC 产品 DescribeEipAddresses）。
func (s *DataSourceService) syncAliyunEip(ds *model.DataSource, region string, report *SyncReport, seenEip *[]uint) error {
	eipID := s.inst.ModelIDByObjID("eip")
	if eipID == 0 {
		return nil // 无 弹性公网IP 模型则跳过
	}
	page := 1
	for {
		body, err := aliRPC(vpcEndpoint(region), ds.Username, ds.Password, "2016-04-28", "DescribeEipAddresses",
			map[string]string{"RegionId": region, "PageSize": "100", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			report.skipProduct("eip", "EIP(弹性公网IP) 同步跳过("+region+")："+err.Error())
			return nil
		}
		var r struct {
			TotalCount   int `json:"TotalCount"`
			PageSize     int `json:"PageSize"`
			EipAddresses struct {
				EipAddress []struct {
					AllocationID       string      `json:"AllocationId"`
					Name               string      `json:"Name"`
					IPAddress          string      `json:"IpAddress"`
					Bandwidth          json.Number `json:"Bandwidth"`
					InternetChargeType string      `json:"InternetChargeType"`
					ISP                string      `json:"ISP"`
					Status             string      `json:"Status"`
					InstanceType       string      `json:"InstanceType"`
					RegionID           string      `json:"RegionId"`
				} `json:"EipAddress"`
			} `json:"EipAddresses"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 EIP 响应失败")
		}
		for _, e := range r.EipAddresses.EipAddress {
			ip := strings.TrimSpace(e.IPAddress)
			if ip == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"ip_addr":        ip,
				"cloud_provider": "aliyun",
				"status":         eipStatusID(e.Status),
			}
			if e.AllocationID != "" {
				data["resource_id"] = e.AllocationID // 阿里云 EIP 实例ID
			}
			if e.Name != "" {
				data["inst_name"] = e.Name
			}
			if n, er := e.Bandwidth.Int64(); er == nil && n > 0 {
				data["bandwidth"] = float64(n)
			}
			if v := eipBillingID(e.InternetChargeType); v != "" {
				data["billing"] = v
			}
			if v := eipISPID(e.ISP); v != "" {
				data["isp"] = v
			}
			if v := eipBindID(e.Status, e.InstanceType); v != "" {
				data["bind_type"] = v
			}
			if e.RegionID != "" {
				data["region"] = e.RegionID
			}
			inst, created, er := s.inst.CollectCloudInstance(eipID, "ip_addr", data, ds.Name, ds.ID)
			if er != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, ip+": "+er.Error())
				}
				continue
			}
			*seenEip = append(*seenEip, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.EipAddresses.EipAddress) == 0 {
			break
		}
		page++
	}
	return nil
}

// aliEipIPByAlloc 将 EIP 实例ID(eip-xxx/AllocationId) 解析为公网IP；失败返回空。
func (s *DataSourceService) aliEipIPByAlloc(ds *model.DataSource, region, allocID string) string {
	body, err := aliRPC(vpcEndpoint(region), ds.Username, ds.Password, "2016-04-28", "DescribeEipAddresses",
		map[string]string{"RegionId": region, "AllocationId": allocID, "PageSize": "50"})
	if err != nil {
		return ""
	}
	var r struct {
		EipAddresses struct {
			EipAddress []struct {
				AllocationID string `json:"AllocationId"`
				IPAddress    string `json:"IpAddress"`
			} `json:"EipAddress"`
		} `json:"EipAddresses"`
	}
	if json.Unmarshal(body, &r) != nil {
		return ""
	}
	for _, e := range r.EipAddresses.EipAddress {
		if e.AllocationID == allocID {
			return strings.TrimSpace(e.IPAddress)
		}
	}
	return ""
}

// ecStatusID 阿里云 高速通道物理专线 状态(Status) → 内置服务状态枚举 id。
func ecStatusID(s string) string {
	switch s {
	case "Enabled":
		return "running"
	case "Initial", "Applying", "Approved", "Allocating", "Allocated", "Confirmed", "Connecting":
		return "creating"
	case "Rejected", "Canceled", "Terminating", "Terminated", "AllocationFailed":
		return "stopped"
	}
	return "abnormal"
}

// lineOperatorName 阿里云物理专线运营商(LineOperator) → 中文展示。
func lineOperatorName(op string) string {
	switch op {
	case "CT":
		return "中国电信"
	case "CU":
		return "中国联通"
	case "CM":
		return "中国移动"
	case "CO":
		return "中国其他"
	case "Equinix":
		return "Equinix"
	case "Other":
		return "其他"
	}
	return op
}

// syncAliyunExpressConnect 同步单地域高速通道物理专线 到 高速通道 模型（VPC 产品 DescribePhysicalConnections）。
func (s *DataSourceService) syncAliyunExpressConnect(ds *model.DataSource, region string, report *SyncReport, seenEc *[]uint) error {
	ecID := s.inst.ModelIDByObjID("express_connect")
	if ecID == 0 {
		return nil
	}
	page := 1
	for {
		body, err := aliRPC(vpcEndpoint(region), ds.Username, ds.Password, "2016-04-28", "DescribePhysicalConnections",
			map[string]string{"RegionId": region, "PageSize": "50", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			report.skipProduct("express_connect", "高速通道(物理专线) 同步跳过("+region+")："+err.Error())
			return nil
		}
		var r struct {
			TotalCount             int `json:"TotalCount"`
			PageSize               int `json:"PageSize"`
			PhysicalConnectionSet struct {
				PhysicalConnectionType []struct {
					PhysicalConnectionID string      `json:"PhysicalConnectionId"`
					Name                 string      `json:"Name"`
					Status               string      `json:"Status"`
					Bandwidth            json.Number `json:"Bandwidth"`
					Spec                 string      `json:"Spec"`
					ExpectSpec           string      `json:"ExpectSpec"`
					LineOperator         string      `json:"LineOperator"`
					AccessPointID        string      `json:"AccessPointId"`
					PortType             string      `json:"PortType"`
					PeerLocation         string      `json:"PeerLocation"`
					Type                 string      `json:"Type"`
				} `json:"PhysicalConnectionType"`
			} `json:"PhysicalConnectionSet"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 高速通道 响应失败")
		}
		for _, p := range r.PhysicalConnectionSet.PhysicalConnectionType {
			name := firstNonEmpty(p.Name, p.PhysicalConnectionID)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"inst_name":      name,
				"cloud_provider": "aliyun",
				"status":         ecStatusID(p.Status),
				"region":         region,
			}
			if p.PhysicalConnectionID != "" {
				data["resource_id"] = p.PhysicalConnectionID
			}
			if n, e := p.Bandwidth.Int64(); e == nil && n > 0 {
				data["bandwidth"] = float64(n)
			}
			if v := firstNonEmpty(p.Spec, p.ExpectSpec); v != "" {
				data["spec"] = v
			}
			if p.LineOperator != "" {
				data["line_operator"] = lineOperatorName(p.LineOperator)
			}
			if p.AccessPointID != "" {
				data["access_point"] = p.AccessPointID
			}
			if p.PortType != "" {
				data["port_type"] = p.PortType
			}
			if p.PeerLocation != "" {
				data["peer_location"] = p.PeerLocation
			}
			inst, created, e := s.inst.CollectCloudInstance(ecID, "inst_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenEc = append(*seenEc, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.PhysicalConnectionSet.PhysicalConnectionType) == 0 {
			break
		}
		page++
	}
	return nil
}

// vbrStatusID 阿里云 边界路由器(VBR) 状态(Status) → 内置服务状态枚举 id。
func vbrStatusID(s string) string {
	switch s {
	case "active", "recovering":
		return "running"
	case "unconfirmed", "enabling":
		return "creating"
	case "terminating", "terminated", "deleting":
		return "stopped"
	}
	return "abnormal"
}

// syncAliyunVbr 同步单地域边界路由器(VBR) 到 边界路由器 模型（VPC 产品 DescribeVirtualBorderRouters）。
func (s *DataSourceService) syncAliyunVbr(ds *model.DataSource, region string, report *SyncReport, seenVbr *[]uint) error {
	vbrID := s.inst.ModelIDByObjID("vbr")
	if vbrID == 0 {
		return nil
	}
	page := 1
	for {
		body, err := aliRPC(vpcEndpoint(region), ds.Username, ds.Password, "2016-04-28", "DescribeVirtualBorderRouters",
			map[string]string{"RegionId": region, "PageSize": "50", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			report.skipProduct("vbr", "边界路由器(VBR) 同步跳过("+region+")："+err.Error())
			return nil
		}
		var r struct {
			TotalCount              int `json:"TotalCount"`
			PageSize                int `json:"PageSize"`
			VirtualBorderRouterSet struct {
				VirtualBorderRouterType []struct {
					VbrID                string      `json:"VbrId"`
					Name                 string      `json:"Name"`
					Status               string      `json:"Status"`
					PhysicalConnectionID string      `json:"PhysicalConnectionId"`
					LocalGatewayIP       string      `json:"LocalGatewayIp"`
					PeerGatewayIP        string      `json:"PeerGatewayIp"`
					VlanID               json.Number `json:"VlanId"`
					Bandwidth            json.Number `json:"Bandwidth"`
					AccessPointID        string      `json:"AccessPointId"`
					AssociatedCens       struct {
						AssociatedCen []struct {
							CenID string `json:"CenId"`
						} `json:"AssociatedCen"`
					} `json:"AssociatedCens"`
				} `json:"VirtualBorderRouterType"`
			} `json:"VirtualBorderRouterSet"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 VBR 响应失败")
		}
		for _, v := range r.VirtualBorderRouterSet.VirtualBorderRouterType {
			name := firstNonEmpty(v.Name, v.VbrID)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"inst_name":      name,
				"cloud_provider": "aliyun",
				"status":         vbrStatusID(v.Status),
				"region":         region,
			}
			if v.VbrID != "" {
				data["resource_id"] = v.VbrID
			}
			if v.LocalGatewayIP != "" {
				data["local_gateway_ip"] = v.LocalGatewayIP
			}
			if v.PeerGatewayIP != "" {
				data["peer_gateway_ip"] = v.PeerGatewayIP
			}
			if vlan := v.VlanID.String(); vlan != "" && vlan != "0" {
				data["vlan_id"] = vlan
			}
			if n, e := v.Bandwidth.Int64(); e == nil && n > 0 {
				data["bandwidth"] = float64(n)
			}
			if v.PhysicalConnectionID != "" {
				data["physical_connection"] = v.PhysicalConnectionID
			}
			if v.AccessPointID != "" {
				data["access_point"] = v.AccessPointID
			}
			cens := []string{}
			for _, c := range v.AssociatedCens.AssociatedCen {
				if c.CenID != "" {
					cens = append(cens, c.CenID)
				}
			}
			if len(cens) > 0 {
				data["cen"] = strings.Join(cens, ",")
			}
			inst, created, e := s.inst.CollectCloudInstance(vbrID, "inst_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenVbr = append(*seenVbr, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.VirtualBorderRouterSet.VirtualBorderRouterType) == 0 {
			break
		}
		page++
	}
	return nil
}

// havipStatusID 阿里云 高可用虚拟IP(HaVip) 状态(Status) → 内置服务状态枚举 id。
func havipStatusID(s string) string {
	switch s {
	case "Available", "InUse":
		return "running"
	case "Creating", "Associating", "Unassociating":
		return "creating"
	case "Deleting":
		return "stopped"
	}
	return "abnormal"
}

// syncAliyunHavip 同步单地域高可用虚拟IP(HaVip) 到 高可用虚拟IP 模型（VPC 产品 DescribeHaVips）。
func (s *DataSourceService) syncAliyunHavip(ds *model.DataSource, region string, report *SyncReport, seenHavip *[]uint) error {
	havipID := s.inst.ModelIDByObjID("havip")
	if havipID == 0 {
		return nil // 无 高可用虚拟IP 模型则跳过
	}
	page := 1
	for {
		body, err := aliRPC(vpcEndpoint(region), ds.Username, ds.Password, "2016-04-28", "DescribeHaVips",
			map[string]string{"RegionId": region, "PageSize": "50", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			report.skipProduct("havip", "HaVip(高可用虚拟IP) 同步跳过("+region+")："+err.Error())
			return nil
		}
		var r struct {
			TotalCount int `json:"TotalCount"`
			PageSize   int `json:"PageSize"`
			HaVips     struct {
				HaVip []struct {
					HaVipID     string `json:"HaVipId"`
					Name        string `json:"Name"`
					IPAddress   string `json:"IpAddress"`
					Status      string `json:"Status"`
					VpcID       string `json:"VpcId"`
					VSwitchID   string `json:"VSwitchId"`
					Description string `json:"Description"`
					RegionID    string `json:"RegionId"`
					AssociatedInstances struct {
						AssociatedInstance []string `json:"AssociatedInstance"`
					} `json:"AssociatedInstances"`
					AssociatedEipAddresses struct {
						// 注意：阿里云该接口内层键为小写且拼写少一个 s（typo），需精确匹配
						AssociatedEipAddress []string `json:"associatedEipAddresse"`
					} `json:"AssociatedEipAddresses"`
				} `json:"HaVip"`
			} `json:"HaVips"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 HaVip 响应失败")
		}
		for _, h := range r.HaVips.HaVip {
			ip := strings.TrimSpace(h.IPAddress)
			if ip == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"ip_addr":        ip,
				"cloud_provider": "aliyun",
				"status":         havipStatusID(h.Status),
			}
			if h.HaVipID != "" {
				data["resource_id"] = h.HaVipID
			}
			if h.Name != "" {
				data["inst_name"] = h.Name
			}
			if len(h.AssociatedInstances.AssociatedInstance) > 0 {
				data["bound_instance"] = strings.Join(h.AssociatedInstances.AssociatedInstance, ",")
			}
			// 接口返回的是 EIP 实例ID(eip-xxx)，解析成公网IP 后入库（eip 字段按 IP 校验）
			if len(h.AssociatedEipAddresses.AssociatedEipAddress) > 0 {
				ips := []string{}
				for _, alloc := range h.AssociatedEipAddresses.AssociatedEipAddress {
					if ip := s.aliEipIPByAlloc(ds, region, alloc); ip != "" {
						ips = append(ips, ip)
					}
				}
				if len(ips) > 0 {
					data["eip"] = strings.Join(ips, ",")
				}
			}
			if h.VpcID != "" {
				data["vpc_id"] = h.VpcID
			}
			if h.Description != "" {
				data["description"] = h.Description
			}
			if h.RegionID != "" {
				data["region"] = h.RegionID
			} else {
				data["region"] = region
			}
			inst, created, er := s.inst.CollectCloudInstance(havipID, "ip_addr", data, ds.Name, ds.ID)
			if er != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, ip+": "+er.Error())
				}
				continue
			}
			*seenHavip = append(*seenHavip, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.HaVips.HaVip) == 0 {
			break
		}
		page++
	}
	return nil
}

// slbEndpoint 返回阿里云传统型负载均衡 SLB(CLB) 地域化 endpoint。
func slbEndpoint(region string) string {
	if region == "" {
		region = "cn-shanghai"
	}
	return "https://slb." + region + ".aliyuncs.com"
}

// clbAddrTypeID 阿里云 CLB 网络类型(AddressType) → 内置枚举 id。
func clbAddrTypeID(t string) string {
	switch t {
	case "internet":
		return "internet"
	case "intranet":
		return "intranet"
	}
	return ""
}

// clbStatusID 阿里云 CLB 状态(LoadBalancerStatus) → 内置服务状态枚举 id。
func clbStatusID(s string) string {
	switch s {
	case "active":
		return "running"
	case "inactive":
		return "stopped"
	case "locked":
		return "abnormal"
	}
	return "abnormal"
}

// syncAliyunClb 同步单地域传统型负载均衡 CLB(经典 SLB) 到 CLB 模型（SLB 产品 DescribeLoadBalancers）。
// slbBoundEips 返回本地域内「绑定到 SLB 实例的 EIP」映射：lb实例ID → 公网IP。
// 经典 CLB 内网实例可绑 EIP 做公网入口，SLB 接口不返回，需从 EIP 侧查。
func (s *DataSourceService) slbBoundEips(ds *model.DataSource, region string) map[string]string {
	m := map[string]string{}
	page := 1
	for {
		body, err := aliRPC(vpcEndpoint(region), ds.Username, ds.Password, "2016-04-28", "DescribeEipAddresses",
			map[string]string{"RegionId": region, "PageSize": "100", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			return m
		}
		var r struct {
			TotalCount   int `json:"TotalCount"`
			PageSize     int `json:"PageSize"`
			EipAddresses struct {
				EipAddress []struct {
					IPAddress    string `json:"IpAddress"`
					InstanceID   string `json:"InstanceId"`
					InstanceType string `json:"InstanceType"`
				} `json:"EipAddress"`
			} `json:"EipAddresses"`
		}
		if json.Unmarshal(body, &r) != nil {
			return m
		}
		for _, e := range r.EipAddresses.EipAddress {
			if e.InstanceType == "SlbInstance" && e.InstanceID != "" && e.IPAddress != "" {
				m[e.InstanceID] = e.IPAddress
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.EipAddresses.EipAddress) == 0 {
			break
		}
		page++
	}
	return m
}

func (s *DataSourceService) syncAliyunClb(ds *model.DataSource, region string, report *SyncReport, seenClb *[]uint) error {
	clbID := s.inst.ModelIDByObjID("clb")
	if clbID == 0 {
		return nil // 无 CLB 模型则跳过
	}
	slbEips := s.slbBoundEips(ds, region) // 绑到各 SLB 的公网 EIP
	page := 1
	for {
		body, err := aliRPC(slbEndpoint(region), ds.Username, ds.Password, "2014-05-15", "DescribeLoadBalancers",
			map[string]string{"RegionId": region, "PageSize": "100", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			// CLB 产品级接口错误（如 RAM 无 SLB 权限 403）不中断整体同步，记录提示并跳过
			report.skipProduct("clb", "CLB["+region+"] 跳过同步："+err.Error()+"（如需同步请为 RAM 用户授予 SLB 只读权限 AliyunSLBReadOnlyAccess）")
			return nil
		}
		var r struct {
			TotalCount    int `json:"TotalCount"`
			PageSize      int `json:"PageSize"`
			LoadBalancers struct {
				LoadBalancer []struct {
					LoadBalancerID     string      `json:"LoadBalancerId"`
					LoadBalancerName   string      `json:"LoadBalancerName"`
					Address            string      `json:"Address"`
					AddressType        string      `json:"AddressType"`
					LoadBalancerStatus string      `json:"LoadBalancerStatus"`
					LoadBalancerSpec   string      `json:"LoadBalancerSpec"`
					Bandwidth          json.Number `json:"Bandwidth"`
					RegionID           string      `json:"RegionId"`
					MasterZoneID       string      `json:"MasterZoneId"`
				} `json:"LoadBalancer"`
			} `json:"LoadBalancers"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 CLB 响应失败")
		}
		for _, lb := range r.LoadBalancers.LoadBalancer {
			name := firstNonEmpty(lb.LoadBalancerName, lb.LoadBalancerID)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"inst_name":      name,
				"cloud_provider": "aliyun",
				"status":         clbStatusID(lb.LoadBalancerStatus),
			}
			if lb.LoadBalancerID != "" {
				data["resource_id"] = lb.LoadBalancerID // 阿里云 CLB 实例ID
			}
			if lb.Address != "" {
				data["vip"] = lb.Address
			}
			// 内网 SLB 若绑了公网 EIP，补记公网 IP（SLB 接口不返回，来自 EIP 侧）
			if eip := slbEips[lb.LoadBalancerID]; eip != "" && eip != lb.Address {
				data["eip"] = eip
			}
			if v := clbAddrTypeID(lb.AddressType); v != "" {
				data["addr_type"] = v
			}
			if lb.LoadBalancerSpec != "" {
				data["spec"] = lb.LoadBalancerSpec
			}
			if n, e := lb.Bandwidth.Int64(); e == nil && n > 0 {
				data["bandwidth"] = float64(n)
			}
			if lb.RegionID != "" {
				data["region"] = lb.RegionID
			}
			if lb.MasterZoneID != "" {
				data["cloud_az"] = lb.MasterZoneID // CLB 主可用区
			}
			inst, created, e := s.inst.CollectCloudInstance(clbID, "inst_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenClb = append(*seenClb, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.LoadBalancers.LoadBalancer) == 0 {
			break
		}
		page++
	}
	return nil
}

// nlbEndpoint 返回阿里云网络型负载均衡(NLB)地域化 endpoint。
func nlbEndpoint(region string) string {
	if region == "" {
		region = "cn-shanghai"
	}
	return "https://nlb." + region + ".aliyuncs.com"
}

// nlbStatusID 阿里云 NLB 状态 → 内置服务状态枚举 id。
func nlbStatusID(s string) string {
	switch s {
	case "Active":
		return "running"
	case "Inactive":
		return "stopped"
	case "Provisioning", "Configuring", "Creating":
		return "creating"
	}
	return "abnormal"
}

// syncAliyunNlb 同步阿里云网络型负载均衡(NLB) → NLB 模型。NLB 为多可用区、无固定 VIP，
// 取各可用区地址(公网优先)为 vip、DNSName 不入库（模型无对应字段）。
func (s *DataSourceService) syncAliyunNlb(ds *model.DataSource, region string, report *SyncReport, seenNlb *[]uint) error {
	nlbID := s.inst.ModelIDByObjID("nlb")
	if nlbID == 0 {
		return nil
	}
	nextToken := ""
	for {
		params := map[string]string{"RegionId": region, "MaxResults": "100"}
		if nextToken != "" {
			params["NextToken"] = nextToken
		}
		body, err := aliRPC(nlbEndpoint(region), ds.Username, ds.Password, "2022-04-30", "ListLoadBalancers", params)
		if err != nil {
			report.skipProduct("nlb", "NLB["+region+"] 跳过同步："+err.Error()+"（如需同步请为 RAM 用户授予 NLB 只读权限）")
			return nil
		}
		var r struct {
			NextToken     string `json:"NextToken"`
			LoadBalancers []struct {
				LoadBalancerID            string `json:"LoadBalancerId"`
				LoadBalancerName          string `json:"LoadBalancerName"`
				DNSName                   string `json:"DNSName"`
				LoadBalancerStatus        string `json:"LoadBalancerStatus"`
				AddressType               string `json:"AddressType"`
				RegionID                  string `json:"RegionId"`
				LoadBalancerBillingConfig struct {
					InternetBandwidth json.Number `json:"InternetBandwidth"`
				} `json:"LoadBalancerBillingConfig"`
				ZoneMappings []struct {
					ZoneID                string `json:"ZoneId"`
					LoadBalancerAddresses []struct {
						PublicIPv4Address  string `json:"PublicIPv4Address"`
						PrivateIPv4Address string `json:"PrivateIPv4Address"`
					} `json:"LoadBalancerAddresses"`
				} `json:"ZoneMappings"`
			} `json:"LoadBalancers"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 NLB 响应失败")
		}
		for _, lb := range r.LoadBalancers {
			name := firstNonEmpty(lb.LoadBalancerName, lb.LoadBalancerID)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			// 收集各可用区地址（公网优先，回退内网）与可用区
			ips := []string{}
			zones := []string{}
			for _, zm := range lb.ZoneMappings {
				if zm.ZoneID != "" {
					zones = append(zones, zm.ZoneID)
				}
				for _, a := range zm.LoadBalancerAddresses {
					if a.PublicIPv4Address != "" {
						ips = append(ips, a.PublicIPv4Address)
					} else if a.PrivateIPv4Address != "" {
						ips = append(ips, a.PrivateIPv4Address)
					}
				}
			}
			data := map[string]interface{}{
				"inst_name":      name,
				"cloud_provider": "aliyun",
				"status":         nlbStatusID(lb.LoadBalancerStatus),
			}
			if lb.LoadBalancerID != "" {
				data["resource_id"] = lb.LoadBalancerID
			}
			// NLB 无 VIP，各可用区地址(外网=EIP/内网=私网IP)全写、逗号分隔
			if len(ips) > 0 {
				data["eip"] = strings.Join(ips, ",")
			}
			if lb.DNSName != "" {
				data["dns_name"] = lb.DNSName
			}
			if v := clbAddrTypeID(strings.ToLower(lb.AddressType)); v != "" {
				data["addr_type"] = v // internet/intranet
			}
			if n, e := lb.LoadBalancerBillingConfig.InternetBandwidth.Int64(); e == nil && n > 0 {
				data["bandwidth"] = float64(n)
			}
			if lb.RegionID != "" {
				data["region"] = lb.RegionID
			}
			if len(zones) > 0 {
				data["cloud_az"] = strings.Join(zones, ",")
			}
			inst, created, e := s.inst.CollectCloudInstance(nlbID, "inst_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenNlb = append(*seenNlb, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.NextToken == "" || len(r.LoadBalancers) == 0 {
			break
		}
		nextToken = r.NextToken
	}
	return nil
}

// albEndpoint 返回阿里云应用型负载均衡(ALB)地域化 endpoint。
func albEndpoint(region string) string {
	if region == "" {
		region = "cn-shanghai"
	}
	return "https://alb." + region + ".aliyuncs.com"
}

// albStatusID 阿里云 ALB 状态 → 内置服务状态枚举 id。
func albStatusID(s string) string {
	switch s {
	case "Active":
		return "running"
	case "Inactive":
		return "stopped"
	case "Provisioning", "Configuring", "Creating":
		return "creating"
	}
	return "abnormal"
}

// albAddresses 调 ALB 详情接口取各可用区地址（公网 Address / 内网 IntranetAddress）与可用区。
func (s *DataSourceService) albAddresses(ds *model.DataSource, region, lbID string, internet bool) (ips, zones []string) {
	if lbID == "" {
		return nil, nil
	}
	body, err := aliRPCv3(albEndpoint(region), ds.Username, ds.Password, "2020-06-16", "GetLoadBalancerAttribute",
		map[string]string{"LoadBalancerId": lbID})
	if err != nil {
		return nil, nil
	}
	var r struct {
		ZoneMappings []struct {
			ZoneID                string `json:"ZoneId"`
			LoadBalancerAddresses []struct {
				Address         string `json:"Address"`
				IntranetAddress string `json:"IntranetAddress"`
			} `json:"LoadBalancerAddresses"`
		} `json:"ZoneMappings"`
	}
	if json.Unmarshal(body, &r) != nil {
		return nil, nil
	}
	for _, zm := range r.ZoneMappings {
		if zm.ZoneID != "" {
			zones = append(zones, zm.ZoneID)
		}
		for _, a := range zm.LoadBalancerAddresses {
			ip := a.IntranetAddress
			if internet && a.Address != "" {
				ip = a.Address
			}
			if ip != "" {
				ips = append(ips, ip)
			}
		}
	}
	return ips, zones
}

// syncAliyunAlb 同步阿里云应用型负载均衡(ALB) → ALB 模型。ALB 为新一代产品，仅支持 V3 签名(aliRPCv3)。
// 字段同 NLB：多可用区取首个地址为 vip、DNSName 入 dns_name、AddressType 入 addr_type。
func (s *DataSourceService) syncAliyunAlb(ds *model.DataSource, region string, report *SyncReport, seenAlb *[]uint) error {
	albID := s.inst.ModelIDByObjID("alb")
	if albID == 0 {
		return nil
	}
	nextToken := ""
	for {
		params := map[string]string{"RegionId": region, "MaxResults": "100"}
		if nextToken != "" {
			params["NextToken"] = nextToken
		}
		body, err := aliRPCv3(albEndpoint(region), ds.Username, ds.Password, "2020-06-16", "ListLoadBalancers", params)
		if err != nil {
			report.skipProduct("alb", "ALB["+region+"] 跳过同步："+err.Error()+"（如需同步请为 RAM 用户授予 ALB 只读权限 AliyunALBReadOnlyAccess）")
			return nil
		}
		var r struct {
			NextToken     string `json:"NextToken"`
			LoadBalancers []struct {
				LoadBalancerID            string `json:"LoadBalancerId"`
				LoadBalancerName          string `json:"LoadBalancerName"`
				DNSName                   string `json:"DNSName"`
				LoadBalancerStatus        string `json:"LoadBalancerStatus"`
				AddressType               string `json:"AddressType"`
				LoadBalancerBillingConfig struct {
					InternetBandwidth json.Number `json:"InternetBandwidth"`
				} `json:"LoadBalancerBillingConfig"`
				ZoneMappings []struct {
					ZoneID                string `json:"ZoneId"`
					LoadBalancerAddresses []struct {
						Address         string `json:"Address"`
						IntranetAddress string `json:"IntranetAddress"`
					} `json:"LoadBalancerAddresses"`
				} `json:"ZoneMappings"`
			} `json:"LoadBalancers"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 ALB 响应失败")
		}
		for _, lb := range r.LoadBalancers {
			name := firstNonEmpty(lb.LoadBalancerName, lb.LoadBalancerID)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			internet := strings.EqualFold(lb.AddressType, "Internet")
			// ALB 列表接口不返回 IP，调详情接口取各可用区地址（公网取 Address，内网取 IntranetAddress）
			ips, zones := s.albAddresses(ds, region, lb.LoadBalancerID, internet)
			data := map[string]interface{}{
				"inst_name":      name,
				"cloud_provider": "aliyun",
				"status":         albStatusID(lb.LoadBalancerStatus),
			}
			if lb.LoadBalancerID != "" {
				data["resource_id"] = lb.LoadBalancerID
			}
			if len(ips) > 0 {
				data["eip"] = strings.Join(ips, ",") // ALB 无 VIP，各可用区 EIP 全写、逗号分隔
			}
			if lb.DNSName != "" {
				data["dns_name"] = lb.DNSName
			}
			if v := clbAddrTypeID(strings.ToLower(lb.AddressType)); v != "" {
				data["addr_type"] = v
			}
			if n, e := lb.LoadBalancerBillingConfig.InternetBandwidth.Int64(); e == nil && n > 0 {
				data["bandwidth"] = float64(n)
			}
			data["region"] = region
			if len(zones) > 0 {
				data["cloud_az"] = strings.Join(zones, ",")
			}
			inst, created, e := s.inst.CollectCloudInstance(albID, "inst_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenAlb = append(*seenAlb, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.NextToken == "" || len(r.LoadBalancers) == 0 {
			break
		}
		nextToken = r.NextToken
	}
	return nil
}

// cenStatusID 阿里云 云企业网(CEN) 状态(Status) → 内置服务状态枚举 id。
func cenStatusID(s string) string {
	switch s {
	case "Active":
		return "running"
	case "Creating":
		return "creating"
	case "Deleting":
		return "stopped"
	}
	return "abnormal"
}

// syncAliyunCen 同步阿里云云企业网(CEN) → 云企业网模型。CEN 为全局资源（跨地域/跨 VPC 骨干网），
// 用全局 endpoint cbn.aliyuncs.com 一次拉全，按 PageNumber 翻页。
func (s *DataSourceService) syncAliyunCen(ds *model.DataSource, report *SyncReport, seenCen *[]uint) error {
	cenID := s.inst.ModelIDByObjID("cen")
	if cenID == 0 {
		return nil
	}
	page := 1
	collected := 0
	for {
		params := map[string]string{"PageSize": "50", "PageNumber": strconv.Itoa(page)}
		body, err := aliRPC("https://cbn.aliyuncs.com", ds.Username, ds.Password, "2017-09-12", "DescribeCens", params)
		if err != nil {
			report.skipProduct("cen", "CEN 同步跳过："+err.Error()+"（如需同步请为 RAM 用户授予云企业网只读权限）")
			return nil
		}
		var r struct {
			TotalCount int `json:"TotalCount"`
			Cens       struct {
				Cen []struct {
					CenID           string `json:"CenId"`
					Name            string `json:"Name"`
					Status          string `json:"Status"`
					Description     string `json:"Description"`
					ProtectionLevel string `json:"ProtectionLevel"`
				} `json:"Cen"`
			} `json:"Cens"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 CEN 响应失败")
		}
		for _, c := range r.Cens.Cen {
			name := firstNonEmpty(c.Name, c.CenID)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"inst_name":      name,
				"cloud_provider": "aliyun",
				"status":         cenStatusID(c.Status),
			}
			if c.CenID != "" {
				data["resource_id"] = c.CenID
			}
			if c.Description != "" {
				data["description"] = c.Description
			}
			if c.ProtectionLevel != "" {
				data["protection_level"] = c.ProtectionLevel
			}
			inst, created, e := s.inst.CollectCloudInstance(cenID, "inst_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenCen = append(*seenCen, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		collected += len(r.Cens.Cen)
		if len(r.Cens.Cen) == 0 || collected >= r.TotalCount {
			break
		}
		page++
	}
	return nil
}

// natgwStatusID 阿里云 NAT 网关状态(Status) → 内置服务状态枚举 id。
func natgwStatusID(s string) string {
	switch s {
	case "Available":
		return "running"
	case "Initiating", "Creating", "Pending", "Modifying", "Converting":
		return "creating"
	case "Deleting":
		return "stopped"
	}
	return "abnormal"
}

// natgwSpecName NAT 网关规格/类型 → 中文展示（增强型 Spec 为空时用类型名）。
func natgwSpecName(spec, natType string) string {
	switch spec {
	case "Small":
		return "小型"
	case "Middle":
		return "中型"
	case "Large":
		return "大型"
	case "XLarge.1":
		return "超大型"
	case "":
		if natType == "Enhanced" {
			return "增强型"
		}
	}
	return spec
}

// syncAliyunNatgw 同步单地域 NAT 网关到 NAT网关 模型（VPC 产品 DescribeNatGateways）。
func (s *DataSourceService) syncAliyunNatgw(ds *model.DataSource, region string, report *SyncReport, seenNatgw *[]uint) error {
	natgwID := s.inst.ModelIDByObjID("natgw")
	if natgwID == 0 {
		return nil // 无 NAT网关 模型则跳过
	}
	page := 1
	for {
		body, err := aliRPC(vpcEndpoint(region), ds.Username, ds.Password, "2016-04-28", "DescribeNatGateways",
			map[string]string{"RegionId": region, "PageSize": "50", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			// 产品级接口错误（如 RAM 无权限）不中断整体同步，记录提示并跳过
			report.skipProduct("natgw", "NAT网关["+region+"] 跳过同步："+err.Error())
			return nil
		}
		var r struct {
			TotalCount  int `json:"TotalCount"`
			PageSize    int `json:"PageSize"`
			NatGateways struct {
				NatGateway []struct {
					NatGatewayID string `json:"NatGatewayId"`
					Name         string `json:"Name"`
					Status       string `json:"Status"`
					Spec         string `json:"Spec"`
					NatType      string `json:"NatType"`
					RegionID     string `json:"RegionId"`
					IPLists      struct {
						IPList []struct {
							IPAddress string `json:"IpAddress"`
						} `json:"IpList"`
					} `json:"IpLists"`
					PrivateInfo struct {
						IzNo string `json:"IzNo"` // 增强型 NAT 主可用区
					} `json:"NatGatewayPrivateInfo"`
				} `json:"NatGateway"`
			} `json:"NatGateways"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 NAT 网关响应失败")
		}
		for _, ng := range r.NatGateways.NatGateway {
			name := firstNonEmpty(ng.Name, ng.NatGatewayID)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"inst_name":      name,
				"cloud_provider": "aliyun",
				"status":         natgwStatusID(ng.Status),
				"eip_count":      float64(len(ng.IPLists.IPList)),
			}
			if ng.NatGatewayID != "" {
				data["resource_id"] = ng.NatGatewayID // 阿里云 NAT 网关实例ID
			}
			if v := natgwSpecName(ng.Spec, ng.NatType); v != "" {
				data["spec"] = v
			}
			if ng.RegionID != "" {
				data["region"] = ng.RegionID
			}
			if ng.PrivateInfo.IzNo != "" {
				data["cloud_az"] = ng.PrivateInfo.IzNo // 增强型 NAT 主可用区
			}
			inst, created, e := s.inst.CollectCloudInstance(natgwID, "inst_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenNatgw = append(*seenNatgw, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.NatGateways.NatGateway) == 0 {
			break
		}
		page++
	}
	return nil
}

// vpcStatusID 阿里云 VPC 状态 → 内置服务状态枚举 id。
func vpcStatusID(s string) string {
	switch s {
	case "Available":
		return "running"
	case "Pending":
		return "creating"
	}
	return ""
}

// syncAliyunVpc 同步单地域专有网络 VPC（DescribeVpcs 2016-04-28）到 VPC 模型。
// 按 vpc_id upsert：名称/网段/是否默认/交换机数/地域/状态。VPC 为地域级资源无可用区。
func (s *DataSourceService) syncAliyunVpc(ds *model.DataSource, region string, report *SyncReport, seenVpc *[]uint) error {
	vpcID := s.inst.ModelIDByObjID("vpc")
	if vpcID == 0 {
		return nil // 无 VPC 模型则跳过
	}
	page := 1
	for {
		body, err := aliRPC(vpcEndpoint(region), ds.Username, ds.Password, "2016-04-28", "DescribeVpcs",
			map[string]string{"RegionId": region, "PageSize": "50", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			report.skipProduct("vpc", "VPC 同步跳过("+region+")："+err.Error())
			return nil
		}
		var r struct {
			TotalCount int `json:"TotalCount"`
			PageSize   int `json:"PageSize"`
			Vpcs       struct {
				Vpc []struct {
					VpcId      string `json:"VpcId"`
					VpcName    string `json:"VpcName"`
					CidrBlock  string `json:"CidrBlock"`
					IsDefault  bool   `json:"IsDefault"`
					Status     string `json:"Status"`
					RegionId   string `json:"RegionId"`
					VSwitchIds struct {
						VSwitchId []string `json:"VSwitchId"`
					} `json:"VSwitchIds"`
				} `json:"Vpc"`
			} `json:"Vpcs"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云VPC响应失败")
		}
		for _, v := range r.Vpcs.Vpc {
			id := strings.TrimSpace(v.VpcId)
			if id == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"vpc_id":         id,
				"vpc_name":       v.VpcName,
				"cidr":           v.CidrBlock,
				"vswitch_count":  float64(len(v.VSwitchIds.VSwitchId)),
				"region":         firstNonEmpty(v.RegionId, region),
				"cloud_provider": "aliyun",
			}
			if v.IsDefault {
				data["is_default"] = "yes"
			} else {
				data["is_default"] = "no"
			}
			if st := vpcStatusID(v.Status); st != "" {
				data["status"] = st
			}
			inst, created, e := s.inst.UpsertInstance(vpcID, "vpc_id", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, id+": "+e.Error())
				}
				continue
			}
			*seenVpc = append(*seenVpc, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.Vpcs.Vpc) == 0 {
			break
		}
		page++
	}
	return nil
}

// syncAliyunVswitch 同步单地域交换机 VSwitch（DescribeVSwitches 2016-04-28）到 交换机 模型，
// 并按 VpcId 建立「交换机 属于 VPC」的实例关联（供拓扑展示）。按 vswitch_id upsert。
func (s *DataSourceService) syncAliyunVswitch(ds *model.DataSource, region string, report *SyncReport, seenVsw *[]uint) error {
	vswID := s.inst.ModelIDByObjID("vswitch")
	if vswID == 0 {
		return nil // 无 交换机 模型则跳过
	}
	vpcID := s.inst.ModelIDByObjID("vpc")
	assocID := s.inst.AssocIDByCode("belong_vswitch_vpc")
	page := 1
	for {
		body, err := aliRPC(vpcEndpoint(region), ds.Username, ds.Password, "2016-04-28", "DescribeVSwitches",
			map[string]string{"RegionId": region, "PageSize": "50", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			report.skipProduct("vswitch", "交换机(VSwitch) 同步跳过("+region+")："+err.Error())
			return nil
		}
		var r struct {
			TotalCount int `json:"TotalCount"`
			PageSize   int `json:"PageSize"`
			VSwitches  struct {
				VSwitch []struct {
					VSwitchId               string `json:"VSwitchId"`
					VSwitchName             string `json:"VSwitchName"`
					CidrBlock               string `json:"CidrBlock"`
					ZoneId                  string `json:"ZoneId"`
					AvailableIpAddressCount int64  `json:"AvailableIpAddressCount"`
					VpcId                   string `json:"VpcId"`
					Status                  string `json:"Status"`
				} `json:"VSwitch"`
			} `json:"VSwitches"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云交换机响应失败")
		}
		for _, v := range r.VSwitches.VSwitch {
			id := strings.TrimSpace(v.VSwitchId)
			if id == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"vswitch_id":     id,
				"vswitch_name":   v.VSwitchName,
				"cidr":           v.CidrBlock,
				"zone":           v.ZoneId,
				"available_ip":   float64(v.AvailableIpAddressCount),
				"vpc_id":         v.VpcId,
				"region":         region,
				"cloud_provider": "aliyun",
				"cloud_az":       v.ZoneId,
			}
			if st := vpcStatusID(v.Status); st != "" {
				data["status"] = st
			}
			inst, created, e := s.inst.UpsertInstance(vswID, "vswitch_id", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, id+": "+e.Error())
				}
				continue
			}
			*seenVsw = append(*seenVsw, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
			// 建立「交换机 属于 VPC」实例关联（拓扑连线），幂等
			if vpcID != 0 && assocID != 0 && v.VpcId != "" {
				if vpcInstID := s.inst.FindInstanceIDByProp(vpcID, "vpc_id", v.VpcId, ds.ID); vpcInstID != 0 {
					_ = s.inst.EnsureRelation(assocID, inst.ID, vpcInstID)
				}
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.VSwitches.VSwitch) == 0 {
			break
		}
		page++
	}
	return nil
}

// cdnStatusID 阿里云 CDN 域名状态 → 内置服务状态枚举 id。
func cdnStatusID(s string) string {
	switch s {
	case "online":
		return "running"
	case "offline":
		return "stopped"
	case "configuring", "checking", "check_failed", "stopping", "deleting":
		return "creating"
	}
	return "abnormal"
}

// syncAliyunCdn 同步阿里云 CDN 加速域名到 CDN 模型（全局资源，不分地域）。
// DescribeUserDomains(2018-05-10, cdn.aliyuncs.com) 分页取全部域名及其 CNAME/类型/源站/区域/HTTPS/状态。
// 容错：无权限（需 AliyunCDNReadOnlyAccess）记入 report.Errors 并跳过，不中断整体同步。
func (s *DataSourceService) syncAliyunCdn(ds *model.DataSource, report *SyncReport, seenCdn *[]uint) error {
	cdnID := s.inst.ModelIDByObjID("cdn")
	if cdnID == 0 {
		return nil // 无 CDN 模型则跳过
	}
	page := 1
	for {
		body, err := aliRPC("https://cdn.aliyuncs.com", ds.Username, ds.Password, "2018-05-10", "DescribeUserDomains",
			map[string]string{"PageSize": "50", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			report.skipProduct("cdn", "CDN 同步跳过："+err.Error()+"（如未开通 CDN 或 RAM 用户无 AliyunCDNReadOnlyAccess 权限）")
			return nil
		}
		var r struct {
			TotalCount int `json:"TotalCount"`
			PageSize   int `json:"PageSize"`
			Domains    struct {
				PageData []struct {
					DomainName   string      `json:"DomainName"`
					DomainId     json.Number `json:"DomainId"`
					Cname        string      `json:"Cname"`
					CdnType      string      `json:"CdnType"`
					DomainStatus string      `json:"DomainStatus"`
					SslProtocol  string      `json:"SslProtocol"`
					Coverage     string      `json:"Coverage"`
					Description  string      `json:"Description"`
					Sources      struct {
						Source []struct {
							Content string `json:"Content"`
						} `json:"Source"`
					} `json:"Sources"`
				} `json:"PageData"`
			} `json:"Domains"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云CDN响应失败")
		}
		for _, d := range r.Domains.PageData {
			name := strings.TrimSpace(d.DomainName)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			srcs := []string{}
			for _, so := range d.Sources.Source {
				if so.Content != "" {
					srcs = append(srcs, so.Content)
				}
			}
			data := map[string]interface{}{
				"domain":         name,
				"cname":          d.Cname,
				"cdn_type":       d.CdnType,
				"source":         strings.Join(srcs, ","),
				"status":         cdnStatusID(d.DomainStatus),
				"cloud_provider": "aliyun",
			}
			if d.Coverage != "" {
				data["coverage"] = d.Coverage
			}
			if d.SslProtocol == "on" || d.SslProtocol == "off" {
				data["ssl"] = d.SslProtocol
			}
			if d.Description != "" {
				data["description"] = d.Description
			}
			if id := d.DomainId.String(); id != "" && id != "0" {
				data["resource_id"] = id
			}
			inst, created, e := s.inst.UpsertInstance(cdnID, "domain", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenCdn = append(*seenCdn, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.Domains.PageData) == 0 {
			break
		}
		page++
	}
	return nil
}

// aliROA 阿里云 ROA(RESTful)风格 OpenAPI 调用（GET + 老版 acs HMAC-SHA1 header 签名），
// 用于 Elasticsearch 等 ROA 产品。返回响应体。
func aliROA(endpoint, path string, query map[string]string, ak, sk, version string) ([]byte, error) {
	date := time.Now().UTC().Format(http.TimeFormat) // RFC1123 GMT
	headers := map[string]string{
		"x-acs-version":           version,
		"x-acs-signature-nonce":   aliNonce(),
		"x-acs-signature-method":  "HMAC-SHA1",
		"x-acs-signature-version": "1.0",
	}
	hkeys := make([]string, 0, len(headers))
	for k := range headers {
		hkeys = append(hkeys, k)
	}
	sort.Strings(hkeys)
	canonHeaders := ""
	for _, k := range hkeys {
		canonHeaders += k + ":" + headers[k] + "\n"
	}
	canonResource := path
	if len(query) > 0 {
		qk := make([]string, 0, len(query))
		for k := range query {
			qk = append(qk, k)
		}
		sort.Strings(qk)
		parts := make([]string, 0, len(qk))
		for _, k := range qk {
			parts = append(parts, k+"="+query[k])
		}
		canonResource += "?" + strings.Join(parts, "&")
	}
	accept := "application/json"
	stringToSign := "GET\n" + accept + "\n\n\n" + date + "\n" + canonHeaders + canonResource
	mac := hmac.New(sha1.New, []byte(sk))
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	uq := url.Values{}
	for k, v := range query {
		uq.Set(k, v)
	}
	reqURL := endpoint + path
	if len(uq) > 0 {
		reqURL += "?" + uq.Encode()
	}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("Date", date)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", "acs "+ak+":"+sig)
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		var e struct{ Code, Message string }
		_ = json.Unmarshal(body, &e)
		if e.Message != "" {
			return nil, newValidationErr("阿里云接口错误：%s", e.Message)
		}
		return nil, newValidationErr("阿里云接口返回 HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// aliSha256Hex 返回字符串的 SHA256 十六进制摘要。
func aliSha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// aliRPCV3 阿里云新版 V3(ACS3-HMAC-SHA256)签名的 ROA(RESTful) GET 调用，
// 用于云原生API网关(APIG, 2024-03-27)等仅支持 V3 签名的新产品。返回响应体。
func aliRPCV3(endpoint, path string, query map[string]string, ak, sk, version, action string) ([]byte, error) {
	host := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	bodyHash := aliSha256Hex("")
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	// 规范化查询串：按 key 排序，key/value 均做 RFC3986 百分号编码
	qk := make([]string, 0, len(query))
	for k := range query {
		qk = append(qk, k)
	}
	sort.Strings(qk)
	parts := make([]string, 0, len(qk))
	for _, k := range qk {
		parts = append(parts, aliPercentEncode(k)+"="+aliPercentEncode(query[k]))
	}
	canonQuery := strings.Join(parts, "&")

	headers := map[string]string{
		"host":                  host,
		"x-acs-action":          action,
		"x-acs-version":         version,
		"x-acs-date":            now,
		"x-acs-signature-nonce": aliNonce(),
		"x-acs-content-sha256":  bodyHash,
	}
	hk := make([]string, 0, len(headers))
	for k := range headers {
		hk = append(hk, k)
	}
	sort.Strings(hk)
	var canonHeaders strings.Builder
	for _, k := range hk {
		canonHeaders.WriteString(k + ":" + strings.TrimSpace(headers[k]) + "\n")
	}
	signedHeaders := strings.Join(hk, ";")

	canonReq := "GET\n" + path + "\n" + canonQuery + "\n" + canonHeaders.String() + "\n" + signedHeaders + "\n" + bodyHash
	stringToSign := "ACS3-HMAC-SHA256\n" + aliSha256Hex(canonReq)
	mac := hmac.New(sha256.New, []byte(sk))
	mac.Write([]byte(stringToSign))
	sig := hex.EncodeToString(mac.Sum(nil))
	auth := "ACS3-HMAC-SHA256 Credential=" + ak + ",SignedHeaders=" + signedHeaders + ",Signature=" + sig

	uq := url.Values{}
	for k, v := range query {
		uq.Set(k, v)
	}
	reqURL := endpoint + path
	if len(uq) > 0 {
		reqURL += "?" + uq.Encode()
	}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		var e struct{ Code, Message string }
		_ = json.Unmarshal(body, &e)
		if e.Message != "" {
			return nil, newValidationErr("阿里云接口错误：%s", e.Message)
		}
		return nil, newValidationErr("阿里云接口返回 HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// apigEndpoint 返回阿里云云原生API网关(APIG)地域化 endpoint。
func apigEndpoint(region string) string {
	if region == "" {
		region = "cn-hangzhou"
	}
	return "https://apig." + region + ".aliyuncs.com"
}

// apigStatusID 阿里云 APIG 网关状态 → 内置服务状态枚举 id。
func apigStatusID(s string) string {
	switch strings.ToLower(s) {
	case "running":
		return "running"
	case "creating", "upgrading", "restarting", "scaling", "updating", "modifying":
		return "creating"
	case "deleting", "deleted", "stopped":
		return "stopped"
	case "failed", "error":
		return "abnormal"
	}
	return "abnormal"
}

// apigPaidType 阿里云 APIG chargeType(PREPAY/POSTPAY) → 内置计费方式枚举 id。
func apigPaidType(s string) string {
	if strings.EqualFold(s, "PREPAY") {
		return "PrePaid"
	}
	return "PostPaid"
}

// syncAliyunApig 同步单地域云原生API网关(APIG)实例到 API网关 模型（ROA V3 ListGateways）。
func (s *DataSourceService) syncAliyunApig(ds *model.DataSource, region string, report *SyncReport, seenApig *[]uint) error {
	apigID := s.inst.ModelIDByObjID("apigw")
	if apigID == 0 {
		return nil // 无 API网关 模型则跳过
	}
	page := 1
	const size = 50
	for {
		body, err := aliRPCV3(apigEndpoint(region), "/v1/gateways",
			map[string]string{"pageNumber": strconv.Itoa(page), "pageSize": strconv.Itoa(size)},
			ds.Username, ds.Password, "2024-03-27", "ListGateways")
		if err != nil {
			report.skipProduct("apigw", "云原生API网关["+region+"] 跳过同步："+err.Error())
			return nil
		}
		var r struct {
			Data struct {
				TotalSize int `json:"totalSize"`
				Items     []struct {
					GatewayID      string `json:"gatewayId"`
					Name           string `json:"name"`
					GatewayType    string `json:"gatewayType"`
					GatewayEdition string `json:"gatewayEdition"`
					Spec           string `json:"spec"`
					Version        string `json:"version"`
					Replicas       int    `json:"replicas"`
					ChargeType     string `json:"chargeType"`
					Status         string `json:"status"`
					SubDomainInfos []struct {
						Name        string `json:"name"`
						NetworkType string `json:"networkType"`
					} `json:"subDomainInfos"`
					LoadBalancers []struct {
						Address     string `json:"address"`
						AddressType string `json:"addressType"`
					} `json:"loadBalancers"`
				} `json:"items"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 APIG 响应失败")
		}
		if len(r.Data.Items) == 0 {
			break
		}
		for _, gw := range r.Data.Items {
			name := firstNonEmpty(gw.Name, gw.GatewayID)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"inst_name":      name,
				"cloud_provider": "aliyun",
				"status":         apigStatusID(gw.Status),
				"paid_type":      apigPaidType(gw.ChargeType),
				"region":         region,
			}
			if gw.GatewayID != "" {
				data["resource_id"] = gw.GatewayID
			}
			if gw.GatewayType != "" {
				data["gateway_type"] = gw.GatewayType
			}
			if gw.GatewayEdition != "" {
				data["edition"] = gw.GatewayEdition
			}
			if gw.Spec != "" {
				data["spec"] = gw.Spec
			}
			if gw.Version != "" {
				data["version"] = gw.Version
			}
			if gw.Replicas > 0 {
				data["replicas"] = float64(gw.Replicas)
			}
			// 接入地址：优先用网关二级域名(subDomainInfos，env-xxx.alicloudapi.com，真正的访问入口)，
			// 同类多个以 "; " 拼接；无二级域名时回退到负载均衡地址(loadBalancers)。
			var internet, intranet []string
			for _, sd := range gw.SubDomainInfos {
				if sd.Name == "" {
					continue
				}
				if strings.EqualFold(sd.NetworkType, "Internet") {
					internet = append(internet, sd.Name)
				} else if strings.EqualFold(sd.NetworkType, "Intranet") {
					intranet = append(intranet, sd.Name)
				}
			}
			if len(internet) == 0 && len(intranet) == 0 {
				for _, lb := range gw.LoadBalancers {
					if lb.Address == "" {
						continue
					}
					if strings.EqualFold(lb.AddressType, "Internet") {
						internet = append(internet, lb.Address)
					} else if strings.EqualFold(lb.AddressType, "Intranet") {
						intranet = append(intranet, lb.Address)
					}
				}
			}
			if len(internet) > 0 {
				data["internet_endpoint"] = strings.Join(internet, "; ")
			}
			if len(intranet) > 0 {
				data["intranet_endpoint"] = strings.Join(intranet, "; ")
			}
			inst, created, e := s.inst.CollectCloudInstance(apigID, "inst_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenApig = append(*seenApig, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if len(r.Data.Items) < size || page*size >= r.Data.TotalSize {
			break
		}
		page++
	}
	return nil
}

// mseEndpoint 返回阿里云微服务引擎(MSE)地域化 endpoint。
func mseEndpoint(region string) string {
	if region == "" {
		region = "cn-hangzhou"
	}
	return "https://mse." + region + ".aliyuncs.com"
}

// mseEngineID 阿里云 MSE 集群类型(ClusterType) → 引擎类型枚举 id。
func mseEngineID(clusterType string) string {
	t := strings.ToLower(clusterType)
	switch {
	case strings.Contains(t, "nacos"):
		return "nacos"
	case strings.Contains(t, "zookeeper") || strings.Contains(t, "zk"):
		return "zookeeper"
	case strings.Contains(t, "eureka"):
		return "eureka"
	}
	return ""
}

// msePort 按引擎类型返回默认客户端端口。
func msePort(engineID string) int {
	switch engineID {
	case "nacos":
		return 8848
	case "zookeeper":
		return 2181
	case "eureka":
		return 8761
	}
	return 0
}

// mseSpecName 阿里云 MSE 版本(MseVersion) → 规格中文名。
func mseSpecName(v string) string {
	switch strings.ToLower(v) {
	case "mse_pro":
		return "专业版"
	case "mse_dev":
		return "开发版"
	case "mse_serverless":
		return "Serverless版"
	case "mse_basic":
		return "基础版"
	}
	return v
}

// mseStatusID 阿里云 MSE 初始化状态(InitStatus) → 内置服务状态枚举 id。
func mseStatusID(s string) string {
	u := strings.ToUpper(s)
	switch {
	case u == "INIT_SUCCESS":
		return "running"
	case strings.Contains(u, "ING"): // INIT_ING / SCALING / RESTARTING 等进行中
		return "creating"
	case strings.Contains(u, "FAILED") || strings.Contains(u, "ERROR"):
		return "abnormal"
	case strings.Contains(u, "DESTROY") || strings.Contains(u, "RELEASE"):
		return "stopped"
	}
	return "running"
}

// msePaidType 阿里云 MSE ChargeType(PREPAY/POSTPAY) → 内置计费方式枚举 id。
func msePaidType(s string) string {
	if strings.EqualFold(s, "PREPAY") {
		return "PrePaid"
	}
	return "PostPaid"
}

// syncAliyunMse 同步单地域微服务引擎(MSE)注册配置中心集群（ListClusters）：
// Nacos 集群写入 Nacos 模型，ZooKeeper / Eureka 写入 注册配置中心(mse) 模型。
func (s *DataSourceService) syncAliyunMse(ds *model.DataSource, region string, report *SyncReport, seenMse, seenNacos *[]uint) error {
	mseID := s.inst.ModelIDByObjID("mse")
	nacosID := s.inst.ModelIDByObjID("nacos")
	if mseID == 0 && nacosID == 0 {
		return nil // 无目标模型则跳过
	}
	page := 1
	const size = 50
	for {
		body, err := aliRPC(mseEndpoint(region), ds.Username, ds.Password, "2019-05-31", "ListClusters",
			map[string]string{"RegionId": region, "PageNum": strconv.Itoa(page), "PageSize": strconv.Itoa(size)})
		if err != nil {
			report.skipProduct("mse", "注册配置中心(MSE)["+region+"] 跳过同步："+err.Error())
			return nil
		}
		var r struct {
			TotalCount int `json:"TotalCount"`
			Data       []struct {
				InstanceID       string `json:"InstanceId"`
				ClusterID        string `json:"ClusterId"`
				ClusterType      string `json:"ClusterType"`
				ClusterAliasName string `json:"ClusterAliasName"`
				ClusterName      string `json:"ClusterName"`
				AppVersion       string `json:"AppVersion"`
				MseVersion       string `json:"MseVersion"`
				InstanceCount    int    `json:"InstanceCount"`
				IntranetDomain   string `json:"IntranetDomain"`
				IntranetAddress  string `json:"IntranetAddress"`
				ChargeType       string `json:"ChargeType"`
				InitStatus       string `json:"InitStatus"`
			} `json:"Data"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 MSE 响应失败")
		}
		if len(r.Data) == 0 {
			break
		}
		for _, c := range r.Data {
			name := firstNonEmpty(c.ClusterAliasName, c.ClusterName, c.ClusterID)
			if name == "" {
				report.Skipped++
				continue
			}
			engine := mseEngineID(c.ClusterType)
			ep := firstNonEmpty(c.IntranetDomain, c.IntranetAddress)
			port := msePort(engine)

			// 目标模型：Nacos → Nacos 模型；其余（ZooKeeper/Eureka）→ 注册配置中心模型
			var targetID uint
			var seen *[]uint
			var data map[string]interface{}
			if engine == "nacos" {
				targetID, seen = nacosID, seenNacos
				data = map[string]interface{}{
					"inst_name":      name,
					"cloud_provider": "aliyun",
					"status":         mseStatusID(c.InitStatus),
					"region":         region,
					"mode":           "cluster",
				}
				if c.InstanceCount == 1 {
					data["mode"] = "standalone"
				}
			} else {
				targetID, seen = mseID, seenMse
				data = map[string]interface{}{
					"inst_name":      name,
					"cloud_provider": "aliyun",
					"status":         mseStatusID(c.InitStatus),
					"paid_type":      msePaidType(c.ChargeType),
					"region":         region,
				}
				if engine != "" {
					data["engine_type"] = engine
				}
				if spec := mseSpecName(c.MseVersion); spec != "" {
					data["spec"] = spec
				}
			}
			if targetID == 0 {
				continue // 对应模型不存在则跳过该集群
			}
			report.Discovered++
			if c.InstanceID != "" {
				data["resource_id"] = c.InstanceID
			}
			if c.AppVersion != "" {
				data["version"] = c.AppVersion
			}
			if c.InstanceCount > 0 {
				data["node_count"] = float64(c.InstanceCount)
			}
			if ep != "" {
				data["endpoint"] = ep
			}
			if port > 0 {
				data["port"] = float64(port)
			}
			inst, created, e := s.inst.CollectCloudInstance(targetID, "inst_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seen = append(*seen, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if len(r.Data) < size || page*size >= r.TotalCount {
			break
		}
		page++
	}
	return nil
}

// ackStatusID 阿里云 ACK 集群状态(state) → 内置服务状态枚举 id。
func ackStatusID(s string) string {
	switch strings.ToLower(s) {
	case "running":
		return "running"
	case "initial", "scaling", "updating", "upgrading", "removing", "migrating":
		return "creating"
	case "failed", "init_failed", "delete_failed", "update_failed":
		return "abnormal"
	case "deleting", "deleted", "stopped", "inactive":
		return "stopped"
	}
	return "running"
}

// syncAliyunK8s 同步阿里云容器服务 ACK 集群到 Kubernetes 模型（全局，CS DescribeClustersV1）。
// cs.aliyuncs.com/clusters 一次返回全部地域集群，故作全局资源同步一次。
func (s *DataSourceService) syncAliyunK8s(ds *model.DataSource, report *SyncReport, seenK8s *[]uint) error {
	k8sID := s.inst.ModelIDByObjID("kubernetes")
	if k8sID == 0 {
		return nil // 无 Kubernetes 模型则跳过
	}
	body, err := aliROA("https://cs.aliyuncs.com", "/clusters", nil, ds.Username, ds.Password, "2015-12-15")
	if err != nil {
		report.skipProduct("kubernetes", "容器服务 ACK 跳过同步："+err.Error()+"（如需同步请为 RAM 用户授予 AliyunCSReadOnlyAccess）")
		return nil
	}
	var clusters []struct {
		Name           string                 `json:"name"`
		ClusterID      string                 `json:"cluster_id"`
		CurrentVersion string                 `json:"current_version"`
		Size           int                    `json:"size"`
		RegionID       string                 `json:"region_id"`
		ZoneID         string                 `json:"zone_id"`
		State          string                 `json:"state"`
		ContainerCIDR  string                 `json:"container_cidr"`
		SubnetCIDR     string                 `json:"subnet_cidr"`
		MasterURL      string                 `json:"master_url"`
		Profile        string                 `json:"profile"`      // Default/Edge/Serverless/Lingjun…
		ClusterType    string                 `json:"cluster_type"` // ManagedKubernetes/Kubernetes…
		Parameters     map[string]interface{} `json:"parameters"`
	}
	if json.Unmarshal(body, &clusters) != nil {
		return newValidationErr("解析阿里云 ACK 响应失败")
	}
	for _, c := range clusters {
		name := firstNonEmpty(c.Name, c.ClusterID)
		if name == "" {
			report.Skipped++
			continue
		}
		report.Discovered++
		data := map[string]interface{}{
			"cluster_name":   name,
			"cloud_provider": "aliyun",
			"status":         ackStatusID(c.State),
		}
		if c.ClusterID != "" {
			data["resource_id"] = c.ClusterID
		}
		if c.Profile != "" {
			data["cluster_profile"] = c.Profile // Edge=边缘托管，Default=标准托管，Serverless=ASK…
		}
		if c.CurrentVersion != "" {
			data["version"] = c.CurrentVersion
		}
		if c.Size > 0 {
			data["node_count"] = float64(c.Size)
		}
		if cidr := firstNonEmpty(c.ContainerCIDR, c.SubnetCIDR); cidr != "" {
			data["pod_cidr"] = cidr
		}
		if c.RegionID != "" {
			data["region"] = c.RegionID
		}
		if c.ZoneID != "" {
			data["cloud_az"] = c.ZoneID
		}
		// API Server 地址（master_url 为 JSON 字符串，优先公网 endpoint）
		if c.MasterURL != "" {
			var mu struct {
				Public   string `json:"api_server_endpoint"`
				Intranet string `json:"intranet_api_server_endpoint"`
			}
			if json.Unmarshal([]byte(c.MasterURL), &mu) == nil {
				if ep := firstNonEmpty(mu.Public, mu.Intranet); ep != "" {
					data["api_server"] = ep
				}
			}
		}
		// 网络插件：参数里的 Network（flannel / terway-eniip 等）
		if c.Parameters != nil {
			if net, ok := c.Parameters["Network"].(string); ok && net != "" {
				data["cni"] = net
			}
		}
		inst, created, e := s.inst.CollectCloudInstance(k8sID, "cluster_name", data, ds.Name, ds.ID)
		if e != nil {
			report.Failed++
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, name+": "+e.Error())
			}
			continue
		}
		*seenK8s = append(*seenK8s, inst.ID)
		if created {
			report.Created++
		} else {
			report.Updated++
		}
	}
	return nil
}

// parseEsSpecInfo 解析 Elasticsearch 节点规格明细 specInfo（如 "4C 16G"）为 CPU 核数与内存 GB。
func parseEsSpecInfo(s string) (cpu, memGB int) {
	for _, f := range strings.Fields(s) {
		up := strings.ToUpper(f)
		switch {
		case strings.HasSuffix(up, "C"):
			cpu, _ = strconv.Atoi(strings.TrimRight(up, "C"))
		case strings.HasSuffix(up, "GB"):
			memGB, _ = strconv.Atoi(strings.TrimSuffix(up, "GB"))
		case strings.HasSuffix(up, "G"):
			memGB, _ = strconv.Atoi(strings.TrimRight(up, "G"))
		}
	}
	return
}

// esEndpoint 返回阿里云 Elasticsearch 地域化 endpoint。
func esEndpoint(region string) string {
	if region == "" {
		region = "cn-shanghai"
	}
	return "https://elasticsearch." + region + ".aliyuncs.com"
}

// esStatusID 阿里云 Elasticsearch 实例状态(status) → 内置服务状态枚举 id。
func esStatusID(s string) string {
	switch s {
	case "active":
		return "running"
	case "activating", "initializing":
		return "creating"
	case "inactive":
		return "stopped"
	case "invalid":
		return "abnormal"
	}
	return "abnormal"
}

// syncAliyunElastic 同步单地域阿里云 Elasticsearch 实例到 Elasticsearch 模型（ROA ListInstance）。
func (s *DataSourceService) syncAliyunElastic(ds *model.DataSource, region string, report *SyncReport, seenES *[]uint) error {
	esID := s.inst.ModelIDByObjID("elastic")
	if esID == 0 {
		return nil // 无 Elasticsearch 模型则跳过
	}
	page := 1
	const size = 100
	for {
		body, err := aliROA(esEndpoint(region), "/openapi/instances",
			map[string]string{"page": strconv.Itoa(page), "size": strconv.Itoa(size)},
			ds.Username, ds.Password, "2017-06-13")
		if err != nil {
			// 产品级接口错误（如 RAM 无 elasticsearch 权限）不中断整体同步，记录提示并跳过
			report.skipProduct("elastic", "Elasticsearch["+region+"] 跳过同步："+err.Error()+"（如需同步请为 RAM 用户授予 AliyunElasticsearchReadOnlyAccess）")
			return nil
		}
		var r struct {
			Result []struct {
				InstanceID  string `json:"instanceId"`
				Description string `json:"description"`
				EsVersion   string `json:"esVersion"`
				Status      string `json:"status"`
				NodeAmount  int    `json:"nodeAmount"`
				NodeSpec    struct {
					Spec     string `json:"spec"`
					Disk     int    `json:"disk"`
					SpecInfo string `json:"specInfo"` // 如 "4C 16G"
				} `json:"nodeSpec"`
				Domain        string      `json:"domain"`
				Port          json.Number `json:"port"`
				NetworkConfig struct {
					VsArea string `json:"vsArea"` // vswitch 所在可用区
				} `json:"networkConfig"`
			} `json:"Result"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云 Elasticsearch 响应失败")
		}
		for _, es := range r.Result {
			name := firstNonEmpty(es.Description, es.InstanceID)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"inst_name":      name,
				"cloud_provider": "aliyun",
				"status":         esStatusID(es.Status),
				"region":         region,
			}
			if es.InstanceID != "" {
				data["resource_id"] = es.InstanceID
			}
			if es.EsVersion != "" {
				data["version"] = es.EsVersion
			}
			if es.NodeSpec.Spec != "" {
				data["spec"] = es.NodeSpec.Spec
			}
			if es.NodeAmount > 0 {
				data["node_count"] = float64(es.NodeAmount)
			}
			if es.NodeSpec.Disk > 0 {
				data["disk_gb"] = float64(es.NodeSpec.Disk)
			}
			if cpu, mem := parseEsSpecInfo(es.NodeSpec.SpecInfo); cpu > 0 || mem > 0 {
				if cpu > 0 {
					data["cpu_cores"] = float64(cpu)
				}
				if mem > 0 {
					data["mem_gb"] = float64(mem)
				}
			}
			if es.Domain != "" {
				data["endpoint"] = es.Domain
			}
			if n, e := es.Port.Int64(); e == nil && n > 0 {
				data["port"] = float64(n)
			}
			if es.NetworkConfig.VsArea != "" {
				data["cloud_az"] = es.NetworkConfig.VsArea
			}
			inst, created, e := s.inst.CollectCloudInstance(esID, "inst_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenES = append(*seenES, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if len(r.Result) < size {
			break
		}
		page++
	}
	return nil
}

// domainStatusID 按到期日期推断域名状态：已过期 / 30 天内即将过期 / 正常。
func domainStatusID(expDate string) string {
	if len(expDate) < 10 {
		return "normal"
	}
	t, err := time.Parse("2006-01-02", expDate[:10])
	if err != nil {
		return "normal"
	}
	d := time.Until(t)
	switch {
	case d < 0:
		return "expired"
	case d < 30*24*time.Hour:
		return "expiring"
	default:
		return "normal"
	}
}

// aliDomainDetail 查单个域名详情，返回持有者(优先中文 Zh* 字段)与联系人邮箱。
func (s *DataSourceService) aliDomainDetail(ds *model.DataSource, domainName string) (holder, email string) {
	body, err := aliRPC("https://domain.aliyuncs.com", ds.Username, ds.Password, "2018-01-29", "QueryDomainByDomainName",
		map[string]string{"DomainName": domainName})
	if err != nil {
		return "", ""
	}
	var r struct {
		ZhRegistrantOrganization string `json:"ZhRegistrantOrganization"`
		ZhRegistrantName         string `json:"ZhRegistrantName"`
		RegistrantOrganization   string `json:"RegistrantOrganization"`
		RegistrantName           string `json:"RegistrantName"`
		Email                    string `json:"Email"`
	}
	if json.Unmarshal(body, &r) != nil {
		return "", ""
	}
	// 持有者：优先中文（单位 > 个人），回退英文
	holder = firstNonEmpty(r.ZhRegistrantOrganization, r.ZhRegistrantName, r.RegistrantOrganization, r.RegistrantName)
	email = strings.TrimSpace(r.Email)
	// 邮箱可能被隐私保护脱敏（含 *），非合法邮箱不写入（避免格式校验失败）
	if email == "" || strings.Contains(email, "*") || !emailRe.MatchString(email) {
		email = ""
	}
	return holder, email
}

// syncAliyunDomain 同步阿里云已注册域名（域名注册 API，全局资源）到 域名 模型。
func (s *DataSourceService) syncAliyunDomain(ds *model.DataSource, report *SyncReport, seenDomain *[]uint) error {
	domainID := s.inst.ModelIDByObjID("domain")
	if domainID == 0 {
		return nil // 无 域名 模型则跳过
	}
	page := 1
	for {
		body, err := aliRPC("https://domain.aliyuncs.com", ds.Username, ds.Password, "2018-01-29", "QueryDomainList",
			map[string]string{"PageSize": "100", "PageNum": strconv.Itoa(page)})
		if err != nil {
			report.skipProduct("domain", "域名 同步跳过："+err.Error())
			return nil
		}
		var r struct {
			TotalItemNum   int `json:"TotalItemNum"`
			CurrentPageNum int `json:"CurrentPageNum"`
			PageSize       int `json:"PageSize"`
			Data           struct {
				Domain []struct {
					DomainName       string `json:"DomainName"`
					ExpirationDate   string `json:"ExpirationDate"`
					RegistrationDate string `json:"RegistrationDate"`
				} `json:"Domain"`
			} `json:"Data"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云域名响应失败")
		}
		for _, d := range r.Data.Domain {
			name := strings.TrimSpace(d.DomainName)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"domain_name": name,
				"registrar":   "阿里云",
				"status":      domainStatusID(d.ExpirationDate),
			}
			if len(d.ExpirationDate) >= 10 {
				data["expire_date"] = d.ExpirationDate[:10] // YYYY-MM-DD
			}
			if len(d.RegistrationDate) >= 10 {
				data["registration_date"] = d.RegistrationDate[:10] // YYYY-MM-DD
			}
			// 域名持有者(中文) + 联系人邮箱（逐域名查详情 QueryDomainByDomainName）
			holder, email := s.aliDomainDetail(ds, name)
			if holder != "" {
				data["dns_provider"] = holder
			}
			if email != "" {
				data["contact_email"] = email
			}
			inst, created, e := s.inst.UpsertInstance(domainID, "domain_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenDomain = append(*seenDomain, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalItemNum || len(r.Data.Domain) == 0 {
			break
		}
		page++
	}
	return nil
}

// aliDnsRecord DNS解析模型 records 字段里每条解析记录的结构。
type aliDnsRecord struct {
	RR        string `json:"rr"`
	FQDN      string `json:"fqdn"` // 完整记录名（rr + 域名），供搜索与展示
	Type      string `json:"type"`
	Value     string `json:"value"`
	TTL       int64  `json:"ttl"`
	Line      string `json:"line"`
	Status    string `json:"status"` // enable/disable
	Remark    string `json:"remark"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// aliMsToTime 毫秒时间戳 → "2006-01-02 15:04:05"，0/空返回空串。
func aliMsToTime(n json.Number) string {
	ms, err := n.Int64()
	if err != nil || ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
}

// aliAlidnsDomainInfo 接入云解析的域名及其套餐版本。
type aliAlidnsDomainInfo struct {
	Name    string
	Version string
}

// syncAliyunDnsRecords 同步阿里云云解析(Alidns)到 DNS解析 模型（域名级）：
// 先 DescribeDomains 取接入云解析的域名(含套餐版本)，再逐域名 DescribeDomainRecords 拉全部记录，
// 聚合为一条实例（域名/记录数/套餐版本/records 明细JSON），按域名 upsert。
func (s *DataSourceService) syncAliyunDnsRecords(ds *model.DataSource, report *SyncReport, seenDns *[]uint) error {
	dnsID := s.inst.ModelIDByObjID("dns_record")
	if dnsID == 0 {
		return nil // 无 DNS解析 模型则跳过
	}
	domains, err := s.aliAlidnsDomains(ds)
	if err != nil {
		report.skipProduct("dns_record", "DNS解析 同步跳过："+err.Error())
		return nil
	}
	for _, dn := range domains {
		records := []aliDnsRecord{}
		page := 1
		for {
			body, e := aliRPC("https://alidns.aliyuncs.com", ds.Username, ds.Password, "2015-01-09", "DescribeDomainRecords",
				map[string]string{"DomainName": dn.Name, "PageSize": "100", "PageNumber": strconv.Itoa(page)})
			if e != nil {
				report.skipProduct("dns_record", "DNS解析 同步跳过("+dn.Name+")："+e.Error())
				return nil
			}
			var r struct {
				TotalCount    int `json:"TotalCount"`
				PageSize      int `json:"PageSize"`
				DomainRecords struct {
					Record []struct {
						RR              string      `json:"RR"`
						Type            string      `json:"Type"`
						Value           string      `json:"Value"`
						TTL             json.Number `json:"TTL"`
						Line            string      `json:"Line"`
						Status          string      `json:"Status"`
						Remark          string      `json:"Remark"`
						CreateTimestamp json.Number `json:"CreateTimestamp"`
						UpdateTimestamp json.Number `json:"UpdateTimestamp"`
					} `json:"Record"`
				} `json:"DomainRecords"`
			}
			if json.Unmarshal(body, &r) != nil {
				return newValidationErr("解析阿里云 DNS 记录响应失败")
			}
			for _, rec := range r.DomainRecords.Record {
				ttl, _ := rec.TTL.Int64()
				status := "enable"
				if rec.Status == "DISABLE" {
					status = "disable"
				}
				fqdn := dn.Name
				if rr := strings.TrimSpace(rec.RR); rr != "" && rr != "@" {
					fqdn = rr + "." + dn.Name
				}
				records = append(records, aliDnsRecord{
					RR: rec.RR, FQDN: fqdn, Type: rec.Type, Value: rec.Value, TTL: ttl, Line: rec.Line, Status: status,
					Remark: rec.Remark, CreatedAt: aliMsToTime(rec.CreateTimestamp), UpdatedAt: aliMsToTime(rec.UpdateTimestamp),
				})
			}
			if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.DomainRecords.Record) == 0 {
				break
			}
			page++
		}
		report.Discovered++
		recJSON, _ := json.Marshal(records)
		data := map[string]interface{}{
			"domain_name":  dn.Name,
			"record_count": float64(len(records)),
			"records":      string(recJSON),
		}
		if dn.Version != "" {
			data["version"] = dn.Version
		}
		inst, created, e := s.inst.UpsertInstance(dnsID, "domain_name", data, ds.Name, ds.ID)
		if e != nil {
			report.Failed++
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, dn.Name+": "+e.Error())
			}
			continue
		}
		*seenDns = append(*seenDns, inst.ID)
		if created {
			report.Created++
		} else {
			report.Updated++
		}
	}
	return nil
}

// aliAlidnsDomains 取接入阿里云云解析(Alidns)的域名列表（含套餐版本名）。
func (s *DataSourceService) aliAlidnsDomains(ds *model.DataSource) ([]aliAlidnsDomainInfo, error) {
	out := []aliAlidnsDomainInfo{}
	page := 1
	for {
		body, err := aliRPC("https://alidns.aliyuncs.com", ds.Username, ds.Password, "2015-01-09", "DescribeDomains",
			map[string]string{"PageSize": "100", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			return nil, err
		}
		var r struct {
			TotalCount int `json:"TotalCount"`
			PageSize   int `json:"PageSize"`
			Domains    struct {
				Domain []struct {
					DomainName  string `json:"DomainName"`
					VersionName string `json:"VersionName"`
				} `json:"Domain"`
			} `json:"Domains"`
		}
		if json.Unmarshal(body, &r) != nil {
			return nil, newValidationErr("解析阿里云云解析域名响应失败")
		}
		for _, d := range r.Domains.Domain {
			if strings.TrimSpace(d.DomainName) != "" {
				out = append(out, aliAlidnsDomainInfo{Name: d.DomainName, Version: d.VersionName})
			}
		}
		if r.PageSize == 0 || page*r.PageSize >= r.TotalCount || len(r.Domains.Domain) == 0 {
			break
		}
		page++
	}
	return out, nil
}

func firstIP(lists ...[]string) string {
	for _, l := range lists {
		for _, ip := range l {
			if strings.TrimSpace(ip) != "" {
				return strings.TrimSpace(ip)
			}
		}
	}
	return ""
}

// aliOSS 以 OSS 专有签名(V1)发起 GET 请求。canonResource 为签名用规范资源
// （如 "/" 或 "/<bucket>/?stat"），query 为实际 URL query（含 stat/bucketInfo 等子资源）。
// 返回非 200 时解析错误 XML（<Error><Code><Message>）给出可读信息。
func aliOSS(ak, sk, host, canonResource string, query map[string]string) ([]byte, error) {
	date := time.Now().UTC().Format(http.TimeFormat) // RFC1123 GMT
	stringToSign := "GET\n\n\n" + date + "\n" + canonResource
	mac := hmac.New(sha1.New, []byte(sk))
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	reqURL := "https://" + host + "/"
	if len(query) > 0 {
		uq := url.Values{}
		for k, v := range query {
			uq.Set(k, v)
		}
		reqURL += "?" + uq.Encode()
	}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Host = host
	req.Header.Set("Date", date)
	req.Header.Set("Authorization", "OSS "+ak+":"+sig)
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		var e struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		}
		_ = xml.Unmarshal(body, &e)
		if e.Message != "" {
			return nil, newValidationErr("阿里云OSS接口错误：%s", e.Message)
		}
		return nil, newValidationErr("阿里云OSS接口返回 HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// ossStorageClassID 阿里云 OSS 存储类型 → 模型枚举 id。
func ossStorageClassID(s string) string {
	switch s {
	case "Standard":
		return "standard"
	case "IA":
		return "ia"
	case "Archive":
		return "archive"
	case "ColdArchive", "DeepColdArchive":
		return "cold"
	}
	return ""
}

// ossAclID 阿里云 OSS Bucket 读写权限(Grant) → 模型枚举 id。
func ossAclID(grant string) string {
	switch grant {
	case "private":
		return "private"
	case "public-read":
		return "public-read"
	case "public-read-write":
		return "public-rw"
	}
	return ""
}

// syncAliyunOss 同步阿里云对象存储OSS Bucket 到 OSS 模型（全局资源，不分地域）。
// ListBuckets 枚举全部 Bucket；逐 Bucket GetBucketStat 取已用容量、GetBucketInfo 取读写权限。
// 容错：ListBuckets 失败（多为 RAM 子账号无 OSS 权限）记入 report.Errors 并跳过，不中断整体同步。
func (s *DataSourceService) syncAliyunOss(ds *model.DataSource, report *SyncReport, seenOss *[]uint) error {
	ossID := s.inst.ModelIDByObjID("oss")
	if ossID == 0 {
		return nil // 无 OSS 模型则跳过
	}
	marker := ""
	for {
		q := map[string]string{"max-keys": "500"}
		if marker != "" {
			q["marker"] = marker
		}
		// ListBuckets 为全局接口，用任一地域 endpoint 即可返回所有 Bucket
		body, err := aliOSS(ds.Username, ds.Password, "oss-cn-hangzhou.aliyuncs.com", "/", q)
		if err != nil {
			report.skipProduct("oss", "OSS 同步跳过："+err.Error()+"（需为 RAM 用户授权 AliyunOSSReadOnlyAccess）")
			return nil
		}
		var r struct {
			IsTruncated bool   `xml:"IsTruncated"`
			NextMarker  string `xml:"NextMarker"`
			Buckets     struct {
				Bucket []struct {
					Name             string `xml:"Name"`
					Location         string `xml:"Location"`
					Region           string `xml:"Region"`
					ExtranetEndpoint string `xml:"ExtranetEndpoint"`
					StorageClass     string `xml:"StorageClass"`
				} `xml:"Bucket"`
			} `xml:"Buckets"`
		}
		if xml.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云OSS响应失败")
		}
		for _, b := range r.Buckets.Bucket {
			name := strings.TrimSpace(b.Name)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"bucket":   name,
				"endpoint": b.ExtranetEndpoint,
				"region":   b.Region,
				"status":   "running",
			}
			if sc := ossStorageClassID(b.StorageClass); sc != "" {
				data["storage_class"] = sc
			}
			host := name + "." + b.Location + ".aliyuncs.com"
			// 已用容量：GetBucketStat（失败则留空，不影响建档）
			if sb, e := aliOSS(ds.Username, ds.Password, host, "/"+name+"/?stat", map[string]string{"stat": ""}); e == nil {
				var st struct {
					Storage json.Number `xml:"Storage"`
				}
				if xml.Unmarshal(sb, &st) == nil {
					if bytes, e2 := st.Storage.Int64(); e2 == nil {
						data["used_gb"] = float64((bytes + (1 << 30) - 1) / (1 << 30)) // 向上取整到 GB
					}
				}
			}
			// 读写权限：GetBucketInfo（失败则留空）
			if ib, e := aliOSS(ds.Username, ds.Password, host, "/"+name+"/?bucketInfo", map[string]string{"bucketInfo": ""}); e == nil {
				var info struct {
					Grant string `xml:"Bucket>AccessControlList>Grant"`
				}
				if xml.Unmarshal(ib, &info) == nil {
					if acl := ossAclID(info.Grant); acl != "" {
						data["acl"] = acl
					}
				}
			}
			inst, created, e := s.inst.UpsertInstance(ossID, "bucket", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenOss = append(*seenOss, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if !r.IsTruncated {
			break
		}
		if r.NextMarker != "" {
			marker = r.NextMarker
		} else if n := len(r.Buckets.Bucket); n > 0 {
			marker = r.Buckets.Bucket[n-1].Name
		} else {
			break
		}
	}
	return nil
}


// wafEndpoint 阿里云 WAF2.0(云盾) OpenAPI endpoint（中国大陆统一用 cn-hangzhou）。
func wafEndpoint() string { return "https://wafopenapi.cn-hangzhou.aliyuncs.com" }

// wafVersionName WAF 套餐版本代码 → 中文。
func wafVersionName(v string) string {
	switch v {
	case "version_3":
		return "高级版"
	case "version_4":
		return "企业版"
	case "version_5":
		return "旗舰版"
	case "version_exclusive":
		return "独享版"
	case "version_pro":
		return "高级版"
	case "version_enterprise":
		return "企业版"
	}
	return v
}

// wafModeID WAF 防护模式 Mode(0=防护/拦截,1=预警/告警) → 模型枚举 id。
// wafEditionName WAF3.0 套餐版本代码(Edition) → 中文。
func wafEditionName(e string) string {
	switch strings.ToLower(e) {
	case "default", "basic":
		return "基础版"
	case "pro":
		return "高级版"
	case "enterprise":
		return "企业版"
	case "ultimate":
		return "旗舰版"
	case "exclusive":
		return "独享版"
	}
	return e
}

func wafModeID(mode int) string {
	switch mode {
	case 0:
		return "block"
	case 1:
		return "alert"
	}
	return ""
}

// syncAliyunWaf 同步阿里云 WAF 防护域名到 WAF 模型（全局资源，一个防护域名一条实例）。
// 自适应兼容 WAF2.0 与 WAF3.0：两版本共用同一 endpoint(wafopenapi.cn-hangzhou)，仅 API 版本不同。
// 先用 WAF3.0(2021-10-01) DescribeInstance 探测 3.0 实例，有则走 3.0 路径；否则回退 WAF2.0(2019-09-10)。
// 容错：无 WAF 实例或无权限（需 AliyunYundunWAFReadOnlyAccess）记入 report.Errors 并跳过，不中断整体同步。
func (s *DataSourceService) syncAliyunWaf(ds *model.DataSource, report *SyncReport, seenWaf *[]uint) error {
	wafID := s.inst.ModelIDByObjID("waf")
	if wafID == 0 {
		return nil // 无 WAF 模型则跳过
	}
	ep := wafEndpoint()
	// 1) 优先探测 WAF3.0 实例
	if b3, err3 := aliRPC(ep, ds.Username, ds.Password, "2021-10-01", "DescribeInstance",
		map[string]string{"RegionId": "cn-hangzhou"}); err3 == nil {
		var r3 struct {
			InstanceId string `json:"InstanceId"`
			Status     int    `json:"Status"`
			Edition    string `json:"Edition"`
		}
		if json.Unmarshal(b3, &r3) == nil && r3.InstanceId != "" {
			return s.syncWaf3(ds, wafID, r3.InstanceId, r3.Status, r3.Edition, report, seenWaf)
		}
	}
	// 2) 回退 WAF2.0
	ib, err := aliRPC(ep, ds.Username, ds.Password, "2019-09-10", "DescribeInstanceInfo", map[string]string{"Region": "cn"})
	if err != nil {
		report.skipProduct("waf", "WAF 同步跳过："+err.Error()+"（如未开通 WAF 或 RAM 用户无 AliyunYundunWAFReadOnlyAccess 权限）")
		return nil
	}
	var info struct {
		InstanceInfo struct {
			InstanceId string `json:"InstanceId"`
			Status     int    `json:"Status"`
			Version    string `json:"Version"`
			Region     string `json:"Region"`
		} `json:"InstanceInfo"`
	}
	if json.Unmarshal(ib, &info) != nil || info.InstanceInfo.InstanceId == "" {
		report.skipProduct("waf", "WAF 同步跳过：未查询到 WAF 实例")
		return nil
	}
	return s.syncWaf2(ds, wafID, info.InstanceInfo.InstanceId, info.InstanceInfo.Version,
		info.InstanceInfo.Region, info.InstanceInfo.Status, report, seenWaf)
}

// upsertWafDomain 把一个 WAF 防护域名条目写入 WAF 模型（含统计与 seen 跟踪）。
func (s *DataSourceService) upsertWafDomain(wafID uint, domain string, data map[string]interface{},
	ds *model.DataSource, report *SyncReport, seenWaf *[]uint) {
	inst, created, e := s.inst.UpsertInstance(wafID, "inst_name", data, ds.Name, ds.ID)
	if e != nil {
		report.Failed++
		if len(report.Errors) < 100 {
			report.Errors = append(report.Errors, domain+": "+e.Error())
		}
		return
	}
	*seenWaf = append(*seenWaf, inst.ID)
	if created {
		report.Created++
	} else {
		report.Updated++
	}
}

// syncWaf2 WAF2.0(2019-09-10) 路径：DescribeDomainNames 取域名，逐域名 DescribeDomain 取 CNAME/回源、
// DescribeProtectionModuleMode 取防护模式。
func (s *DataSourceService) syncWaf2(ds *model.DataSource, wafID uint, iid, version, region string,
	status int, report *SyncReport, seenWaf *[]uint) error {
	ep := wafEndpoint()
	const ver = "2019-09-10"
	specName := wafVersionName(version)
	regionCN := "中国大陆"
	if region != "" && region != "cn" {
		regionCN = region
	}
	instStatus := "running"
	if status != 1 {
		instStatus = "stopped"
	}
	db, err := aliRPC(ep, ds.Username, ds.Password, ver, "DescribeDomainNames", map[string]string{"InstanceId": iid, "Region": "cn"})
	if err != nil {
		report.skipProduct("waf", "WAF 防护域名列表查询失败："+err.Error())
		return nil
	}
	var dn struct {
		DomainNames []string `json:"DomainNames"`
	}
	if json.Unmarshal(db, &dn) != nil {
		return newValidationErr("解析阿里云WAF域名列表失败")
	}
	for _, domain := range dn.DomainNames {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			report.Skipped++
			continue
		}
		report.Discovered++
		data := map[string]interface{}{
			"inst_name":   domain,
			"region":      regionCN,
			"status":      instStatus,
			"resource_id": iid,
		}
		if specName != "" {
			data["spec"] = specName
		}
		// 接入 CNAME 与回源地址（失败则留空）
		if dd, e := aliRPC(ep, ds.Username, ds.Password, ver, "DescribeDomain",
			map[string]string{"InstanceId": iid, "Region": "cn", "Domain": domain}); e == nil {
			var dr struct {
				Domain struct {
					Cname     string   `json:"Cname"`
					SourceIps []string `json:"SourceIps"`
				} `json:"Domain"`
			}
			if json.Unmarshal(dd, &dr) == nil {
				if dr.Domain.Cname != "" {
					data["cname"] = dr.Domain.Cname
				}
				if len(dr.Domain.SourceIps) > 0 {
					data["source_addr"] = strings.Join(dr.Domain.SourceIps, ",")
				}
			}
		}
		// 防护模式（失败则留空）
		if mb, e := aliRPC(ep, ds.Username, ds.Password, ver, "DescribeProtectionModuleMode",
			map[string]string{"InstanceId": iid, "Region": "cn", "Domain": domain, "DefenseType": "waf"}); e == nil {
			var mm struct {
				Mode int `json:"Mode"`
			}
			if json.Unmarshal(mb, &mm) == nil {
				if m := wafModeID(mm.Mode); m != "" {
					data["mode"] = m
				}
			}
		}
		s.upsertWafDomain(wafID, domain, data, ds, report, seenWaf)
	}
	return nil
}

// syncWaf3 WAF3.0(2021-10-01) 路径：DescribeDomains 分页取域名(对象或字符串均兼容)，
// 逐域名 DescribeDomainDetail 取接入 CNAME 与回源地址(Redirect.Backends)。规格取实例 Edition。
// 3.0 防护模式按「规则防护引擎」防护场景/模板配置（非域名级单一值），接口不返回 per-domain Mode，故 mode 留空。
func (s *DataSourceService) syncWaf3(ds *model.DataSource, wafID uint, iid string, status int, edition string,
	report *SyncReport, seenWaf *[]uint) error {
	ep := wafEndpoint()
	const ver = "2021-10-01"
	const region = "cn-hangzhou"
	instStatus := "running"
	if status != 0 && status != 1 {
		instStatus = "stopped"
	}
	specName := wafEditionName(edition)
	const pageSize = 50 // WAF3.0 DescribeDomains 的 PageSize 上限为 50（传 100 会报 InvalidPageSize）
	page := 1
	for {
		db, err := aliRPC(ep, ds.Username, ds.Password, ver, "DescribeDomains",
			map[string]string{"InstanceId": iid, "RegionId": region, "PageNumber": strconv.Itoa(page), "PageSize": strconv.Itoa(pageSize)})
		if err != nil {
			report.skipProduct("waf", "WAF3.0 防护域名列表查询失败："+err.Error())
			return nil
		}
		var dr struct {
			Domains    []json.RawMessage `json:"Domains"`
			TotalCount int               `json:"TotalCount"`
		}
		if json.Unmarshal(db, &dr) != nil {
			return newValidationErr("解析阿里云WAF3.0域名列表失败")
		}
		for _, raw := range dr.Domains {
			// Domains 元素可能是对象 {Domain:...} 或直接字符串，两者兼容
			domain := ""
			var obj struct {
				Domain string `json:"Domain"`
			}
			if json.Unmarshal(raw, &obj) == nil && obj.Domain != "" {
				domain = obj.Domain
			} else {
				var str string
				if json.Unmarshal(raw, &str) == nil {
					domain = str
				}
			}
			domain = strings.TrimSpace(domain)
			if domain == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"inst_name":   domain,
				"region":      "中国大陆",
				"status":      instStatus,
				"resource_id": iid,
			}
			if specName != "" {
				data["spec"] = specName
			}
			// 接入 CNAME 与回源地址（失败则留空）
			if dd, e := aliRPC(ep, ds.Username, ds.Password, ver, "DescribeDomainDetail",
				map[string]string{"InstanceId": iid, "RegionId": region, "Domain": domain}); e == nil {
				var det struct {
					Cname    string `json:"Cname"`
					Redirect struct {
						Backends []json.RawMessage `json:"Backends"`
					} `json:"Redirect"`
				}
				if json.Unmarshal(dd, &det) == nil {
					if det.Cname != "" {
						data["cname"] = det.Cname
					}
					backs := []string{}
					for _, rb := range det.Redirect.Backends {
						// Backends 元素可能是 {Backend:"..."} 或直接字符串
						var bo struct {
							Backend string `json:"Backend"`
						}
						if json.Unmarshal(rb, &bo) == nil && bo.Backend != "" {
							backs = append(backs, bo.Backend)
						} else {
							var bs string
							if json.Unmarshal(rb, &bs) == nil && bs != "" {
								backs = append(backs, bs)
							}
						}
					}
					if len(backs) > 0 {
						data["source_addr"] = strings.Join(backs, ",")
					}
				}
			}
			s.upsertWafDomain(wafID, domain, data, ds, report, seenWaf)
		}
		if len(dr.Domains) == 0 || page*pageSize >= dr.TotalCount {
			break
		}
		page++
	}
	return nil
}

// mcEndpoint 返回 MaxCompute 地域化 endpoint。
func mcEndpoint(region string) string {
	if region == "" {
		region = "cn-shanghai"
	}
	return "https://maxcompute." + region + ".aliyuncs.com"
}

// mcStatusID MaxCompute 项目状态 → 内置服务状态枚举 id。
func mcStatusID(s string) string {
	switch s {
	case "AVAILABLE":
		return "running"
	case "FROZEN", "READONLY":
		return "stopped"
	case "DELETING", "CREATING":
		return "creating"
	}
	return "abnormal"
}

// syncAliyunMaxCompute 同步单地域 MaxCompute 项目到 MaxCompute 模型。
// 用 RESTful(ROA) API GET /api/v1/projects(2022-01-04, 老版 acs HMAC-SHA1 签名，复用 aliROA)，
// marker 分页。按 project_name upsert。容错：无权限（需 AliyunMaxComputeFullAccess/只读）记入 report.Errors 跳过。
func (s *DataSourceService) syncAliyunMaxCompute(ds *model.DataSource, region string, report *SyncReport, seenMc *[]uint) error {
	mcID := s.inst.ModelIDByObjID("maxcompute")
	if mcID == 0 {
		return nil // 无 MaxCompute 模型则跳过
	}
	marker := ""
	for {
		query := map[string]string{"pageSize": "100"}
		if marker != "" {
			query["marker"] = marker
		}
		body, err := aliROA(mcEndpoint(region), "/api/v1/projects", query, ds.Username, ds.Password, "2022-01-04")
		if err != nil {
			report.skipProduct("maxcompute", "MaxCompute 同步跳过："+err.Error()+"（如未开通 MaxCompute 或 RAM 用户无相应权限）")
			return nil
		}
		var r struct {
			Data struct {
				Marker   string `json:"marker"`
				Projects []struct {
					Name         string      `json:"name"`
					Owner        string      `json:"owner"`
					Type         string      `json:"type"`
					DefaultQuota string      `json:"defaultQuota"`
					RegionId     string      `json:"regionId"`
					CostStorage  json.Number `json:"costStorage"`
					CreatedTime  json.Number `json:"createdTime"`
					Status       string      `json:"status"`
				} `json:"projects"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云MaxCompute响应失败")
		}
		for _, p := range r.Data.Projects {
			name := strings.TrimSpace(p.Name)
			if name == "" {
				report.Skipped++
				continue
			}
			// 跨地域去重：仅处理本地域项目（防止 endpoint 返回其它地域项目重复入库）
			if p.RegionId != "" && region != "" && p.RegionId != region {
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"project_name":   name,
				"owner":          p.Owner,
				"proj_type":      p.Type,
				"quota":          p.DefaultQuota,
				"region":         firstNonEmpty(p.RegionId, region),
				"status":         mcStatusID(p.Status),
				"cloud_provider": "aliyun",
			}
			if bytes, e := p.CostStorage.Int64(); e == nil && bytes > 0 {
				data["storage_gb"] = float64((bytes + (1 << 30) - 1) / (1 << 30)) // 向上取整到 GB
			}
			if ct := aliMsToTime(p.CreatedTime); ct != "" {
				data["created_time"] = ct
			}
			inst, created, e := s.inst.UpsertInstance(mcID, "project_name", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, name+": "+e.Error())
				}
				continue
			}
			*seenMc = append(*seenMc, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.Data.Marker == "" || len(r.Data.Projects) == 0 {
			break
		}
		marker = r.Data.Marker
	}
	return nil
}

// aliROAPost 老版 acs HMAC-SHA1 ROA 签名的 POST 请求（JSON body），用于 Hologres 等 RESTful 产品的
// POST 列表接口。签名串 = POST\nAccept\n\nContentType\nDate\nCanonHeaders+path。返回响应体。
func aliROAPost(endpoint, path, body, ak, sk, version string) ([]byte, error) {
	date := time.Now().UTC().Format(http.TimeFormat) // RFC1123 GMT
	const accept = "application/json"
	const contentType = "application/json"
	headers := map[string]string{
		"x-acs-version":           version,
		"x-acs-signature-nonce":   aliNonce(),
		"x-acs-signature-method":  "HMAC-SHA1",
		"x-acs-signature-version": "1.0",
	}
	hkeys := make([]string, 0, len(headers))
	for k := range headers {
		hkeys = append(hkeys, k)
	}
	sort.Strings(hkeys)
	canonHeaders := ""
	for _, k := range hkeys {
		canonHeaders += k + ":" + headers[k] + "\n"
	}
	stringToSign := "POST\n" + accept + "\n\n" + contentType + "\n" + date + "\n" + canonHeaders + path
	mac := hmac.New(sha1.New, []byte(sk))
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequest("POST", endpoint+path, bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Date", date)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", "acs "+ak+":"+sig)
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		var e struct{ Code, Message string }
		_ = json.Unmarshal(respBody, &e)
		if e.Message != "" {
			return nil, newValidationErr("阿里云接口错误：%s", e.Message)
		}
		return nil, newValidationErr("阿里云接口返回 HTTP %d", resp.StatusCode)
	}
	return respBody, nil
}

// holoEndpoint 返回 Hologres 地域化 endpoint。
func holoEndpoint(region string) string {
	if region == "" {
		region = "cn-shanghai"
	}
	return "https://hologram." + region + ".aliyuncs.com"
}

// holoStatusID Hologres 实例状态 → 内置服务状态枚举 id。
func holoStatusID(s string) string {
	switch s {
	case "Running":
		return "running"
	case "Stopped", "Suspended":
		return "stopped"
	case "Creating", "Upgrading", "Scaling", "Starting", "Stopping", "Restarting":
		return "creating"
	}
	return "abnormal"
}

// isoToDateTime 把 ISO8601(2021-10-12T10:39:55Z) 转为 "2006-01-02 15:04:05"，解析失败返回空。
func isoToDateTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format("2006-01-02 15:04:05")
	}
	return ""
}

// syncAliyunHologres 同步单地域 Hologres 实例到 Hologres 模型。
// 用 RESTful API POST /api/v1/instances(2022-06-01, 老版 acs HMAC-SHA1 签名)，响应 InstanceList。
// 容错：无权限（需 hologram 只读权限）记入 report.Errors 并跳过。
func (s *DataSourceService) syncAliyunHologres(ds *model.DataSource, region string, report *SyncReport, seenHolo *[]uint) error {
	holoID := s.inst.ModelIDByObjID("hologres")
	if holoID == 0 {
		return nil // 无 Hologres 模型则跳过
	}
	body, err := aliROAPost(holoEndpoint(region), "/api/v1/instances", "{}", ds.Username, ds.Password, "2022-06-01")
	if err != nil {
		report.skipProduct("hologres", "Hologres 同步跳过："+err.Error()+"（如未开通 Hologres 或 RAM 用户无相应权限）")
		return nil
	}
	var r struct {
		InstanceList []struct {
			InstanceId         string `json:"InstanceId"`
			InstanceName       string `json:"InstanceName"`
			InstanceType       string `json:"InstanceType"`
			InstanceStatus     string `json:"InstanceStatus"`
			InstanceChargeType string `json:"InstanceChargeType"`
			Version            string `json:"Version"`
			RegionId           string `json:"RegionId"`
			ZoneId             string `json:"ZoneId"`
			CreationTime       string `json:"CreationTime"`
			Endpoints          []struct {
				Type     string `json:"Type"`
				Endpoint string `json:"Endpoint"`
			} `json:"Endpoints"`
		} `json:"InstanceList"`
	}
	if json.Unmarshal(body, &r) != nil {
		return newValidationErr("解析阿里云Hologres响应失败")
	}
	for _, li := range r.InstanceList {
		id := strings.TrimSpace(li.InstanceId)
		if id == "" {
			report.Skipped++
			continue
		}
		// 跨地域去重：仅处理本地域实例
		if li.RegionId != "" && region != "" && li.RegionId != region {
			continue
		}
		report.Discovered++
		// 列表接口不含计算资源，逐实例 GetInstance 取 CPU/内存/节点数（GET，复用 aliROA）
		it := li
		var cpu, mem, nodes int
		if db, e := aliROA(holoEndpoint(region), "/api/v1/instances/"+id, nil, ds.Username, ds.Password, "2022-06-01"); e == nil {
			var d2 struct {
				Instance struct {
					InstanceName       string `json:"InstanceName"`
					InstanceType       string `json:"InstanceType"`
					InstanceStatus     string `json:"InstanceStatus"`
					InstanceChargeType string `json:"InstanceChargeType"`
					Version            string `json:"Version"`
					ZoneId             string `json:"ZoneId"`
					CreationTime       string `json:"CreationTime"`
					Cpu                int    `json:"Cpu"`
					Memory             int    `json:"Memory"`
					ComputeNodeCount   int    `json:"ComputeNodeCount"`
					Endpoints          []struct {
						Type     string `json:"Type"`
						Endpoint string `json:"Endpoint"`
					} `json:"Endpoints"`
				} `json:"Instance"`
			}
			if json.Unmarshal(db, &d2) == nil {
				di := d2.Instance
				if di.InstanceName != "" {
					it.InstanceName = di.InstanceName
				}
				if di.InstanceType != "" {
					it.InstanceType = di.InstanceType
				}
				if di.InstanceStatus != "" {
					it.InstanceStatus = di.InstanceStatus
				}
				if di.InstanceChargeType != "" {
					it.InstanceChargeType = di.InstanceChargeType
				}
				if di.Version != "" {
					it.Version = di.Version
				}
				if di.ZoneId != "" {
					it.ZoneId = di.ZoneId
				}
				if di.CreationTime != "" {
					it.CreationTime = di.CreationTime
				}
				if len(di.Endpoints) > 0 {
					it.Endpoints = di.Endpoints
				}
				cpu, mem, nodes = di.Cpu, di.Memory, di.ComputeNodeCount
			}
		}
		endpoints := map[string]string{}
		for _, ep := range it.Endpoints {
			if ep.Endpoint != "" {
				endpoints[ep.Type] = ep.Endpoint
			}
		}
		data := map[string]interface{}{
			"instance_id":      id,
			"inst_name":        it.InstanceName,
			"holo_type":        it.InstanceType,
			"version":          it.Version,
			"endpoint_vpc":     firstNonEmpty(endpoints["VPCAnyTunnel"], endpoints["VPCSingleTunnel"]),
			"endpoint_public":  endpoints["Internet"],
			"endpoint_classic": endpoints["Intranet"],
			"region":           firstNonEmpty(it.RegionId, region),
			"status":           holoStatusID(it.InstanceStatus),
			"cloud_provider":   "aliyun",
		}
		if cpu > 0 {
			data["cpu_cores"] = float64(cpu)
		}
		if mem > 0 {
			data["mem_gb"] = float64(mem)
		}
		if nodes > 0 {
			data["compute_nodes"] = float64(nodes)
		}
		if it.InstanceChargeType == "PrePaid" || it.InstanceChargeType == "PostPaid" {
			data["billing"] = it.InstanceChargeType
		}
		if ct := isoToDateTime(it.CreationTime); ct != "" {
			data["created_time"] = ct
		}
		if it.ZoneId != "" {
			data["cloud_az"] = it.ZoneId
		}
		inst, created, e := s.inst.UpsertInstance(holoID, "instance_id", data, ds.Name, ds.ID)
		if e != nil {
			report.Failed++
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, id+": "+e.Error())
			}
			continue
		}
		*seenHolo = append(*seenHolo, inst.ID)
		if created {
			report.Created++
		} else {
			report.Updated++
		}
	}
	return nil
}

// dwEndpoint 返回 DataWorks 地域化 endpoint。
func dwEndpoint(region string) string {
	if region == "" {
		region = "cn-shanghai"
	}
	return "https://dataworks." + region + ".aliyuncs.com"
}

// dwStatusID DataWorks 工作空间状态码 → 内置服务状态枚举 id。
func dwStatusID(s string) string {
	switch s {
	case "AVAILABLE", "NORMAL":
		return "running"
	case "FROZEN", "FORBIDDEN", "DISABLE":
		return "stopped"
	case "INITIALIZING", "INIT":
		return "creating"
	}
	return "abnormal"
}

// syncAliyunDataWorks 同步单地域 DataWorks 工作空间到 DataWorks 模型。
// ListProjects(2020-05-18) 为 RPC-over-POST，返回 PageResult.ProjectList。按 project_id upsert。
// 该接口仅返回调用者所属租户/有权限的工作空间；容错：出错（如地域未开通/无权限）记入 report.Errors 并跳过。
func (s *DataSourceService) syncAliyunDataWorks(ds *model.DataSource, region string, report *SyncReport, seenDw *[]uint) error {
	dwID := s.inst.ModelIDByObjID("dataworks")
	if dwID == 0 {
		return nil // 无 DataWorks 模型则跳过
	}
	page := 1
	for {
		body, err := aliRPCPost(dwEndpoint(region), ds.Username, ds.Password, "2020-05-18", "ListProjects",
			map[string]string{"RegionId": region, "PageSize": "100", "PageNumber": strconv.Itoa(page)})
		if err != nil {
			report.skipProduct("dataworks", "DataWorks 同步跳过("+region+")："+err.Error()+"（如该地域未开通或 RAM 用户无权限/未加入工作空间）")
			return nil
		}
		var r struct {
			PageResult struct {
				TotalCount  int `json:"TotalCount"`
				PageSize    int `json:"PageSize"`
				ProjectList []struct {
					ProjectId         json.Number `json:"ProjectId"`
					ProjectName       string      `json:"ProjectName"`
					ProjectIdentifier string      `json:"ProjectIdentifier"`
					ProjectDescription string     `json:"ProjectDescription"`
					ProjectOwnerBaseId string     `json:"ProjectOwnerBaseId"`
					ProjectStatusCode string      `json:"ProjectStatusCode"`
					IsDefault         int         `json:"IsDefault"`
				} `json:"ProjectList"`
			} `json:"PageResult"`
		}
		if json.Unmarshal(body, &r) != nil {
			return newValidationErr("解析阿里云DataWorks响应失败")
		}
		for _, p := range r.PageResult.ProjectList {
			id := strings.TrimSpace(p.ProjectId.String())
			if id == "" || id == "0" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"project_id":     id,
				"project_name":   p.ProjectName,
				"identifier":     p.ProjectIdentifier,
				"description":    p.ProjectDescription,
				"owner":          p.ProjectOwnerBaseId,
				"region":         region,
				"status":         dwStatusID(p.ProjectStatusCode),
				"cloud_provider": "aliyun",
			}
			if p.IsDefault == 1 {
				data["is_default"] = "yes"
			} else {
				data["is_default"] = "no"
			}
			inst, created, e := s.inst.UpsertInstance(dwID, "project_id", data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, id+": "+e.Error())
				}
				continue
			}
			*seenDw = append(*seenDw, inst.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		if r.PageResult.PageSize == 0 || page*r.PageResult.PageSize >= r.PageResult.TotalCount || len(r.PageResult.ProjectList) == 0 {
			break
		}
		page++
	}
	return nil
}
