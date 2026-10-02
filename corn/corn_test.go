package corn

import (
	"os"
	"testing"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/config"
	"github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.WarnLevel)
	code := m.Run()
	os.RemoveAll("resource")
	os.RemoveAll("log")
	os.Exit(code)
}

// 守护池自己的休眠是0，节奏全靠探测间隔：间隔是零的话一轮接一轮地空转，所以这条得钉住
func TestInit(t *testing.T) {
	ctx := util.GenCtx()
	if interval := config.GetConfig(ctx).ProbeIntervalSec; interval <= 0 {
		t.Fatalf("探测间隔应为正，实际 %d", interval)
	}
	if err := Init(ctx); err != nil {
		t.Fatalf("定时任务启动异常: %+v", err)
	}
}
