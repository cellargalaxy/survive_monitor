package main

import (
	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/corn"
	"github.com/cellargalaxy/survive_monitor/handler"
)

func main() {
	ctx := util.GenCtx()
	err := corn.Init(ctx)
	if err != nil {
		panic(err)
	}
	err = handler.Init(ctx)
	if err != nil {
		panic(err)
	}
}
