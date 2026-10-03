package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	transport "github.com/ealink1/super-link/internal/infra/shell"
)

func shellMetricDisplay(metrics transport.Metrics) (values, details [4]string, memory, disk float64) {
	values = [4]string{"—", "—", "—", "—"}
	details = [4]string{"暂无负载数据", "暂无内存数据", "暂无磁盘数据", "暂无运行时间"}
	load := strings.Fields(metrics.Load)
	if len(load) >= 3 {
		values[0], details[0] = load[0], strings.Join(load[:3], " / ")
	}
	if metrics.MemoryTotal > 0 && metrics.MemoryUsed <= metrics.MemoryTotal {
		memory = float64(metrics.MemoryUsed) / float64(metrics.MemoryTotal)
		values[1] = fmt.Sprintf("%.0f%%", memory*100)
		details[1] = fmt.Sprintf("%.1f / %.1f GiB", float64(metrics.MemoryUsed)/(1<<30), float64(metrics.MemoryTotal)/(1<<30))
	}
	for _, line := range strings.Split(metrics.Disk, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 || fields[5] != "/" {
			continue
		}
		percent, err := strconv.ParseFloat(strings.TrimSuffix(fields[4], "%"), 64)
		if err != nil || math.IsNaN(percent) || percent < 0 || percent > 100 {
			continue
		}
		disk = percent / 100
		values[2], details[2] = fields[4], "/ · 已用 / 总量"
		total, errTotal := strconv.ParseUint(fields[1], 10, 64)
		used, errUsed := strconv.ParseUint(fields[2], 10, 64)
		if errTotal == nil && errUsed == nil {
			details[2] = fmt.Sprintf("%.1f / %.1f GiB", float64(used)/(1<<20), float64(total)/(1<<20))
		}
	}
	uptime := strings.Fields(metrics.Uptime)
	if len(uptime) > 0 {
		seconds, err := strconv.ParseFloat(uptime[0], 64)
		if err == nil && seconds >= 0 && seconds < 1e12 {
			hours := int64(seconds) / 3600
			values[3], details[3] = fmt.Sprintf("%dd %dh", hours/24, hours%24), metrics.System
		}
	}
	return values, details, memory, disk
}
