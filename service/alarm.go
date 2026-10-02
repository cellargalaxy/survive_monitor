package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/cellargalaxy/survive_monitor/service/view"
	"github.com/sirupsen/logrus"
)

// alarmUrl 把本轮该发的离线与恢复合并成一条消息。
// 发送成功才写回告警记录，失败就留着下一轮重新判定、重新发，这就是全部的重试逻辑
func alarmUrl(ctx context.Context, conf model.Config) {
	now := time.Now().Unix()
	offlineTexts, recoverTexts, saves, dels := judgeAlarm(ctx, conf, now)
	if len(offlineTexts) == 0 && len(recoverTexts) == 0 {
		return
	}
	var sections []string
	if len(offlineTexts) > 0 {
		sections = append(sections, fmt.Sprintf("服务离线\n%s", strings.Join(offlineTexts, "\n")))
	}
	if len(recoverTexts) > 0 {
		sections = append(sections, fmt.Sprintf("服务恢复\n%s", strings.Join(recoverTexts, "\n")))
	}
	err := util.SendWxMsg(ctx, "", strings.Join(sections, "\n\n"))
	if err != nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"offline": len(offlineTexts), "recover": len(recoverTexts), "err": err}).Warn("发送服务告警，异常")
		return
	}

	for url := range saves {
		view.SaveAlarm(ctx, url, saves[url])
	}
	for i := range dels {
		view.RecoverAlarm(ctx, dels[i], conf.SnapshotExpireRound)
	}
	logrus.WithContext(ctx).WithFields(logrus.Fields{"offline": len(offlineTexts), "recover": len(recoverTexts)}).Info("发送服务告警，完成")
}

// judgeAlarm 决定本轮该发什么、发完要怎么写记录，不碰发送本身。
// 离线要全体一致才算，恢复只要有一个实例说在线就算，这对不对称是故意的：
// 前者保守是为了不误报，后者宽松是为了别让一个抖动的实例把恢复通知卡死
func judgeAlarm(ctx context.Context, conf model.Config, now int64) ([]string, []string, map[string]model.Alarm, []string) {
	var offlineTexts []string
	var recoverTexts []string
	saves := make(map[string]model.Alarm)
	var dels []string

	for i := range conf.Urls {
		url := conf.Urls[i]
		offline, online, count := view.Judge(ctx, url, conf.SnapshotExpireRound)
		//记录是全局的，对端已经发过就轮不到自己再发一遍
		alarm, exist := view.GetAlarm(ctx, url, conf.SnapshotExpireRound)

		if offline {
			if exist && now-alarm.LastSendTime < int64(getBackoff(conf.AlarmBackoffSec, alarm.SendCount)) {
				continue
			}
			startTime := now
			sendCount := 1
			if exist {
				startTime = alarm.StartTime
				sendCount = alarm.SendCount + 1
			}
			offlineTexts = append(offlineTexts, fmt.Sprintf("%s (%d个实例确认，始于%s)", url, count, util.Unix2Str(ctx, util.DateLayout_2006_01_02_15_04_05, startTime, nil)))
			saves[url] = model.Alarm{StartTime: startTime, LastSendTime: now, SendCount: sendCount}
			continue
		}
		if online && exist {
			recoverTexts = append(recoverTexts, fmt.Sprintf("%s (离线%s)", url, getDuration(now-alarm.StartTime)))
			dels = append(dels, url)
		}
	}
	return offlineTexts, recoverTexts, saves, dels
}

// alarmResource 本机资源的阈值告警。资源是各实例自己采的，也就只有自己有资格判它持续超了几轮，
// 别的服务器超没超由它自己那个实例去告警。
// 告警与恢复都要连续resource_round轮才算：只要求告警攒轮数的话，卡在阈值附近抖动时会告警、恢复来回刷，退避也压不住，
// 因为恢复会把记录删掉，下一次告警又从首发算起。
// complete是本轮各项是否都采到了：采不到的项是零值，零值不会误报超阈值，却会误报恢复，所以没采全时不据此判恢复
func alarmResource(ctx context.Context, conf model.Config, resource model.Resource, complete bool) {
	now := time.Now().Unix()
	overTexts := getOverTexts(conf, resource)
	if len(overTexts) == 0 && !complete {
		logrus.WithContext(ctx).WithFields(logrus.Fields{}).Warn("本机资源没采全，本轮不判恢复")
		return
	}
	round := view.MarkResourceOver(ctx, len(overTexts) > 0)
	alarm, exist := view.GetResourceAlarm(ctx)

	if len(overTexts) == 0 {
		if !exist {
			return
		}
		if round < conf.ResourceRound {
			logrus.WithContext(ctx).WithFields(logrus.Fields{"round": round, "limit": conf.ResourceRound}).Info("本机资源回落到阈值以下，尚未攒够轮数")
			return
		}
		err := util.SendWxMsg(ctx, "", fmt.Sprintf("资源恢复\n%s", getResourceText(resource)))
		if err != nil {
			logrus.WithContext(ctx).WithFields(logrus.Fields{"err": err}).Warn("发送资源告警，异常")
			return
		}
		view.DelResourceAlarm(ctx)
		logrus.WithContext(ctx).WithFields(logrus.Fields{}).Info("发送资源告警，恢复完成")
		return
	}

	if round < conf.ResourceRound {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"round": round, "limit": conf.ResourceRound}).Warn("本机资源超阈值，尚未攒够轮数")
		return
	}
	if exist && now-alarm.LastSendTime < int64(getBackoff(conf.AlarmBackoffSec, alarm.SendCount)) {
		return
	}
	startTime := now
	sendCount := 1
	if exist {
		startTime = alarm.StartTime
		sendCount = alarm.SendCount + 1
	}
	err := util.SendWxMsg(ctx, "", fmt.Sprintf("资源超阈值\n%s", strings.Join(overTexts, "\n")))
	if err != nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"err": err}).Warn("发送资源告警，异常")
		return
	}
	view.SaveResourceAlarm(ctx, model.Alarm{StartTime: startTime, LastSendTime: now, SendCount: sendCount})
	logrus.WithContext(ctx).WithFields(logrus.Fields{"over": len(overTexts)}).Info("发送资源告警，完成")
}

func getOverTexts(conf model.Config, resource model.Resource) []string {
	var texts []string
	if percent := resource.CpuPercent(); percent >= conf.CpuUsageLimit {
		texts = append(texts, fmt.Sprintf("CPU %.1f%% (阈值%.1f%%，共%d核)", percent, conf.CpuUsageLimit, resource.CpuNum))
	}
	if percent := resource.MemPercent(); percent >= conf.MemUsageLimit {
		texts = append(texts, fmt.Sprintf("内存 %.1f%% (阈值%.1f%%)", percent, conf.MemUsageLimit))
	}
	if percent := resource.DiskPercent(); percent >= conf.DiskUsageLimit {
		texts = append(texts, fmt.Sprintf("磁盘 %.1f%% (阈值%.1f%%，路径%s)", percent, conf.DiskUsageLimit, resource.DiskPath))
	}
	return texts
}

func getResourceText(resource model.Resource) string {
	return fmt.Sprintf("CPU %.1f%%，内存 %.1f%%，磁盘 %.1f%%", resource.CpuPercent(), resource.MemPercent(), resource.DiskPercent())
}

// getBackoff 取退避阶梯上第sendCount次发送该等多久，走到末阶之后就一直保持末阶
func getBackoff(steps []int, sendCount int) int {
	if len(steps) == 0 {
		return 0
	}
	index := sendCount - 1
	if index < 0 {
		index = 0
	}
	if index >= len(steps) {
		index = len(steps) - 1
	}
	return steps[index]
}

func getDuration(second int64) string {
	if second < 0 {
		second = 0
	}
	if second < 60 {
		return fmt.Sprintf("%d秒", second)
	}
	if second < 3600 {
		return fmt.Sprintf("%d分钟", second/60)
	}
	if second < 86400 {
		return fmt.Sprintf("%d小时%d分钟", second/3600, second%3600/60)
	}
	return fmt.Sprintf("%d天%d小时", second/86400, second%86400/3600)
}
