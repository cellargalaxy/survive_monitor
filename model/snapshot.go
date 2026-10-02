package model

// SelfSource 本实例快照在交换载荷里的身份键。产出时留空，由接收方回填成「我从哪个URL拿到的」，
// 所以本实例无须知道自己对外是哪个URL，配置里也就不必多一项
const SelfSource = ""

type Snapshot struct {
	Id            string           `json:"id"`             //实例标识，启动时生成；用来认出被对端转发回来的自己，否则同一份观测会在判定分母里投两票
	Source        string           `json:"source"`         //身份键，见SelfSource
	Time          int64            `json:"time"`           //原始产生时间，秒级；本实例的快照内容每变一次就严格递增一次，转发链路上只读不改，否则挂掉实例的旧结论会永远新鲜
	Probes        map[string]bool  `json:"probes"`         //URL -> 本实例是否认为它在线，已经是连续多轮收敛后的结论
	Resource      *Resource        `json:"resource"`       //本机资源，采集不到为空
	Alarms        map[string]Alarm `json:"alarms"`         //URL -> 离线告警发送记录
	Recovers      map[string]int64 `json:"recovers"`       //URL -> 本实例发过恢复通知的事件的起始时间；起始时间不晚于它的告警记录都是已恢复的旧事件，任何实例都不再据此告警或恢复
	ResourceAlarm *Alarm           `json:"resource_alarm"` //本机资源告警发送记录
}

type View struct {
	Snapshots map[string]Snapshot `json:"snapshots"` //身份键 -> 快照
}

// 健康条每一格的状态
const (
	BarUnknown = 0 //这一格没有明细
	BarOnline  = 1 //这一格的探测全部成功
	BarOffline = 2 //这一格至少失败过一次
)

type Status struct {
	View            View             `json:"view"`              //全局视图
	Bars            map[string][]int `json:"bars"`              //URL -> 健康条，明细保留窗口等分成若干格，取值见BarUnknown等；只有本实例的观测，各实例之间不要求一致
	RecordWindowSec int              `json:"record_window_sec"` //明细保留窗口，秒；看板健康条按它铺满，跟着配置走
	CpuUsageLimit   float64          `json:"cpu_usage_limit"`   //资源告警阈值，看板资源条按它标红，跟着配置走
	MemUsageLimit   float64          `json:"mem_usage_limit"`
	DiskUsageLimit  float64          `json:"disk_usage_limit"`
}

type Record struct {
	Time  int64 `json:"time"`  //探测时间，秒级
	Alive bool  `json:"alive"` //是否在线
}
