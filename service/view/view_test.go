package view

import (
	"os"
	"testing"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.WarnLevel)
	code := m.Run()
	os.RemoveAll("log")
	os.Exit(code)
}

// judge 是「离线要全体一致、恢复只要一人」这条不变式的落点，两边不对称是故意的
func TestJudge(t *testing.T) {
	cases := map[string]struct {
		list    []model.Snapshot
		offline bool
		online  bool
		count   int
	}{
		"没有任何结论": {
			list: []model.Snapshot{{Probes: map[string]bool{"other": false}}},
		},
		"唯一结论说离线": {
			list:    []model.Snapshot{{Probes: map[string]bool{"url": false}}},
			offline: true,
			count:   1,
		},
		"全体一致说离线": {
			list:    []model.Snapshot{{Probes: map[string]bool{"url": false}}, {Probes: map[string]bool{"url": false}}, {Probes: map[string]bool{"url": false}}},
			offline: true,
			count:   3,
		},
		"有一个说在线就不算离线": {
			list:   []model.Snapshot{{Probes: map[string]bool{"url": false}}, {Probes: map[string]bool{"url": false}}, {Probes: map[string]bool{"url": true}}},
			online: true,
			count:  3,
		},
		"没结论的实例不进分母": {
			list:    []model.Snapshot{{Probes: map[string]bool{"url": false}}, {Probes: nil}, {Probes: map[string]bool{"other": true}}},
			offline: true,
			count:   1,
		},
	}

	for name := range cases {
		one := cases[name]
		offline, online, count := judge(one.list, "url")
		if offline != one.offline || online != one.online || count != one.count {
			t.Errorf("[%s] 期望 offline=%v online=%v count=%d，实际 offline=%v online=%v count=%d",
				name, one.offline, one.online, one.count, offline, online, count)
		}
	}
}

// 明细攒不够轮数就不许判离线，否则服务刚起来会把全世界报死
func TestConverge(t *testing.T) {
	cases := map[string]struct {
		list   []model.Record
		round  int
		expect bool
	}{
		"没有明细":      {list: nil, round: 3, expect: true},
		"明细不够轮数":    {list: []model.Record{{Alive: false}, {Alive: false}}, round: 3, expect: true},
		"连续三轮全失败":   {list: []model.Record{{Alive: false}, {Alive: false}, {Alive: false}}, round: 3, expect: false},
		"最近三轮里有成功":  {list: []model.Record{{Alive: false}, {Alive: true}, {Alive: false}}, round: 3, expect: true},
		"早先失败但最近成功": {list: []model.Record{{Alive: false}, {Alive: false}, {Alive: false}, {Alive: true}}, round: 3, expect: true},
		"最近三轮全失败":   {list: []model.Record{{Alive: true}, {Alive: false}, {Alive: false}, {Alive: false}}, round: 3, expect: false},
		"轮数为一":      {list: []model.Record{{Alive: false}}, round: 1, expect: false},
	}

	for name := range cases {
		one := cases[name]
		actual := converge(one.list, one.round)
		if actual != one.expect {
			t.Errorf("[%s] 期望 %v，实际 %v", name, one.expect, actual)
		}
	}
}

// 合并有三条不变式：空身份键回填成来源、转发回来的自己要丢掉、旧数据不许盖掉新数据
func TestMergeFillSource(t *testing.T) {
	ctx := util.GenCtx()
	peers := make(map[string]model.Snapshot)
	merge(ctx, peers, make(map[string]int64), make(map[string]int64), 1, "my-id", "https://peer/api/view", model.View{Snapshots: map[string]model.Snapshot{
		model.SelfSource: {Id: "peer-id", Time: 100, Probes: map[string]bool{"url": true}},
	}})

	snapshot, ok := peers["https://peer/api/view"]
	if !ok {
		t.Fatalf("空身份键应回填成来源URL，实际: %+v", peers)
	}
	if snapshot.Source != "https://peer/api/view" {
		t.Fatalf("身份键字段也该回填，期望 https://peer/api/view，实际 %s", snapshot.Source)
	}
	if _, ok = peers[model.SelfSource]; ok {
		t.Fatalf("回填之后不该再留空键: %+v", peers)
	}
}

func TestMergeDropSelf(t *testing.T) {
	ctx := util.GenCtx()
	peers := make(map[string]model.Snapshot)
	merge(ctx, peers, make(map[string]int64), make(map[string]int64), 1, "my-id", "https://peer/api/view", model.View{Snapshots: map[string]model.Snapshot{
		"https://me/api/view":    {Id: "my-id", Time: 100, Probes: map[string]bool{"url": true}},
		"https://other/api/view": {Id: "other-id", Time: 100, Probes: map[string]bool{"url": true}},
	}})

	if _, ok := peers["https://me/api/view"]; ok {
		t.Fatalf("转发回来的自己必须丢掉，否则同一份观测在分母里投两票: %+v", peers)
	}
	if len(peers) != 1 {
		t.Fatalf("期望只留下1条对端快照，实际 %d 条: %+v", len(peers), peers)
	}
}

func TestMergeKeepNewer(t *testing.T) {
	ctx := util.GenCtx()
	peers := map[string]model.Snapshot{
		"https://peer/api/view": {Id: "peer-id", Time: 200, Probes: map[string]bool{"url": false}},
	}
	merge(ctx, peers, make(map[string]int64), make(map[string]int64), 1, "my-id", "https://relay/api/view", model.View{Snapshots: map[string]model.Snapshot{
		"https://peer/api/view": {Id: "peer-id", Time: 100, Probes: map[string]bool{"url": true}},
	}})

	if peers["https://peer/api/view"].Time != 200 {
		t.Fatalf("旧数据不许盖掉新数据，期望 time=200，实际 %d", peers["https://peer/api/view"].Time)
	}

	merge(ctx, peers, make(map[string]int64), make(map[string]int64), 1, "my-id", "https://relay/api/view", model.View{Snapshots: map[string]model.Snapshot{
		"https://peer/api/view": {Id: "peer-id", Time: 300, Probes: map[string]bool{"url": true}},
	}})
	if peers["https://peer/api/view"].Time != 300 {
		t.Fatalf("新数据应当盖掉旧数据，期望 time=300，实际 %d", peers["https://peer/api/view"].Time)
	}
}

// 裁剪只许裁窗口外的，不许把整份明细一锅端
func TestTrimRecords(t *testing.T) {
	list := []model.Record{{Time: 10}, {Time: 50}, {Time: 90}}

	kept := trimRecords(list, 100, 100)
	if len(kept) != 3 {
		t.Errorf("全在窗口内应一条不裁，实际剩 %d 条", len(kept))
	}

	kept = trimRecords(list, 60, 100)
	if len(kept) != 2 || kept[0].Time != 50 {
		t.Errorf("期望裁掉窗口外的第一条，实际: %+v", kept)
	}

	kept = trimRecords(list, 1, 1000)
	if len(kept) != 0 {
		t.Errorf("全部过期应裁空，实际: %+v", kept)
	}

	if trimRecords(nil, 60, 100) != nil {
		t.Errorf("空明细裁出来还该是空")
	}
}

// 过期按轮数：拿到新数据之后连续expireRound轮都没再拿到，才算过期
func TestExpire(t *testing.T) {
	cases := map[string]struct {
		updated int64
		current int64
		expect  bool
	}{
		"本轮刚拿到新数据":     {updated: 10, current: 10, expect: false},
		"差一轮才到过期轮数":    {updated: 10, current: 12, expect: false},
		"连续3轮没新数据算过期":  {updated: 10, current: 13, expect: true},
		"从没拿到过新数据也算过期": {updated: 0, current: 13, expect: true},
	}

	for name := range cases {
		one := cases[name]
		actual := expire(one.updated, one.current, 3)
		if actual != one.expect {
			t.Errorf("[%s] 期望 %v，实际 %v", name, one.expect, actual)
		}
	}
}

// 只有原始产生时间更新的快照才算新数据，再拉到一份同样的旧快照不能续命，
// 否则实例之间来回转发一份挂掉实例的旧快照，它就永远不过期
func TestMergeRefreshRound(t *testing.T) {
	ctx := util.GenCtx()
	peers := make(map[string]model.Snapshot)
	rounds := make(map[string]int64)

	merge(ctx, peers, rounds, make(map[string]int64), 1, "my-id", "https://peer/api/view", model.View{Snapshots: map[string]model.Snapshot{
		model.SelfSource: {Id: "peer-id", Time: 100},
	}})
	if rounds["https://peer/api/view"] != 1 {
		t.Fatalf("第1轮拿到新数据，期望记成1，实际 %d", rounds["https://peer/api/view"])
	}

	merge(ctx, peers, rounds, make(map[string]int64), 2, "my-id", "https://relay/api/view", model.View{Snapshots: map[string]model.Snapshot{
		"https://peer/api/view": {Id: "peer-id", Time: 100},
	}})
	if rounds["https://peer/api/view"] != 1 {
		t.Fatalf("同一份旧快照被转发回来不算新数据，期望仍是1，实际 %d", rounds["https://peer/api/view"])
	}

	merge(ctx, peers, rounds, make(map[string]int64), 3, "my-id", "https://peer/api/view", model.View{Snapshots: map[string]model.Snapshot{
		model.SelfSource: {Id: "peer-id", Time: 130},
	}})
	if rounds["https://peer/api/view"] != 3 {
		t.Fatalf("第3轮拿到更新的快照，期望记成3，实际 %d", rounds["https://peer/api/view"])
	}
}

// 自己的快照永远在分母里，对端的只算没过期的
func TestSnapshotList(t *testing.T) {
	lock.Lock()
	round = 100
	selfSnapshot = model.Snapshot{Id: "self", Time: 100}
	peerSnapshots = map[string]model.Snapshot{
		"fresh": {Id: "fresh", Time: 95},
		"stale": {Id: "stale", Time: 10},
	}
	peerRounds = map[string]int64{"fresh": 99, "stale": 90}
	lock.Unlock()

	lock.RLock()
	list := snapshotList(3)
	lock.RUnlock()

	if len(list) != 2 {
		t.Fatalf("期望分母为2（自己+新鲜对端），实际 %d: %+v", len(list), list)
	}
	for i := range list {
		if list[i].Id == "stale" {
			t.Fatalf("过期对端不该进分母: %+v", list)
		}
	}
}

// 过期的对端要连同轮数记录一起清掉，不然内存只增不减
func TestCleanExpire(t *testing.T) {
	ctx := util.GenCtx()
	lock.Lock()
	round = 100
	peerSnapshots = map[string]model.Snapshot{"fresh": {Id: "fresh", Time: 95}, "stale": {Id: "stale", Time: 10}}
	peerRounds = map[string]int64{"fresh": 99, "stale": 90}
	lock.Unlock()

	Clean(ctx, nil, 3, 600)

	lock.RLock()
	defer lock.RUnlock()
	if _, ok := peerSnapshots["stale"]; ok {
		t.Errorf("过期快照该被清掉")
	}
	if _, ok := peerRounds["stale"]; ok {
		t.Errorf("过期快照的轮数记录该一起清掉")
	}
	if _, ok := peerSnapshots["fresh"]; !ok {
		t.Errorf("新鲜快照不该被清掉")
	}
}

// 交换载荷和看板拿走的快照和自己那条共用告警map，写告警记录必须写时复制，
// 否则出了锁之后的JSON序列化会和写入撞上map并发读写
func TestSaveAlarmCopyOnWrite(t *testing.T) {
	ctx := util.GenCtx()
	SaveAlarm(ctx, "https://cow/a", model.Alarm{StartTime: 1})
	before := GetView(ctx, 5).Snapshots[model.SelfSource].Alarms

	SaveAlarm(ctx, "https://cow/b", model.Alarm{StartTime: 2})
	RecoverAlarm(ctx, "https://cow/a", 5)
	if _, ok := before["https://cow/b"]; ok {
		t.Fatalf("已经交出去的map不许被原地新增")
	}
	if _, ok := before["https://cow/a"]; !ok {
		t.Fatalf("已经交出去的map不许被原地删除")
	}
	after := GetView(ctx, 5).Snapshots[model.SelfSource].Alarms
	if _, ok := after["https://cow/a"]; ok {
		t.Fatalf("恢复之后自己的记录该删掉")
	}
	if _, ok := after["https://cow/b"]; !ok {
		t.Fatalf("新写的记录该在")
	}
}

// 告警与恢复都要连续攒轮数，状态一翻转就从1重新数
func TestMarkResourceOver(t *testing.T) {
	ctx := util.GenCtx()
	MarkResourceOver(ctx, true)
	steps := []struct {
		over   bool
		expect int
	}{{false, 1}, {false, 2}, {true, 1}, {true, 2}, {true, 3}, {false, 1}}
	for i := range steps {
		if actual := MarkResourceOver(ctx, steps[i].over); actual != steps[i].expect {
			t.Errorf("第 %d 步 over=%v 期望 %d，实际 %d", i, steps[i].over, steps[i].expect, actual)
		}
	}
}

// 健康条与原先前端的口径一致：窗口外的不要，一格里有一次失败就标失败，落在窗口末尾的算最后一格
func TestBuildBar(t *testing.T) {
	now := int64(10000)
	window := 960 //每格10秒
	bar := buildBar([]model.Record{
		{Time: now - 2000, Alive: false}, //窗口外
		{Time: now - 955, Alive: true},   //第0格
		{Time: now - 945, Alive: false},  //第1格
		{Time: now - 941, Alive: true},   //第1格，失败过就保持失败
		{Time: now, Alive: true},         //窗口末尾，算最后一格
	}, window, now)

	if len(bar) != barSlotCount {
		t.Fatalf("格数期望 %d，实际 %d", barSlotCount, len(bar))
	}
	if bar[0] != model.BarOnline || bar[1] != model.BarOffline || bar[2] != model.BarUnknown || bar[barSlotCount-1] != model.BarOnline {
		t.Fatalf("健康条不对，实际: %+v", bar)
	}
	for _, slot := range buildBar(nil, window, now) {
		if slot != model.BarUnknown {
			t.Fatalf("没有明细时每格都该是未知")
		}
	}
}

// 从配置里删掉的URL，明细与自己的告警记录要一起清掉；恢复时间在没有旧记录可压之后也要清掉
func TestCleanUnconfigured(t *testing.T) {
	ctx := util.GenCtx()
	SaveRecord(ctx, "https://keep/", true, 600)
	SaveRecord(ctx, "https://drop/", true, 600)
	SaveAlarm(ctx, "https://keep/", model.Alarm{StartTime: 1})
	SaveAlarm(ctx, "https://drop/", model.Alarm{StartTime: 1})
	SaveAlarm(ctx, "https://gone/", model.Alarm{StartTime: 1})
	RecoverAlarm(ctx, "https://gone/", 5)

	Clean(ctx, []string{"https://keep/"}, 5, 600)

	lock.RLock()
	defer lock.RUnlock()
	if _, ok := records["https://drop/"]; ok {
		t.Errorf("删掉的URL的明细该清掉")
	}
	if _, ok := records["https://keep/"]; !ok {
		t.Errorf("还在配置里的URL的明细不该清掉")
	}
	if _, ok := selfSnapshot.Alarms["https://drop/"]; ok {
		t.Errorf("删掉的URL的告警记录该清掉")
	}
	if _, ok := selfSnapshot.Alarms["https://keep/"]; !ok {
		t.Errorf("还在配置里的URL的告警记录不该清掉")
	}
	if _, ok := selfSnapshot.Recovers["https://gone/"]; ok {
		t.Errorf("没有旧记录可压的已恢复事件该清掉")
	}
}

// 过期出局的快照，清掉之后再拉到同一份旧快照(别的实例转发回来，或者卡住的对端一直返回)也不许复活，
// 否则挂掉实例的旧结论过期一轮又复活一轮，一直在分母里投票；比出局时那份更新的照常收
func TestMergeExpiredNotRevive(t *testing.T) {
	ctx := util.GenCtx()
	peers := make(map[string]model.Snapshot)
	rounds := make(map[string]int64)
	expired := map[string]int64{"https://dead/api/view": 100}

	merge(ctx, peers, rounds, expired, 10, "my-id", "https://relay/api/view", model.View{Snapshots: map[string]model.Snapshot{
		"https://dead/api/view": {Id: "dead-id", Time: 100},
	}})
	if _, ok := peers["https://dead/api/view"]; ok {
		t.Fatalf("出局时那份旧快照不许复活: %+v", peers)
	}

	merge(ctx, peers, rounds, expired, 11, "my-id", "https://dead/api/view", model.View{Snapshots: map[string]model.Snapshot{
		model.SelfSource: {Id: "dead-id", Time: 101},
	}})
	if peers["https://dead/api/view"].Time != 101 || rounds["https://dead/api/view"] != 11 {
		t.Fatalf("比出局时更新的快照该收，实际 %+v %+v", peers, rounds)
	}
	if _, ok := expired["https://dead/api/view"]; ok {
		t.Fatalf("重新收进来之后出局记录该删掉")
	}
}

// 卡住的对端一直返回同一份旧快照：过期之后要一直出局，不能隔几轮复活一次
func TestCleanExpiredNotRevive(t *testing.T) {
	ctx := util.GenCtx()
	stuck := model.View{Snapshots: map[string]model.Snapshot{model.SelfSource: {Id: "stuck-id", Time: 100}}}
	fresh := func() bool {
		lock.RLock()
		defer lock.RUnlock()
		_, ok := getView(2).Snapshots["https://stuck/api/view"]
		return ok
	}

	for i := 0; i < 8; i++ {
		NextRound(ctx)
		Merge(ctx, "https://stuck/api/view", stuck)
		Clean(ctx, nil, 2, 600)
		if expect := i < 2; fresh() != expect {
			t.Fatalf("第 %d 轮期望新鲜=%v，实际 %v", i, expect, fresh())
		}
	}
}

// 别的实例发过恢复，本实例还留着的同一事件的记录不再认，并且要在清理时删掉；
// 同一事件被各自首发过、起始时间不同时，恢复要一起盖住
func TestRecoverAlarmExchange(t *testing.T) {
	ctx := util.GenCtx()
	url := "https://exchange/"
	SaveAlarm(ctx, url, model.Alarm{StartTime: 100, LastSendTime: 500, SendCount: 3})
	Merge(ctx, "https://other-sender/api/view", model.View{Snapshots: map[string]model.Snapshot{
		model.SelfSource: {Id: "other-sender", Time: time.Now().Unix(), Alarms: map[string]model.Alarm{url: {StartTime: 110, LastSendTime: 110, SendCount: 1}}},
	}})
	RecoverAlarm(ctx, url, 5)
	if startTime := GetView(ctx, 5).Snapshots[model.SelfSource].Recovers[url]; startTime != 110 {
		t.Fatalf("已恢复事件该取最晚的起始时间，期望 110，实际 %d", startTime)
	}
	if _, exist := GetAlarm(ctx, url, 5); exist {
		t.Fatalf("发过恢复之后，各实例同一事件的记录都不该再认")
	}

	other := "https://exchange-peer/"
	SaveAlarm(ctx, other, model.Alarm{StartTime: 200, LastSendTime: 200, SendCount: 1})
	Merge(ctx, "https://peer-recover/api/view", model.View{Snapshots: map[string]model.Snapshot{
		model.SelfSource: {Id: "peer-recover", Time: time.Now().Unix(), Recovers: map[string]int64{other: 200}},
	}})
	if _, exist := GetAlarm(ctx, other, 5); exist {
		t.Fatalf("对端已经发过恢复，自己那条同一事件的记录不该再认")
	}
	Clean(ctx, []string{url, other}, 5, 600)
	if _, ok := GetView(ctx, 5).Snapshots[model.SelfSource].Alarms[other]; ok {
		t.Fatalf("对端已经发过恢复，自己那条同一事件的记录该清掉")
	}

	SaveAlarm(ctx, other, model.Alarm{StartTime: 201, LastSendTime: 201, SendCount: 1})
	if _, exist := GetAlarm(ctx, other, 5); !exist {
		t.Fatalf("恢复之后再开的新事件要认")
	}
}
