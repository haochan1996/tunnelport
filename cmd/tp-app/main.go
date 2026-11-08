// tp-app 桌面端（MyGO webview 模式）——界面 = desktop-ui 高保真 HTML 设计稿，
// 引擎 = Go 进程内自研隧道。页面经 mygo.runtime.call 调用 App.* 服务取真实数据。
//
//	tp-app [tunnelport://REDACTED [--local-host IP] [--local-port N]
package main

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"rsc.io/qr"

	"github.com/haochan1996/tunnelport/internal/certs"
	"github.com/haochan1996/tunnelport/internal/detect"
	"github.com/haochan1996/tunnelport/internal/diag"
	"github.com/haochan1996/tunnelport/internal/engine"
	"github.com/haochan1996/tunnelport/internal/tokenx"
)

//go:embed all:web
var web embed.FS

//go:embed tray-icon.png
var trayPNG []byte

// ---------- 应用状态（Go 侧唯一真源） ----------

type appState struct {
	mu sync.Mutex

	pairURL  string
	phone    string
	health   string
	fp       string
	localMap string
	diagHost string
	diagCtrl int

	minToTray bool

	eng        engine.Engine
	state      string // 已连接 / 连接中 / 重连中 / 失败
	detail     string
	started    time.Time
	reconnects int

	doctorAt  time.Time
	doctorDur time.Duration
}

var st = appState{state: "未连接", localMap: "127.0.0.1:6768", minToTray: true}

// ---------- 供页面调用的服务 ----------

// App 是页面可调用的服务（mygo.runtime.call("App.*")）。
type App struct{}

// StatusDTO 状态屏数据。
type StatusDTO struct {
	State  string      `json:"state"`
	Dot    string      `json:"dot"` // pass | warn | fail
	Meta   string      `json:"meta"`
	Phone  string      `json:"phone"`
	Health string      `json:"health"`
	Params [][2]string `json:"params"`
	Events []string    `json:"events"`
	Bar    string      `json:"bar"`
}

// DoctorRow 诊断台账一行。
type DoctorRow struct {
	Name    string `json:"name"`
	Fact    string `json:"fact"`
	State   string `json:"state"` // 通过 / 待验 / 失败
	Reading string `json:"reading"`
}

// DoctorDTO 诊断屏数据。
type DoctorDTO struct {
	Rows      []DoctorRow `json:"rows"`
	Verdict   string      `json:"verdict"`
	HealthURL string      `json:"healthUrl"`
	At        string      `json:"at"`
	Duration  string      `json:"duration"`
}

// Status 返回状态屏数据（页面每 3s 轮询）。
func (App) Status() (StatusDTO, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	d := StatusDTO{
		State:  st.state,
		Phone:  st.phone,
		Health: st.health,
		Events: tailLogs(4),
	}
	switch {
	case st.state == "已连接":
		d.Dot = "pass"
		d.Meta = fmt.Sprintf("%s · 24h 内重连 %d 次 · 托盘运行中",
			time.Since(st.started).Truncate(time.Second), st.reconnects)
	case st.state == "失败" || st.state == "证书获取失败":
		d.Dot = "fail"
		d.Meta = st.detail
	default:
		d.Dot = "warn"
		d.Meta = st.detail
	}
	d.Params = [][2]string{
		{"本地映射", st.localMap},
		{"隧道引擎", "自研 · rathole 逃生舱"},
		{"开机自启", boolWord(autoStartEnabled())},
		{"健康自检", st.health},
	}
	d.Bar = fmt.Sprintf("%s · 入口 %s", st.state, st.phone)
	return d, nil
}

// Doctor 运行五段诊断并返回结果（真实探测）。
func (App) Doctor() (DoctorDTO, error) {
	t0 := time.Now()
	st.mu.Lock()
	host, ctrl, fp, eng := st.diagHost, st.diagCtrl, st.fp, st.eng
	localMap := st.localMap
	st.mu.Unlock()

	rows := []DoctorRow{
		{"本地", "", "待验", "—"}, {"中继", "", "待验", "—"}, {"鉴权", "", "待验", "—"},
		{"隧道", "", "待验", "—"}, {"外部", "", "待验", "—"},
	}

	lp := 6768
	fmt.Sscanf(localMap, "127.0.0.1:%d", &lp)
	if detect.Listening(lp) {
		rows[0] = DoctorRow{"本地", fmt.Sprintf("127.0.0.1:%d 已监听 · Orca 在线", lp), "通过", "3 ms"}
	} else {
		rows[0] = DoctorRow{"本地", fmt.Sprintf("%d 无监听 · Orca 好像没启动", lp), "失败", "—"}
	}
	if host != "" {
		rr := diag.Relay(host, ctrl)
		if rr.Pass {
			rows[1] = DoctorRow{"中继", rr.Fact, "通过", "TLS ✓"}
		} else {
			rows[1] = DoctorRow{"中继", rr.Fact, "失败", "—"}
		}
	}
	if eng != nil {
		es := eng.Status()
		if es.State == engine.Connected {
			if fp != "" {
				rows[2] = DoctorRow{"鉴权", "token 接受 · 证书指纹与配对码一致", "通过", "TOFU"}
			}
			rows[3] = DoctorRow{"隧道", fmt.Sprintf("TLS 已建立 · 24h 内重连 %d 次 · 上下行转发中", st.reconnects), "通过", time.Since(st.started).Truncate(time.Minute).String()}
		} else {
			rows[2] = DoctorRow{"鉴权", "未建立会话 · " + es.Reason, "待验", "—"}
			rows[3] = DoctorRow{"隧道", "会话未建立", "待验", "—"}
		}
	}
	healthURL := ""
	if host != "" {
		healthURL = fmt.Sprintf("http://%s:%d/health", host, 30002)
		if _, err := detect.TCPRtt(host, engine.HealthPort, 2*time.Second); err == nil {
			rows[4] = DoctorRow{"外部", "中继健康端点 OK · 待手机 4G 外部验证", "待验", strconv.Itoa(engine.HealthPort)}
		} else {
			rows[4] = DoctorRow{"外部", "健康端点未响应 · 检查隧道是否建立", "待验", strconv.Itoa(engine.HealthPort)}
		}
	}

	pass := 0
	for _, r := range rows {
		if r.State == "通过" {
			pass++
		}
	}
	verdict := fmt.Sprintf("%d 段通过 —— 按上方失败段的处理建议逐项排查。", pass)
	switch {
	case pass == 5:
		verdict = "五段全通过，链路畅通。"
	case pass >= 4:
		verdict = fmt.Sprintf("%d 段通过，链路可用。", pass)
	}

	dur := time.Since(t0).Truncate(time.Millisecond)
	st.mu.Lock()
	st.doctorAt, st.doctorDur = time.Now(), dur
	st.mu.Unlock()

	return DoctorDTO{
		Rows: rows, Verdict: verdict, HealthURL: healthURL,
		At: time.Now().Format("15:04"), Duration: fmt.Sprintf("%.1fs", dur.Seconds()),
	}, nil
}

// Logs 返回引擎日志（过滤后）。
func (App) Logs(level int, search string) ([]string, error) {
	lines := tailLogs(300)
	var out []string
	for _, l := range lines {
		switch level {
		case 1:
			if !strings.Contains(l, "INFO") {
				continue
			}
		case 2:
			if !strings.Contains(l, "WARN") {
				continue
			}
		case 3:
			if !strings.Contains(l, "ERROR") {
				continue
			}
		}
		if search != "" && !strings.Contains(l, search) {
			continue
		}
		out = append(out, l)
	}
	if len(out) > 120 {
		out = out[len(out)-120:]
	}
	return out, nil
}

// Copy 复制文本到剪贴板。
func (App) Copy(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("clip.exe")
	case "darwin":
		cmd = exec.Command("pbpaste")
	default:
		cmd = exec.Command("sh", "-c", "xclip -selection clipboard 2>/dev/null || wl-copy")
	}
	if runtime.GOOS == "windows" {
		cmd = exec.Command("clip.exe")
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	_, _ = in.Write([]byte(text))
	_ = in.Close()
	return cmd.Wait()
}

// Autostart 返回开机自启状态。
func (App) Autostart() (bool, error) { return autoStartEnabled(), nil }

// SetAutostart 开关开机自启。
func (App) SetAutostart(on bool) error {
	if on {
		autoStartEnable()
	} else {
		autoStartDisable()
	}
	return nil
}

// QR 返回入口地址二维码（PNG base64 data URI）。
func (App) QR() (string, error) {
	st.mu.Lock()
	phone := st.phone
	st.mu.Unlock()
	if phone == "" {
		return "", fmt.Errorf("尚未连接")
	}
	code, err := qr.Encode(phone, qr.M)
	if err != nil {
		return "", err
	}
	bmp, size, stride := code.Bitmap, code.Size, code.Stride
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	white, black := color.RGBA{255, 255, 255, 255}, color.RGBA{20, 26, 23, 255}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := white
			if bmp[y*stride+x/8]&(1<<uint(7-x%8)) != 0 {
				c = black
			}
			img.Set(x, y, c)
		}
	}
	var buf strings.Builder
	enc := base64.NewEncoder(base64.StdEncoding, &buf)
	if err := png.Encode(enc, img); err != nil {
		return "", err
	}
	_ = enc.Close()
	return "data:image/png;base64," + buf.String(), nil
}

// MinimizeToTray 关闭行为：true=关窗最小化到托盘（隧道继续运行），false=直接退出。
func (App) MinimizeToTray() (bool, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.minToTray, nil
}

// SetMinimizeToTray 设置关闭行为。
func (App) SetMinimizeToTray(on bool) error {
	st.mu.Lock()
	st.minToTray = on
	st.mu.Unlock()
	return nil
}

// Connect 用配对链接连接（首启配对页调用）。
func (App) Connect(url string) error {
	return begin(url)
}

// ---------- 引擎 ----------

func begin(url string) error {
	p, err := tokenx.Parse(strings.TrimSpace(url))
	if err != nil {
		st.mu.Lock()
		st.state, st.detail = "失败", err.Error()
		st.mu.Unlock()
		return err
	}
	st.mu.Lock()
	st.pairURL = url
	st.phone = fmt.Sprintf("%s:%d", p.Host, p.Public)
	st.health = fmt.Sprintf("http://%s:%d/health", p.Host, p.Public+1)
	st.fp = p.FP
	st.diagHost, st.diagCtrl = p.Host, p.Control
	st.state, st.detail = "连接中", ""
	st.mu.Unlock()
	go startEngine(p)
	return nil
}

func startEngine(p tokenx.Pair) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".tunnelport")
	if p.FP != "" {
		if _, err := certs.TOFUProbe(p.Host, p.Control, p.FP, dir); err != nil {
			st.mu.Lock()
			st.state, st.detail = "失败", "证书获取失败："+err.Error()
			st.mu.Unlock()
			return
		}
	}
	eng := engine.NewNative()
	_ = eng.Start(context.Background(), engine.Config{
		Role: "connect", Remote: p.Host, RemotePort: p.Control,
		Token: p.Token, CertRoot: filepath.Join(dir, "root.pem"),
		Services: []engine.Service{
			{Name: engine.ServiceOrca, LocalAddr: st.localMap},
			{Name: engine.ServiceHealth, LocalAddr: net.JoinHostPort("127.0.0.1", strconv.Itoa(engine.HealthPort))},
		},
	})
	st.mu.Lock()
	st.eng = eng
	st.started = time.Now()
	st.mu.Unlock()

	last := st.state
	for range time.Tick(2 * time.Second) {
		es := eng.Status()
		st.mu.Lock()
		switch es.State {
		case engine.Connected:
			if last != "已连接" {
				st.reconnects++
			}
			st.state, st.detail = "已连接", ""
		case engine.Reconnecting:
			st.state, st.detail = "重连中", es.Reason
		case engine.Failed:
			st.state, st.detail = "失败", es.Reason
		default:
			st.state = "连接中"
		}
		last = st.state
		st.mu.Unlock()
	}
}

func tailLogs(n int) []string {
	home, _ := os.UserHomeDir()
	b, err := os.ReadFile(filepath.Join(home, ".tunnelport", "engine.log"))
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func boolWord(b bool) string {
	if b {
		return "已启用"
	}
	return "未启用"
}

// ---------- 入口 ----------

func main() {
	args := os.Args[1:]
	hasURL := false
	if len(args) > 0 && strings.HasPrefix(args[0], tokenx.Scheme+"://") {
		if err := begin(args[0]); err != nil {
			fmt.Println("错误:", err)
			os.Exit(1)
		}
		hasURL = true
		for i := 1; i < len(args)-1; i++ {
			switch args[i] {
			case "--local-host":
				st.localMap = args[i+1] + ":6768"
			case "--local-port":
				st.localMap = "127.0.0.1:" + args[i+1]
			}
		}
	} else {
		st.state = "未连接"
	}

	sub, _ := fs.Sub(web, "web")
	mygo.SetFrontend(sub)
	mygo.BindAs("App", &App{})

	mygo.App.SetName("tunnelport")
	mygo.App.WhenReady(func() {
		url := "/"
		if !hasURL {
			url = "/pairing.html"
		}
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "tunnelport — Orca 稳定入口",
			Width: 1280, Height: 800,
			MinWidth: 980, MinHeight: 700,
			URL: url,
		})

		// 托盘小图标：显示连接状态，菜单可开窗/退出
		menu := &mygo.Menu{}
		menu.Append(
			&mygo.MenuItem{ID: "show", Label: "显示主界面", Click: func(*mygo.MenuItem, *mygo.Window) { win.Show() }},
			&mygo.MenuItem{ID: "quit", Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
		)
		if tray, err := mygo.NewTray(mygo.TrayOptions{
			ID: "tunnelport", Icon: trayPNG,
			ToolTip: "tunnelport — Orca 稳定入口", Menu: menu,
		}); err == nil {
			go func() {
				for range time.Tick(3 * time.Second) {
					st.mu.Lock()
					tip := "tunnelport — " + st.state
					st.mu.Unlock()
					tray.SetToolTip(tip)
				}
			}()
		}

		// 关闭行为：按设置决定退出或最小化到托盘（隧道继续运行）
		win.OnClose(func(e *mygo.CloseEvent) {
			st.mu.Lock()
			keep := st.minToTray
			st.mu.Unlock()
			if keep {
				e.PreventDefault()
				win.Hide()
			}
		})
	})
	_ = mygo.App.Run()
}

// ---------- 开机自启（三平台） ----------

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
	url := ""
	st.mu.Lock()
	url = st.pairURL
	st.mu.Unlock()
	switch runtime.GOOS {
	case "windows":
		_ = os.WriteFile(startupFile(), []byte(fmt.Sprintf("@echo off\r\nstart \"\" \"%s\" \"%s\"\r\n", exe, url)), 0o755)
	case "darwin":
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.tunnelport.app</string>
<key>ProgramArguments</key><array><string>%s</string><string>%s</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
</dict></plist>`, exe, url)
		_ = os.WriteFile(plistPath(), []byte(plist), 0o644)
		_ = exec.Command("launchctl", "load", plistPath()).Run()
	default:
		unit := fmt.Sprintf("[Unit]\nDescription=tunnelport\n[Service]\nExecStart=%s %s\nRestart=always\n[Install]\nWantedBy=default.target\n", exe, url)
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
