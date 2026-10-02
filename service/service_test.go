package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/cellargalaxy/survive_monitor/service/view"
)

// probeUrls 要同时做三件事：串行探完、落下明细、把能解析的视图按来源收集起来。
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
		Urls:            []string{peer.URL, plain.URL, deadUrl},
		ProbeTimeoutSec: 3,
		RecordWindowSec: 600,
		OfflineRound:    1,
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

// 串行探测：同一时刻最多只有一个请求在飞，这是取消并发的全部目的
func TestProbeUrlsSerial(t *testing.T) {
	var doing, maxDoing atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		current := doing.Add(1)
		defer doing.Add(-1)
		for {
			old := maxDoing.Load()
			if current <= old || maxDoing.CompareAndSwap(old, current) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	conf := model.Config{ProbeTimeoutSec: 3, RecordWindowSec: 600, OfflineRound: 1}
	for i := 0; i < 5; i++ {
		conf.Urls = append(conf.Urls, fmt.Sprintf("%s/serial/%d", server.URL, i))
	}
	probeUrls(util.GenCtx(), conf)

	if maxDoing.Load() != 1 {
		t.Errorf("串行探测同一时刻应只有1个请求，实际最多 %d 个", maxDoing.Load())
	}
}

// 单轮不设预算：前面的URL再慢，后面的也要实打实探一次，每个URL只受单次超时约束
func TestProbeUrlsTimeout(t *testing.T) {
	var count atomic.Int32
	slow := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count.Add(1)
		time.Sleep(1500 * time.Millisecond)
	}))
	defer slow.Close()

	conf := model.Config{
		Urls:            []string{slow.URL + "/timeout/0", slow.URL + "/timeout/1"},
		ProbeTimeoutSec: 1,
		RecordWindowSec: 600,
		OfflineRound:    1,
	}
	ctx := util.GenCtx()
	begin := time.Now()
	probeUrls(ctx, conf)
	cost := time.Since(begin)

	if count.Load() != 2 {
		t.Errorf("每个URL都该发出请求，期望服务端收到 2 次，实际 %d 次", count.Load())
	}
	if cost < 2*time.Second || cost > 2500*time.Millisecond {
		t.Errorf("两个URL各被单次超时1秒掐断，整轮该在2秒左右，实际 %v", cost)
	}
	probes := view.Converge(ctx, conf.Urls, conf.OfflineRound)
	for _, url := range conf.Urls {
		if probes[url] {
			t.Errorf("超时的URL该记失败: %s", url)
		}
	}
}

// 每探完一个URL都要休眠探测间隔，最后一个之后也睡：A 休眠 B 休眠 C 休眠
func TestProbeUrlsInterval(t *testing.T) {
	var lock sync.Mutex
	var times []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		lock.Lock()
		defer lock.Unlock()
		times = append(times, time.Now())
	}))
	defer server.Close()

	conf := model.Config{ProbeIntervalSec: 1, ProbeTimeoutSec: 3, RecordWindowSec: 600, OfflineRound: 1}
	for i := 0; i < 3; i++ {
		conf.Urls = append(conf.Urls, fmt.Sprintf("%s/interval/%d", server.URL, i))
	}
	begin := time.Now()
	probeUrls(util.GenCtx(), conf)
	cost := time.Since(begin)

	if len(times) != 3 {
		t.Fatalf("期望收到 3 次请求，实际 %d 次", len(times))
	}
	for i := 1; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap < time.Second {
			t.Errorf("第 %d 和第 %d 个URL之间应至少隔1秒，实际 %v", i, i+1, gap)
		}
	}
	if cost < 3*time.Second || cost > 4*time.Second {
		t.Errorf("3个URL各休眠1秒，最后一个之后也要睡，整轮该在3秒出头，实际 %v", cost)
	}
}

// 没有URL时一轮只剩资源采集加轮末休眠：资源采集 休眠 资源采集 ...
func TestMonitorSleep(t *testing.T) {
	conf := model.Config{
		ProbeIntervalSec:    1,
		ProbeTimeoutSec:     3,
		OfflineRound:        3,
		SnapshotExpireRound: 5,
		RecordWindowSec:     600,
		DiskPath:            "/",
		CpuUsageLimit:       1000,
		MemUsageLimit:       1000,
		DiskUsageLimit:      1000,
		ResourceRound:       3,
	}
	begin := time.Now()
	monitor(util.GenCtx(), conf)
	cost := time.Since(begin)

	//CPU采样固定1秒，加上轮末休眠1秒
	if cost < 2*time.Second || cost > 3*time.Second {
		t.Errorf("一轮应是CPU采样1秒+轮末休眠1秒，实际 %v", cost)
	}
}

// URL列表为空时探测无事可做，但本机资源照样要采、要落进自己的快照，看板和资源告警才不会断
func TestMonitorEmptyUrls(t *testing.T) {
	conf := model.Config{
		ProbeTimeoutSec:     3,
		OfflineRound:        3,
		SnapshotExpireRound: 5,
		RecordWindowSec:     600,
		DiskPath:            "/",
		//阈值拉满，保证这一轮不会真的去发告警
		CpuUsageLimit:  1000,
		MemUsageLimit:  1000,
		DiskUsageLimit: 1000,
		ResourceRound:  3,
	}

	ctx := util.GenCtx()
	monitor(ctx, conf)

	self, ok := view.GetView(ctx, conf.SnapshotExpireRound).Snapshots[model.SelfSource]
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

// 对端一直返回同一份旧快照(产生时间不变)，等于拿不到新数据：连续snapshot_expire_round轮之后就过期出局
func TestMonitorExpireRound(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		view := model.View{Snapshots: map[string]model.Snapshot{
			model.SelfSource: {Id: "stuck-peer", Time: 1700000000, Probes: map[string]bool{"https://x/": true}},
		}}
		writer.Header().Set("Content-Type", "application/json")
		writer.Write(util.JsonStruct2Data(util.NewHttpRespByErr(view, nil)))
	}))
	defer peer.Close()

	conf := model.Config{
		Urls:                []string{peer.URL},
		ProbeTimeoutSec:     3,
		OfflineRound:        3,
		SnapshotExpireRound: 2,
		RecordWindowSec:     600,
		DiskPath:            "/",
		CpuUsageLimit:       1000,
		MemUsageLimit:       1000,
		DiskUsageLimit:      1000,
		ResourceRound:       3,
	}
	ctx := util.GenCtx()
	exist := func() bool {
		_, ok := view.GetView(ctx, conf.SnapshotExpireRound).Snapshots[peer.URL]
		return ok
	}

	monitor(ctx, conf)
	if !exist() {
		t.Fatalf("第1轮拿到新数据，对端快照该在")
	}
	monitor(ctx, conf)
	if !exist() {
		t.Fatalf("才1轮没拿到新数据，还没到2轮，对端快照该在")
	}
	monitor(ctx, conf)
	if exist() {
		t.Fatalf("连续2轮没拿到新数据，对端快照该过期")
	}
	//过期清掉之后对端还在返回同一份旧快照，不许被当成新数据收回来，过期一轮又复活一轮
	monitor(ctx, conf)
	monitor(ctx, conf)
	if exist() {
		t.Fatalf("过期之后再拉到同一份旧快照，不该复活")
	}
}
