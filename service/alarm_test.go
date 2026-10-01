package service

import (
	"context"
	"os"
	"strings"
	"testing"

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

	edge := getOverTexts(conf, model.Resource{CpuNum: 2, CpuUsage: 180, MemTotal: 100, MemUsed: 90, DiskTotal: 100, DiskUsed: 90})
	if len(edge) != 3 {
		t.Errorf("三项都正好卡在阈值上应全部命中，实际 %d 条: %+v", len(edge), edge)
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
	conf := model.Config{Urls: []string{"https://dead/"}, SnapshotExpireSec: 300, AlarmBackoffSec: []int{300, 1800}}
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
	conf := model.Config{Urls: []string{"https://back/"}, SnapshotExpireSec: 300, AlarmBackoffSec: []int{300}}
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
	view.DelAlarm(ctx, "https://back/")
	setView(t, ctx, map[string]bool{"https://back/": true}, nil)
	_, recovers, _, _ = judgeAlarm(ctx, conf, now)
	if len(recovers) != 0 {
		t.Fatalf("没告警过不该发恢复，实际 %+v", recovers)
	}
}

// 没有任何新鲜结论时保持现状，既不告警也不恢复
func TestJudgeAlarmNoConclusion(t *testing.T) {
	ctx := util.GenCtx()
	conf := model.Config{Urls: []string{"https://unknown/"}, SnapshotExpireSec: 300, AlarmBackoffSec: []int{300}}

	view.DelAlarm(ctx, "https://unknown/")
	setView(t, ctx, map[string]bool{"https://other/": false}, nil)
	offlines, recovers, _, _ := judgeAlarm(ctx, conf, 1700000000)
	if len(offlines) != 0 || len(recovers) != 0 {
		t.Fatalf("没结论该什么都不发，实际 offline=%+v recover=%+v", offlines, recovers)
	}
}
