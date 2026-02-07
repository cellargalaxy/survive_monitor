package controller

import (
	"net/http"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/service"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func Status(c *gin.Context) {
	logrus.WithContext(c).WithFields(logrus.Fields{"claims": util.GetClaims(c)}).Info("Status")
	c.JSON(http.StatusOK, util.NewHttpRespByErr(service.GetStatusStore().GetAllStatus(), nil))
}
