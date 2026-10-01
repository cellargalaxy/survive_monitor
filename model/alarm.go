package model

type Alarm struct {
	StartTime    int64 `json:"start_time"`     //事件起始时间，秒级；恢复后记录被删，再次离线就是新事件
	LastSendTime int64 `json:"last_send_time"` //上次发送时间，秒级
	SendCount    int   `json:"send_count"`     //已发送次数，决定退避走到第几阶
}
