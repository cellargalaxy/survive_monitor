package model

import (
	"github.com/cellargalaxy/go_common/util"
)

const serverName = "survive_monitor"

func init() {
	util.Init(serverName)
}

type Config struct {
	Urls              []string `json:"urls" yaml:"urls"`                               //探测的URL列表，所有实例共用同一份
	ProbeIntervalSec  int      `json:"probe_interval_sec" yaml:"probe_interval_sec"`   //探测间隔，秒；服务启动时读一次，改完要重启
	ProbeBudgetSec    int      `json:"probe_budget_sec" yaml:"probe_budget_sec"`       //单轮探测预算，秒；超预算未完成的URL按失败计
	ProbeTimeoutSec   int      `json:"probe_timeout_sec" yaml:"probe_timeout_sec"`     //单次探测超时，秒
	ProbeConcurrency  int      `json:"probe_concurrency" yaml:"probe_concurrency"`     //探测并发数
	OfflineRound      int      `json:"offline_round" yaml:"offline_round"`             //连续失败多少轮，本实例才判定该URL离线
	SnapshotExpireSec int      `json:"snapshot_expire_sec" yaml:"snapshot_expire_sec"` //快照过期时长，秒；过期的不参与判定
	RecordWindowSec   int      `json:"record_window_sec" yaml:"record_window_sec"`     //看板明细保留窗口，秒
	AlarmBackoffSec   []int    `json:"alarm_backoff_sec" yaml:"alarm_backoff_sec"`     //告警退避阶梯，秒；持续告警按阶梯逐级降频，走到末阶后保持末阶
	DiskPath          string   `json:"disk_path" yaml:"disk_path"`                     //磁盘采集路径
	CpuUsageLimit     float64  `json:"cpu_usage_limit" yaml:"cpu_usage_limit"`         //CPU使用率告警阈值，占总核数的百分比
	MemUsageLimit     float64  `json:"mem_usage_limit" yaml:"mem_usage_limit"`         //内存使用率告警阈值，百分比
	DiskUsageLimit    float64  `json:"disk_usage_limit" yaml:"disk_usage_limit"`       //磁盘使用率告警阈值，百分比
	ResourceRound     int      `json:"resource_round" yaml:"resource_round"`           //资源连续超阈值多少轮才告警
}

func (this Config) String() string {
	return util.JsonStruct2Str(this)
}
