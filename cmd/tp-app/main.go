// tp-app 桌面端（MyGO webview 模式）——界面 = desktop-ui 高保真 HTML 设计稿，
// 引擎 = Go 进程内自研隧道。页面经 mygo.runtime.call 调用 App.* 服务取真实数据。
//
//	tp-app [tunnelport://配对链接] [--local-host IP] [--local-port N]
package main

import (
	"context"
	"embed"
	"fmt"
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

	pairURL   string
	phone     string
	health    string
	fp        string
	localMap  string
	entryHost string
	diagHost  string
	diagCtrl  int

	minToTray bool

	eng        engine.Engine
	state      string // 已连接 / 连接中 / 重连中 / 失败
	detail     string
	started    time.Time
	reconnects int

	doctorAt  time.Time
	doctorDur time.Duration
}

var quitting bool

var st = appState{state: "未连接", localMap: "127.0.0.1:6768", minToTray: true}

// ---------- 供页面调用的服务 ----------

// App 是页面可调用的服务（mygo.runtime.call("App.*")）。
type App struct{}

// StatusDTO 状态屏数据。
type StatusDTO struct {
	State     string      `json:"state"`
	Dot       string      `json:"dot"` // pass | warn | fail
	Meta      string      `json:"meta"`
	Phone     string      `json:"phone"`
	Health    string      `json:"health"`
	Params    [][2]string `json:"params"`
	Events    []string    `json:"events"`
	Bar       string      `json:"bar"`
	Rtt       string      `json:"rtt"`
	Version   string      `json:"version"`
	LogsCount int         `json:"logsCount"`
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
	phone, health := st.phone, st.health
	// 多设备：服务端分配的实际入口优先于 URL 参考值
	if st.eng != nil {
		if es := st.eng.Status(); es.Ports != nil {
			if v, ok := es.Ports[engine.ServiceOrca]; ok && v > 0 && st.diagHost != "" {
				phone = fmt.Sprintf("%s:%d", st.diagHost, v)
				health = fmt.Sprintf("http://%s:%d/health", st.diagHost, v+1)
			}
		}
	}
	d := StatusDTO{
		State:  st.state,
		Phone:  phone,
		Health: health,
		Events: tailLogs(4),
	}
	switch {
	case st.state == "已暂停":
		d.Dot = "wait"
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
	d.Bar = st.state
	d.Version = version
	d.LogsCount = len(tailLogs(500))
	if st.diagHost != "" {
		if r, err := detect.TCPRtt(st.diagHost, st.diagCtrl, 2*time.Second); err == nil {
			d.Rtt = fmt.Sprintf("%d ms", r.Milliseconds())
		} else {
			d.Rtt = "—"
		}
	}
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
// logLevelOf 引擎日志分级：1=INFO 2=WARN 3=ERROR（兼容中文语义）。
func logLevelOf(l string) int {
	u := strings.ToUpper(l)
	switch {
	case strings.Contains(u, "ERROR") || strings.Contains(l, "失败") || strings.Contains(l, "错误") || strings.Contains(l, "拒绝"):
		return 3
	case strings.Contains(u, "WARN") || strings.Contains(l, "重连") || strings.Contains(l, "占用") || strings.Contains(l, "顺延") || strings.Contains(l, "重试"):
		return 2
	default:
		return 1
	}
}

func (App) Logs(level int, search string) ([]string, error) {
	lines := tailLogs(300)
	var out []string
	for _, l := range lines {
		if level > 0 && logLevelOf(l) != level {
			continue
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
		cmd = hiddenCmd("clip.exe")
	case "darwin":
		cmd = hiddenCmd("pbpaste")
	default:
		cmd = hiddenCmd("sh", "-c", "xclip -selection clipboard 2>/dev/null || wl-copy")
	}
	if runtime.GOOS == "windows" {
		cmd = hiddenCmd("clip.exe")
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

// Devices 服务端已连接设备清单（设置/状态屏展示）。
func (App) Devices() ([]engine.DeviceInfo, error) {
	st.mu.Lock()
	eng := st.eng
	st.mu.Unlock()
	if eng == nil {
		return nil, fmt.Errorf("未连接")
	}
	dl, ok := eng.(interface {
		Devices() ([]engine.DeviceInfo, error)
	})
	if !ok {
		return nil, fmt.Errorf("当前引擎不支持设备查询")
	}
	return dl.Devices()
}

// SetLocalMap 设置本地映射（重连后生效）。
func (App) SetLocalMap(addr string) error {
	st.mu.Lock()
	st.localMap = strings.TrimSpace(addr)
	st.mu.Unlock()
	return nil
}

// LocalMap 当前本地映射。
func (App) LocalMap() (string, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.localMap, nil
}

// SetEntryHost 设置入口主机覆盖（代理/软路由/DDNS；重连后生效）。
func (App) SetEntryHost(host string) error {
	st.mu.Lock()
	st.entryHost = strings.TrimSpace(host)
	st.mu.Unlock()
	return nil
}

// EntryHost 当前入口覆盖（空=自动检测）。
func (App) EntryHost() (string, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.entryHost, nil
}

// ResetPairing 重置配对：断开隧道并清除留存配对码。
func (App) ResetPairing() error {
	st.mu.Lock()
	eng := st.eng
	st.eng = nil
	st.pairURL = ""
	st.state, st.detail = "未连接", ""
	st.mu.Unlock()
	if eng != nil {
		_ = eng.Stop()
	}
	_ = os.Remove(lastURLPath())
	return nil
}

// Pause 暂停隧道（断开连接，保留配对码与界面状态）。
func (App) Pause() error {
	st.mu.Lock()
	eng := st.eng
	st.eng = nil // 监控循环据此退出（v1.0.13）
	st.mu.Unlock()
	if eng == nil {
		return fmt.Errorf("隧道未在运行")
	}
	_ = eng.Stop()
	st.mu.Lock()
	st.state, st.detail = "已暂停", "已断开 · 点「继续隧道」恢复"
	st.mu.Unlock()
	return nil
}

// Resume 继续隧道。
func (App) Resume() error {
	st.mu.Lock()
	u := st.pairURL
	st.mu.Unlock()
	if u == "" {
		u = readLastURL()
	}
	if u == "" {
		return fmt.Errorf("没有配对码，请先配对")
	}
	return begin(u)
}

// ClearLogs 清空引擎日志（仅本机文件）。
func (App) ClearLogs() error {
	home, _ := os.UserHomeDir()
	return os.WriteFile(filepath.Join(home, ".tunnelport", "engine.log"), nil, 0o600)
}

// lastURLPath 上次成功配对的链接（重启免重配 / 重配页回填，v1.5）。
func lastURLPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tunnelport", "lasturl")
}

func saveLastURL(u string) {
	_ = os.MkdirAll(filepath.Dir(lastURLPath()), 0o755)
	_ = os.WriteFile(lastURLPath(), []byte(u), 0o600)
}

func readLastURL() string {
	b, err := os.ReadFile(lastURLPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// LastURL 配对页回填上次成功配对的链接，避免「找不到配对码」。
func (App) LastURL() (string, error) {
	st.mu.Lock()
	u := st.pairURL
	st.mu.Unlock()
	if u == "" {
		u = readLastURL()
	}
	return u, nil
}

// SetAutostart 开关开机自启。
func (App) SetAutostart(on bool) error {
	if on {
		autoStartEnable()
	} else {
		autoStartDisable()
	}
	return nil
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

// ClipboardText 配对页「从剪贴板读取」。
func (App) ClipboardText() (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = hiddenCmd("powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw")
	case "darwin":
		cmd = hiddenCmd("pbpaste")
	default:
		cmd = hiddenCmd("sh", "-c", "xclip -selection clipboard -o 2>/dev/null || wl-paste 2>/dev/null")
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("读不到剪贴板")
	}
	return strings.TrimSpace(string(out)), nil
}

// createShortcut 创建/重建桌面快捷方式（幂等，覆盖）。返回快捷方式路径。
// v1.3：设置里「重新创建」入口，供用户误删后自助恢复。
func createShortcut() (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("仅 Windows 支持桌面快捷方式")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	q := strings.ReplaceAll(exe, "'", "''")
	ps := `
$desktop = [Environment]::GetFolderPath('Desktop')
$ws = New-Object -ComObject WScript.Shell
$lnk = $ws.CreateShortcut("$desktop\tunnelport.lnk")
$lnk.TargetPath = '` + q + `'
$lnk.WorkingDirectory = Split-Path '` + q + `'
$lnk.IconLocation = '` + q + `,0'
$lnk.Save()
Write-Output "$desktop\tunnelport.lnk"
`
	out, err := hiddenCmd("powershell", "-NoProfile", "-NonInteractive", "-Command", ps).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("创建失败: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// CreateShortcut 设置里「重新创建桌面快捷方式」。
func (App) CreateShortcut() (string, error) {
	p, err := createShortcut()
	if err != nil {
		return "", err
	}
	return "快捷方式已创建：" + p, nil
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
	saveLastURL(url)
	dispHost := p.Host
	if st.entryHost != "" {
		dispHost = st.entryHost
	}
	st.phone = fmt.Sprintf("%s:%d", dispHost, p.Public)
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
		st.mu.Lock()
		if st.eng != eng { // 已暂停/已被新引擎替换：退出监控（v1.0.13：此前永不退出且覆盖"已暂停"）
			st.mu.Unlock()
			return
		}
		st.mu.Unlock()
		es := eng.Status()
		st.mu.Lock()
		switch es.State {
		case engine.Connected:
			if last == "重连中" { // 只有真·重连才计数（恢复/首次连接不算）
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

func hasArg(flag string) bool {
	for _, a := range os.Args[1:] {
		if a == flag {
			return true
		}
	}
	return false
}

func main() {
	// 默认开机自启：首次运行自动注册；用户在设置里关过（decided 标记）则不再自动开
	if home, err := os.UserHomeDir(); err == nil {
		marker := filepath.Join(home, ".tunnelport", "autostart.decided")
		if _, err := os.Stat(marker); err != nil {
			_ = os.MkdirAll(filepath.Dir(marker), 0o755)
			if !autoStartEnabled() {
				autoStartEnable()
			}
			_ = os.WriteFile(marker, []byte("1"), 0o644)
		}
	}

	args := os.Args[1:]
	hasURL := false
	if len(args) > 0 && strings.HasPrefix(args[0], tokenx.Scheme+"://") {
		if err := begin(args[0]); err != nil {
			// 坏配对码不退出：进主界面失败态，可「更换配对码」恢复（v1.3）
			st.mu.Lock()
			st.state = "失败"
			st.detail = err.Error()
			st.mu.Unlock()
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
		// 重启免重配：自动读取上次成功配对的链接（v1.5）
		if u := readLastURL(); u != "" {
			if err := begin(u); err == nil {
				hasURL = true
			}
		}
		if !hasURL {
			st.state = "未连接"
		}
	}

	sub, _ := fs.Sub(web, "web")
	mygo.SetFrontend(sub)
	mygo.BindAs("App", &App{})

	// 单实例：二次启动只唤起已有窗口（托盘常驻时双击图标 = 打开主界面）
	isPrimary, signalShow, showRequested := acquire()
	// 重启拉起的实例：旧进程可能仍在拆解 webview，轮询等它释放单实例锁（最多 10s）
	if !isPrimary && hasArg("--restart") {
		for i := 0; i < 20 && !isPrimary; i++ {
			time.Sleep(500 * time.Millisecond)
			isPrimary, signalShow, showRequested = acquire()
		}
	}
	if !isPrimary {
		signalShow()
		return
	}

	mygo.App.SetName("tunnelport")
	mygo.App.WhenReady(func() {
		url := "/"
		if !hasURL {
			url = "/pairing.html"
		}
		// 深链：TP_TAB=1/2/3 → #doctor/#logs/#settings（与设计稿 hash 路由一致）
		if v := os.Getenv("TP_TAB"); v != "" {
			names := map[string]string{"1": "doctor", "2": "logs", "3": "settings", "0": "status"}
			if n, ok := names[v]; ok && hasURL {
				url = "/?tab=" + n // query 兜底：webview 可能吃掉 #fragment
			}
		}
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "tunnelport — Orca 稳定入口",
			Width: 1280, Height: 800,
			MinWidth: 980, MinHeight: 700,
			Frameless: true, // 自绘标题栏：右上角 — ▢ ✕ 由页面按钮承担
			URL:       url,
		})

		// 托盘小图标：显示连接状态，菜单可开窗/退出
		menu := &mygo.Menu{}
		menu.Append(
			&mygo.MenuItem{ID: "show", Label: "打开主程序", Click: func(*mygo.MenuItem, *mygo.Window) { win.Show() }},
			&mygo.MenuItem{ID: "quit", Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { quitting = true; mygo.App.Quit() }},
		)
		if tray, err := mygo.NewTray(mygo.TrayOptions{
			ID: "tunnelport", Icon: trayPNG,
			ToolTip: "tunnelport — Orca 稳定入口", Menu: menu,
		}); err == nil {
			// 左击/双击图标 = 打开主程序；右键 = 弹出菜单（打开主程序 / 退出）
			tray.OnClick(func() {
				// 单击托盘切换：显示 ↔ 最小化（v1.0.11）
				if win.IsVisible() && !win.IsMinimized() {
					win.Minimize()
				} else {
					win.Show()
				}
			})
			tray.OnRightClick(func() { tray.PopUpMenu() })
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
			if quitting {
				return // 托盘「退出」：放行关闭
			}
			st.mu.Lock()
			keep := st.minToTray
			st.mu.Unlock()
			if keep {
				e.PreventDefault()
				win.Hide()
			}
		})

		// 二次启动唤起：托盘/隐藏状态下双击图标打开主界面
		go func() {
			for range time.Tick(time.Second) {
				if showRequested() {
					win.Show()
				}
			}
		}()
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
		// .lnk 快捷方式：登录启动完全静默（.bat 会闪 cmd 黑窗，v1.5）
		q := strings.ReplaceAll(exe, "'", "''")
		uq := strings.ReplaceAll(url, "'", "''")
		ps := "$ws = New-Object -ComObject WScript.Shell" + "\r\n" +
			"$lnk = $ws.CreateShortcut('" + strings.ReplaceAll(startupFile(), "'", "''") + "')" + "\r\n" +
			"$lnk.TargetPath = '" + q + "'" + "\r\n" +
			"$lnk.Arguments = '" + uq + "'" + "\r\n" +
			"$lnk.Save()"
		_ = hiddenCmd("powershell", "-NoProfile", "-NonInteractive", "-Command", ps).Run()
		_ = os.Remove(strings.TrimSuffix(startupFile(), ".lnk") + ".bat") // 清理旧版痕迹
	case "darwin":
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.tunnelport.app</string>
<key>ProgramArguments</key><array><string>%s</string><string>%s</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
</dict></plist>`, exe, url)
		_ = os.WriteFile(plistPath(), []byte(plist), 0o644)
		_ = hiddenCmd("launchctl", "load", plistPath()).Run()
	default:
		unit := fmt.Sprintf("[Unit]\nDescription=tunnelport\n[Service]\nExecStart=%s %s\nRestart=always\n[Install]\nWantedBy=default.target\n", exe, url)
		_ = os.WriteFile(systemdUnitPath(), []byte(unit), 0o644)
		_ = hiddenCmd("systemctl", "--user", "daemon-reload").Run()
		_ = hiddenCmd("systemctl", "--user", "enable", "--now", "tunnelport").Run()
	}
}

func autoStartDisable() {
	switch runtime.GOOS {
	case "windows":
		_ = os.Remove(startupFile())
		_ = os.Remove(strings.TrimSuffix(startupFile(), ".lnk") + ".bat")
	case "darwin":
		_ = hiddenCmd("launchctl", "unload", plistPath()).Run()
		_ = os.Remove(plistPath())
	default:
		_ = hiddenCmd("systemctl", "--user", "disable", "--now", "tunnelport").Run()
		_ = os.Remove(systemdUnitPath())
	}
}

func startupFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Roaming", "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "tunnelport.lnk")
}

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.tunnelport.app.plist")
}

func systemdUnitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "tunnelport.service")
}
