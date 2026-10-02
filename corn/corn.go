package corn

import (
	"context"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/config"
	"github.com/cellargalaxy/survive_monitor/service"
	"github.com/sirupsen/logrus"
)

// Init 启动监听轮次。用守护单协程池而不是cron：池子是单协程的，任务跑完才投下一轮，
// 「上一轮没结束就不开新一轮」是白送的。cron按点硬触发，赶上一轮探测跑超时，轮次就会叠起来。
// 池子自己的休眠设为0，节奏全交给service.Monitor：每个URL之后、每轮末尾各休眠probe_interval_sec，
// 这样间隔每轮现读配置，改完不用重启
func Init(ctx context.Context) error {
	_, err := util.NewDaemonSingleGoPool(ctx, "Monitor", 0, monitor)
	if err != nil {
		return err
	}

	logrus.WithContext(ctx).WithFields(logrus.Fields{"interval": config.GetConfig(ctx).ProbeIntervalSec}).Info("定时任务，监听服务")
	return nil
}

func monitor(cancelCtx context.Context, pool *util.SingleGoPool) {
	//每轮换一个新上下文，这样一轮探测的全部日志能靠日志ID串在一起。
	//池子传进来的那个上下文从main一路带着同一个日志ID，接着用会让所有轮次的日志糊成一片
	ctx := util.GenCtx()

	service.Monitor(ctx)
}
