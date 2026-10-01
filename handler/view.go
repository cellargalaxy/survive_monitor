package handler

import (
	"net/http"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/service"
	"github.com/gin-gonic/gin"
)

// GinView 实例之间交换全局视图。不做鉴权：交换的内容本身不敏感，而加了校验反而会在凭据不一致时把互相监听掐断
func GinView(c *gin.Context) {
	ctx := c.Request.Context()

	c.JSON(http.StatusOK, util.NewHttpRespByErr(service.GetView(ctx), nil))
}
