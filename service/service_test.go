package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/cellargalaxy/survive_monitor/service/view"
)

// probeUrls 要同时做三件事：并发探完、落下明细、把能解析的视图按来源收集起来。
// 这里用本地服务替掉真实URL，顺带验了「普通URL只知道活着」和「探不通就是失败」两条分支
func TestProbeUrls(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		view := model.View{Snapshots: map[string]model.Snapshot{
			model.SelfSource: {Id: "peer-id", Time: time.Now().Unix(), Probes: map[string]bool{"https://x/": true}},
		}}
		writer.Header().Set("Content-Type", "application/json")
		writer.Write(util.JsonStruct2Data(util.NewHttpRespByErr(view, nil)))
	}))
	defer peer.Close()

	plain := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("hello"))
	}))
	defer plain.Close()

	//先开后关，拿一个必然连不上的地址
	dead := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {}))
	deadUrl := dead.URL
	dead.Close()

	conf := model.Config{
		Urls:             []string{peer.URL, plain.URL, deadUrl},
		ProbeBudgetSec:   10,
		ProbeTimeoutSec:  3,
		ProbeConcurrency: 4,
		RecordWindowSec:  600,
		OfflineRound:     1,
	}

	ctx := util.GenCtx()
	views := probeUrls(ctx, conf)

	if len(views) != 1 {
		t.Fatalf("只有对端实例那条该解析出视图，期望 1 条，实际 %d 条: %+v", len(views), views)
	}
	if views[peer.URL] == nil {
		t.Fatalf("视图该按来源URL收集，实际: %+v", views)
	}
	if views[plain.URL] != nil {
		t.Fatalf("普通URL不该被认领成实例")
	}

	probes := view.Converge(ctx, conf.Urls, conf.OfflineRound)
	if !probes[peer.URL] || !probes[plain.URL] {
		t.Errorf("通得上的URL该收敛成在线，实际 peer=%v plain=%v", probes[peer.URL], probes[plain.URL])
	}
	if probes[deadUrl] {
		t.Errorf("连不上的URL该收敛成离线")
	}
}

// URL列表为空时探测无事可做，但本机资源照样要采、要落进自己的快照，看板和资源告警才不会断
func TestMonitorEmptyUrls(t *testing.T) {
	conf := model.Config{
		ProbeBudgetSec:    10,
		ProbeTimeoutSec:   3,
		ProbeConcurrency:  4,
		OfflineRound:      3,
		SnapshotExpireSec: 300,
		RecordWindowSec:   600,
		DiskPath:          "/",
		//阈值拉满，保证这一轮不会真的去发告警
		CpuUsageLimit:  1000,
		MemUsageLimit:  1000,
		DiskUsageLimit: 1000,
		ResourceRound:  3,
	}

	ctx := util.GenCtx()
	monitor(ctx, conf)

	self, ok := view.GetView(ctx, conf.SnapshotExpireSec).Snapshots[model.SelfSource]
	if !ok {
		t.Fatalf("URL列表为空也该落下自己的快照")
	}
	if self.Resource == nil || self.Resource.Time <= 0 {
		t.Fatalf("URL列表为空也该采到本机资源，实际: %+v", self.Resource)
	}
	if self.Resource.CpuNum <= 0 || self.Resource.MemTotal == 0 || self.Resource.DiskTotal == 0 {
		t.Errorf("本机资源取值非法: %+v", *self.Resource)
	}
	if len(self.Probes) != 0 {
		t.Errorf("没有URL就不该有探测结论，实际: %+v", self.Probes)
	}
}
