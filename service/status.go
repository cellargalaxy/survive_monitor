package service

import (
	"context"

	"github.com/cellargalaxy/survive_monitor/config"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/cellargalaxy/survive_monitor/service/view"
)

// GetView 交换载荷。实例之间互相拉这个，顺带就把对方探活了
func GetView(ctx context.Context) model.View {
	return view.GetView(ctx, config.GetConfig(ctx).SnapshotExpireRound)
}

// GetStatus 看板载荷。明细窗口一并带上，前端健康条的时间轴才能跟着record_window_sec走，而不是写死
func GetStatus(ctx context.Context) model.Status {
	conf := config.GetConfig(ctx)
	status := view.GetStatus(ctx, conf.SnapshotExpireRound)
	status.RecordWindowSec = conf.RecordWindowSec
	return status
}
