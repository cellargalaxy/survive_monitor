package service

import (
	"context"
	"sync"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/config"
	"github.com/cellargalaxy/survive_monitor/machine"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/cellargalaxy/survive_monitor/probe"
	"github.com/cellargalaxy/survive_monitor/service/view"
	"github.com/sirupsen/logrus"
)

// Monitor 跑一轮监听：并发探测 -> 落明细 -> 收敛本实例结论 -> 采集本机资源 -> 合并对端视图 -> 判定 -> 告警
func Monitor(ctx context.Context) {
	monitor(ctx, config.GetConfig(ctx))
}

func monitor(ctx context.Context, conf model.Config) {
	//URL列表为空也照常走完一轮：资源采集和资源告警不依赖URL，探测、合并、URL告警那几步遍历空列表自然什么都不做
	if len(conf.Urls) == 0 {
		logrus.WithContext(ctx).WithFields(logrus.Fields{}).Warn("监听一轮，URL列表为空，只采集本机资源")
	}

	//单轮预算一到就掐断，没跑完的URL本轮按失败计。预算不设的话失败URL会把一轮拖到比间隔还长，轮次就堆起来了
	budgetCtx, cancel := context.WithTimeout(ctx, time.Duration(conf.ProbeBudgetSec)*time.Second)
	defer util.CancelCtx(cancel)

	views := probeUrls(budgetCtx, conf)
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

// probeUrls 在预算内并发探测全部URL，落下明细，并把解析到全局视图的那些按来源URL收集起来
func probeUrls(ctx context.Context, conf model.Config) map[string]*model.View {
	timeout := time.Duration(conf.ProbeTimeoutSec) * time.Second
	//并发数为零会让令牌通道变成无缓冲的，拿令牌的协程就永远等不到人来收，整轮卡死
	concurrency := conf.ProbeConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	token := make(chan struct{}, concurrency)
	var group sync.WaitGroup
	var collectLock sync.Mutex
	views := make(map[string]*model.View)

	for i := range conf.Urls {
		url := conf.Urls[i]
		group.Add(1)
		go func() {
			defer group.Done()
			defer util.Defer(func(panic any, stack string) {
				if panic != nil {
					logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "panic": panic, "stack": stack}).Error("探测URL，异常")
				}
			})

			select {
			case token <- struct{}{}:
			case <-ctx.Done():
				//预算用完还没排上队，本轮就按失败计，不必再白跑一次必然超时的请求
				view.SaveRecord(ctx, url, false, conf.RecordWindowSec)
				return
			}
			defer func() { <-token }()

			alive, peer := probe.Probe(ctx, url, timeout)
			view.SaveRecord(ctx, url, alive, conf.RecordWindowSec)
			if peer == nil {
				return
			}
			collectLock.Lock()
			defer collectLock.Unlock()
			views[url] = peer
		}()
	}
	group.Wait()

	logrus.WithContext(ctx).WithFields(logrus.Fields{"url": len(conf.Urls), "view": len(views)}).Info("监听一轮，探测完成")
	return views
}
