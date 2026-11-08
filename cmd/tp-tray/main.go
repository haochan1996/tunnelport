// tp-tray 桌面托盘：右下角图标显示连接状态，菜单提供 信息 / 配对码 / 开机自启。
//
//	tp-tray <tunnelport://配对链接> [--local-host IP] [--local-port N]
//
// 托盘本身内嵌隧道（自研引擎进程内直连），无需再开命令行；状态与 tp status 同源。
package main

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"fyne.io/systray"
	"rsc.io/qr"

	"github.com/haochan1996/tunnelport/internal/certs"
	"github.com/haochan1996/tunnelport/internal/engine"
	"github.com/haochan1996/tunnelport/internal/tokenx"
)

//go:embed icon.png
var iconPNG []byte

//go:embed icon.ico
var iconICO []byte

var (
	pairURL   string
	phoneAddr string
	localHost = "127.0.0.1"
	localPort = 6768
	eng       engine.Engine
	started   time.Time
	mStatus   *systray.MenuItem
	mAuto     *systray.MenuItem
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 || !strings.HasPrefix(args[0], tokenx.Scheme+"://") {
		fmt.Println("用法: tp-tray <tunnelport://配对链接> [--local-host IP] [--local-port N]")
		fmt.Println("（配对码可在 serve 端用 tp status --show-pairing 取回）")
		os.Exit(1)
	}
	pairURL = args[0]
	for i := 1; i < len(args)-1; i++ {
		switch args[i] {
		case "--local-host":
			localHost = args[i+1]
		case "--local-port":
			localPort, _ = strconv.Atoi(args[i+1])
		}
	}
	p, err := tokenx.Parse(pairURL)
	if err != nil {
		fmt.Println("错误:", err)
		os.Exit(1)
	}
	phoneAddr = fmt.Sprintf("%s:%d", p.Host, p.Public)

	systray.Run(onReady, onExit)
}

// trayLog GUI 无控制台：关键事件落盘 ~/.tunnelport/tray.log（自诊断）。
func trayLog(format string, a ...any) {
	home, _ := os.UserHomeDir()
	_ = os.MkdirAll(filepath.Join(home, ".tunnelport"), 0o700)
	f, err := os.OpenFile(filepath.Join(home, ".tunnelport", "tray.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, time.Now().Format(time.RFC3339)+" "+format+"\n", a...)
}

func onReady() {
	trayLog("启动：配对码 host=%s", pairURL[:min(40, len(pairURL))])
	if runtime.GOOS == "windows" {
		systray.SetIcon(iconICO)
	} else {
		systray.SetIcon(iconPNG)
	}
	systray.SetTooltip("tunnelport — Orca 稳定入口")
	systray.SetTitle("tp")

	mStatus = systray.AddMenuItem("状态：连接中…", "当前连接状态")
	mStatus.Disable()
	systray.AddSeparator()
	mCopy := systray.AddMenuItem("复制入口地址", "复制到剪贴板，粘贴到 Orca 桌面端")
	mQR := systray.AddMenuItem("显示入口二维码", "浏览器打开（手机可扫）")
	systray.AddSeparator()
	mAuto = systray.AddMenuItem("开机自启", "重启后自动连接")
	if autoStartEnabled() {
		mAuto.SetTitle("开机自启 ✓")
	}
	mAuto.SetTooltip("开关开机自动启动（当前状态见标题）")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "停止连接并退出")

	// 启动隧道（进程内引擎）；首连先做 TOFU 证书获取（指纹在配对码里）
	p, _ := tokenx.Parse(pairURL)
	dir, _ := os.UserHomeDir()
	dir = filepath.Join(dir, ".tunnelport")
	if p.FP != "" {
		if _, err := certs.TOFUProbe(p.Host, p.Control, p.FP, dir); err != nil {
			mStatus.SetTitle("状态：证书获取失败")
			systray.SetTooltip("tunnelport — " + err.Error())
			trayLog("TOFU 失败: %v", err)
			return
		}
		trayLog("TOFU 通过：%s:%d", p.Host, p.Control)
	}
	eng = engine.NewNative()
	_ = eng.Start(context.Background(), engine.Config{
		Role: "connect", Remote: p.Host, RemotePort: p.Control,
		Token: p.Token, CertRoot: filepath.Join(dir, "root.pem"),
		Services: []engine.Service{
			{Name: engine.ServiceOrca, LocalAddr: net.JoinHostPort(localHost, strconv.Itoa(localPort))},
			{Name: engine.ServiceHealth, LocalAddr: net.JoinHostPort("127.0.0.1", strconv.Itoa(engine.HealthPort))},
		},
	})
	started = time.Now()
	trayLog("引擎启动：%s:%d -> %s", p.Host, p.Control, net.JoinHostPort(localHost, strconv.Itoa(localPort)))
	go func() {
		for range time.Tick(3 * time.Second) {
			st := eng.Status()
			switch st.State {
			case engine.Connected:
				mStatus.SetTitle(fmt.Sprintf("状态：已连接 %s", time.Since(started).Truncate(time.Minute)))
				systray.SetTooltip("tunnelport — 已连接 " + phoneAddr)
			case engine.Reconnecting:
				mStatus.SetTitle("状态：重连中…")
				systray.SetTooltip("tunnelport — 重连中")
			case engine.Failed:
				mStatus.SetTitle("状态：失败 — " + st.Reason)
				systray.SetTooltip("tunnelport — 失败")
				trayLog("引擎失败: %s", st.Reason)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-mCopy.ClickedCh:
				copyText(phoneAddr)
			case <-mQR.ClickedCh:
				showQR()
			case <-mAuto.ClickedCh:
				if autoStartEnabled() {
					autoStartDisable()
					mAuto.SetTitle("开机自启")
				} else {
					autoStartEnable()
					mAuto.SetTitle("开机自启 ✓")
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func onExit() {
	if eng != nil {
		_ = eng.Stop()
	}
}

// ---------- 小工具 ----------

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

// showQR 生成含入口地址二维码的 HTML 并用浏览器打开。
func showQR() {
	code, err := qr.Encode(phoneAddr, qr.M)
	if err != nil {
		return
	}
	bmp := code.Bitmap // 位打包：每行 Stride 字节，高位在前
	size := code.Size
	stride := code.Stride
	scale := 6
	w := size * scale
	var sb strings.Builder
	sb.WriteString(`<!doctype html><meta charset="utf-8"><title>tunnelport</title>`)
	sb.WriteString(`<body style="font-family:system-ui;background:#F5F6F4;color:#141A17;text-align:center;padding:40px">`)
	sb.WriteString(`<h2 style="margin-bottom:4px">手机 Orca 填写 / 扫码</h2>`)
	sb.WriteString(fmt.Sprintf(`<p style="font-size:22px;letter-spacing:.05em">%s</p>`, phoneAddr))
	sb.WriteString(`<svg width="` + strconv.Itoa(w) + `" height="` + strconv.Itoa(w) + `" viewBox="0 0 ` + strconv.Itoa(size) + ` ` + strconv.Itoa(size) + `">`)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if bmp[y*stride+x/8]&(1<<uint(7-x%8)) != 0 {
				sb.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="1" height="1"/>`, x, y))
			}
		}
	}
	sb.WriteString(`</svg><p style="color:#5C6963">配对二维码由 Orca 桌面端生成（Set up Mobile）</p></body>`)
	out := filepath.Join(os.TempDir(), "tunnelport-qr.html")
	_ = os.WriteFile(out, []byte(sb.String()), 0o644)
	openBrowser(out)
}

func openBrowser(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	_ = cmd.Start()
}

// ---------- 开机自启 ----------

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
		_ = os.WriteFile(startupFile(),
			[]byte(fmt.Sprintf("@echo off\r\nstart \"\" \"%s\" \"%s\"\r\n", exe, pairURL)), 0o755)
	case "darwin":
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.tunnelport.tray</string>
<key>ProgramArguments</key><array><string>%s</string><string>%s</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
</dict></plist>`, exe, pairURL)
		_ = os.WriteFile(plistPath(), []byte(plist), 0o644)
		_ = exec.Command("launchctl", "load", plistPath()).Run()
	default:
		unit := fmt.Sprintf("[Unit]\nDescription=tunnelport tray\n[Service]\nExecStart=%s %s\nRestart=always\n[Install]\nWantedBy=default.target\n", exe, pairURL)
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
	return filepath.Join(home, "Library", "LaunchAgents", "com.tunnelport.tray.plist")
}

func systemdUnitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "tunnelport.service")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
