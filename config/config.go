package config

import (
	"context"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/sirupsen/logrus"
)

const (
	ConfigPath = "resource/survive_monitor.yaml"
)

const (
	probeIntervalSec  = 2     //探测间隔：2秒，每个URL之后、每轮末尾各休眠一次
	probeTimeoutSec   = 5     //单次探测超时：5秒
	offlineRound      = 3     //连续3轮失败才判离线
	snapshotExpireSec = 300   //快照过期：5分钟
	recordWindowSec   = 86400 //看板明细保留：1天
	diskPath          = "/"   //磁盘采集路径
	cpuUsageLimit     = 90    //CPU使用率告警阈值：90%
	memUsageLimit     = 90    //内存使用率告警阈值：90%
	diskUsageLimit    = 90    //磁盘使用率告警阈值：90%
	resourceRound     = 3     //资源连续3轮超阈值才告警
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

	//首次启动时配置文件由GetDefault生成，URL列表必然为空，所以这里不能报错把服务拦死
	if len(config.Urls) == 0 {
		logrus.WithContext(ctx).WithFields(logrus.Fields{}).Warn("加载配置，URL列表为空")
	}
	//单轮不设预算，两次落快照最多相隔「URL数×(单次超时+探测间隔) + 轮末探测间隔」。
	//这段间隔一旦超过快照过期时长，对端会在两轮之间把本实例当成过期，判定分母里就少了本实例这一票
	if worstSec := len(config.Urls)*(config.ProbeTimeoutSec+config.ProbeIntervalSec) + config.ProbeIntervalSec; worstSec >= config.SnapshotExpireSec {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"url": len(config.Urls), "timeout": config.ProbeTimeoutSec, "interval": config.ProbeIntervalSec, "worst": worstSec, "expire": config.SnapshotExpireSec}).Warn("加载配置，最坏轮次间隔不短于快照过期时长")
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
	if config.SnapshotExpireSec <= 0 {
		config.SnapshotExpireSec = snapshotExpireSec
	}
	if config.RecordWindowSec <= 0 {
		config.RecordWindowSec = recordWindowSec
	}
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
