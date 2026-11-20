package service

import (
	"strconv"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// cronMatch 判断标准 5 段 cron 表达式（分 时 日 月 周）是否匹配给定时间。
// 支持 *、具体数字、*/n、a-b 范围、逗号列表；周 0 和 7 均表示周日。
func cronMatch(expr string, t time.Time) bool {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false
	}
	wd := int(t.Weekday()) // 周日=0
	return cronField(fields[0], t.Minute(), 0, 59) &&
		cronField(fields[1], t.Hour(), 0, 23) &&
		cronField(fields[2], t.Day(), 1, 31) &&
		cronField(fields[3], int(t.Month()), 1, 12) &&
		cronWeekday(fields[4], wd)
}

func cronWeekday(field string, wd int) bool {
	// 7 视为周日(0)
	if cronField(field, wd, 0, 7) {
		return true
	}
	if wd == 0 {
		return cronField(field, 7, 0, 7)
	}
	return false
}

// cronField 判断单段是否匹配 value。
func cronField(field string, value, min, max int) bool {
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		step := 1
		if i := strings.Index(part, "/"); i >= 0 {
			step, _ = strconv.Atoi(part[i+1:])
			if step <= 0 {
				step = 1
			}
			part = part[:i]
		}
		lo, hi := min, max
		if part == "*" || part == "" {
			// 全范围
		} else if j := strings.Index(part, "-"); j >= 0 {
			lo, _ = strconv.Atoi(part[:j])
			hi, _ = strconv.Atoi(part[j+1:])
		} else {
			n, err := strconv.Atoi(part)
			if err != nil {
				continue
			}
			lo, hi = n, n
		}
		if value < lo || value > hi {
			continue
		}
		if (value-lo)%step == 0 {
			return true
		}
	}
	return false
}

// validCron 简单校验 cron 表达式是否为 5 段。
func validCron(expr string) bool {
	return len(strings.Fields(expr)) == 5
}

// StartScheduler 启动定时同步调度：每分钟检查启用且配置了 cron 的数据源。
func (s *DataSourceService) StartScheduler() {
	go func() {
		lastRun := map[uint]string{} // id -> 已触发资源同步的分钟
		for {
			now := time.Now()
			key := now.Format("2006-01-02 15:04")
			// 资源同步（cron）
			var list []model.DataSource
			s.db.Where("enabled = ? AND cron <> ''", true).Find(&list)
			for _, ds := range list {
				if !validCron(ds.Cron) || !cronMatch(ds.Cron, now) {
					continue
				}
				if lastRun[ds.ID] == key {
					continue
				}
				lastRun[ds.ID] = key
				go s.runScheduled(ds.ID)
			}
			// 睡到下一分钟整
			next := now.Truncate(time.Minute).Add(time.Minute)
			time.Sleep(time.Until(next))
		}
	}()
}

func (s *DataSourceService) runScheduled(id uint) {
	// 同步结果（含失败）由 SyncPrometheus 内部统一记录到 last_sync
	_, _ = s.Sync(id)
}
