package controller

import (
	"fmt"
	common_model "github.com/cellargalaxy/go_common/model"
	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/gin-gonic/gin"
)

func Controller() error {
	engine := gin.Default()
	engine.Use(util.GinLog)

	engine.GET(common_model.PingPath, util.Ping)

	err := engine.Run(model.ListenAddress)
	if err != nil {
		panic(fmt.Errorf("web服务启动，异常: %+v", err))
	}
	return nil
}
