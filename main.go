package main

import (
	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/controller"
	_ "github.com/cellargalaxy/survive_monitor/corn"
	"github.com/cellargalaxy/survive_monitor/model"
)

func init() {
	util.Init(model.DefaultServerName)
}

func main() {
	err := controller.Controller()
	if err != nil {
		panic(err)
	}
}
