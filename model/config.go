package model

import (
	"github.com/cellargalaxy/go_common/util"
)

const (
	DefaultServerName = "survive_monitor"
	ListenAddress     = ":4343"
)

func init() {
	util.Init(DefaultServerName)
}

type Config struct {
	BoardUrl string   `yaml:"board_url" json:"board_url"`
	Cron     string   `yaml:"cron" json:"cron"`
	Urls     []string `yaml:"urls" json:"urls"`
}

func (this Config) String() string {
	return util.JsonStruct2String(this)
}
