package machine

import (
	"os"
	"testing"

	"github.com/cellargalaxy/go_common/util"
	"github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.WarnLevel)
	code := m.Run()
	os.RemoveAll("log")
	os.Exit(code)
}

func TestLoadResource(t *testing.T) {
	ctx := util.GenCtx()
	resource, err := LoadResource(ctx, "/")
	if err != nil {
		t.Fatalf("本机资源应能采到: %+v", err)
	}

	if resource.Time <= 0 {
		t.Errorf("采集时间不该为空")
	}
	if resource.CpuNum <= 0 {
		t.Errorf("CPU核数期望为正，实际 %d", resource.CpuNum)
	}
	if resource.MemTotal == 0 || resource.MemUsed > resource.MemTotal {
		t.Errorf("内存取值非法，已用 %d 总共 %d", resource.MemUsed, resource.MemTotal)
	}
	if resource.DiskTotal == 0 || resource.DiskUsed > resource.DiskTotal {
		t.Errorf("磁盘取值非法，已用 %d 总共 %d", resource.DiskUsed, resource.DiskTotal)
	}
	if percent := resource.CpuPercent(); percent < 0 || percent > 100 {
		t.Errorf("CPU使用率折算后该落在0-100，实际 %v", percent)
	}
}

// 路径不存在时要报错，不能悄悄给个零值让上层以为磁盘是空的
func TestLoadResourceIllegalPath(t *testing.T) {
	ctx := util.GenCtx()
	resource, err := LoadResource(ctx, "/no/such/path/survive_monitor")
	if err == nil {
		t.Fatalf("路径不存在应报错")
	}
	if resource.CpuNum <= 0 {
		t.Errorf("磁盘采不到，CPU那几项照样该采到，实际核数 %d", resource.CpuNum)
	}
}

// 配置路径是内存盘(比如flatpak沙箱里的根目录)时，要回退到工作目录，不能把tmpfs当磁盘上报
func TestResolveDiskPathMemoryFs(t *testing.T) {
	ctx := util.GenCtx()
	if !isMemoryFs(ctx, "/dev/shm") {
		t.Skip("本机/dev/shm不是内存盘，跳过")
	}
	workPath, err := os.Getwd()
	if err != nil {
		t.Fatalf("取工作目录异常: %+v", err)
	}
	expect := workPath
	if isMemoryFs(ctx, workPath) {
		expect = "/dev/shm" //工作目录也是内存盘就没得回退，原样用配置路径
	}
	if actual := resolveDiskPath(ctx, "/dev/shm"); actual != expect {
		t.Errorf("期望 %s，实际 %s", expect, actual)
	}

	resource, _ := LoadResource(ctx, "/dev/shm")
	if resource.DiskPath != expect {
		t.Errorf("上报的实际采集路径期望 %s，实际 %s", expect, resource.DiskPath)
	}
}

// 正常磁盘路径和取不到类型的路径都原样返回
func TestResolveDiskPathKeep(t *testing.T) {
	ctx := util.GenCtx()
	workPath, err := os.Getwd()
	if err != nil {
		t.Fatalf("取工作目录异常: %+v", err)
	}
	if !isMemoryFs(ctx, workPath) {
		if actual := resolveDiskPath(ctx, workPath); actual != workPath {
			t.Errorf("非内存盘路径应原样返回，期望 %s，实际 %s", workPath, actual)
		}
	}
	path := "/no/such/path/survive_monitor"
	if actual := resolveDiskPath(ctx, path); actual != path {
		t.Errorf("取不到类型的路径应原样返回，期望 %s，实际 %s", path, actual)
	}
}
