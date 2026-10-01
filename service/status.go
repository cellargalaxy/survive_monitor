package service

import (
	"context"

	"github.com/cellargalaxy/survive_monitor/config"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/cellargalaxy/survive_monitor/service/view"
)

// GetView 交换载荷。实例之间互相拉这个，顺带就把对方探活了
func GetView(ctx context.Context) model.View {
	return view.GetView(ctx, config.GetConfig(ctx).SnapshotExpireSec)
}

// GetStatus 看板载荷
func GetStatus(ctx context.Context) model.Status {
	return view.GetStatus(ctx, config.GetConfig(ctx).SnapshotExpireSec)
}
