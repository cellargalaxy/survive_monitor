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

// wxTextLimit 微信模板消息正文最多这么多个字，并且不允许换行。
// 正文只放摘要，点开消息跳到看板看全貌；完整明细写进日志，凭消息里的日志ID查
const wxTextLimit = 20

// alarmUrl 把本轮该发的离线与恢复合并成一条消息。
// 发送成功才写回告警记录，失败就留着下一轮重新判定、重新发，这就是全部的重试逻辑
func alarmUrl(ctx context.Context, conf model.Config) {
	now := time.Now().Unix()
	offlineTexts, recoverTexts, saves, dels := judgeAlarm(ctx, conf, now)
	if len(offlineTexts) == 0 && len(recoverTexts) == 0 {
		return
	}
	err := sendWxMsg(ctx, conf, getUrlAlarmText(conf.Urls, saves, dels))
	if err != nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"offline": offlineTexts, "recover": recoverTexts, "err": err}).Warn("发送服务告警，异常")
		return
	}

	for url := range saves {
		view.SaveAlarm(ctx, url, saves[url])
	}
	for i := range dels {
		view.RecoverAlarm(ctx, dels[i], conf.SnapshotExpireRound)
	}
	logrus.WithContext(ctx).WithFields(logrus.Fields{"offline": offlineTexts, "recover": recoverTexts}).Info("发送服务告警，完成")
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
			offlineTexts = append(offlineTexts, fmt.Sprintf("离线 %s：%d个实例确认，始于%s，已持续%s，第%d次提醒，仍离线则%s后再提醒",
				url, count, util.Unix2Str(ctx, util.DateLayout_2006_01_02_15_04_05, startTime, nil), getDuration(now-startTime), sendCount, getDuration(int64(getBackoff(conf.AlarmBackoffSec, sendCount)))))
			saves[url] = model.Alarm{StartTime: startTime, LastSendTime: now, SendCount: sendCount}
			continue
		}
		if online && exist {
			recoverTexts = append(recoverTexts, fmt.Sprintf("恢复 %s：离线%s，始于%s，共提醒%d次",
				url, getDuration(now-alarm.StartTime), util.Unix2Str(ctx, util.DateLayout_2006_01_02_15_04_05, alarm.StartTime, nil), alarm.SendCount))
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
		detail := fmt.Sprintf("%s，超阈值%s，始于%s，共提醒%d次",
			getResourceText(resource), getDuration(now-alarm.StartTime), util.Unix2Str(ctx, util.DateLayout_2006_01_02_15_04_05, alarm.StartTime, nil), alarm.SendCount)
		err := sendWxMsg(ctx, conf, fmt.Sprintf("资源恢复:超阈值%s", getDuration(now-alarm.StartTime)))
		if err != nil {
			logrus.WithContext(ctx).WithFields(logrus.Fields{"detail": detail, "err": err}).Warn("发送资源告警，异常")
			return
		}
		view.DelResourceAlarm(ctx)
		logrus.WithContext(ctx).WithFields(logrus.Fields{"detail": detail}).Info("发送资源告警，恢复完成")
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
	detail := fmt.Sprintf("%s，已持续%s，第%d次提醒，仍超阈值则%s后再提醒",
		strings.Join(overTexts, "，"), getDuration(now-startTime), sendCount, getDuration(int64(getBackoff(conf.AlarmBackoffSec, sendCount))))
	err := sendWxMsg(ctx, conf, getOverShort(conf, resource))
	if err != nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"detail": detail, "err": err}).Warn("发送资源告警，异常")
		return
	}
	view.SaveResourceAlarm(ctx, model.Alarm{StartTime: startTime, LastSendTime: now, SendCount: sendCount})
	logrus.WithContext(ctx).WithFields(logrus.Fields{"detail": detail}).Info("发送资源告警，完成")
}

// getUrlAlarmText 拼服务告警的微信正文：离线在前、恢复在后，各自列出URL，比如「离线2:a.com、b.com；恢复:c.com」。
// 只有一个时不写个数，省下的字留给URL；超长由sendWxMsg截断，所以离线排在前面，被截掉的总是不那么要紧的恢复
func getUrlAlarmText(urls []string, saves map[string]model.Alarm, dels []string) string {
	//saves是map，按配置里的URL顺序取，每轮的先后才稳定
	var offlines []string
	for i := range urls {
		if _, ok := saves[urls[i]]; ok {
			offlines = append(offlines, trimUrl(urls[i]))
		}
	}
	var recovers []string
	for i := range dels {
		recovers = append(recovers, trimUrl(dels[i]))
	}
	var segments []string
	if len(offlines) > 0 {
		segments = append(segments, getSegment("离线", offlines))
	}
	if len(recovers) > 0 {
		segments = append(segments, getSegment("恢复", recovers))
	}
	return strings.Join(segments, "；")
}

func getSegment(title string, urls []string) string {
	if len(urls) == 1 {
		return fmt.Sprintf("%s:%s", title, urls[0])
	}
	return fmt.Sprintf("%s%d:%s", title, len(urls), strings.Join(urls, "、"))
}

// trimUrl 去掉URL的协议头和末尾的斜杠，只用在微信正文里：正文只有20个字，这两样都不提供区分度，日志里照样是完整URL
func trimUrl(url string) string {
	if index := strings.Index(url, "://"); index >= 0 {
		url = url[index+3:]
	}
	return strings.TrimSuffix(url, "/")
}

// sendWxMsg 发微信消息。正文里的换行、连续空白压成一个空格，超过wxTextLimit个字就截断、末尾补省略号；
// 截断按字数算而不是字节，否则中文会被从中间切开
func sendWxMsg(ctx context.Context, conf model.Config, text string) error {
	return util.SendWxMsg(ctx, conf.BoardUrl, limitText(text, wxTextLimit))
}

func limitText(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}

// getOverShort 超阈值各项的微信正文，只列超了的项、取整，比如「超阈值:CPU95%内存92%」；三项全超、都是两位数时刚好20个字，再长由sendWxMsg截断
func getOverShort(conf model.Config, resource model.Resource) string {
	text := "超阈值:"
	if percent := resource.CpuPercent(); percent >= conf.CpuUsageLimit {
		text += fmt.Sprintf("CPU%.0f%%", percent)
	}
	if percent := resource.MemPercent(); percent >= conf.MemUsageLimit {
		text += fmt.Sprintf("内存%.0f%%", percent)
	}
	if percent := resource.DiskPercent(); percent >= conf.DiskUsageLimit {
		text += fmt.Sprintf("磁盘%.0f%%", percent)
	}
	return text
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
