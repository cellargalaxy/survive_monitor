package config

import (
	"os"
	"testing"

	"github.com/cellargalaxy/go_common/util"
	"github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.WarnLevel)
	code := m.Run()
	os.RemoveAll("resource")
	os.RemoveAll("log")
	os.Exit(code)
}

// 配置项全开放给用户填，所以每一项都得有默认值兜底，少填一项不能让服务跑出零值行为
func TestGetConfigDefault(t *testing.T) {
	ctx := util.GenCtx()
	conf := GetConfig(ctx)

	if conf.ProbeIntervalSec != probeIntervalSec {
		t.Errorf("探测间隔期望 %d，实际 %d", probeIntervalSec, conf.ProbeIntervalSec)
	}
	if conf.ProbeBudgetSec != probeBudgetSec {
		t.Errorf("单轮预算期望 %d，实际 %d", probeBudgetSec, conf.ProbeBudgetSec)
	}
	if conf.ProbeTimeoutSec != probeTimeoutSec {
		t.Errorf("单次超时期望 %d，实际 %d", probeTimeoutSec, conf.ProbeTimeoutSec)
	}
	if conf.OfflineRound != offlineRound {
		t.Errorf("离线轮次期望 %d，实际 %d", offlineRound, conf.OfflineRound)
	}
	if conf.DiskPath != diskPath {
		t.Errorf("磁盘路径期望 %s，实际 %s", diskPath, conf.DiskPath)
	}
	if len(conf.AlarmBackoffSec) != len(alarmBackoffSec) {
		t.Errorf("退避阶梯期望 %d 阶，实际 %d 阶", len(alarmBackoffSec), len(conf.AlarmBackoffSec))
	}

	//单轮预算必须短于探测间隔，否则一轮没跑完下一轮就该发车了
	if conf.ProbeBudgetSec >= conf.ProbeIntervalSec {
		t.Errorf("单轮预算 %d 不该不短于探测间隔 %d", conf.ProbeBudgetSec, conf.ProbeIntervalSec)
	}
}

func TestParseFillDefault(t *testing.T) {
	ctx := util.GenCtx()
	handler := new(ConfigHandler)

	conf, err := handler.Parse(ctx, "urls:\n  - https://a/\n")
	if err != nil {
		t.Fatalf("只填URL应能解析: %+v", err)
	}
	if len(conf.Urls) != 1 || conf.Urls[0] != "https://a/" {
		t.Fatalf("URL期望 [https://a/]，实际 %+v", conf.Urls)
	}
	if conf.ProbeIntervalSec != probeIntervalSec || conf.ResourceRound != resourceRound {
		t.Fatalf("没填的项该被兜底，实际 interval=%d resourceRound=%d", conf.ProbeIntervalSec, conf.ResourceRound)
	}

	//URL为空只能告警不能报错：首次启动时配置文件由GetDefault生成，URL列表必然是空的
	conf, err = handler.Parse(ctx, handler.GetDefault(ctx))
	if err != nil {
		t.Fatalf("默认配置必须能解析，否则服务一起来就panic: %+v", err)
	}
	if len(conf.Urls) != 0 {
		t.Fatalf("默认配置的URL列表该是空的，实际 %+v", conf.Urls)
	}
}

func TestParseIllegal(t *testing.T) {
	ctx := util.GenCtx()
	handler := new(ConfigHandler)

	_, err := handler.Parse(ctx, "urls: 这不是列表\n\tbad indent")
	if err == nil {
		t.Fatalf("非法YAML应报错")
	}
}
