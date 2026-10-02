package view

import (
	"context"
	"sync"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/sirupsen/logrus"
)

// 全局视图是本服务唯一的共享状态，锁就收敛在这一把上。
// 自己的快照单独放，不混进peerSnapshots：它永远是权威且最新的，不该被任何转发回来的副本覆盖
var lock = &sync.RWMutex{}
var selfId = util.GenStrId()
var selfSnapshot = model.Snapshot{Id: selfId, Source: model.SelfSource}
var peerSnapshots = make(map[string]model.Snapshot)

// peerRounds 身份键 -> 最近一次拿到新数据时是本实例的第几轮。过期按轮数算：之后连续若干轮都没拿到更新的快照就算过期。
// 「新数据」只认原始产生时间更新的快照，单纯又拉到一份同样的旧快照不算，否则实例之间来回转发一份挂掉实例的旧快照能把它永远续命
var peerRounds = make(map[string]int64)

// round 本实例当前是第几轮，每轮开头由NextRound加一
var round int64
var records = make(map[string][]model.Record)

// recoveredTimes URL -> 本实例发出恢复通知的时间。只留在本机、不进交换载荷。
// 对端的告警记录只有对端自己能删，对端看不到本实例的结论时(比如它探不到本实例)会一直留着那条记录；
// 本实例发完恢复后，起始时间不晚于恢复时间的记录一律当作已恢复的旧事件，否则每轮都会据此再发一遍恢复
var recoveredTimes = make(map[string]int64)

// resourceOver、resourceRound 本机资源当前是否超阈值，以及这个状态已经连续了几轮
var resourceOver bool
var resourceRound int

// barSlotCount 看板健康条的格数，明细保留窗口按它等分
const barSlotCount = 96

func SelfId() string {
	return selfId
}

// SaveRecord 落一条探测明细，顺手裁掉窗口外的。明细只给看板画状态条，不进交换载荷
func SaveRecord(ctx context.Context, url string, alive bool, windowSec int) {
	lock.Lock()
	defer lock.Unlock()

	now := time.Now().Unix()
	list := append(records[url], model.Record{Time: now, Alive: alive})
	records[url] = trimRecords(list, windowSec, now)
}

// Converge 把本实例最近几轮的明细收敛成「在线/离线」结论。只有连续offlineRound轮全失败才算离线，
// 明细还没攒够轮数时一律算在线，免得服务刚起来就把全世界报死
func Converge(ctx context.Context, urls []string, offlineRound int) map[string]bool {
	lock.RLock()
	defer lock.RUnlock()

	probes := make(map[string]bool, len(urls))
	for i := range urls {
		probes[urls[i]] = converge(records[urls[i]], offlineRound)
	}
	return probes
}

// SaveSelf 落本实例本轮的快照。时间戳在这里写一次，之后被对端转发也不会被刷新
func SaveSelf(ctx context.Context, probes map[string]bool, resource *model.Resource) {
	lock.Lock()
	defer lock.Unlock()

	selfSnapshot.Time = time.Now().Unix()
	selfSnapshot.Probes = probes
	selfSnapshot.Resource = resource
}

// NextRound 开始新的一轮，快照过期的轮数就按它数
func NextRound(ctx context.Context) {
	lock.Lock()
	defer lock.Unlock()

	round++
}

// Merge 合并从source拿到的对端视图。身份键为空的那条就是对端自己，回填成source；
// 其余条目是对端转发的二手数据，原样保留它们的身份键与产生时间
func Merge(ctx context.Context, source string, view model.View) {
	lock.Lock()
	defer lock.Unlock()

	merge(ctx, peerSnapshots, peerRounds, round, selfId, source, view)
}

// Judge 判定某个URL。offline要全部有新鲜结论的实例一致说离线，online只要任一实例说在线，
// count是给出了结论的实例数，也就是判定的分母。两边都为false说明没有任何新鲜结论可依据，保持现状、不告警
func Judge(ctx context.Context, url string, expireRound int) (bool, bool, int) {
	lock.RLock()
	defer lock.RUnlock()

	return judge(snapshotList(expireRound), url)
}

// GetAlarm 取全局最新的离线告警记录。自己的和对端的一起看，对端已经发过就轮不到自己再发；
// 本实例已经发过恢复的旧事件不算，见recoveredTimes
func GetAlarm(ctx context.Context, url string, expireRound int) (model.Alarm, bool) {
	lock.RLock()
	defer lock.RUnlock()

	var newest model.Alarm
	var exist bool
	for _, snapshot := range snapshotList(expireRound) {
		alarm, ok := snapshot.Alarms[url]
		if !ok || recovered(url, alarm) {
			continue
		}
		if !exist || alarm.LastSendTime > newest.LastSendTime {
			newest = alarm
			exist = true
		}
	}
	return newest, exist
}

// SaveAlarm 写本实例的告警记录。交换载荷和看板拿走的是快照的浅拷贝，和这里共用同一个map，
// 所以只能写时复制、不能原地改，否则出了锁之后的JSON序列化会和这里撞上map并发读写，进程直接崩
func SaveAlarm(ctx context.Context, url string, alarm model.Alarm) {
	lock.Lock()
	defer lock.Unlock()

	alarms := cloneAlarms(selfSnapshot.Alarms)
	alarms[url] = alarm
	selfSnapshot.Alarms = alarms
}

// RecoverAlarm 发完恢复之后调用：删掉本实例的告警记录，并记下恢复时间，对端还留着的同一事件的记录从此不再认
func RecoverAlarm(ctx context.Context, url string, now int64) {
	lock.Lock()
	defer lock.Unlock()

	delAlarm(url)
	recoveredTimes[url] = now
}

// GetResourceAlarm 资源告警只看本机记录。资源是各实例自己采的，也就只有自己有资格判它持续超了多久
func GetResourceAlarm(ctx context.Context) (model.Alarm, bool) {
	lock.RLock()
	defer lock.RUnlock()

	if selfSnapshot.ResourceAlarm == nil {
		return model.Alarm{}, false
	}
	return *selfSnapshot.ResourceAlarm, true
}

func SaveResourceAlarm(ctx context.Context, alarm model.Alarm) {
	lock.Lock()
	defer lock.Unlock()

	selfSnapshot.ResourceAlarm = &alarm
}

func DelResourceAlarm(ctx context.Context) {
	lock.Lock()
	defer lock.Unlock()

	selfSnapshot.ResourceAlarm = nil
}

// MarkResourceOver 记下本机资源本轮是否超阈值，返回这个状态已经连续了几轮(含本轮)。
// 超与不超各自计数，状态一翻转就从1重新数
func MarkResourceOver(ctx context.Context, over bool) int {
	lock.Lock()
	defer lock.Unlock()

	if over != resourceOver {
		resourceOver = over
		resourceRound = 0
	}
	resourceRound++
	return resourceRound
}

// GetView 组装交换载荷。自己那条的身份键要留空，交给接收方回填
func GetView(ctx context.Context, expireRound int) model.View {
	lock.RLock()
	defer lock.RUnlock()

	return getView(expireRound)
}

// GetStatus 组装看板载荷。明细只有本实例的观测，各实例之间本就不要求一致。
// 明细在这里就按窗口聚合成健康条，不把原始明细整份下发：7天窗口下原始明细有几十万条，看板每次拉取要好几MB
func GetStatus(ctx context.Context, expireRound, windowSec int) model.Status {
	lock.RLock()
	defer lock.RUnlock()

	now := time.Now().Unix()
	bars := make(map[string][]int, len(records))
	for url, list := range records {
		bars[url] = buildBar(list, windowSec, now)
	}
	return model.Status{View: getView(expireRound), Bars: bars}
}

// Clean 清掉过期快照、窗口外明细，以及已经从配置里删掉的URL留下的明细和告警记录。
// 判定时本来就会跳过过期数据，这里是别让内存一直涨，也别让删掉的URL的旧告警记录一直被交换出去，
// 哪天重新加回来时还被当成同一事件续上退避
func Clean(ctx context.Context, urls []string, expireRound, windowSec int) {
	lock.Lock()
	defer lock.Unlock()

	configured := make(map[string]bool, len(urls))
	for i := range urls {
		configured[urls[i]] = true
	}

	now := time.Now().Unix()
	for source := range peerSnapshots {
		if expire(peerRounds[source], round, expireRound) {
			delete(peerSnapshots, source)
			delete(peerRounds, source)
		}
	}
	for url, list := range records {
		list = trimRecords(list, windowSec, now)
		if len(list) == 0 || !configured[url] {
			delete(records, url)
			continue
		}
		records[url] = list
	}
	for url := range selfSnapshot.Alarms {
		if !configured[url] {
			delAlarm(url)
		}
	}
	//恢复时间只是用来压住对端的旧记录，没有任何新鲜快照还带着这类旧记录了，它也就没用了
	list := snapshotList(expireRound)
	for url := range recoveredTimes {
		stale := false
		for i := range list {
			if alarm, ok := list[i].Alarms[url]; ok && recovered(url, alarm) {
				stale = true
				break
			}
		}
		if !stale {
			delete(recoveredTimes, url)
		}
	}
}

// delAlarm 删本实例的告警记录，同样写时复制，见SaveAlarm。调用方必须已经持有锁
func delAlarm(url string) {
	if _, ok := selfSnapshot.Alarms[url]; !ok {
		return
	}
	alarms := cloneAlarms(selfSnapshot.Alarms)
	delete(alarms, url)
	selfSnapshot.Alarms = alarms
}

func cloneAlarms(alarms map[string]model.Alarm) map[string]model.Alarm {
	cloned := make(map[string]model.Alarm, len(alarms)+1)
	for url, alarm := range alarms {
		cloned[url] = alarm
	}
	return cloned
}

// recovered 这条告警记录是不是本实例已经发过恢复的旧事件。调用方必须已经持有锁
func recovered(url string, alarm model.Alarm) bool {
	recoverTime, ok := recoveredTimes[url]
	return ok && alarm.StartTime <= recoverTime
}

// buildBar 把一个URL的明细按窗口等分成barSlotCount格。一格里只要有一次失败就标失败，宁可显眼也不要把抖动藏起来
func buildBar(list []model.Record, windowSec int, now int64) []int {
	bar := make([]int, barSlotCount)
	if windowSec <= 0 {
		return bar
	}
	begin := now - int64(windowSec)
	span := float64(windowSec) / barSlotCount
	for i := range list {
		if list[i].Time < begin {
			continue
		}
		index := int(float64(list[i].Time-begin) / span)
		if index >= barSlotCount {
			index = barSlotCount - 1
		}
		if bar[index] == model.BarOffline {
			continue
		}
		if list[i].Alive {
			bar[index] = model.BarOnline
		} else {
			bar[index] = model.BarOffline
		}
	}
	return bar
}

func trimRecords(list []model.Record, windowSec int, now int64) []model.Record {
	cutoff := now - int64(windowSec)
	begin := 0
	for begin < len(list) && list[begin].Time < cutoff {
		begin++
	}
	if begin == 0 {
		return list
	}
	return append([]model.Record(nil), list[begin:]...)
}

func converge(list []model.Record, offlineRound int) bool {
	if offlineRound <= 0 || len(list) < offlineRound {
		return true
	}
	for i := len(list) - offlineRound; i < len(list); i++ {
		if list[i].Alive {
			return true
		}
	}
	return false
}

// merge 合并的纯逻辑。只有拿到原始产生时间更新的快照，才在rounds里把该身份键记成本轮拿到了新数据
func merge(ctx context.Context, peers map[string]model.Snapshot, rounds map[string]int64, current int64, selfId, source string, view model.View) {
	for key, snapshot := range view.Snapshots {
		if key == model.SelfSource {
			//对端自己那条的身份键是空的，回填成「我从哪个URL拿到的」
			key = source
			snapshot.Source = source
		}
		if key == model.SelfSource {
			//回填完还是空，说明source本身是空的，宁可丢掉也不能污染自己那条
			continue
		}
		//转发回来的自己要丢掉，否则同一份观测会在判定分母里投两票，而且那票还是旧的
		if snapshot.Id != "" && snapshot.Id == selfId {
			continue
		}
		old, ok := peers[key]
		if ok && old.Time >= snapshot.Time {
			continue
		}
		peers[key] = snapshot
		rounds[key] = current
	}
	logrus.WithContext(ctx).WithFields(logrus.Fields{"source": source, "len": len(peers)}).Info("合并全局视图")
}

func judge(list []model.Snapshot, url string) (bool, bool, int) {
	var count int
	var online bool
	for i := range list {
		alive, ok := list[i].Probes[url]
		if !ok {
			continue
		}
		count++
		if alive {
			online = true
		}
	}
	if count == 0 {
		return false, false, 0
	}
	return !online, online, count
}

// getView 组装交换载荷的纯逻辑。调用方必须已经持有锁
func getView(expireRound int) model.View {
	snapshots := make(map[string]model.Snapshot, len(peerSnapshots)+1)
	for source, snapshot := range peerSnapshots {
		if expire(peerRounds[source], round, expireRound) {
			continue
		}
		snapshots[source] = snapshot
	}
	snapshots[model.SelfSource] = selfSnapshot
	return model.View{Snapshots: snapshots}
}

// snapshotList 判定分母：本实例的快照永远算，对端的只算没过期的。
// 调用方必须已经持有锁
func snapshotList(expireRound int) []model.Snapshot {
	list := make([]model.Snapshot, 0, len(peerSnapshots)+1)
	list = append(list, selfSnapshot)
	for source, snapshot := range peerSnapshots {
		if expire(peerRounds[source], round, expireRound) {
			continue
		}
		list = append(list, snapshot)
	}
	return list
}

// expire 在第updated轮拿到新数据之后，到第current轮已经连续expireRound轮没拿到新数据，就算过期。
// 本轮刚拿到新数据时差值是0，永远新鲜
func expire(updated, current int64, expireRound int) bool {
	return current-updated >= int64(expireRound)
}
