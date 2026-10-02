package machine

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/sirupsen/logrus"
)

// cpuUsageInterval CPU使用率的采样窗口。取值要在两次快照之间等一段时间，窗口太短会抖成0或者100
const cpuUsageInterval = time.Second

// memoryFsTypes 内存型文件系统。路径落在这上面时量到的是内存盘，不是磁盘：
// 比如在flatpak沙箱(GoLand等)里跑，根目录就是一块tmpfs，看板上会出现「136KB / 7.5GB」这种数
var memoryFsTypes = map[string]bool{"tmpfs": true, "ramfs": true, "devtmpfs": true}

// fallbackWarned 记下已经提示过回退的路径，每分钟一轮，不能每轮都刷一条Warn
var fallbackWarned sync.Map

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
	diskPath = resolveDiskPath(ctx, diskPath)
	resource.DiskPath = diskPath
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

// resolveDiskPath 确定实际采集的磁盘路径。配置路径落在内存型文件系统上时，回退到工作目录：
// 配置文件和日志都放在工作目录下，它所在的分区就是本服务真正在用的磁盘。
// 工作目录也是内存盘、或者哪一步取不到，就原样用配置路径，采不到的错误留给后面的采集去报
func resolveDiskPath(ctx context.Context, diskPath string) string {
	if !isMemoryFs(ctx, diskPath) {
		return diskPath
	}
	workPath, err := os.Getwd()
	if err != nil || isMemoryFs(ctx, workPath) {
		return diskPath
	}
	if _, warned := fallbackWarned.LoadOrStore(diskPath, true); !warned {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"diskPath": diskPath, "workPath": workPath}).Warn("采集磁盘，配置路径是内存盘，回退到工作目录")
	}
	return workPath
}

// isMemoryFs 判断路径是否落在内存型文件系统上，取不到文件系统类型的一律当作不是
func isMemoryFs(ctx context.Context, path string) bool {
	stat, err := disk.UsageWithContext(ctx, path)
	if err != nil || stat == nil {
		return false
	}
	return memoryFsTypes[stat.Fstype]
}
