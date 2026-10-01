package machine

import (
	"context"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
)

// cpuUsageInterval CPU使用率的采样窗口。取值要在两次快照之间等一段时间，窗口太短会抖成0或者100
const cpuUsageInterval = time.Second

// LoadResource 采集本机资源。单项采不到就留零值并返回错误，已经采到的那几项照样可用，
// 零值在阈值比较里天然不会触发告警，所以调用方拿着不完整的结果继续走是安全的
func LoadResource(ctx context.Context, diskPath string) (model.Resource, error) {
	var resource model.Resource
	resource.Time = time.Now().Unix()

	var lastErr error
	cpuNum, err := util.GetCpuNum(ctx)
	if err != nil {
		lastErr = err
	} else {
		resource.CpuNum = cpuNum
	}
	cpuUsage, err := util.GetCpuUsage(ctx, cpuUsageInterval)
	if err != nil {
		lastErr = err
	} else {
		resource.CpuUsage = cpuUsage
	}
	memTotal, err := util.GetMemTotal(ctx)
	if err != nil {
		lastErr = err
	} else {
		resource.MemTotal = memTotal
	}
	memUsed, err := util.GetMemUsed(ctx)
	if err != nil {
		lastErr = err
	} else {
		resource.MemUsed = memUsed
	}
	diskTotal, err := util.GetDiskTotal(ctx, diskPath)
	if err != nil {
		lastErr = err
	} else {
		resource.DiskTotal = diskTotal
	}
	diskUsed, err := util.GetDiskUsed(ctx, diskPath)
	if err != nil {
		lastErr = err
	} else {
		resource.DiskUsed = diskUsed
	}
	return resource, lastErr
}
