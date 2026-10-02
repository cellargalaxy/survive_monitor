package handler

import (
	"context"
	"net/http"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/config"
	"github.com/cellargalaxy/survive_monitor/static"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func Init(ctx context.Context) error {
	engine := NewEngine(ctx)
	err := engine.Run(config.ListenAddress)
	if err != nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"err": err}).Error("API启动，异常")
		return errors.Errorf("API启动，异常: %+v", err)
	}
	return nil
}

func NewEngine(ctx context.Context) *gin.Engine {
	engine := gin.Default()
	engine.Use(util.GinLog)

	engine.GET(util.PathPing, util.GinPing)
	engine.GET(config.PathView, GinView)
	engine.GET(config.PathStatus, GinStatus)

	//不挂util.StaticCache：看板只有一个index.html，脚本全内联、文件名也不带版本号，
	//缓存一天的话升级之后浏览器还跑着旧页面，后端加了字段前端也看不到。文件才十几KB，每次重新拉不费事
	engine.StaticFS(util.PathStatic, http.FS(static.StaticFile))
	return engine
}
