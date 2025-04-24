package main

import (
	"github.com/cellargalaxy/survive_monitor/controller"
	_ "github.com/cellargalaxy/survive_monitor/corn"
)

func main() {
	err := controller.Controller()
	if err != nil {
		panic(err)
	}
}
