package corn

import (
	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/service"
	"github.com/robfig/cron/v3"
)

func init() {
	var err error
	cronObject := cron.New()

	_, err = cronObject.AddJob("*/5 * * * *", new(MonitorJob))
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
