package main

import (
	"testing"
	"time"
)

// TestBootReveal 固化 issue #3 的开机行为：
// 开机自启只驻托盘；连不上（失败/无配对/超时）才把主窗口亮出来；连上就一直不打扰。
func TestBootReveal(t *testing.T) {
	cases := []struct {
		name      string
		state     string
		waited    time.Duration
		wantShow  bool
		wantDone  bool
	}{
		{"已连接不打扰", "已连接", 0, false, true},
		{"连接中继续等", "连接中", 5 * time.Second, false, false},
		{"重连中继续等", "重连中", 5 * time.Second, false, false},
		{"失败立刻亮窗", "失败", 1 * time.Second, true, true},
		{"无配对立刻亮窗", "未连接", 0, true, true},
		{"超时仍不亮", "连接中", bootRevealDelay - time.Second, false, false},
		{"超时亮窗", "重连中", bootRevealDelay, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			show, done := bootReveal(c.state, c.waited)
			if show != c.wantShow || done != c.wantDone {
				t.Fatalf("bootReveal(%q, %v) = (%v,%v)，期望 (%v,%v)",
					c.state, c.waited, show, done, c.wantShow, c.wantDone)
			}
		})
	}
}

// TestHasArgHiddenFlag 保证 --hidden 只认精确参数，不会把 --hidden-x 误判为隐藏启动。
func TestHasArgHiddenFlag(t *testing.T) {
	old := args
	defer func() { args = old }()

	args = []string{"tunnelport://host", "--hidden"}
	if !hasArg(hiddenFlag) {
		t.Fatal("带 --hidden 时应识别为隐藏启动")
	}
	args = []string{"tunnelport://host", "--hidden-x"}
	if hasArg(hiddenFlag) {
		t.Fatal("--hidden-x 不应被识别为隐藏启动")
	}
	args = nil
	if hasArg(hiddenFlag) {
		t.Fatal("无参数时不应识别为隐藏启动")
	}
}
