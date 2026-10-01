package model

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.WarnLevel)
	code := m.Run()
	os.RemoveAll("log")
	os.Exit(code)
}

// CPU使用率是按核累加的，折算成百分比才和内存、磁盘同量纲；除零要兜住
func TestResourcePercent(t *testing.T) {
	resource := Resource{CpuNum: 4, CpuUsage: 220, MemTotal: 1000, MemUsed: 250, DiskTotal: 200, DiskUsed: 150}
	if percent := resource.CpuPercent(); percent != 55 {
		t.Errorf("CPU期望 55，实际 %v", percent)
	}
	if percent := resource.MemPercent(); percent != 25 {
		t.Errorf("内存期望 25，实际 %v", percent)
	}
	if percent := resource.DiskPercent(); percent != 75 {
		t.Errorf("磁盘期望 75，实际 %v", percent)
	}

	var empty Resource
	if empty.CpuPercent() != 0 || empty.MemPercent() != 0 || empty.DiskPercent() != 0 {
		t.Errorf("采集不到时三项都该是0，实际 %v/%v/%v", empty.CpuPercent(), empty.MemPercent(), empty.DiskPercent())
	}
}
