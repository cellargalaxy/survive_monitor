package config

import (
	"context"
	"strings"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/sirupsen/logrus"
)

const (
	ConfigPath = "resource/survive_monitor.yaml"
)

const (
	probeIntervalSec    = 2      //探测间隔：2秒，每个URL之后、每轮末尾各休眠一次
	probeTimeoutSec     = 5      //单次探测超时：5秒
	offlineRound        = 3      //连续3轮失败才判离线
	snapshotExpireRound = 5      //快照过期：连续5轮拿不到新数据
	recordWindowSec     = 604800 //看板明细保留：7天
	diskPath            = "/"    //磁盘采集路径
	cpuUsageLimit       = 90     //CPU使用率告警阈值：90%
	memUsageLimit       = 90     //内存使用率告警阈值：90%
	diskUsageLimit      = 90     //磁盘使用率告警阈值：90%
	resourceRound       = 3      //资源连续3轮超阈值才告警，连续3轮不超才恢复
)

// alarmBackoffSec 告警退避阶梯：首发之后5分钟、30分钟、2小时，之后保持2小时
var alarmBackoffSec = []int{300, 1800, 7200}

var configService *util.ConfigService[model.Config]

func init() {
	ctx := util.GenCtx()
	configService = util.NewConfigService[model.Config](new(ConfigHandler))
	err := configService.Start(ctx)
	if err != nil {
		panic(err)
	}
}

func GetConfig(ctx context.Context) model.Config {
	return configService.GetConfig(ctx)
}

type ConfigHandler struct {
}

func (this *ConfigHandler) GetPath(ctx context.Context) string {
	return ConfigPath
}
func (this *ConfigHandler) GetDefault(ctx context.Context) string {
	var config model.Config
	fillConfig(&config)
	return util.YamlStruct2Str(ctx, config)
}
func (this *ConfigHandler) Parse(ctx context.Context, text string) (model.Config, error) {
	var config model.Config
	err := util.YamlStr2Struct(ctx, text, &config)
	if err != nil {
		return config, err
	}
	fillConfig(&config)
	config.Urls = cleanUrls(config.Urls)
	config.BoardUrl = strings.TrimSpace(config.BoardUrl)

	//首次启动时配置文件由GetDefault生成，URL列表必然为空，所以这里不能报错把服务拦死
	if len(config.Urls) == 0 {
		logrus.WithContext(ctx).WithFields(logrus.Fields{}).Warn("加载配置，URL列表为空")
	}
	if expireRoundTooSmall(config) {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"snapshotExpireRound": config.SnapshotExpireRound, "probeIntervalSec": config.ProbeIntervalSec, "probeTimeoutSec": config.ProbeTimeoutSec}).Warn("加载配置，快照过期轮数偏小，对端探测变慢时会被误判过期")
	}
	logrus.WithContext(ctx).WithFields(logrus.Fields{"config": config}).Info("加载配置")
	return config, nil
}

func fillConfig(config *model.Config) {
	if config.ProbeIntervalSec <= 0 {
		config.ProbeIntervalSec = probeIntervalSec
	}
	if config.ProbeTimeoutSec <= 0 {
		config.ProbeTimeoutSec = probeTimeoutSec
	}
	if config.OfflineRound <= 0 {
		config.OfflineRound = offlineRound
	}
	if config.SnapshotExpireRound <= 0 {
		config.SnapshotExpireRound = snapshotExpireRound
	}
	if config.RecordWindowSec <= 0 {
		config.RecordWindowSec = recordWindowSec
	}
	config.AlarmBackoffSec = cleanBackoff(config.AlarmBackoffSec)
	if len(config.AlarmBackoffSec) == 0 {
		config.AlarmBackoffSec = alarmBackoffSec
	}
	if config.DiskPath == "" {
		config.DiskPath = diskPath
	}
	if config.CpuUsageLimit <= 0 {
		config.CpuUsageLimit = cpuUsageLimit
	}
	if config.MemUsageLimit <= 0 {
		config.MemUsageLimit = memUsageLimit
	}
	if config.DiskUsageLimit <= 0 {
		config.DiskUsageLimit = diskUsageLimit
	}
	if config.ResourceRound <= 0 {
		config.ResourceRound = resourceRound
	}
}

// expireRoundTooSmall 快照过期轮数是否偏小。过期按本实例的轮数数，可对端一轮多长由对端自己的探测快慢决定：
// URL数多时，URL全都超时的对端，一轮约是URL全都秒回的本实例的(间隔+超时)/间隔倍，对端也就隔这么多轮才产出一份新快照。
// 过期轮数不比这个倍数大，对端每轮都会在本实例这里过期一次，判定分母里时有时无，离线告警、恢复跟着来回刷
func expireRoundTooSmall(config model.Config) bool {
	return config.SnapshotExpireRound*config.ProbeIntervalSec <= config.ProbeIntervalSec+config.ProbeTimeoutSec
}

// cleanUrls 去掉首尾空白、空项与重复项，保持原有顺序。
// 重复的URL每轮会被探两次，明细攒得快一倍，连续失败轮数就名不副实，告警文案里也会列两遍；空项则每轮必然探测失败，凭空报一条离线
func cleanUrls(urls []string) []string {
	exist := make(map[string]bool, len(urls))
	list := make([]string, 0, len(urls))
	for i := range urls {
		url := strings.TrimSpace(urls[i])
		if url == "" || exist[url] {
			continue
		}
		exist[url] = true
		list = append(list, url)
	}
	return list
}

// cleanBackoff 去掉不为正的阶。退避为0等于不退避，持续离线时每轮都发一条，几十秒一条地刷屏
func cleanBackoff(steps []int) []int {
	list := make([]int, 0, len(steps))
	for i := range steps {
		if steps[i] > 0 {
			list = append(list, steps[i])
		}
	}
	return list
}
