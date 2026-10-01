package config

const (
	ListenAddress = ":4343"
)

const (
	PathView   = "/api/view"   //实例之间交换全局视图，配置里指向别的实例时就填这个路径
	PathStatus = "/api/status" //看板数据，只给前端用
)
