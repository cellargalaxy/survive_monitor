package service

import (
	"context"
	"fmt"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/msg_gateway/sdk"
	"github.com/cellargalaxy/survive_monitor/config"
	"github.com/sirupsen/logrus"
)

func MonitorConfig(ctx context.Context) {
	urls := config.Config.Urls
	for _, url := range urls {
		MonitorAndAlarm(ctx, url)
	}
}
func MonitorAndAlarm(ctx context.Context, url string) bool {
	ok := MonitorSurvive(ctx, url)
	if ok {
		return ok
	}
	sdk.SendTemplateText(ctx, "通用消息", config.Config.BoardUrl, fmt.Sprintf("服务离线：%s", url))
	return ok
}
func MonitorSurvive(ctx context.Context, url string) bool {
	for i := 0; i < 5; i++ {
		ok := monitorSurvive(ctx, url)
		if ok {
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}
func monitorSurvive(ctx context.Context, url string) bool {
	response, err := util.GetHttpSpiderRequest(ctx).Get(url)
	if err != nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "err": err}).Error("检测存活，请求异常")
		return false
	}
	if response == nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "err": err}).Error("检测存活，响应为空")
		return false
	}
	statusCode := response.StatusCode()
	logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "statusCode": statusCode}).Info("检测存活，响应")
	return statusCode > 0 && statusCode < 500
}
