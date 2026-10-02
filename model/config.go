package model

import (
	"github.com/cellargalaxy/go_common/util"
)

const serverName = "survive_monitor"

func init() {
	util.Init(serverName)
}

type Config struct {
	Urls                []string `json:"urls" yaml:"urls"`                                   //探测的URL列表，所有实例共用同一份
	ProbeIntervalSec    int      `json:"probe_interval_sec" yaml:"probe_interval_sec"`       //探测间隔，秒；每探完一个URL、每轮采完资源之后各休眠这么久，每轮现读，改完不用重启
	ProbeTimeoutSec     int      `json:"probe_timeout_sec" yaml:"probe_timeout_sec"`         //单次探测超时，秒
	OfflineRound        int      `json:"offline_round" yaml:"offline_round"`                 //连续失败多少轮，本实例才判定该URL离线
	SnapshotExpireRound int      `json:"snapshot_expire_round" yaml:"snapshot_expire_round"` //快照过期轮数；某身份键的快照之后连续这么多轮都没拿到更新的快照就算过期，过期的不参与判定
	RecordWindowSec     int      `json:"record_window_sec" yaml:"record_window_sec"`         //看板明细保留窗口，秒
	AlarmBackoffSec     []int    `json:"alarm_backoff_sec" yaml:"alarm_backoff_sec"`         //告警退避阶梯，秒；持续告警按阶梯逐级降频，走到末阶后保持末阶
	DiskPath            string   `json:"disk_path" yaml:"disk_path"`                         //磁盘采集路径
	CpuUsageLimit       float64  `json:"cpu_usage_limit" yaml:"cpu_usage_limit"`             //CPU使用率告警阈值，占总核数的百分比
	MemUsageLimit       float64  `json:"mem_usage_limit" yaml:"mem_usage_limit"`             //内存使用率告警阈值，百分比
	DiskUsageLimit      float64  `json:"disk_usage_limit" yaml:"disk_usage_limit"`           //磁盘使用率告警阈值，百分比
	ResourceRound       int      `json:"resource_round" yaml:"resource_round"`               //资源连续超阈值多少轮才告警；恢复同样要连续这么多轮不超，免得在阈值附近抖动时告警、恢复来回刷
	BoardUrl            string   `json:"board_url" yaml:"board_url"`                         //看板地址，作为微信消息的跳转链接，点开消息直达看板；允许为空，为空时消息不带跳转
}

func (this Config) String() string {
	return util.JsonStruct2Str(this)
}
