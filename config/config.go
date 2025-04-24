package config

import (
	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/sirupsen/logrus"
)

var Config = model.Config{}

func init() {
	ctx := util.GenCtx()

	text, err := util.ReadFile2String(ctx, "survive_monitor.yaml", "")
	if err != nil {
		panic(err)
	}

	var config model.Config
	err = util.YamlString2Struct(ctx, text, &config)
	if err != nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"err": err}).Error("加载配置，反序列化异常")
		panic(err)
	}

	Config = config
	logrus.WithContext(ctx).WithFields(logrus.Fields{"Config": Config}).Info("加载配置")
}
