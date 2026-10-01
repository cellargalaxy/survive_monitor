package handler

import (
	"net/http"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/service"
	"github.com/gin-gonic/gin"
)

func GinStatus(c *gin.Context) {
	ctx := c.Request.Context()

	c.JSON(http.StatusOK, util.NewHttpRespByErr(service.GetStatus(ctx), nil))
}
