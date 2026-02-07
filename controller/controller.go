package controller

import (
	"fmt"
	"net/http"
	"strings"

	common_model "github.com/cellargalaxy/go_common/model"
	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/cellargalaxy/survive_monitor/static"
	"github.com/gin-gonic/gin"
)

func Controller() error {
	engine := gin.Default()
	engine.Use(util.GinLog)

	engine.GET(common_model.PingPath, util.Ping)

	engine.Use(staticCache)
	engine.StaticFS(common_model.StaticPath, http.FS(static.StaticFile))

	engine.GET(model.StatusPath, Status)

	err := engine.Run(model.ListenAddress)
	if err != nil {
		panic(fmt.Errorf("web服务启动，异常: %+v", err))
	}
	return nil
}

func staticCache(c *gin.Context) {
	if strings.HasPrefix(c.Request.RequestURI, common_model.StaticPath) {
		c.Header("Cache-Control", "max-age=86400")
	}
}
