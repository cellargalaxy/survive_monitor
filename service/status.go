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

// GetStatus 看板载荷。明细窗口与资源阈值一并带上，前端健康条的时间轴、资源条的标红才能跟着配置走，而不是写死
func GetStatus(ctx context.Context) model.Status {
	conf := config.GetConfig(ctx)
	status := view.GetStatus(ctx, conf.SnapshotExpireRound, conf.RecordWindowSec)
	status.RecordWindowSec = conf.RecordWindowSec
	status.CpuUsageLimit = conf.CpuUsageLimit
	status.MemUsageLimit = conf.MemUsageLimit
	status.DiskUsageLimit = conf.DiskUsageLimit
	return status
}
