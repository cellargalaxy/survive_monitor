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
var resourceOverRound int

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

// GetAlarm 取全局最新的离线告警记录。自己的和对端的一起看，对端已经发过就轮不到自己再发
func GetAlarm(ctx context.Context, url string, expireRound int) (model.Alarm, bool) {
	lock.RLock()
	defer lock.RUnlock()

	var newest model.Alarm
	var exist bool
	for _, snapshot := range snapshotList(expireRound) {
		alarm, ok := snapshot.Alarms[url]
		if !ok {
			continue
		}
		if !exist || alarm.LastSendTime > newest.LastSendTime {
			newest = alarm
			exist = true
		}
	}
	return newest, exist
}

func SaveAlarm(ctx context.Context, url string, alarm model.Alarm) {
	lock.Lock()
	defer lock.Unlock()

	if selfSnapshot.Alarms == nil {
		selfSnapshot.Alarms = make(map[string]model.Alarm)
	}
	selfSnapshot.Alarms[url] = alarm
}

func DelAlarm(ctx context.Context, url string) {
	lock.Lock()
	defer lock.Unlock()

	delete(selfSnapshot.Alarms, url)
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

// MarkResourceOver 累计本机资源连续超阈值的轮数，没超就归零，返回累计后的轮数
func MarkResourceOver(ctx context.Context, over bool) int {
	lock.Lock()
	defer lock.Unlock()

	if !over {
		resourceOverRound = 0
		return 0
	}
	resourceOverRound++
	return resourceOverRound
}

// GetView 组装交换载荷。自己那条的身份键要留空，交给接收方回填
func GetView(ctx context.Context, expireRound int) model.View {
	lock.RLock()
	defer lock.RUnlock()

	return getView(expireRound)
}

// GetStatus 组装看板载荷。明细只有本实例的观测，各实例之间本就不要求一致
func GetStatus(ctx context.Context, expireRound int) model.Status {
	lock.RLock()
	defer lock.RUnlock()

	copied := make(map[string][]model.Record, len(records))
	for url, list := range records {
		copied[url] = append([]model.Record(nil), list...)
	}
	return model.Status{View: getView(expireRound), Records: copied}
}

// Clean 清掉过期快照与窗口外明细。判定时本来就会跳过过期数据，这里只是别让内存一直涨
func Clean(ctx context.Context, expireRound, windowSec int) {
	lock.Lock()
	defer lock.Unlock()

	now := time.Now().Unix()
	for source := range peerSnapshots {
		if expire(peerRounds[source], round, expireRound) {
			delete(peerSnapshots, source)
			delete(peerRounds, source)
		}
	}
	for url, list := range records {
		list = trimRecords(list, windowSec, now)
		if len(list) == 0 {
			delete(records, url)
			continue
		}
		records[url] = list
	}
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
