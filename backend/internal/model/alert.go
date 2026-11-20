package model

import "time"

// Alert 告警记录（从阿里云云监控等拉取的报警历史/事件，独立于资源实例表）。
// 按 (source_id, log_id) 去重；按 instance_id 关联到 CMDB 资源实例。
type Alert struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	SourceID     uint      `gorm:"not null;uniqueIndex:idx_alert_src_log,priority:1" json:"source_id"` // 数据源
	LogID        string    `gorm:"size:64;uniqueIndex:idx_alert_src_log,priority:2" json:"log_id"`     // 云厂商告警日志 id（去重）
	Level        string    `gorm:"size:16;index" json:"level"`                                        // critical/warning/info/ok
	Status       string    `gorm:"size:16;index" json:"status"`                                       // firing/resolved
	RuleName     string    `gorm:"size:255" json:"rule_name"`                                         // 报警规则名
	Product      string    `gorm:"size:64" json:"product"`                                            // 云产品（ECS/RDS…）
	MetricName   string    `gorm:"size:128" json:"metric_name"`                                       // 监控项
	InstanceID   string    `gorm:"size:128;index" json:"instance_id"`                                 // 云资源 id
	InstanceName string    `gorm:"size:255" json:"instance_name"`                                     // 云资源名
	Dimensions   string    `gorm:"size:512" json:"dimensions"`                                        // 告警维度（如 Kafka 消费者组、数据库名、磁盘等），"key=value · key=value"
	ResInstID    uint      `gorm:"index;default:0" json:"res_inst_id"`                                // 关联到的 CMDB 实例 id（0=未匹配）
	ResModel     string    `gorm:"size:64" json:"res_model"`                                          // 关联资源的模型名
	Expression   string     `gorm:"size:512" json:"expression"`                                       // 触发表达式（规则条件，原始 PromQL 等）
	TriggerValue string     `gorm:"size:128" json:"trigger_value"`                                    // 触发时的实际值（如 90.02）
	GroupName    string     `gorm:"size:128" json:"group_name"`                                       // 应用分组
	AlertTime    time.Time  `gorm:"index" json:"alert_time"`                                          // 触发时间
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`                                            // 恢复时间（status=resolved 时记录，firing 为空）
	CreatedAt    time.Time  `json:"created_at"`

	// SourceName 数据源名（只读，不入库）
	SourceName string `gorm:"-" json:"source_name,omitempty"`
	// Cluster k8s 集群标识（告警 cluster/origin_prometheus 标签，仅用于关联集群实例，不入库）
	Cluster string `gorm:"-" json:"-"`
	// Namespace/Pod/Workload k8s 应用级标识（告警标签，用于关联到 k8s_app 实例，不入库）
	Namespace string `gorm:"-" json:"-"`
	Pod       string `gorm:"-" json:"-"`
	Workload  string `gorm:"-" json:"-"`
}

func (Alert) TableName() string { return "alert" }
