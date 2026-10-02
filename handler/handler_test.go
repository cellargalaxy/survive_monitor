package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/config"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/cellargalaxy/survive_monitor/probe"
	"github.com/cellargalaxy/survive_monitor/service/view"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.WarnLevel)
	gin.SetMode(gin.TestMode)
	code := m.Run()
	os.RemoveAll("resource")
	os.RemoveAll("log")
	os.Exit(code)
}

// get 发起一次请求，是辅助函数，失败要报到真正的调用处
func get(t *testing.T, engine *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s 期望 200，实际 %d: %s", path, recorder.Code, recorder.Body.String())
	}
	return recorder
}

// 交换接口产出的东西必须能被探测侧认领回去，这是实例之间能互相监听的全部前提。
// 从最外层入口发起，顺带把路由、响应封装、视图组装整条链路的装配一起验了
func TestGinViewClaimableByProbe(t *testing.T) {
	ctx := util.GenCtx()
	engine := NewEngine(ctx)

	recorder := get(t, engine, config.PathView)
	parsed, ok := probe.ParseView(ctx, recorder.Body.Bytes())
	if !ok {
		t.Fatalf("交换接口的响应应当被探测侧认领: %s", recorder.Body.String())
	}

	snapshot, ok := parsed.Snapshots[model.SelfSource]
	if !ok {
		t.Fatalf("交换载荷里自己那条的身份键必须留空，交给接收方回填: %+v", parsed.Snapshots)
	}
	if snapshot.Id != view.SelfId() {
		t.Fatalf("自己那条的实例标识期望 %s，实际 %s", view.SelfId(), snapshot.Id)
	}
}

func TestGinStatus(t *testing.T) {
	ctx := util.GenCtx()
	engine := NewEngine(ctx)

	recorder := get(t, engine, config.PathStatus)
	var resp struct {
		Code int          `json:"code"`
		Data model.Status `json:"data"`
	}
	err := util.JsonData2Struct(recorder.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("看板响应应能解析: %+v", err)
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("业务码期望 200，实际 %d", resp.Code)
	}
	if len(resp.Data.View.Snapshots) == 0 {
		t.Fatalf("看板至少该带上自己那条快照")
	}
	//前端健康条的时间轴靠这个字段铺，必须跟配置一致，不能是零值
	if expect := config.GetConfig(ctx).RecordWindowSec; expect <= 0 || resp.Data.RecordWindowSec != expect {
		t.Errorf("明细窗口期望 %d，实际 %d", expect, resp.Data.RecordWindowSec)
	}
}

func TestGinPing(t *testing.T) {
	ctx := util.GenCtx()
	engine := NewEngine(ctx)

	get(t, engine, util.PathPing)
}
