// tp-app 桌面端（MyGO 原生 UI）：连接状态 / 入口地址 / 配对码 / 开机自启。
//
//	tp-app <tunnelport://配对链接> [--local-host IP] [--local-port N]
//
// 内嵌自研隧道引擎（进程内直连）；UI 用 egoist/mygo 的原生 UI（GPU 绘制，无 webview）。
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/haochan1996/tunnelport-internal/internal/certs"
	"github.com/haochan1996/tunnelport-internal/internal/engine"
	"github.com/haochan1996/tunnelport-internal/internal/tokenx"
)

type appState struct {
	pairURL   string
	phone     string
	health    string
	localHost string
	localPort int

	eng     engine.Engine
	status  string
	detail  string
	started time.Time
	autoOn  bool
	win     *mygo.Window
}

var s appState

func main() {
	args := os.Args[1:]
	if len(args) == 0 || !strings.HasPrefix(args[0], tokenx.Scheme+"://") {
		fmt.Println("用法: tp-app <tunnelport://配对链接> [--local-host IP] [--local-port N]")
		fmt.Println("（配对码可用 tp status --show-pairing 取回）")
		os.Exit(1)
	}
	s.pairURL = args[0]
	s.localHost, s.localPort = "127.0.0.1", 6768
	s.status, s.detail = "连接中…", ""
	for i := 1; i < len(args)-1; i++ {
		switch args[i] {
		case "--local-host":
			s.localHost = args[i+1]
		case "--local-port":
			s.localPort, _ = strconv.Atoi(args[i+1])
		}
	}
	p, err := tokenx.Parse(s.pairURL)
	if err != nil {
		fmt.Println("错误:", err)
		os.Exit(1)
	}
	s.phone = fmt.Sprintf("%s:%d", p.Host, p.Public)
	s.health = fmt.Sprintf("http://%s:%d/health", p.Host, p.Public+1)
	s.autoOn = autoStartEnabled()

	mygo.App.SetName("tunnelport")
	mygo.App.WhenReady(func() {
		s.win = mygo.NewWindow(mygo.WindowOptions{
			Title: "tunnelport — Orca 稳定入口",
			Width: 520, Height: 560,
			Content: ui.View(view),
		})
		go startEngine(p)
	})
	_ = mygo.App.Run()
}

// startEngine 进程内启动隧道，周期刷新状态。
func startEngine(p tokenx.Pair) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".tunnelport")
	if p.FP != "" {
		if _, err := certs.TOFUProbe(p.Host, p.Control, p.FP, dir); err != nil {
			s.status, s.detail = "证书获取失败", err.Error()
			refresh()
			return
		}
	}
	s.eng = engine.NewNative()
	_ = s.eng.Start(context.Background(), engine.Config{
		Role: "connect", Remote: p.Host, RemotePort: p.Control,
		Token: p.Token, CertRoot: filepath.Join(dir, "root.pem"),
		Services: []engine.Service{
			{Name: engine.ServiceOrca, LocalAddr: net.JoinHostPort(s.localHost, strconv.Itoa(s.localPort))},
			{Name: engine.ServiceHealth, LocalAddr: net.JoinHostPort("127.0.0.1", strconv.Itoa(engine.HealthPort))},
		},
	})
	s.started = time.Now()
	for range time.Tick(2 * time.Second) {
		st := s.eng.Status()
		switch st.State {
		case engine.Connected:
			s.status = "已连接 " + time.Since(s.started).Truncate(time.Minute).String()
			s.detail = ""
		case engine.Reconnecting:
			s.status, s.detail = "重连中…", st.Reason
		case engine.Failed:
			s.status, s.detail = "失败", st.Reason
		default:
			s.status = "连接中…"
		}
		refresh()
	}
}

func refresh() {
	if s.win != nil {
		s.win.Invalidate()
	}
}

// view 主界面（MyGO 原生 UI）
func view(c *ui.Context) {
	ui.Column(c).Fill().Padding(28).Gap(18).Children(func() {
		ui.Row(c).Gap(10).Children(func() {
			ui.Text(c, "tunnelport").FontSize(22).Bold()
			ui.Text(c, s.status).TextColor(c.Theme().TextMuted)
		})
		ui.Divider(c)

		ui.Field(c, "入口地址 — 粘贴到 Orca 桌面端「自定义网络地址」", func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, s.phone).Grow(1)
				ui.Button(c, "复制").OnClick(func() { copyText(s.phone) })
			})
		})

		ui.Field(c, "配对链接 — 换电脑 / 重装时给 tp connect", func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, clip(s.pairURL, 46)).Grow(1)
				ui.Button(c, "复制").OnClick(func() { copyText(s.pairURL) })
			})
		})

		ui.Checkbox(c, &s.autoOn, "开机自动连接").OnChange(func() {
			if s.autoOn {
				autoStartEnable()
			} else {
				autoStartDisable()
			}
		})

		ui.Divider(c)
		ui.Text(c, "手机配对：Orca 桌面端 Set up Mobile → 生成二维码 → 手机扫码").TextColor(c.Theme().TextMuted)
		ui.Text(c, "验证：手机 4G 打开 "+s.health).TextColor(c.Theme().TextMuted)
		if s.detail != "" {
			ui.Text(c, s.detail).TextColor(c.Theme().TextMuted)
		}
	})
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func copyText(s string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("clip.exe")
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		cmd = exec.Command("sh", "-c", "xclip -selection clipboard 2>/dev/null || wl-copy")
	}
	in, _ := cmd.StdinPipe()
	_ = cmd.Start()
	_, _ = in.Write([]byte(s))
	_ = in.Close()
	_ = cmd.Wait()
}

// ---------- 开机自启（与托盘同策略） ----------

func autoStartEnabled() bool {
	switch runtime.GOOS {
	case "windows":
		_, err := os.Stat(startupFile())
		return err == nil
	case "darwin":
		_, err := os.Stat(plistPath())
		return err == nil
	default:
		_, err := os.Stat(systemdUnitPath())
		return err == nil
	}
}

func autoStartEnable() {
	exe, _ := os.Executable()
	switch runtime.GOOS {
	case "windows":
		_ = os.WriteFile(startupFile(), []byte(fmt.Sprintf("@echo off\r\nstart \"\" \"%s\" \"%s\"\r\n", exe, s.pairURL)), 0o755)
	case "darwin":
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.tunnelport.app</string>
<key>ProgramArguments</key><array><string>%s</string><string>%s</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
</dict></plist>`, exe, s.pairURL)
		_ = os.WriteFile(plistPath(), []byte(plist), 0o644)
		_ = exec.Command("launchctl", "load", plistPath()).Run()
	default:
		unit := fmt.Sprintf("[Unit]\nDescription=tunnelport\n[Service]\nExecStart=%s %s\nRestart=always\n[Install]\nWantedBy=default.target\n", exe, s.pairURL)
		_ = os.WriteFile(systemdUnitPath(), []byte(unit), 0o644)
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		_ = exec.Command("systemctl", "--user", "enable", "--now", "tunnelport").Run()
	}
}

func autoStartDisable() {
	switch runtime.GOOS {
	case "windows":
		_ = os.Remove(startupFile())
	case "darwin":
		_ = exec.Command("launchctl", "unload", plistPath()).Run()
		_ = os.Remove(plistPath())
	default:
		_ = exec.Command("systemctl", "--user", "disable", "--now", "tunnelport").Run()
		_ = os.Remove(systemdUnitPath())
	}
}

func startupFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Roaming", "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "tunnelport.bat")
}

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.tunnelport.app.plist")
}

func systemdUnitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "tunnelport.service")
}
