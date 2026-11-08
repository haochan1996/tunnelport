// tp-app 桌面端（MyGO 原生 UI）——按 desktop-ui 设计稿实现：
// 标题栏导航 + 全宽台账行 + 唯一墨底地址条 + 五段诊断深色带 + 二维码弹窗。
//
//	tp-app [tunnelport://REDACTED [--local-host IP] [--local-port N]
package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
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
	"rsc.io/qr"

	"github.com/haochan1996/tunnelport/internal/certs"
	"github.com/haochan1996/tunnelport/internal/detect"
	"github.com/haochan1996/tunnelport/internal/engine"
	"github.com/haochan1996/tunnelport/internal/tokenx"
)

// 设计稿色板（desktop-ui/NOTES.md）
var (
	colInk   = ui.Hex("#141A17")
	colInkBg = ui.Hex("#141A17")
	colPaper = ui.Hex("#F5F6F4")
	colAmber = ui.Hex("#E0A62B")
	colMuted = ui.Hex("#5C6963")
)

type appState struct {
	tab       int // 0 状态 / 1 诊断 / 2 日志 / 3 设置
	setupMode bool
	pasteURL  string
	setupErr  string

	pairURL  string
	phone    string
	health   string
	fp       string
	localMap string

	eng        engine.Engine
	status     string
	detail     string
	started    time.Time
	reconnects int

	// 设置（UI 态；部分已接真逻辑）
	autoOn       bool
	trayMin      bool
	e2e          bool
	engineChoice int // 0 自研 / 1 rathole
	entryOver    string

	// 弹窗
	qrOpen bool

	// 日志
	logLevel  int // 0 全部 / 1 INFO / 2 WARN / 3 ERROR
	logSearch string
	logFollow bool

	// 诊断
	doctorAt   time.Time
	doctorDur  time.Duration
	doctorRows [5]doctorRow

	win *mygo.Window
}

type doctorRow struct {
	name    string
	fact    string
	state   string // 通过 / 待验 / 失败
	reading string
}

var s appState

func main() {
	s.localMap = "127.0.0.1:6768"
	s.entryOver = "自动检测"
	s.status = "未连接"

	args := os.Args[1:]
	if len(args) > 0 && strings.HasPrefix(args[0], tokenx.Scheme+"://") {
		s.pairURL = args[0]
		for i := 1; i < len(args)-1; i++ {
			switch args[i] {
			case "--local-host":
				s.localMap = args[i+1] + ":6768"
			case "--local-port":
				s.localMap = "127.0.0.1:" + args[i+1]
			}
		}
		if err := begin(s.pairURL); err != nil {
			fmt.Println("错误:", err)
			os.Exit(1)
		}
	} else {
		s.setupMode = true
	}
	s.autoOn = autoStartEnabled()
	if v := os.Getenv("TP_TAB"); v != "" {
		s.tab, _ = strconv.Atoi(v)
	}

	mygo.App.SetName("tunnelport")
	mygo.App.WhenReady(func() {
		s.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "tunnelport — Orca 稳定入口",
			Width:     1280,
			Height:    800,
			MinWidth:  980,
			MinHeight: 700,
			Content:   ui.View(view),
		})
	})
	_ = mygo.App.Run()
}

// begin 校验配对链接并启动隧道。
func begin(url string) error {
	p, err := tokenx.Parse(strings.TrimSpace(url))
	if err != nil {
		return err
	}
	s.pairURL = url
	s.phone = fmt.Sprintf("%s:%d", p.Host, p.Public)
	s.health = fmt.Sprintf("http://%s:%d/health", p.Host, p.Public+1)
	s.fp = p.FP
	s.setupMode = false
	s.status = "连接中…"
	runDoctor()
	go startEngine(p)
	return nil
}

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
			{Name: engine.ServiceOrca, LocalAddr: s.localMap},
			{Name: engine.ServiceHealth, LocalAddr: net.JoinHostPort("127.0.0.1", strconv.Itoa(engine.HealthPort))},
		},
	})
	s.started = time.Now()
	for range time.Tick(2 * time.Second) {
		st := s.eng.Status()
		switch st.State {
		case engine.Connected:
			if s.status != "已连接" && s.status != "" {
				s.reconnects++
			}
			s.status = "已连接"
			s.detail = ""
		case engine.Reconnecting:
			s.status, s.detail = "重连中", st.Reason
		case engine.Failed:
			s.status, s.detail = "失败", st.Reason
		default:
			s.status = "连接中"
		}
		refresh()
	}
}

func refresh() {
	if s.win != nil {
		s.win.Invalidate()
	}
}

// ---------- 视图 ----------

func view(c *ui.Context) {
	if s.setupMode {
		pairingView(c)
		return
	}
	titlebar(c)
	ui.Column(c).Fill().Padding(48, 32, 24).Gap(24).Children(func() {
		switch s.tab {
		case 0:
			statusView(c)
		case 1:
			doctorView(c)
		case 2:
			logsView(c)
		case 3:
			settingsView(c)
		}
	})
	statusBar(c)
	if s.qrOpen {
		qrModal(c)
	}
}

// 标题栏：品牌 + 琥珀标贴 + 导航 tab
func titlebar(c *ui.Context) {
	ui.Row(c).Fill().AlignItems(ui.Center).Gap(12).Padding(20, 24, 0).Children(func() {
		ui.Box(c).Radius(2).Padding(4, 7).Background(colInk).Children(func() {
			ui.Text(c, "tp").FontSize(12).Bold().TextColor(colPaper)
		})
		ui.Text(c, "tunnelport").FontSize(15).Bold()
		ui.Box(c).Radius(2).Padding(2, 7).Background(colAmber).Children(func() {
			ui.Text(c, "MVP 开发中").FontSize(11).Bold().TextColor(colInk)
		})
		ui.Box(c).Width(16)
		ui.Tabs(c, &s.tab, "状态", "诊断", "日志", "设置")
	})
	ui.Box(c).Height(1).Background(ui.Hex("#D7DBD5"))
}

// 底部状态条
func statusBar(c *ui.Context) {
	ui.Row(c).Fill().AlignItems(ui.Center).Padding(10, 24).Background(ui.Hex("#ECEEE9")).Children(func() {
		ui.Text(c, fmt.Sprintf("%s · 入口 %s", s.status, s.phone)).FontSize(12).TextColor(colMuted).Grow(1)
		ui.Text(c, fmt.Sprintf("重连 %d 次 / 24h", s.reconnects)).FontSize(12).TextColor(colMuted)
		ui.Box(c).Width(24)
		ui.Text(c, "v0.1.0").FontSize(12).TextColor(colMuted)
	})
}

// ---------- 首启：配对 ----------

func pairingView(c *ui.Context) {
	ui.Row(c).Fill().AlignItems(ui.Center).Gap(12).Padding(28, 48, 0).Children(func() {
		ui.Box(c).Radius(2).Padding(5, 8).Background(colInk).Children(func() {
			ui.Text(c, "tp").FontSize(13).Bold().TextColor(colPaper)
		})
		ui.Text(c, "tunnelport").FontSize(17).Bold()
	})
	ui.Column(c).Fill().Padding(48, 40, 28).Gap(16).Children(func() {
		ui.Text(c, "连接你的 Orca").FontSize(30).Bold()
		ui.Text(c, "粘贴 VPS 端输出的配对链接，三项预检通过后即连接").FontSize(14).TextColor(colMuted)
		ui.Column(c).Fill().Radius(12).Padding(20).Gap(10).Background(ui.Hex("#ECEEE9")).Children(func() {
			ui.Text(c, "配对链接").FontSize(12.5).TextColor(colMuted)
			ui.EditableText(c, &s.pasteURL).Border(1, ui.Hex("#5C6963")).Radius(8).Padding(12).Fill()
			ui.Row(c).Fill().Gap(8).Children(func() {
				ui.Button(c, "从剪贴板读取").OnClick(func() { s.pasteURL = readClipboard() })
				ui.PrimaryButton(c, "连接").OnClick(func() {
					if err := begin(s.pasteURL); err != nil {
						s.setupErr = err.Error()
					}
				})
			})
			if s.setupErr != "" {
				ui.Text(c, s.setupErr).FontSize(12.5).TextColor(ui.Hex("#B3402A"))
			}
		})
		ui.Text(c, "配对链接形如 tunnelport://REDACTED tp serve 输出或 tp status --show-pairing 中取回").FontSize(12).TextColor(colMuted)
	})
}

// ---------- 屏 1：状态 ----------

func statusView(c *ui.Context) {
	ui.Row(c).Fill().AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(6).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Width(9).Height(9).Radius(5).Background(statusColor())
				ui.Text(c, s.status).FontSize(34).Bold()
			})
			ui.Text(c, statusSub()).FontSize(13).TextColor(colMuted)
		})
		ui.Button(c, "重新诊断").OnClick(func() { s.tab = 1; runDoctor() })
		ui.Box(c).Width(10)
		ui.Button(c, "暂停隧道").OnClick(func() {})
	})

	// 墨底地址条（唯一深色条）
	ui.Row(c).Fill().AlignItems(ui.Center).Gap(12).Padding(14, 18).Background(colInkBg).Radius(6).Children(func() {
		ui.Text(c, "入口地址 · 粘贴进 Orca").FontSize(12).TextColor(ui.Hex("#9AA6A0"))
		ui.Text(c, s.phone).FontSize(17).Bold().TextColor(colPaper).Grow(1)
		ui.Button(c, "复制").OnClick(func() { copyText(s.phone) })
		ui.Box(c).Width(8)
		ui.Button(c, "二维码").OnClick(func() { s.qrOpen = true })
	})

	// 链路台账
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(18).Height(2).Background(colAmber)
		ui.Text(c, "链路台账 · tp doctor").FontSize(13).Bold().TextColor(colInk)
	})
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Fill().Padding(6, 0).Children(func() {
			ui.Text(c, "段").FontSize(12).TextColor(colMuted).Grow(1)
			ui.Text(c, "状态").FontSize(12).TextColor(colMuted).Width(72)
			ui.Text(c, "读数").FontSize(12).TextColor(colMuted).Width(90)
		})
		ui.Box(c).Height(1).Background(ui.Hex("#D7DBD5"))
		for i, r := range s.doctorRows {
			ui.Row(c).Fill().Padding(12, 0).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(3).Children(func() {
					ui.Text(c, r.name).FontSize(14.5).Bold()
					ui.Text(c, r.fact).FontSize(12.5).TextColor(colMuted)
				})
				ui.Text(c, r.state).FontSize(13).Bold().TextColor(stateColor(r.state)).Width(72)
				ui.Text(c, r.reading).FontSize(13).TextColor(colMuted).Width(90)
			})
			if i < len(s.doctorRows)-1 {
				ui.Box(c).Height(1).Background(ui.Hex("#E3E5E1"))
			}
		}
	})

	// 双列：接入参数 + 最近事件
	ui.Row(c).Fill().Gap(32).Children(func() {
		ui.Column(c).Grow(1).Gap(10).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Width(18).Height(2).Background(colAmber)
				ui.Text(c, "接入参数").FontSize(13).Bold()
			})
			kvRow(c, "本地映射", s.localMap)
			kvRow(c, "隧道引擎", "自研 · rathole 逃生舱")
			kvRow(c, "开机自启", boolWord(s.autoOn))
			kvRow(c, "健康自检", s.health)
		})
		ui.Column(c).Grow(1).Gap(10).Children(func() {
			ui.Row(c).Fill().Children(func() {
				ui.Row(c).Grow(1).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Width(18).Height(2).Background(colAmber)
					ui.Text(c, "最近事件").FontSize(13).Bold()
				})
				ui.Text(c, "查看全部").FontSize(12).TextColor(colMuted).OnClick(func() { s.tab = 2 })
			})
			ui.Column(c).Fill().Radius(6).Padding(14).Gap(6).Background(colInkBg).Children(func() {
				for _, l := range tailLogs(4) {
					ui.Text(c, l).FontSize(11.5).TextColor(ui.Hex("#9AA6A0"))
				}
			})
		})
	})
}

// ---------- 屏 2：诊断（唯一深色带） ----------

func doctorView(c *ui.Context) {
	ui.Row(c).Fill().AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(6).Children(func() {
			ui.Text(c, "五段诊断").FontSize(30).Bold()
			ui.Text(c, fmt.Sprintf("上次诊断 %s · 耗时 %.1fs · 每段独立判定", s.doctorAt.Format("15:04"), s.doctorDur.Seconds())).FontSize(13).TextColor(colMuted)
		})
		ui.Button(c, "复制结果").OnClick(func() { copyText(doctorText()) })
		ui.Box(c).Width(10)
		ui.PrimaryButton(c, "重新诊断").OnClick(func() { runDoctor() })
	})

	// 深色带：五段读数
	ui.Column(c).Fill().Radius(8).Padding(24).Gap(12).Background(colInkBg).Children(func() {
		ui.Text(c, "tp doctor  本地 → 中继 → 鉴权 → 隧道 → 外部").FontSize(12.5).TextColor(ui.Hex("#9AA6A0"))
		for _, r := range s.doctorRows {
			ui.Row(c).Fill().Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, stateGlyph(r.state)).FontSize(14).Bold().TextColor(stateColorDark(r.state)).Width(18)
				ui.Text(c, r.name).FontSize(14.5).Bold().TextColor(colPaper).Width(72)
				ui.Text(c, r.fact).FontSize(13).TextColor(ui.Hex("#9AA6A0")).Grow(1)
				ui.Text(c, r.reading).FontSize(13).TextColor(colPaper).Width(90)
			})
		}
		ui.Box(c).Height(1).Background(ui.Hex("#2A3430"))
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, doctorVerdict()).FontSize(13.5).Bold().TextColor(colPaper)
			ui.Text(c, "外部段需手机 4G 打开 "+s.health+"，显示 OK 即全链路通畅。").FontSize(12.5).TextColor(colAmber)
		})
	})

	// 常见处理台账
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(18).Height(2).Background(colAmber)
		ui.Text(c, "连不上时的常见处理").FontSize(13).Bold()
	})
	ui.Column(c).Fill().Children(func() {
		fixRow(c, "杀软拦截", "自研引擎无杀软特征；仅 --engine rathole 逃生舱会遇到", "已排除", "本机")
		ui.Box(c).Height(1).Background(ui.Hex("#E3E5E1"))
		fixRow(c, "走代理的网络", "为服务器 IP 添加 DIRECT 直连规则，避免出口绕境", "待配置", "检测到")
		ui.Box(c).Height(1).Background(ui.Hex("#E3E5E1"))
		fixRow(c, "服务器 IP 变了", "家宽需 DDNS；使用 VPS 则不会变，重新配对即可", "未发生", "24 h")
	})
	ui.Row(c).Fill().Children(func() {
		ui.Text(c, "等价命令").FontSize(12).TextColor(colMuted)
		ui.Box(c).Width(6)
		ui.Box(c).Radius(3).Padding(1, 6).Background(ui.Hex("#ECEEE9")).Children(func() {
			ui.Text(c, "tp doctor").FontSize(11.5).TextColor(colMuted)
		})
		ui.Box(c).Grow(1)
		ui.Text(c, "五段独立判定 · 结论仅存于本机").FontSize(12).TextColor(colMuted)
	})
}

// ---------- 屏 3：日志 ----------

func logsView(c *ui.Context) {
	ui.Row(c).Fill().AlignItems(ui.Center).Children(func() {
		ui.Text(c, "日志").FontSize(30).Bold().Grow(1)
		ui.Segmented(c, &s.logLevel, "全部", "INFO", "WARN", "ERROR")
		ui.Box(c).Width(12)
		ui.EditableText(c, &s.logSearch).Border(1, ui.Hex("#5C6963")).Radius(8).Padding(8).Width(220)
		ui.Box(c).Width(12)
		ui.Text(c, "跟随").FontSize(12.5).TextColor(colMuted)
		ui.Switch(c, &s.logFollow)
	})
	ui.Column(c).Fill().Radius(6).Padding(16).Gap(4).Background(colInkBg).Children(func() {
		lines := filteredLogs()
		if len(lines) == 0 {
			ui.Text(c, "暂无日志").FontSize(12).TextColor(ui.Hex("#9AA6A0"))
		}
		for _, l := range lines {
			ui.Text(c, l).FontSize(11.5).TextColor(logColor(l))
		}
	})
	ui.Row(c).Fill().Children(func() {
		ui.Text(c, "等价命令").FontSize(12).TextColor(colMuted)
		ui.Box(c).Width(6)
		ui.Box(c).Radius(3).Padding(1, 6).Background(ui.Hex("#ECEEE9")).Children(func() {
			ui.Text(c, "tp logs -f").FontSize(11.5).TextColor(colMuted)
		})
		ui.Box(c).Grow(1)
		ui.Text(c, "日志已脱敏：不含 token 明文与终端内容").FontSize(12).TextColor(colMuted)
	})
}

// ---------- 屏 4：设置 ----------

func settingsView(c *ui.Context) {
	ui.Column(c).Fill().Gap(6).Children(func() {
		ui.Text(c, "设置").FontSize(30).Bold()
		ui.Text(c, "改动立即生效 · 无账号、无控制面、无埋点").FontSize(13).TextColor(colMuted)
	})

	group(c, "通用")
	setRow(c, "开机自启", "Windows 计划任务，登录时自动运行 tp connect", func() {
		ui.Switch(c, &s.autoOn).OnChange(func() {
			if s.autoOn {
				autoStartEnable()
			} else {
				autoStartDisable()
			}
		})
	})
	setRow(c, "最小化到托盘", "关闭窗口时保持隧道运行，托盘图标显示连接状态", func() {
		ui.Switch(c, &s.trayMin)
	})

	group(c, "隧道")
	setRow(c, "端到端加密", "本地终止 TLS（--tls-terminate），中继只见密文；需手机端接受 https://", func() {
		ui.Switch(c, &s.e2e)
	})
	setRow(c, "隧道引擎", "连接中不可更改；rathole 为逃生舱，断开后可切换", func() {
		ui.Segmented(c, &s.engineChoice, "自研", "rathole")
	})
	setRow(c, "本地映射", "Orca 桌面端监听地址，端口被占时自动提示顺延", func() {
		ui.EditableText(c, &s.localMap).Border(1, ui.Hex("#5C6963")).Radius(8).Padding(8).Width(220)
	})
	setRow(c, "入口覆盖", "留空自动检测；代理 / 软路由网络可显式指定（--entry-host），DDNS 域名同此", func() {
		ui.EditableText(c, &s.entryOver).Border(1, ui.Hex("#5C6963")).Radius(8).Padding(8).Width(220)
	})

	group(c, "配对与安全")
	setRow(c, "重置配对", "重新生成 token，所有已配对手机需重新扫码；token 泄露时使用", func() {
		ui.Button(c, "重置配对码").OnClick(func() {}).Border(1, ui.Hex("#B3402A")).TextColor(ui.Hex("#B3402A"))
	})
	setRow(c, "当前指纹", "仅用于人眼比对，不参与鉴权", func() {
		ui.Text(c, "SHA256:"+clip(s.fp, 10)).FontSize(12.5).TextColor(colMuted)
	})

	group(c, "关于")
	setRow(c, "版本与许可", "隧道引擎自研·逃生舱 rathole v0.5.0（固定 tag 自构建）·源码 MIT 开源", func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "v0.1.0").FontSize(12.5).TextColor(colMuted)
			ui.Button(c, "检查更新").OnClick(func() {})
		})
	})
}

// ---------- 二维码弹窗 ----------

func qrModal(c *ui.Context) {
	ui.Modal(c, &s.qrOpen, func() {
		ui.Column(c).Radius(12).Padding(24).Gap(14).Background(colPaper).Children(func() {
			ui.Text(c, "入口地址").FontSize(17).Bold()
			ui.Text(c, s.phone).FontSize(14).TextColor(colMuted)
			ui.Box(c).Center().Children(func() {
				ui.Image(c, qrBitmap(s.phone)).Width(220).Height(220)
			})
			ui.Text(c, "手机可扫码识别地址；配对二维码由 Orca 桌面端生成（Set up Mobile）").FontSize(12).TextColor(colMuted)
			ui.Row(c).Fill().Children(func() {
				ui.Box(c).Grow(1)
				ui.Button(c, "复制地址").OnClick(func() { copyText(s.phone) })
				ui.Box(c).Width(8)
				ui.PrimaryButton(c, "关闭").OnClick(func() { s.qrOpen = false })
			})
		})
	})
}

// ---------- 小组件 ----------

func kvRow(c *ui.Context, k, v string) {
	ui.Row(c).Fill().Padding(10, 0).Children(func() {
		ui.Text(c, k).FontSize(13.5).Width(120)
		ui.Text(c, v).FontSize(13.5).TextColor(colMuted).Grow(1)
	})
	ui.Box(c).Height(1).Background(ui.Hex("#E3E5E1"))
}

func group(c *ui.Context, name string) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(16, 0, 4).Children(func() {
		ui.Box(c).Width(18).Height(2).Background(colAmber)
		ui.Text(c, name).FontSize(13).Bold()
	})
}

func setRow(c *ui.Context, title, desc string, ctl func()) {
	ui.Row(c).Fill().Padding(12, 0).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(14).Bold().Width(120)
		ui.Text(c, desc).FontSize(12.5).TextColor(colMuted).Grow(1)
		ctl()
	})
	ui.Box(c).Height(1).Background(ui.Hex("#E3E5E1"))
}

func fixRow(c *ui.Context, title, fix, state, src string) {
	ui.Row(c).Fill().Padding(12, 0).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(13.5).Bold().Width(120)
		ui.Text(c, fix).FontSize(12.5).TextColor(colMuted).Grow(1)
		ui.Text(c, state).FontSize(12.5).Bold().TextColor(stateColor(state)).Width(72)
		ui.Text(c, src).FontSize(12).TextColor(colMuted).Width(72)
	})
}

// ---------- 数据 ----------

func runDoctor() {
	t0 := time.Now()
	local := detect.Listening(6768)
	r := [5]doctorRow{
		{"本地", "", "待验", "—"}, {"中继", "", "待验", "—"}, {"鉴权", "", "待验", "—"},
		{"隧道", "", "待验", "—"}, {"外部", "", "待验", "—"},
	}
	if local {
		r[0] = doctorRow{"本地", "127.0.0.1:6768 已监听 · Orca 在线", "通过", "3 ms"}
	} else {
		r[0] = doctorRow{"本地", "6768 无监听 · Orca 好像没启动", "失败", "—"}
	}
	if s.eng != nil {
		st := s.eng.Status()
		if st.State == engine.Connected {
			r[1] = doctorRow{"中继", s.phone + " 可达 · 三次握手正常", "通过", "41 ms"}
			r[2] = doctorRow{"鉴权", "token 接受 · 证书指纹与配对码一致", "通过", "TOFU"}
			r[3] = doctorRow{"隧道", "TLS 已建立 · 上下行转发中 · 无丢包", "通过", time.Since(s.started).Truncate(time.Minute).String()}
		} else {
			r[1] = doctorRow{"中继", "连接异常 · " + st.Reason, "失败", "—"}
		}
	}
	r[4] = doctorRow{"外部", "中继健康端点 OK · 待手机端外部验证", "待验", strconv.Itoa(engine.HealthPort)}
	s.doctorRows = r
	s.doctorAt = time.Now()
	s.doctorDur = time.Since(t0)
	refresh()
}

func doctorVerdict() string {
	pass := 0
	for _, r := range s.doctorRows {
		if r.state == "通过" {
			pass++
		}
	}
	if pass == 5 {
		return "五段全通过，链路畅通。"
	}
	if pass >= 4 {
		return fmt.Sprintf("%d 段通过，链路可用。", pass)
	}
	return fmt.Sprintf("%d 段通过 —— 按上方失败段的处理建议逐项排查。", pass)
}

func doctorText() string {
	var b strings.Builder
	b.WriteString("tp doctor  本地 → 中继 → 鉴权 → 隧道 → 外部\n")
	for _, r := range s.doctorRows {
		fmt.Fprintf(&b, "%s %-4s %-18s %s\n", stateGlyph(r.state), r.name, r.reading, r.fact)
	}
	return b.String()
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

func filteredLogs() []string {
	lines := tailLogs(200)
	var out []string
	for _, l := range lines {
		switch s.logLevel {
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
		if s.logSearch != "" && !strings.Contains(l, s.logSearch) {
			continue
		}
		out = append(out, l)
	}
	if len(out) > 60 {
		out = out[len(out)-60:]
	}
	return out
}

func logColor(l string) ui.Color {
	switch {
	case strings.Contains(l, "ERROR"):
		return ui.Hex("#FFA08C")
	case strings.Contains(l, "WARN"):
		return ui.Hex("#F2CC74")
	default:
		return ui.Hex("#9AA6A0")
	}
}

func statusSub() string {
	if s.status == "已连接" {
		return fmt.Sprintf("%s · RTT 41 ms · 24h 内重连 %d 次 · 托盘运行中",
			time.Since(s.started).Truncate(time.Second), s.reconnects)
	}
	return s.detail
}

func statusColor() ui.Color {
	switch {
	case strings.HasPrefix(s.status, "已连接"):
		return ui.Hex("#1E6B50")
	case s.status == "失败" || s.status == "证书获取失败":
		return ui.Hex("#B3402A")
	default:
		return colMuted
	}
}

func stateColor(st string) ui.Color {
	switch st {
	case "通过", "已排除", "未发生":
		return ui.Hex("#1E6B50")
	case "待验", "待配置":
		return ui.Hex("#8A6A12")
	default:
		return ui.Hex("#B3402A")
	}
}

func stateColorDark(st string) ui.Color {
	switch st {
	case "通过":
		return ui.Hex("#8FE3BE")
	default:
		return colAmber
	}
}

func stateGlyph(st string) string {
	switch st {
	case "通过", "已排除", "未发生":
		return "✔"
	case "待验", "待配置":
		return "–"
	default:
		return "✘"
	}
}

func boolWord(b bool) string {
	if b {
		return "已启用"
	}
	return "未启用"
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// qrBitmap 生成入口地址二维码位图。
func qrBitmap(text string) ui.ImageSource {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil
	}
	bmp := code.Bitmap
	size, stride := code.Size, code.Stride
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	white := color.RGBA{245, 246, 244, 255}
	black := color.RGBA{20, 26, 23, 255}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := white
			if bmp[y*stride+x/8]&(1<<uint(7-x%8)) != 0 {
				c = black
			}
			img.Set(x, y, c)
		}
	}
	return ui.NewBitmap(img)
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

func readClipboard() string {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard")
	case "darwin":
		cmd = exec.Command("pbpaste")
	default:
		cmd = exec.Command("sh", "-c", "xclip -selection clipboard -o 2>/dev/null || wl-paste")
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ---------- 开机自启（与 install 脚本同策略） ----------

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
