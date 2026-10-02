package model

// SelfSource 本实例快照在交换载荷里的身份键。产出时留空，由接收方回填成「我从哪个URL拿到的」，
// 所以本实例无须知道自己对外是哪个URL，配置里也就不必多一项
const SelfSource = ""

type Snapshot struct {
	Id            string           `json:"id"`             //实例标识，启动时生成；用来认出被对端转发回来的自己，否则同一份观测会在判定分母里投两票
	Source        string           `json:"source"`         //身份键，见SelfSource
	Time          int64            `json:"time"`           //原始产生时间，秒级；转发链路上只读不改，否则挂掉实例的旧结论会永远新鲜
	Probes        map[string]bool  `json:"probes"`         //URL -> 本实例是否认为它在线，已经是连续多轮收敛后的结论
	Resource      *Resource        `json:"resource"`       //本机资源，采集不到为空
	Alarms        map[string]Alarm `json:"alarms"`         //URL -> 离线告警发送记录
	ResourceAlarm *Alarm           `json:"resource_alarm"` //本机资源告警发送记录
}

type View struct {
	Snapshots map[string]Snapshot `json:"snapshots"` //身份键 -> 快照
}

type Status struct {
	View            View                `json:"view"`              //全局视图
	Records         map[string][]Record `json:"records"`           //URL -> 探测明细，只有本实例的观测，各实例之间不要求一致
	RecordWindowSec int                 `json:"record_window_sec"` //明细保留窗口，秒；看板健康条按它铺满，跟着配置走
}

type Record struct {
	Time  int64 `json:"time"`  //探测时间，秒级
	Alive bool  `json:"alive"` //是否在线
}
