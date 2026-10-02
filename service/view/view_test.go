package view

import (
	"os"
	"testing"

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
	merge(ctx, peers, make(map[string]int64), 1, "my-id", "https://peer/api/view", model.View{Snapshots: map[string]model.Snapshot{
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
	merge(ctx, peers, make(map[string]int64), 1, "my-id", "https://peer/api/view", model.View{Snapshots: map[string]model.Snapshot{
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
	merge(ctx, peers, make(map[string]int64), 1, "my-id", "https://relay/api/view", model.View{Snapshots: map[string]model.Snapshot{
		"https://peer/api/view": {Id: "peer-id", Time: 100, Probes: map[string]bool{"url": true}},
	}})

	if peers["https://peer/api/view"].Time != 200 {
		t.Fatalf("旧数据不许盖掉新数据，期望 time=200，实际 %d", peers["https://peer/api/view"].Time)
	}

	merge(ctx, peers, make(map[string]int64), 1, "my-id", "https://relay/api/view", model.View{Snapshots: map[string]model.Snapshot{
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

	merge(ctx, peers, rounds, 1, "my-id", "https://peer/api/view", model.View{Snapshots: map[string]model.Snapshot{
		model.SelfSource: {Id: "peer-id", Time: 100},
	}})
	if rounds["https://peer/api/view"] != 1 {
		t.Fatalf("第1轮拿到新数据，期望记成1，实际 %d", rounds["https://peer/api/view"])
	}

	merge(ctx, peers, rounds, 2, "my-id", "https://relay/api/view", model.View{Snapshots: map[string]model.Snapshot{
		"https://peer/api/view": {Id: "peer-id", Time: 100},
	}})
	if rounds["https://peer/api/view"] != 1 {
		t.Fatalf("同一份旧快照被转发回来不算新数据，期望仍是1，实际 %d", rounds["https://peer/api/view"])
	}

	merge(ctx, peers, rounds, 3, "my-id", "https://peer/api/view", model.View{Snapshots: map[string]model.Snapshot{
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

	Clean(ctx, 3, 600)

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
