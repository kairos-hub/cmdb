package service

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// 腾讯云 OpenAPI（TC3-HMAC-SHA256 签名，零 SDK 自实现）。
// 与阿里云 RPC V1(HMAC-SHA1) 完全不同：POST + JSON body，region 走 X-TC-Region 头，
// endpoint 为 <service>.tencentcloudapi.com（不分地域，地域在头里）。

// tcAPI 调用腾讯云 v3 接口，返回响应体 JSON（Response 字段原样）。
//
//	service  产品名，如 cvm
//	action   接口名，如 DescribeInstances
//	version  接口版本，如 2017-03-12
//	region   地域，如 ap-shanghai
//	payload  请求体（map，会 JSON 序列化）
func tcAPI(service, action, version, region, secretID, secretKey string, payload map[string]interface{}) ([]byte, error) {
	host := service + ".tencentcloudapi.com"
	body, _ := json.Marshal(payload)
	now := time.Now().UTC()
	ts := strconv.FormatInt(now.Unix(), 10)
	date := now.Format("2006-01-02")

	// 1) 规范请求串
	canonicalHeaders := "content-type:application/json; charset=utf-8\n" +
		"host:" + host + "\n" +
		"x-tc-action:" + strings.ToLower(action) + "\n"
	signedHeaders := "content-type;host;x-tc-action"
	hashedPayload := sha256Hex(body)
	canonicalRequest := strings.Join([]string{
		http.MethodPost, "/", "", canonicalHeaders, signedHeaders, hashedPayload,
	}, "\n")

	// 2) 待签字符串
	credentialScope := date + "/" + service + "/tc3_request"
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256", ts, credentialScope, sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	// 3) 计算签名
	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))

	// 4) Authorization 头
	authorization := fmt.Sprintf(
		"TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		secretID, credentialScope, signedHeaders, signature)

	req, err := http.NewRequest(http.MethodPost, "https://"+host, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Timestamp", ts)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Region", region)

	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	// 业务错误：{"Response":{"Error":{"Code":"...","Message":"..."}}}
	var probe struct {
		Response struct {
			Error *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	if json.Unmarshal(respBody, &probe) == nil && probe.Response.Error != nil {
		return nil, fmt.Errorf("%s: %s", probe.Response.Error.Code, probe.Response.Error.Message)
	}
	return respBody, nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

// parseTencentRegions 解析地域（逗号分隔），默认 ap-shanghai。
func parseTencentRegions(s string) []string {
	out := []string{}
	for _, r := range strings.Split(s, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		out = append(out, "ap-shanghai")
	}
	return out
}

// tcCvmInstance CVM DescribeInstances 返回的实例（仅取需要字段）。
type tcCvmInstance struct {
	InstanceID         string   `json:"InstanceId"`
	InstanceName       string   `json:"InstanceName"`
	InstanceState      string   `json:"InstanceState"`
	CPU                int      `json:"CPU"`
	Memory             int      `json:"Memory"` // 单位 GB
	OsName             string   `json:"OsName"`
	PrivateIPAddresses []string `json:"PrivateIpAddresses"`
	Placement          struct {
		Zone string `json:"Zone"`
	} `json:"Placement"`
	SystemDisk struct {
		DiskSize int `json:"DiskSize"`
	} `json:"SystemDisk"`
	DataDisks []struct {
		DiskSize int `json:"DiskSize"`
	} `json:"DataDisks"`
}

type tcCvmResp struct {
	Response struct {
		TotalCount  int             `json:"TotalCount"`
		InstanceSet []tcCvmInstance `json:"InstanceSet"`
	} `json:"Response"`
}

// tcTestConnection 用 CVM DescribeInstances(Limit=1) 验证 SecretId/SecretKey（取第一个地域）。
func (s *DataSourceService) tcTestConnection(ds *model.DataSource) (int, error) {
	if ds.Username == "" || ds.Password == "" {
		return 0, newValidationErr("缺少 SecretId / SecretKey")
	}
	region := parseTencentRegions(ds.Region)[0]
	body, err := tcAPI("cvm", "DescribeInstances", "2017-03-12", region, ds.Username, ds.Password,
		map[string]interface{}{"Limit": 1, "Offset": 0})
	if err != nil {
		return 0, err
	}
	var r tcCvmResp
	if err := json.Unmarshal(body, &r); err != nil {
		return 0, newValidationErr("响应解析失败")
	}
	return r.Response.TotalCount, nil
}

// SyncTencent 同步腾讯云资源到 CMDB（CVM → 主机；域名 → 域名模型）。
func (s *DataSourceService) SyncTencent(ds *model.DataSource) (*SyncReport, error) {
	if ds.Username == "" || ds.Password == "" {
		return nil, newValidationErr("缺少 SecretId / SecretKey")
	}
	report := &SyncReport{Errors: []string{}, skippedModels: map[string]bool{}}
	seenHost := []uint{}
	for _, region := range parseTencentRegions(ds.Region) {
		s.syncTencentCvm(ds, region, report, &seenHost)
	}
	// 域名注册为全局资源（不分地域），单独同步一次
	seenDomain := []uint{}
	s.syncTencentDomain(ds, report, &seenDomain)

	// DNS 解析（DNSPod，全局资源），单独同步一次
	seenDns := []uint{}
	s.syncTencentDns(ds, report, &seenDns)

	// 对象存储 COS（全局 List Service 返回所有桶），单独同步一次
	seenCos := []uint{}
	s.syncTencentCos(ds, report, &seenCos)

	// 失联标记：本次未出现的（该数据源同步的）实例置「已下线」。
	// 有实例级失败时整体跳过；某产品本次因错误被整体跳过时也不标记其下线。
	if report.Failed == 0 {
		markOffline := func(objID string, mid uint, seen []uint) {
			if report.skippedModels[objID] || mid == 0 {
				return
			}
			_, _ = s.inst.MarkOffline(mid, ds.ID, seen)
		}
		markOffline("host", s.inst.HostModelID(), seenHost)
		markOffline("domain", s.inst.ModelIDByObjID("domain"), seenDomain)
		markOffline("dns_record", s.inst.ModelIDByObjID("dns_record"), seenDns)
		markOffline("oss", s.inst.ModelIDByObjID("oss"), seenCos)
	}
	return report, nil
}

// —— 腾讯云 COS（对象存储）专有签名 q-sign-algorithm=sha1，XML 响应 ——

func cosHmacSHA1(key, data string) string {
	m := hmac.New(sha1.New, []byte(key))
	m.Write([]byte(data))
	return hex.EncodeToString(m.Sum(nil))
}

func cosSHA1Hex(s string) string {
	h := sha1.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// cosRawEnc 与 PHP rawurlencode 一致（空格→%20，保留 -_.~），COS 签名要求。
func cosRawEnc(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}

// cosGet 以 COS 专有签名发起 GET 请求，返回响应体（XML）。
func cosGet(host, uri string, params map[string]string, ak, sk string) ([]byte, int, error) {
	now := time.Now().Unix()
	signTime := fmt.Sprintf("%d;%d", now-60, now+3600)
	signKey := cosHmacSHA1(sk, signTime)

	// 参数：key 小写排序，q-url-param-list 为 key 列表，HttpParameters 为 key=value
	pk := make([]string, 0, len(params))
	for k := range params {
		pk = append(pk, strings.ToLower(k))
	}
	sort.Strings(pk)
	pPairs := make([]string, 0, len(pk))
	for _, k := range pk {
		pPairs = append(pPairs, cosRawEnc(k)+"="+cosRawEnc(params[k]))
	}
	urlParamList := strings.Join(pk, ";")
	httpParams := strings.Join(pPairs, "&")

	// 头：仅签 host
	headers := map[string]string{"host": host}
	hk := make([]string, 0, len(headers))
	for k := range headers {
		hk = append(hk, strings.ToLower(k))
	}
	sort.Strings(hk)
	hPairs := make([]string, 0, len(hk))
	for _, k := range hk {
		hPairs = append(hPairs, cosRawEnc(k)+"="+cosRawEnc(headers[k]))
	}
	headerList := strings.Join(hk, ";")
	httpHeaders := strings.Join(hPairs, "&")

	httpString := strings.ToLower(http.MethodGet) + "\n" + uri + "\n" + httpParams + "\n" + httpHeaders + "\n"
	stringToSign := "sha1\n" + signTime + "\n" + cosSHA1Hex(httpString) + "\n"
	signature := cosHmacSHA1(signKey, stringToSign)
	auth := fmt.Sprintf("q-sign-algorithm=sha1&q-ak=%s&q-sign-time=%s&q-key-time=%s&q-header-list=%s&q-url-param-list=%s&q-signature=%s",
		ak, signTime, signTime, headerList, urlParamList, signature)

	q := ""
	if len(params) > 0 {
		ps := make([]string, 0, len(params))
		for k, v := range params {
			if v == "" {
				ps = append(ps, k)
			} else {
				ps = append(ps, k+"="+v)
			}
		}
		q = "?" + strings.Join(ps, "&")
	}
	req, err := http.NewRequest(http.MethodGet, "https://"+host+uri+q, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", auth)
	req.Host = host
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode, nil
}

// syncTencentCos 同步腾讯云 COS 存储桶到 OSS（对象存储）模型。cloud_provider=tencent 区分来源。
func (s *DataSourceService) syncTencentCos(ds *model.DataSource, report *SyncReport, seenCos *[]uint) {
	ossID := s.inst.ModelIDByObjID("oss")
	if ossID == 0 {
		return // 无 对象存储 模型则跳过
	}
	// GET Service（service.cos.myqcloud.com）为全局接口，一次返回所有地域的桶
	body, code, err := cosGet("service.cos.myqcloud.com", "/", nil, ds.Username, ds.Password)
	if err != nil {
		report.skipProduct("oss", "COS 同步跳过："+err.Error())
		return
	}
	if code != 200 {
		report.skipProduct("oss", fmt.Sprintf("COS 同步跳过：List Service HTTP %d（需为子账号授权 COS 只读 QcloudCOSReadOnlyAccess）", code))
		return
	}
	var r struct {
		Buckets struct {
			Bucket []struct {
				Name     string `xml:"Name"`
				Location string `xml:"Location"`
			} `xml:"Bucket"`
		} `xml:"Buckets"`
	}
	if xml.Unmarshal(body, &r) != nil {
		report.skipProduct("oss", "COS 响应解析失败")
		return
	}
	for _, bk := range r.Buckets.Bucket {
		name := strings.TrimSpace(bk.Name)
		if name == "" {
			report.Skipped++
			continue
		}
		report.Discovered++
		host := name + ".cos." + bk.Location + ".myqcloud.com"
		data := map[string]interface{}{
			"bucket":         name,
			"endpoint":       host,
			"region":         bk.Location,
			"status":         "running",
			"cloud_provider": "tencent",
		}
		if bk.Location != "" {
			data["cloud_az"] = bk.Location
		}
		// 读写权限：GET ?acl（失败则留空，不影响建档）
		if ab, acode, e := cosGet(host, "/", map[string]string{"acl": ""}, ds.Username, ds.Password); e == nil && acode == 200 {
			if acl := cosAclID(ab); acl != "" {
				data["acl"] = acl
			}
		}
		// 已用容量：经云监控(QCE/COS)汇总各存储类型容量（COS XML API 无桶统计，失败则留空）
		if gb, ok := s.tcCosUsedGB(ds, cosAppID(name), name, bk.Location); ok {
			data["used_gb"] = float64(int64(gb + 0.5)) // 四舍五入到 GB
		}
		inst, created, e := s.inst.UpsertInstance(ossID, "bucket", data, ds.Name, ds.ID)
		if e != nil {
			report.Failed++
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, name+": "+e.Error())
			}
			continue
		}
		*seenCos = append(*seenCos, inst.ID)
		if created {
			report.Created++
		} else {
			report.Updated++
		}
	}
}

// cosStorageMetrics COS 各存储类型容量监控指标（QCE/COS，单位 MB），求和得总存储量。
var cosStorageMetrics = []string{"StdStorage", "SiaStorage", "ArcStorage", "DeepArcStorage", "MazStdStorage"}

// cosAppID 从 COS 桶名（形如 name-appid）提取 APPID。
func cosAppID(bucket string) string {
	if i := strings.LastIndex(bucket, "-"); i >= 0 && i < len(bucket)-1 {
		return bucket[i+1:]
	}
	return ""
}

// tcCosUsedGB 经云监控(QCE/COS, GetMonitorData 2018-07-24)汇总桶各存储类型容量(MB)→GB。
// COS 的 XML API 无桶统计接口，存储量在云监控里（控制台概览同源）。取不到任何指标返回 ok=false。
func (s *DataSourceService) tcCosUsedGB(ds *model.DataSource, appid, bucket, region string) (float64, bool) {
	if appid == "" || region == "" {
		return 0, false
	}
	end := time.Now()
	start := end.Add(-72 * time.Hour) // 取近 3 天日粒度的最新点
	fmtT := func(t time.Time) string { return t.Format("2006-01-02T15:04:05+08:00") }
	totalMB := 0.0
	got := false
	for _, metric := range cosStorageMetrics {
		payload := map[string]interface{}{
			"Namespace": "QCE/COS", "MetricName": metric, "Period": 86400,
			"StartTime": fmtT(start), "EndTime": fmtT(end),
			"Instances": []map[string]interface{}{{"Dimensions": []map[string]string{
				{"Name": "appid", "Value": appid}, {"Name": "bucket", "Value": bucket}}}},
		}
		body, err := tcAPI("monitor", "GetMonitorData", "2018-07-24", region, ds.Username, ds.Password, payload)
		if err != nil {
			continue // 该指标无权限/不适用：跳过，不影响其它
		}
		var r struct {
			Response struct {
				DataPoints []struct {
					Values []json.Number `json:"Values"`
				} `json:"DataPoints"`
			} `json:"Response"`
		}
		if json.Unmarshal(body, &r) != nil || len(r.Response.DataPoints) == 0 {
			continue
		}
		vs := r.Response.DataPoints[0].Values
		if len(vs) == 0 {
			continue
		}
		if v, e := vs[len(vs)-1].Float64(); e == nil {
			totalMB += v
			got = true
		}
	}
	if !got {
		return 0, false
	}
	return totalMB / 1024.0, true // MB → GB
}

// cosAclID 解析 COS ?acl 响应，判断桶读写权限 → OSS 模型 acl 枚举 id。
// 有授予「所有用户(AllUsers)」READ→public-read，WRITE→public-rw；否则 private。
func cosAclID(aclXML []byte) string {
	var p struct {
		Grants []struct {
			URI        string `xml:"Grantee>URI"`
			Permission string `xml:"Permission"`
		} `xml:"AccessControlList>Grant"`
	}
	if xml.Unmarshal(aclXML, &p) != nil {
		return ""
	}
	publicRead, publicWrite := false, false
	for _, g := range p.Grants {
		if !strings.Contains(g.URI, "AllUsers") {
			continue
		}
		switch strings.ToUpper(g.Permission) {
		case "READ":
			publicRead = true
		case "WRITE":
			publicWrite = true
		case "FULL_CONTROL":
			publicRead, publicWrite = true, true
		}
	}
	switch {
	case publicWrite:
		return "public-rw"
	case publicRead:
		return "public-read"
	default:
		return "private"
	}
}

// tcDnspodDomain DNSPod 接入域名（含套餐标题）。
type tcDnspodDomain struct {
	Name    string
	Version string
}

// tcDnspodDomains 取接入腾讯云 DNSPod 的域名列表（含套餐版本名）。
func (s *DataSourceService) tcDnspodDomains(ds *model.DataSource) ([]tcDnspodDomain, error) {
	out := []tcDnspodDomain{}
	offset := 0
	const limit = 100
	for {
		body, err := tcAPI("dnspod", "DescribeDomainList", "2021-03-23", "ap-guangzhou", ds.Username, ds.Password,
			map[string]interface{}{"Offset": offset, "Limit": limit})
		if err != nil {
			return nil, err
		}
		var r struct {
			Response struct {
				DomainCountInfo struct {
					DomainTotal int `json:"DomainTotal"`
				} `json:"DomainCountInfo"`
				DomainList []struct {
					Name       string `json:"Name"`
					GradeTitle string `json:"GradeTitle"`
				} `json:"DomainList"`
			} `json:"Response"`
		}
		if json.Unmarshal(body, &r) != nil {
			return nil, newValidationErr("解析 DNSPod 域名响应失败")
		}
		for _, d := range r.Response.DomainList {
			if strings.TrimSpace(d.Name) != "" {
				out = append(out, tcDnspodDomain{Name: d.Name, Version: d.GradeTitle})
			}
		}
		offset += limit
		if r.Response.DomainCountInfo.DomainTotal == 0 || offset >= r.Response.DomainCountInfo.DomainTotal || len(r.Response.DomainList) == 0 {
			break
		}
	}
	return out, nil
}

// syncTencentDns 同步腾讯云 DNSPod 解析记录到 DNS解析 模型（一个域名一条，records 存全部记录明细）。
func (s *DataSourceService) syncTencentDns(ds *model.DataSource, report *SyncReport, seenDns *[]uint) {
	dnsID := s.inst.ModelIDByObjID("dns_record")
	if dnsID == 0 {
		return // 无 DNS解析 模型则跳过
	}
	domains, err := s.tcDnspodDomains(ds)
	if err != nil {
		report.skipProduct("dns_record", "DNS解析 同步跳过："+err.Error())
		return
	}
	for _, dn := range domains {
		records := []aliDnsRecord{}
		offset := 0
		const limit = 100
		for {
			body, e := tcAPI("dnspod", "DescribeRecordList", "2021-03-23", "ap-guangzhou", ds.Username, ds.Password,
				map[string]interface{}{"Domain": dn.Name, "Offset": offset, "Limit": limit})
			if e != nil {
				report.skipProduct("dns_record", "DNS解析 同步跳过("+dn.Name+")："+e.Error())
				return
			}
			var r struct {
				Response struct {
					RecordCountInfo struct {
						TotalCount int `json:"TotalCount"`
					} `json:"RecordCountInfo"`
					RecordList []struct {
						Name      string      `json:"Name"`
						Type      string      `json:"Type"`
						Value     string      `json:"Value"`
						TTL       json.Number `json:"TTL"`
						Line      string      `json:"Line"`
						Status    string      `json:"Status"`
						Remark    string      `json:"Remark"`
						UpdatedOn string      `json:"UpdatedOn"`
					} `json:"RecordList"`
				} `json:"Response"`
			}
			if json.Unmarshal(body, &r) != nil {
				report.skipProduct("dns_record", "DNS解析 响应解析失败("+dn.Name+")")
				return
			}
			for _, rec := range r.Response.RecordList {
				ttl, _ := rec.TTL.Int64()
				status := "enable"
				if strings.EqualFold(rec.Status, "DISABLE") {
					status = "disable"
				}
				fqdn := dn.Name
				if rr := strings.TrimSpace(rec.Name); rr != "" && rr != "@" {
					fqdn = rr + "." + dn.Name
				}
				records = append(records, aliDnsRecord{
					RR: rec.Name, FQDN: fqdn, Type: rec.Type, Value: rec.Value, TTL: ttl, Line: rec.Line,
					Status: status, Remark: rec.Remark, UpdatedAt: rec.UpdatedOn, // DNSPod 无单条创建时间，CreatedAt 留空
				})
			}
			offset += limit
			if r.Response.RecordCountInfo.TotalCount == 0 || offset >= r.Response.RecordCountInfo.TotalCount || len(r.Response.RecordList) == 0 {
				break
			}
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
}

// syncTencentCvm 同步单地域 CVM → 主机（容错：产品级错误记录并跳过）。
func (s *DataSourceService) syncTencentCvm(ds *model.DataSource, region string, report *SyncReport, seenHost *[]uint) {
	offset := 0
	const limit = 100
	for {
		body, err := tcAPI("cvm", "DescribeInstances", "2017-03-12", region, ds.Username, ds.Password,
			map[string]interface{}{"Limit": limit, "Offset": offset})
		if err != nil {
			report.skipProduct("host", "CVM(主机) 同步跳过("+region+")："+err.Error())
			return
		}
		var r tcCvmResp
		if err := json.Unmarshal(body, &r); err != nil {
			report.skipProduct("host", "CVM(主机) 响应解析失败("+region+")")
			return
		}
		for _, inst := range r.Response.InstanceSet {
			ip := firstIP(inst.PrivateIPAddresses)
			if ip == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"bk_host_innerip": ip,
				"bk_host_name":    firstNonEmpty(inst.InstanceName, inst.InstanceID),
				"cloud_provider":  "tencent",
			}
			if inst.InstanceID != "" {
				data["resource_id"] = inst.InstanceID
			}
			if inst.OsName != "" {
				data["bk_os_name"] = inst.OsName
			}
			if inst.CPU > 0 {
				data["bk_cpu"] = float64(inst.CPU)
			}
			if inst.Memory > 0 {
				data["bk_mem"] = float64(inst.Memory * 1024) // 腾讯云内存单位 GB → 模型要求 MB
			}
			if d := inst.SystemDisk.DiskSize + tcDataDiskTotal(inst.DataDisks); d > 0 {
				data["bk_disk"] = float64(d)
			}
			if inst.Placement.Zone != "" {
				data["cloud_az"] = inst.Placement.Zone
			}
			data["bk_zone"] = region
			if inst.InstanceState == "RUNNING" {
				data["bk_host_status"] = "1"
			} else {
				data["bk_host_status"] = "2"
			}
			h, created, e := s.inst.CollectCloudHost(data, ds.Name, ds.ID)
			if e != nil {
				report.Failed++
				if len(report.Errors) < 100 {
					report.Errors = append(report.Errors, ip+": "+e.Error())
				}
				continue
			}
			*seenHost = append(*seenHost, h.ID)
			if created {
				report.Created++
			} else {
				report.Updated++
			}
		}
		offset += limit
		if r.Response.TotalCount == 0 || offset >= r.Response.TotalCount || len(r.Response.InstanceSet) == 0 {
			break
		}
	}
}

// syncTencentDomain 同步腾讯云已注册域名（域名注册 API，全局资源）到 域名 模型。
func (s *DataSourceService) syncTencentDomain(ds *model.DataSource, report *SyncReport, seenDomain *[]uint) {
	domainID := s.inst.ModelIDByObjID("domain")
	if domainID == 0 {
		return // 无 域名 模型则跳过
	}
	// 持有者/邮箱在「信息模板」里，但列表/详情接口都不返回域名绑定的模板ID。
	// 现实：多数账号所有域名共用同一信息模板——仅当恰好 1 个模板时用它填充（否则留空不瞎填）。
	holder, email := s.tcDomainContact(ds)

	offset := 0
	const limit = 100
	for {
		// 域名注册为全局资源，region 任填一个即可
		body, err := tcAPI("domain", "DescribeDomainNameList", "2018-08-08", "ap-guangzhou", ds.Username, ds.Password,
			map[string]interface{}{"Offset": offset, "Limit": limit})
		if err != nil {
			report.skipProduct("domain", "域名 同步跳过："+err.Error())
			return
		}
		var r struct {
			Response struct {
				TotalCount int `json:"TotalCount"`
				DomainSet  []struct {
					DomainName     string `json:"DomainName"`
					CreationDate   string `json:"CreationDate"`
					ExpirationDate string `json:"ExpirationDate"`
				} `json:"DomainSet"`
			} `json:"Response"`
		}
		if json.Unmarshal(body, &r) != nil {
			report.skipProduct("domain", "域名 响应解析失败")
			return
		}
		for _, d := range r.Response.DomainSet {
			name := strings.TrimSpace(d.DomainName)
			if name == "" {
				report.Skipped++
				continue
			}
			report.Discovered++
			data := map[string]interface{}{
				"domain_name": name,
				"registrar":   "腾讯云",
				"status":      domainStatusID(d.ExpirationDate),
			}
			if len(d.ExpirationDate) >= 10 {
				data["expire_date"] = d.ExpirationDate[:10]
			}
			if len(d.CreationDate) >= 10 {
				data["registration_date"] = d.CreationDate[:10]
			}
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
		offset += limit
		if r.Response.TotalCount == 0 || offset >= r.Response.TotalCount || len(r.Response.DomainSet) == 0 {
			break
		}
	}
}

// tcDomainContact 取腾讯云域名持有者与联系人邮箱。
// 持有者/邮箱存于「信息模板」，但读接口不返回域名→模板绑定，故仅当账号恰好 1 个模板时
// 用其联系信息（多数账号所有域名共用一个模板）；0 或多个模板时返回空（无法可靠对应）。
func (s *DataSourceService) tcDomainContact(ds *model.DataSource) (holder, email string) {
	body, err := tcAPI("domain", "DescribeTemplateList", "2018-08-08", "ap-guangzhou", ds.Username, ds.Password,
		map[string]interface{}{"Offset": 0, "Limit": 50})
	if err != nil {
		return "", ""
	}
	var r struct {
		Response struct {
			TemplateSet []struct {
				ContactInfo struct {
					OrganizationNameCN string `json:"OrganizationNameCN"`
					OrganizationName   string `json:"OrganizationName"`
					LastNameCN         string `json:"LastNameCN"`
					FirstNameCN        string `json:"FirstNameCN"`
					LastName           string `json:"LastName"`
					FirstName          string `json:"FirstName"`
					Email              string `json:"Email"`
				} `json:"ContactInfo"`
			} `json:"TemplateSet"`
		} `json:"Response"`
	}
	if json.Unmarshal(body, &r) != nil || len(r.Response.TemplateSet) != 1 {
		return "", "" // 无模板或多个模板：无法可靠对应到具体域名，留空
	}
	c := r.Response.TemplateSet[0].ContactInfo
	// 持有者：优先中文（单位 > 个人姓名），回退英文
	holder = firstNonEmpty(c.OrganizationNameCN, strings.TrimSpace(c.LastNameCN+c.FirstNameCN),
		c.OrganizationName, strings.TrimSpace(c.LastName+c.FirstName))
	email = strings.TrimSpace(c.Email)
	// 邮箱被隐私保护脱敏（含 *）或非法时不写入，避免字段格式校验失败
	if email == "" || strings.Contains(email, "*") || !emailRe.MatchString(email) {
		email = ""
	}
	return holder, email
}

func tcDataDiskTotal(disks []struct {
	DiskSize int `json:"DiskSize"`
}) int {
	total := 0
	for _, d := range disks {
		total += d.DiskSize
	}
	return total
}
