package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/haochan1996/tunnelport-internal/internal/address"
	"github.com/haochan1996/tunnelport-internal/internal/certs"
	"github.com/haochan1996/tunnelport-internal/internal/config"
	"github.com/haochan1996/tunnelport-internal/internal/detect"
	"github.com/haochan1996/tunnelport-internal/internal/engine"
	"github.com/haochan1996/tunnelport-internal/internal/tokenx"
)

func connectCmd() *cobra.Command {
	var (
		flagToken     string
		flagHost      string
		flagPort      int
		flagLocal     int
		flagLocalHost string
		flagBin       string
		flagTransport string
		flagTerm      bool
		flagEngine    string
	)
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "在跑 Orca 的机器上连中继，映射本地 6768",
		RunE: func(cmd *cobra.Command, args []string) error {
			t := flagToken
			if t == "" && len(args) > 0 {
				t = args[0] // 允许位置参数：tp connect tunnelport://…
			}
			return runConnect(t, flagHost, flagPort, flagLocal, flagLocalHost, flagBin, flagTransport, flagTerm, flagEngine)
		},
	}
	cmd.Flags().StringVar(&flagToken, "token", os.Getenv("TP_TOKEN"), "token 或完整配对 URL（tunnelport://host:port?fp=证书指纹#token）")
	cmd.Flags().StringVar(&flagHost, "host", "", "中继地址（用纯 token 时必填）")
	cmd.Flags().IntVar(&flagPort, "port", envInt("TP_CONTROL_PORT", 0), "中继控制端口（URL 里自带，纯 token 时必填）")
	cmd.Flags().IntVar(&flagLocal, "local-port", 6768, "Orca 端口")
	cmd.Flags().StringVar(&flagLocalHost, "local-host", "127.0.0.1", "Orca 所在地址（默认本机；NAS 场景填电脑的局域网 IP，如 192.168.1.10，实现电脑零安装）")
	cmd.Flags().StringVar(&flagBin, "rathole", "", "rathole 可执行文件路径（engine=rathole 时用）")
	cmd.Flags().StringVar(&flagEngine, "engine", "native", "隧道引擎：native（默认，自研，杀软无特征）或 rathole")
	cmd.Flags().StringVar(&flagTransport, "transport", "tls", "隧道传输：tls（默认，D-8）或 tcp（对端旧版本回退）")
	cmd.Flags().BoolVar(&flagTerm, "tls-terminate", false,
		"端到端模式：在本地终止 TLS，手机用 https://入口:端口 访问时内容全程加密（要求手机端接受自签证书）")
	return cmd
}

func runConnect(tokenArg, flagHost string, flagPort, flagLocal int, flagLocalHost, flagBin, flagTransport string, flagTerm bool, flagEngine string) error {
	mustStateDir()
	if tokenArg == "" {
		return fmt.Errorf("缺少 --token；手机扫码或在 VPS 上运行 tp serve 可获得")
	}
	p, err := tokenx.Parse(tokenArg)
	if err != nil {
		return err
	}
	// 显式 --host / --port 优先于 URL 内嵌值：serve 侧自动检测可能拿到代理出口 IP
	// （D-10），用户需用内网/DDNS 地址覆盖（2026-11-08 实测）。
	if flagHost != "" {
		p.Host = flagHost
	}
	if p.Public == 0 {
		p.Public = flagLocal
	}
	if flagPort != 0 {
		p.Control = flagPort
	}
	host, port := p.Host, p.Control
	if port == 0 {
		return fmt.Errorf("缺少控制端口：用完整配对 URL（含端口），或加 --port <控制端口>")
	}

	// A 段预检：Orca 没开也允许连接，但要说清楚（FR-8.1）。
	if !detect.Listening(flagLocal) {
		fmt.Printf("⚠ 本地 %d 没有进程监听 —— Orca 好像没启动，隧道会建立但手机连不上服务。\n", flagLocal)
	}

	dir, _ := config.Dir()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// 链路加密（D-8）：TLS 模式下先做 TOFU 探测 —— 抓对端证书、
	// 与配对码携带的证书指纹比对（防中间人），落盘为 rathole 的 trusted_root。
	certRoot := ""
	if flagTransport == "tls" {
		root, err := certs.TOFUProbe(host, port, p.FP, dir)
		if err != nil {
			return err
		}
		certRoot = root
		if p.FP != "" {
			fmt.Printf("✓ 证书指纹比对通过（%s…），链路启用 TLS。\n", p.FP[:12])
		} else {
			fmt.Println("⚠ 首次连接未携带证书指纹（TOFU 信任模式）。建议重新扫码获取带指纹的配对码。")
		}
	}

	orcaLocal := net.JoinHostPort(flagLocalHost, strconv.Itoa(flagLocal))

	// 端到端终结器（用户要求的方向）：本地起 TLS 服务，
	// 手机 https://入口:端口 → 隧道 → 本地终结 TLS → 明文给 Orca。
	// 全程中继只见密文（前提：手机端接受自签证书，待真机验证）。
	orcaTarget := orcaLocal
	var termLn net.Listener
	if flagTerm {
		cert, err := certs.EnsureTerminator(dir)
		if err != nil {
			return fmt.Errorf("生成终结器证书失败: %w", err)
		}
		termPort := flagLocal + 2
		if termPort == engine.HealthPort {
			termPort++ // 避让健康端点（如 --local-port 6767 时 +2 恰为 6769）
		}
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(termPort)))
		for retry := 0; err != nil && retry < 5; retry++ {
			termPort++
			if termPort == engine.HealthPort {
				termPort++
			}
			ln, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(termPort)))
		}
		if err != nil {
			return fmt.Errorf("终结器端口分配失败（%d 起）: %w", flagLocal+2, err)
		}
		termLn = tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
		orcaTarget = net.JoinHostPort("127.0.0.1", strconv.Itoa(termPort))
		fmt.Printf("✓ 端到端模式已开启：手机用 https://%s:%d 访问（自签证书，如提示不受信任属预期）。\n", host, p.Public)
		go func() {
			for {
				c, err := termLn.Accept()
				if err != nil {
					// TLS 握手失败也会走这里：记录原因，不退出（退出会让后续连接全部挂起）
					select {
					case <-ctx.Done():
						return
					case <-time.After(200 * time.Millisecond):
					}
					fmt.Fprintf(os.Stderr, "[终结器] accept 错误: %v\n", err)
					continue
				}
				fmt.Fprintf(os.Stderr, "[终结器] 新连接\n")
				go func(c net.Conn) {
					defer c.Close()
					up, err := net.Dial("tcp", orcaLocal)
					if err != nil {
						fmt.Fprintf(os.Stderr, "[终结器] 上游拨号失败: %v\n", err)
						return
					}
					defer up.Close()
					fmt.Fprintf(os.Stderr, "[终结器] 已代理到 %s\n", orcaLocal)
					// 双向必须并发：串行会让 TLS 握手饿死（先读上游会永远阻塞）
					done := make(chan struct{}, 2)
					go func() { _, _ = io.Copy(up, c); done <- struct{}{} }() // 客户端→上游（含握手）
					go func() { _, _ = io.Copy(c, up); done <- struct{}{} }() // 上游→客户端
					<-done
				}(c)
			}
		}()
	}

	// 健康端点（FR-7.3 / D-3）：只回状态，不回业务数据。
	healthLn, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(engine.HealthPort)))
	if err != nil {
		return fmt.Errorf("健康端点端口 %d 被占用: %w", engine.HealthPort, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		rtt, derr := detect.TCPRtt(host, port, 2*time.Second)
		if derr != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "RELAY DOWN")
			return
		}
		fmt.Fprintf(w, "OK rtt=%dms %s tls=%s", rtt.Milliseconds(), version, flagTransport)
	})
	go func() { _ = http.Serve(healthLn, mux) }()

	svcOrca := engine.Service{Name: engine.ServiceOrca, LocalAddr: orcaTarget}
	engCfg := engine.Config{
		Role:       "connect",
		Remote:     host,
		RemotePort: port,
		Token:      p.Token,
		RatholeBin: flagBin,
		Transport:  flagTransport,
		CertRoot:   certRoot,
		Services: []engine.Service{
			svcOrca,
			{Name: engine.ServiceHealth, LocalAddr: net.JoinHostPort("127.0.0.1", strconv.Itoa(engine.HealthPort))},
		},
	}
	eng := newEngine(flagEngine)
	if err := eng.Start(ctx, engCfg); err != nil {
		return err
	}
	defer eng.Stop()
	defer healthLn.Close()
	if termLn != nil {
		defer termLn.Close()
	}

	_ = config.SaveState(&config.State{
		Role:    "connect",
		Entry:   address.EntryURL(host, p.Public),
		Control: address.EntryURL(host, port),
		TokenFP: config.Fingerprint(p.Token),
		Version: version,
		PID:     os.Getpid(),
	})

	phoneAddr := fmt.Sprintf("%s:%d", host, p.Public)
	if flagTerm {
		phoneAddr = "https://" + phoneAddr // 端到端模式：手机走 https
	}
	fmt.Printf(`
──────────────────────────────────────────────────
 已连接中继 %s:%d  %s  (tls=%s)  ✓ 链路就绪

 ② 把下面的地址粘贴到 Orca 桌面端「自定义网络地址」:
   %s

 ③ 手机配对：Orca 桌面端 Set up Mobile → 生成配对二维码 → 手机 Orca 扫码

 验证外部可达（手机 4G 打开）:
   %s

 提示：不要在 Orca 里填 wss:// 开头的地址（桌面端已知不兼容）。
 Ctrl-C 停止；本进程断线自动重连（1s→30s 退避）。
──────────────────────────────────────────────────
`, host, port, version, flagTransport, phoneAddr, address.HealthURL(host, engine.HealthPort))

	<-ctx.Done()
	fmt.Println("\n正在停止…")
	return nil
}

var _ = url.Parse
