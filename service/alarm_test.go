package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/cellargalaxy/survive_monitor/service/view"
	"github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.WarnLevel)
	code := m.Run()
	os.RemoveAll("resource")
	os.RemoveAll("log")
	os.Exit(code)
}

// 退避阶梯走到末阶之后要一直停在末阶，不能越界也不能绕回第一阶
func TestGetBackoff(t *testing.T) {
	steps := []int{300, 1800, 7200}
	cases := map[int]int{0: 300, 1: 300, 2: 1800, 3: 7200, 4: 7200, 99: 7200}

	for sendCount := range cases {
		actual := getBackoff(steps, sendCount)
		if actual != cases[sendCount] {
			t.Errorf("sendCount=%d 期望 %d，实际 %d", sendCount, cases[sendCount], actual)
		}
	}
	if getBackoff(nil, 1) != 0 {
		t.Errorf("没有阶梯时不该退避")
	}
}

// 阈值是「达到就算超」，边界值必须命中，否则正好卡在阈值上的机器永远不告警
func TestGetOverTexts(t *testing.T) {
	conf := model.Config{CpuUsageLimit: 90, MemUsageLimit: 90, DiskUsageLimit: 90, DiskPath: "/"}

	none := getOverTexts(conf, model.Resource{CpuNum: 2, CpuUsage: 100, MemTotal: 100, MemUsed: 50, DiskTotal: 100, DiskUsed: 50})
	if len(none) != 0 {
		t.Errorf("都没超阈值不该有文案，实际: %+v", none)
	}

	edge := getOverTexts(conf, model.Resource{CpuNum: 2, CpuUsage: 180, MemTotal: 100, MemUsed: 90, DiskTotal: 100, DiskUsed: 90, DiskPath: "/data"})
	if len(edge) != 3 {
		t.Errorf("三项都正好卡在阈值上应全部命中，实际 %d 条: %+v", len(edge), edge)
	} else if !strings.Contains(edge[2], "路径/data") {
		t.Errorf("磁盘文案要带实际采集的路径，而不是配置路径，实际: %s", edge[2])
	}

	part := getOverTexts(conf, model.Resource{CpuNum: 4, CpuUsage: 40, MemTotal: 100, MemUsed: 95, DiskTotal: 100, DiskUsed: 10})
	if len(part) != 1 {
		t.Errorf("只有内存超阈值应只出1条，实际 %d 条: %+v", len(part), part)
	}

	//采不到的项是零值，零值不该把告警凭空触发出来
	empty := getOverTexts(conf, model.Resource{})
	if len(empty) != 0 {
		t.Errorf("采集不到资源时不该告警，实际: %+v", empty)
	}
}

// 正文只有20个字：离线在前、恢复在后，只有一个时不写个数，URL去掉协议头和末尾斜杠，顺序与配置一致
func TestGetUrlAlarmText(t *testing.T) {
	urls := []string{"https://a/", "http://b:8080/x", "https://c/"}
	saves := map[string]model.Alarm{"http://b:8080/x": {}, "https://a/": {}}

	both := getUrlAlarmText(urls, saves, []string{"https://c/"})
	if both != "离线2:a、b:8080/x；恢复:c" {
		t.Fatalf("离线、恢复都有时文案不对，实际 %s", both)
	}
	only := getUrlAlarmText(urls, nil, []string{"https://c/"})
	if only != "恢复:c" {
		t.Fatalf("只有一个恢复时不该带个数、不该带离线，实际 %s", only)
	}
}

// 微信正文不允许换行、最多20个字：换行要压掉，超长按字数截断补省略号，中文不能被切成乱码
func TestLimitText(t *testing.T) {
	if actual := limitText("离线:a\nb", 20); actual != "离线:a b" {
		t.Errorf("换行该压成空格，实际 %q", actual)
	}
	if actual := limitText("一二三四五六七八九十一二三四五六七八九十", 20); actual != "一二三四五六七八九十一二三四五六七八九十" {
		t.Errorf("正好20个字不该截断，实际 %s", actual)
	}
	actual := limitText("离线:s1.example.com/api/view", 20)
	if actual != "离线:s1.example.com/a…" || len([]rune(actual)) != 20 {
		t.Errorf("超长该截到20个字、末尾是省略号，实际 %s", actual)
	}
}

// 超阈值正文只列超了的项、取整；三项全超、两位数时刚好20个字
func TestGetOverShort(t *testing.T) {
	conf := model.Config{CpuUsageLimit: 90, MemUsageLimit: 90, DiskUsageLimit: 90}

	part := getOverShort(conf, model.Resource{CpuNum: 4, CpuUsage: 40, MemTotal: 100, MemUsed: 95, DiskTotal: 100, DiskUsed: 10})
	if part != "超阈值:内存95%" {
		t.Errorf("只有内存超阈值应只列内存，实际 %s", part)
	}
	all := getOverShort(conf, model.Resource{CpuNum: 2, CpuUsage: 190, MemTotal: 100, MemUsed: 92, DiskTotal: 100, DiskUsed: 91})
	if all != "超阈值:CPU95%内存92%磁盘91%" || len([]rune(all)) > wxTextLimit {
		t.Errorf("三项全超该都列出且不超过%d个字，实际 %s", wxTextLimit, all)
	}
}

// 离线明细要带上确认实例数、持续多久、第几次提醒和下次提醒间隔，这些都是收到消息时判断要不要马上处理的依据
func TestJudgeAlarmOfflineText(t *testing.T) {
	ctx := util.GenCtx()
	conf := model.Config{Urls: []string{"https://text/"}, SnapshotExpireRound: 5, AlarmBackoffSec: []int{300, 1800}}
	now := int64(1700000000)

	setView(t, ctx, map[string]bool{"https://text/": false}, map[string]model.Alarm{
		"https://text/": {StartTime: now - 600, LastSendTime: now - 300, SendCount: 1},
	})
	offlines, _, _, _ := judgeAlarm(ctx, conf, now)
	if len(offlines) != 1 {
		t.Fatalf("退避到点该再发一条，实际 %d 条", len(offlines))
	}
	for _, want := range []string{"https://text/", "1个实例确认", "已持续10分钟", "第2次提醒", "30分钟后再提醒"} {
		if !strings.Contains(offlines[0], want) {
			t.Errorf("离线明细缺少「%s」，实际 %s", want, offlines[0])
		}
	}
}

func TestGetDuration(t *testing.T) {
	cases := map[int64]string{-5: "0秒", 0: "0秒", 59: "59秒", 60: "1分钟", 3599: "59分钟", 3600: "1小时0分钟", 86399: "23小时59分钟", 86400: "1天0小时"}

	for second := range cases {
		actual := getDuration(second)
		if actual != cases[second] {
			t.Errorf("second=%d 期望 %s，实际 %s", second, cases[second], actual)
		}
	}
}

// setView 把全局视图摆成指定状态，是辅助函数
func setView(t *testing.T, ctx context.Context, probes map[string]bool, alarms map[string]model.Alarm) {
	t.Helper()

	view.SaveSelf(ctx, probes, nil)
	for url := range alarms {
		view.SaveAlarm(ctx, url, alarms[url])
	}
}

// 首次判离线就发，发过之后要按退避压住，退避到点才再发；这几条一起决定了不会每轮重复轰炸
func TestJudgeAlarmOffline(t *testing.T) {
	ctx := util.GenCtx()
	conf := model.Config{Urls: []string{"https://dead/"}, SnapshotExpireRound: 5, AlarmBackoffSec: []int{300, 1800}}
	now := int64(1700000000)

	setView(t, ctx, map[string]bool{"https://dead/": false}, nil)
	offlines, recovers, saves, dels := judgeAlarm(ctx, conf, now)
	if len(offlines) != 1 || len(recovers) != 0 || len(dels) != 0 {
		t.Fatalf("首次判离线该发一条离线、不该有恢复，实际 offline=%d recover=%d del=%d", len(offlines), len(recovers), len(dels))
	}
	if saves["https://dead/"].SendCount != 1 || saves["https://dead/"].StartTime != now {
		t.Fatalf("首发该记成第1次、起始时间就是现在，实际 %+v", saves["https://dead/"])
	}

	setView(t, ctx, map[string]bool{"https://dead/": false}, map[string]model.Alarm{
		"https://dead/": {StartTime: now, LastSendTime: now, SendCount: 1},
	})
	offlines, _, _, _ = judgeAlarm(ctx, conf, now+299)
	if len(offlines) != 0 {
		t.Fatalf("退避没到点不该重发，实际发了 %+v", offlines)
	}

	offlines, _, saves, _ = judgeAlarm(ctx, conf, now+300)
	if len(offlines) != 1 {
		t.Fatalf("退避到点该再发一条，实际 %d 条", len(offlines))
	}
	if saves["https://dead/"].SendCount != 2 || saves["https://dead/"].StartTime != now {
		t.Fatalf("重发该累加次数、但起始时间不许动，实际 %+v", saves["https://dead/"])
	}
}

// 恢复只要有一个实例说在线就算，并且要把记录删掉，否则再次离线会被旧记录按成退避
func TestJudgeAlarmRecover(t *testing.T) {
	ctx := util.GenCtx()
	conf := model.Config{Urls: []string{"https://back/"}, SnapshotExpireRound: 5, AlarmBackoffSec: []int{300}}
	now := int64(1700000000)

	setView(t, ctx, map[string]bool{"https://back/": true}, map[string]model.Alarm{
		"https://back/": {StartTime: now - 3600, LastSendTime: now - 60, SendCount: 2},
	})
	offlines, recovers, _, dels := judgeAlarm(ctx, conf, now)
	if len(offlines) != 0 || len(recovers) != 1 {
		t.Fatalf("该只发恢复，实际 offline=%d recover=%d", len(offlines), len(recovers))
	}
	if len(dels) != 1 || dels[0] != "https://back/" {
		t.Fatalf("恢复该把记录删掉，实际 %+v", dels)
	}
	if !strings.Contains(recovers[0], "1小时") {
		t.Fatalf("恢复文案该带上离线了多久，实际 %s", recovers[0])
	}

	//没发过告警就恢复，是从来没离线过，不该凭空发一条恢复
	view.RecoverAlarm(ctx, "https://back/", conf.SnapshotExpireRound)
	setView(t, ctx, map[string]bool{"https://back/": true}, nil)
	_, recovers, _, _ = judgeAlarm(ctx, conf, now)
	if len(recovers) != 0 {
		t.Fatalf("没告警过不该发恢复，实际 %+v", recovers)
	}
}

// 没有任何新鲜结论时保持现状，既不告警也不恢复
func TestJudgeAlarmNoConclusion(t *testing.T) {
	ctx := util.GenCtx()
	conf := model.Config{Urls: []string{"https://unknown/"}, SnapshotExpireRound: 5, AlarmBackoffSec: []int{300}}

	setView(t, ctx, map[string]bool{"https://other/": false}, nil)
	offlines, recovers, _, _ := judgeAlarm(ctx, conf, 1700000000)
	if len(offlines) != 0 || len(recovers) != 0 {
		t.Fatalf("没结论该什么都不发，实际 offline=%+v recover=%+v", offlines, recovers)
	}
}

// 告警记录在对端、而对端看不到本实例的结论(比如它探不到本实例)时，对端那条记录会一直留着。
// 本实例只该发一次恢复，不能每轮都据此再发；对端之后开的新事件照样要认
func TestJudgeAlarmRecoverOnce(t *testing.T) {
	ctx := util.GenCtx()
	url := "https://half/"
	conf := model.Config{Urls: []string{url}, SnapshotExpireRound: 5, AlarmBackoffSec: []int{300}}
	now := int64(1700000000)
	mergePeer := func(peerTime int64, alarm model.Alarm) {
		view.Merge(ctx, "https://peer-half/api/view", model.View{Snapshots: map[string]model.Snapshot{
			model.SelfSource: {Id: "peer-half", Time: peerTime, Probes: map[string]bool{url: false}, Alarms: map[string]model.Alarm{url: alarm}},
		}})
	}

	setView(t, ctx, map[string]bool{url: true}, nil)
	mergePeer(now, model.Alarm{StartTime: now - 3600, LastSendTime: now - 60, SendCount: 2})
	_, recovers, _, dels := judgeAlarm(ctx, conf, now)
	if len(recovers) != 1 || len(dels) != 1 {
		t.Fatalf("看到对端的告警记录、本实例说在线，该发一次恢复，实际 recover=%d del=%d", len(recovers), len(dels))
	}
	view.RecoverAlarm(ctx, url, conf.SnapshotExpireRound)

	offlines, recovers, _, _ := judgeAlarm(ctx, conf, now+30)
	if len(offlines) != 0 || len(recovers) != 0 {
		t.Fatalf("对端还留着同一事件的记录，本实例不该重复发恢复，实际 offline=%d recover=%d", len(offlines), len(recovers))
	}

	mergePeer(now+200, model.Alarm{StartTime: now + 100, LastSendTime: now + 100, SendCount: 1})
	_, recovers, _, _ = judgeAlarm(ctx, conf, now+230)
	if len(recovers) != 1 {
		t.Fatalf("对端在恢复之后开的新事件要认，该再发一次恢复，实际 %d 条", len(recovers))
	}
}

// 别的实例已经发过恢复(交换过来的已恢复事件盖住了这条记录)，本实例不该再各发一遍
func TestJudgeAlarmRecoverByPeer(t *testing.T) {
	ctx := util.GenCtx()
	url := "https://recovered-by-peer/"
	conf := model.Config{Urls: []string{url}, SnapshotExpireRound: 5, AlarmBackoffSec: []int{300}}
	now := int64(1700000000)

	setView(t, ctx, map[string]bool{url: true}, map[string]model.Alarm{
		url: {StartTime: now - 600, LastSendTime: now - 60, SendCount: 2},
	})
	view.Merge(ctx, "https://recover-sender/api/view", model.View{Snapshots: map[string]model.Snapshot{
		model.SelfSource: {Id: "recover-sender", Time: now, Probes: map[string]bool{url: true}, Recovers: map[string]int64{url: now - 600}},
	}})
	offlines, recovers, _, _ := judgeAlarm(ctx, conf, now)
	if len(offlines) != 0 || len(recovers) != 0 {
		t.Fatalf("对端已经发过恢复，本实例不该再发，实际 offline=%d recover=%d", len(offlines), len(recovers))
	}
}

// 实例刚重启、明细还没攒够轮数时不给结论，不能投空头在线票：
// 否则对端还在报离线、告警记录也还在，本实例一票在线就会给一个仍然离线的URL发假恢复
func TestJudgeAlarmRestartNoFakeRecover(t *testing.T) {
	ctx := util.GenCtx()
	url := "https://restart/"
	conf := model.Config{Urls: []string{url}, OfflineRound: 3, SnapshotExpireRound: 5, AlarmBackoffSec: []int{300}}
	now := time.Now().Unix()

	//重启后第一轮：本实例只有一条失败明细
	view.SaveRecord(ctx, url, false, 600)
	view.SaveSelf(ctx, view.Converge(ctx, conf.Urls, conf.OfflineRound), nil)
	view.Merge(ctx, "https://restart-peer/api/view", model.View{Snapshots: map[string]model.Snapshot{
		model.SelfSource: {Id: "restart-peer", Time: now, Probes: map[string]bool{url: false}, Alarms: map[string]model.Alarm{url: {StartTime: now - 600, LastSendTime: now - 60, SendCount: 2}}},
	}})

	offlines, recovers, _, _ := judgeAlarm(ctx, conf, now)
	if len(recovers) != 0 {
		t.Fatalf("对端还在报离线，刚重启的本实例不该发恢复，实际 %+v", recovers)
	}
	if len(offlines) != 0 {
		t.Fatalf("对端刚发过告警，退避没到点不该重发，实际 %+v", offlines)
	}
}
