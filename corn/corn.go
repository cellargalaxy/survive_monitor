package corn

import (
	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/config"
	"github.com/cellargalaxy/survive_monitor/service"
	"github.com/robfig/cron/v3"
)

func init() {
	var err error
	cronObject := cron.New()

	_, err = cronObject.AddJob(config.Config.Cron, new(MonitorJob))
	if err != nil {
		panic(err)
	}

	cronObject.Start()
}

type MonitorJob struct {
}

func (this *MonitorJob) Run() {
	ctx := util.GenCtx()

	service.MonitorConfig(ctx)
}
