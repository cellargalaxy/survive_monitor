package service

import (
	"context"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/config"
	"github.com/cellargalaxy/survive_monitor/machine"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/cellargalaxy/survive_monitor/probe"
	"github.com/cellargalaxy/survive_monitor/service/view"
	"github.com/sirupsen/logrus"
)

// Monitor 跑一轮监听：串行探测 -> 落明细 -> 收敛本实例结论 -> 采集本机资源 -> 合并对端视图 -> 判定 -> 告警
func Monitor(ctx context.Context) {
	monitor(ctx, config.GetConfig(ctx))
}

func monitor(ctx context.Context, conf model.Config) {
	//URL列表为空也照常走完一轮：资源采集和资源告警不依赖URL，探测、合并、URL告警那几步遍历空列表自然什么都不做
	if len(conf.Urls) == 0 {
		logrus.WithContext(ctx).WithFields(logrus.Fields{}).Warn("监听一轮，URL列表为空，只采集本机资源")
	}

	views := probeUrls(ctx, conf)
	probes := view.Converge(ctx, conf.Urls, conf.OfflineRound)

	resource, err := machine.LoadResource(ctx, conf.DiskPath)
	if err != nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"err": err}).Warn("监听一轮，采集本机资源异常")
	}
	view.SaveSelf(ctx, probes, &resource)

	for source := range views {
		view.Merge(ctx, source, *views[source])
	}

	alarmUrl(ctx, conf)
	alarmResource(ctx, conf, resource)
	view.Clean(ctx, conf.SnapshotExpireSec, conf.RecordWindowSec)
}

// probeUrls 逐个串行探测全部URL，落下明细，并把解析到全局视图的那些按来源URL收集起来。
// 串行是为了不给本机和被探测的服务添压力；单轮不设总预算，每个URL都实打实探一次，只靠单次超时兜底，
// 代价是单轮耗时随URL数量线性增长，最坏是URL数×单次超时。轮次不会叠加，守护池是跑完一轮才休眠再投下一轮
func probeUrls(ctx context.Context, conf model.Config) map[string]*model.View {
	timeout := time.Duration(conf.ProbeTimeoutSec) * time.Second
	views := make(map[string]*model.View)

	for i := range conf.Urls {
		url := conf.Urls[i]
		alive, peer := probeUrl(ctx, url, timeout)
		view.SaveRecord(ctx, url, alive, conf.RecordWindowSec)
		if peer != nil {
			views[url] = peer
		}
	}

	logrus.WithContext(ctx).WithFields(logrus.Fields{"url": len(conf.Urls), "view": len(views)}).Info("监听一轮，探测完成")
	return views
}

// probeUrl 探测单个URL并兜住panic，一个URL出问题不能把整轮后面的URL都带崩，panic按失败计
func probeUrl(ctx context.Context, url string, timeout time.Duration) (alive bool, peer *model.View) {
	defer util.Defer(func(panic any, stack string) {
		if panic != nil {
			logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "panic": panic, "stack": stack}).Error("探测URL，异常")
			alive, peer = false, nil
		}
	})
	return probe.Probe(ctx, url, timeout)
}
