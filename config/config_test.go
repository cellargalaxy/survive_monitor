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

// 空项、重复项、首尾空白都要在加载时洗掉，否则重复的URL一轮探两次、空项每轮凭空报离线
func TestParseCleanUrls(t *testing.T) {
	ctx := util.GenCtx()
	handler := new(ConfigHandler)

	conf, err := handler.Parse(ctx, "urls:\n  - https://a/\n  - ' https://b/ '\n  - ''\n  - https://a/\n")
	if err != nil {
		t.Fatalf("应能解析: %+v", err)
	}
	if len(conf.Urls) != 2 || conf.Urls[0] != "https://a/" || conf.Urls[1] != "https://b/" {
		t.Fatalf("URL期望 [https://a/ https://b/]，实际 %+v", conf.Urls)
	}
}

// 退避阶梯里不为正的阶要去掉，全都不为正就回落到默认值，否则持续离线时每轮都发
func TestParseCleanBackoff(t *testing.T) {
	ctx := util.GenCtx()
	handler := new(ConfigHandler)

	conf, err := handler.Parse(ctx, "alarm_backoff_sec: [0, 600, -1, 3600]\n")
	if err != nil {
		t.Fatalf("应能解析: %+v", err)
	}
	if len(conf.AlarmBackoffSec) != 2 || conf.AlarmBackoffSec[0] != 600 || conf.AlarmBackoffSec[1] != 3600 {
		t.Fatalf("退避阶梯期望 [600 3600]，实际 %+v", conf.AlarmBackoffSec)
	}

	conf, err = handler.Parse(ctx, "alarm_backoff_sec: [0]\n")
	if err != nil {
		t.Fatalf("应能解析: %+v", err)
	}
	if len(conf.AlarmBackoffSec) != len(alarmBackoffSec) {
		t.Fatalf("全都不为正该回落到默认值，实际 %+v", conf.AlarmBackoffSec)
	}
}

// 默认配置不能触发过期轮数偏小的提示；超时拉长到间隔的好几倍、过期轮数又没跟着调，就该提示
func TestExpireRoundTooSmall(t *testing.T) {
	ctx := util.GenCtx()
	handler := new(ConfigHandler)

	conf, err := handler.Parse(ctx, handler.GetDefault(ctx))
	if err != nil {
		t.Fatalf("默认配置应能解析: %+v", err)
	}
	if expireRoundTooSmall(conf) {
		t.Fatalf("默认配置不该提示过期轮数偏小: %+v", conf)
	}

	conf, err = handler.Parse(ctx, "probe_interval_sec: 2\nprobe_timeout_sec: 10\nsnapshot_expire_round: 5\n")
	if err != nil {
		t.Fatalf("应能解析: %+v", err)
	}
	if !expireRoundTooSmall(conf) {
		t.Fatalf("对端一轮最慢是本实例的6倍，过期轮数5该提示偏小: %+v", conf)
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
