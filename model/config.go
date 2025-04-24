package model

import (
	"github.com/cellargalaxy/go_common/util"
)

const (
	DefaultServerName = "survive_monitor"
	ListenAddress     = ":4343"
)

type Config struct {
	Urls []string `yaml:"urls" json:"urls"`
}

func (this Config) String() string {
	return util.JsonStruct2String(this)
}
