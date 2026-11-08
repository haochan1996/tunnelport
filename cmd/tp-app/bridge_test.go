package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// TestBridgeContract 保证 web/app.js 调用的桥方法在 Go 侧真实存在：
// 设计稿改造期间多次出现"死按钮"（控件在、处理器无），此测试把接线固化为契约。
func TestBridgeContract(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("web", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)

	// 静态字面量 call("X")
	want := regexp.MustCompile(`call\("([A-Za-z]+)"`).FindAllStringSubmatch(js, -1)
	// 动态调用点（dispatch 表与三元），按字符串出现判定
	for _, dyn := range []string{"Autostart", "SetAutostart", "MinimizeToTray", "SetMinimizeToTray", "Pause", "Resume"} {
		if strings.Contains(js, `"`+dyn+`"`) {
			want = append(want, []string{"", dyn})
		}
	}

	typ := reflect.TypeOf(App{})
	seen := map[string]bool{}
	for _, m := range want {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		mm, ok := typ.MethodByName(name)
		if !ok {
			t.Errorf("app.js 调用了 App.%s，Go 侧不存在（断桥）", name)
			continue
		}
		// 至少返回 error
		if mm.Type.NumOut() < 1 {
			t.Errorf("App.%s 无返回值，mygo 桥需要 error", name)
		}
	}
	if len(seen) < 15 {
		t.Errorf("捕获的桥方法过少（%d），解析可能失效", len(seen))
	}
}

// TestSettingsRoundtrip 设置项读写往返。
func TestSettingsRoundtrip(t *testing.T) {
	if err := (App{}).SetLocalMap("192.168.1.10:6768"); err != nil {
		t.Fatal(err)
	}
	if v, _ := (App{}).LocalMap(); v != "192.168.1.10:6768" {
		t.Errorf("LocalMap 往返失败: %q", v)
	}
	if err := (App{}).SetEntryHost("example.com"); err != nil {
		t.Fatal(err)
	}
	if v, _ := (App{}).EntryHost(); v != "example.com" {
		t.Errorf("EntryHost 往返失败: %q", v)
	}
}

// TestResetPairing 清除留存配对码。
func TestResetPairing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	saveLastURL("tunnelport://example.com:30000?public=1&fp=x#TEST")
	if readLastURL() == "" {
		t.Fatal("saveLastURL 后读不到")
	}
	if err := (App{}).ResetPairing(); err != nil {
		t.Fatal(err)
	}
	if readLastURL() != "" {
		t.Error("ResetPairing 未清除留存配对码")
	}
}

// TestPauseNotRunning 无引擎时暂停应报错而非静默。
func TestPauseNotRunning(t *testing.T) {
	st.mu.Lock()
	st.eng = nil
	st.mu.Unlock()
	if err := (App{}).Pause(); err == nil {
		t.Error("隧道未运行时 Pause 应返回错误")
	}
}
