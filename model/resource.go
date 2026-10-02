package model

type Resource struct {
	Time      int64   `json:"time"`       //采集时间，秒级
	CpuNum    int     `json:"cpu_num"`    //CPU核数
	CpuUsage  float64 `json:"cpu_usage"`  //CPU使用率，按核累加，两核用满是200
	MemTotal  uint64  `json:"mem_total"`  //内存总大小，字节
	MemUsed   uint64  `json:"mem_used"`   //已用内存大小，字节
	DiskTotal uint64  `json:"disk_total"` //磁盘总大小，字节
	DiskUsed  uint64  `json:"disk_used"`  //已用磁盘大小，字节
	DiskPath  string  `json:"disk_path"`  //实际采集的磁盘路径；配置路径是内存盘时会回退成工作目录，与配置不一定相同
}

// CpuPercent 把按核累加的使用率折算成占总核数的百分比，与内存、磁盘的量纲对齐
func (this Resource) CpuPercent() float64 {
	if this.CpuNum <= 0 || this.CpuUsage <= 0 {
		return 0
	}
	return this.CpuUsage / float64(this.CpuNum)
}

func (this Resource) MemPercent() float64 {
	if this.MemTotal == 0 {
		return 0
	}
	return float64(this.MemUsed) / float64(this.MemTotal) * 100
}

func (this Resource) DiskPercent() float64 {
	if this.DiskTotal == 0 {
		return 0
	}
	return float64(this.DiskUsed) / float64(this.DiskTotal) * 100
}
