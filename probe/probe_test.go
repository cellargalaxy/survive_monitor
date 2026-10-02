package probe

import (
	"os"
	"reflect"
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

// 产出视图与解析视图是一对镜像，分别测两侧都通过、合起来却对不上的情况是真实存在的，所以必须测往返
func TestParseViewRoundTrip(t *testing.T) {
	ctx := util.GenCtx()
	resource := model.Resource{Time: 1700000000, CpuNum: 4, CpuUsage: 123.5, MemTotal: 8 << 30, MemUsed: 4 << 30, DiskTotal: 100 << 30, DiskUsed: 60 << 30}
	origin := model.View{Snapshots: map[string]model.Snapshot{
		model.SelfSource: {
			Id:            "self-id",
			Source:        model.SelfSource,
			Time:          1700000001,
			Probes:        map[string]bool{"https://a/": true, "https://b/": false},
			Resource:      &resource,
			Alarms:        map[string]model.Alarm{"https://b/": {StartTime: 1700000000, LastSendTime: 1700000001, SendCount: 2}},
			Recovers:      map[string]int64{"https://c/": 1699990000},
			ResourceAlarm: &model.Alarm{StartTime: 1699999999, LastSendTime: 1700000000, SendCount: 1},
		},
		"https://peer/api/view": {
			Id:     "peer-id",
			Source: "https://peer/api/view",
			Time:   1700000002,
			Probes: map[string]bool{"https://a/": true},
		},
	}}

	data := util.JsonStruct2Data(util.NewHttpRespByErr(origin, nil))
	parsed, ok := ParseView(ctx, data)
	if !ok {
		t.Fatalf("应当认领本家的响应体: %s", data)
	}

	//map的键序每次都不一样，所以结构比较，只在失败时才打JSON给人看
	if !reflect.DeepEqual(origin, *parsed) {
		t.Fatalf("往返不恒等\n期望: %s\n实际: %s", util.JsonStruct2Str(origin), util.JsonStruct2Str(*parsed))
	}
}

// 认领条件要严到不会误认领，宁可不认领也不能认错
func TestParseViewNotClaim(t *testing.T) {
	ctx := util.GenCtx()
	cases := map[string][]byte{
		"空响应":        nil,
		"非JSON":      []byte("<html>hello</html>"),
		"ping响应":     util.JsonStruct2Data(util.NewHttpRespByErr(util.NewPingData(), nil)),
		"业务码失败":      util.JsonStruct2Data(util.NewHttpRespByMsg(model.View{Snapshots: map[string]model.Snapshot{"x": {}}}, "出错了")),
		"data为空":     util.JsonStruct2Data(util.NewHttpRespByErr(nil, nil)),
		"快照为空":       util.JsonStruct2Data(util.NewHttpRespByErr(model.View{}, nil)),
		"快照为空map":    util.JsonStruct2Data(util.NewHttpRespByErr(model.View{Snapshots: map[string]model.Snapshot{}}, nil)),
		"裸视图没有响应体外壳": util.JsonStruct2Data(model.View{Snapshots: map[string]model.Snapshot{"x": {}}}),
	}

	for name := range cases {
		view, ok := ParseView(ctx, cases[name])
		if ok || view != nil {
			t.Errorf("不应认领[%s]，实际认领了: %+v", name, view)
		}
	}
}

func TestGetClientCache(t *testing.T) {
	ctx := util.GenCtx()
	first := getClient(ctx, 3)
	second := getClient(ctx, 3)
	if first != second {
		t.Fatalf("同一超时应复用同一客户端")
	}
	if getClient(ctx, 5) == first {
		t.Fatalf("不同超时应是不同客户端")
	}
}
