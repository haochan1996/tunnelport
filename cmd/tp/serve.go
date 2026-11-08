package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/haochan1996/tunnelport/internal/address"
	"github.com/haochan1996/tunnelport/internal/certs"
	"github.com/haochan1996/tunnelport/internal/config"
	"github.com/haochan1996/tunnelport/internal/detect"
	"github.com/haochan1996/tunnelport/internal/engine"
	"github.com/haochan1996/tunnelport/internal/tokenx"
)

func serveCmd() *cobra.Command {
	var (
		flagPort     int
		flagBind     string
		flagToken    string
		flagPublic   int
		flagHealth   int
		flagBin      string
		flagEntry    string
		flagPortBase int
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "在有公网的机器上起中继，输出地址 / token / 二维码",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(flagPort, flagBind, flagToken, flagPublic, flagHealth, flagBin, flagEntry, flagPortBase)
		},
	}
	cmd.Flags().IntVar(&flagPort, "port", envInt("TP_PORT", 0), "控制通道端口（默认在 30000–40000 自动选）")
	cmd.Flags().StringVar(&flagBind, "bind", "0.0.0.0", "控制通道监听地址")
	cmd.Flags().StringVar(&flagToken, "token", os.Getenv("TP_TOKEN"), "指定 token（默认自动生成）")
	cmd.Flags().IntVar(&flagPublic, "public-port", envInt("TP_PUBLIC_PORT", 6768), "对外暴露的服务端口（手机连这个；被占用时自动顺延并提示）")
	cmd.Flags().IntVar(&flagHealth, "health-port", envInt("TP_HEALTH_PORT", engine.HealthPort), "健康端点端口（高级覆盖，与 --port-base 互斥）")
	cmd.Flags().IntVar(&flagPortBase, "port-base", envInt("TP_PORT_BASE", 0), "基座端口：三个端口连续占用 base/base+1/base+2（防火墙可放行一段）。显式指定被占用将报错。默认自动选择")
	cmd.Flags().StringVar(&flagBin, "rathole", "", "rathole 可执行文件路径（默认从 PATH 找）")
	cmd.Flags().StringVar(&flagEntry, "entry-host", os.Getenv("TP_ENTRY_HOST"), "显式指定入口主机（覆盖自动检测；代理/软路由网络或 DDNS 域名必填）")
	return cmd
}

func runServe(flagPort int, flagBind, flagToken string, flagPublic int, flagHealth int, flagBin, flagEntry string, flagPortBase int) error {
	mustStateDir()

	// 容器内端口必须显式给（FR-2.5）。
	if dockerMode() && os.Getenv("TP_PORT") == "" && os.Getenv("TP_PORT_BASE") == "" && flagPort == 0 && flagPortBase == 0 {
		return fmt.Errorf("容器模式必须指定端口，推荐基座三连:\n  docker run -d --restart unless-stopped -e TP_ROLE=serve -e TP_PORT_BASE=30123 -e TP_TOKEN=<t> -p 30123-30125:30123-30125 -v tp-state:/root/.tunnelport tunnelport")
	}

	// token：命令行 / 环境变量 / 自动生成。
	token := flagToken
	if token == "" {
		t, err := tokenx.New()
		if err != nil {
			return err
		}
		token = t
	}
	if !tokenx.Valid(token) {
		return fmt.Errorf("token 不合法（应为 52 位 Base32，可用 --token 留空自动生成）")
	}

	// FR-1 环境自检（快速路径，不阻塞主流程超过 10s）。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cgnat := detect.CGNATHint()

	// 链路加密证书（D-8）：生成/复用 ~/.tunnelport 下的自签名证书。
	dir, _ := config.Dir()
	certFp, err := certs.Ensure(dir)
	if err != nil {
		return fmt.Errorf("生成 TLS 证书失败: %w", err)
	}

	// 端口分配（FR-2.3 v1.4）：
	//   --port-base N（推荐）→ 控制口=N、Orca 口=N+1、健康口=N+2，三连整块；
	//   显式基座被占用 → 报错（用户可能已按该段放行防火墙，擅自换反而坑人）；
	//   未指定 → 自动找三连空闲块；使用单端口高级参数时 → 走独立分配。
	var ctrl, publicPort, healthPort int
	customIndividual := flagPort != 0 || flagPublic != 6768 || flagHealth != engine.HealthPort
	switch {
	case flagPortBase > 0 && customIndividual:
		return fmt.Errorf("--port-base 与 --port/--public-port/--health-port 互斥，二选一")
	case flagPortBase > 0:
		base := detect.NextFreeBase(flagPortBase, 3)
		if base != flagPortBase {
			return fmt.Errorf("端口 %d-%d 被占用（或含保留端口）。若已放行的是另一段，换 --port-base；或去掉 --port-base 让 tp 自动选择", flagPortBase, flagPortBase+2)
		}
		ctrl, publicPort, healthPort = base, base+1, base+2
	case customIndividual:
		c, err := detect.PickPort(flagPort)
		if err != nil {
			return err
		}
		ctrl = c
		publicPort = flagPublic
		healthPort = flagHealth
	default:
		base := detect.NextFreeBase(30000, 3)
		if base == 0 {
			return fmt.Errorf("30000 起未找到连续三个可用端口，请用 --port-base 指定基座")
		}
		ctrl, publicPort, healthPort = base, base+1, base+2
	}

	// 展示地址：显式指定 > 自动检测 > 本机回退。
	// 代理/软路由网络里自动检测到的是代理出口 IP（D-10），不可信时用 --entry-host 覆盖。
	entryHost := flagEntry
	ip, ipErr := "", error(nil)
	if entryHost == "" {
		fmt.Println("正在检测出口 IP（最多约 6 秒）…")
		ip, ipErr = detect.ExitIPv4(ctx)
		entryHost = ip
	}
	if entryHost == "" {
		entryHost = localEntryHint(flagBind)
	}

	if ctrl == flagPublic {
		flagPublic = detect.NextFreePort(flagPublic+1, ctrl)
		fmt.Printf("⚠ 控制端口与服务端口同为 %d，服务端口已顺延为 %d。\n", ctrl, flagPublic)
	}

	// 独立模式下的服务端口冲突顺延（FR-8.2）；三连模式已在 NextFreeBase 保证整块空闲
	if customIndividual {
		if alt := detect.NextFreePort(publicPort, ctrl); alt != publicPort {
			fmt.Printf("⚠ 端口 %d 被占用，已自动改用 %d。手机 Orca 请填 %s:%d\n", publicPort, alt, entryHost, alt)
			publicPort = alt
		}
		if alt := detect.NextFreePort(healthPort, ctrl, publicPort); alt != healthPort {
			fmt.Printf("⚠ 健康端点端口 %d 被占用，已自动改用 %d。\n", healthPort, alt)
			healthPort = alt
		}
	}

	eng := engine.NewRathole()
	engCfg := engine.Config{
		Role:        "serve",
		Bind:        flagBind,
		ControlPort: ctrl,
		Token:       token,
		RatholeBin:  flagBin,
		Transport:   "tls",
		CertP12:     filepath.ToSlash(filepath.Join(dir, "identity.p12")),
		Services: []engine.Service{
			{Name: engine.ServiceOrca, PublicPort: publicPort},
			{Name: engine.ServiceHealth, PublicPort: healthPort},
		},
	}
	if err := eng.Start(ctx, engCfg); err != nil {
		return err
	}
	defer eng.Stop()

	// 展示地址已在上方解析（含代理网络的 --entry-host 覆盖，D-10）。

	// 持久化状态（不含明文 token，FR-3.4）。
	_ = config.SaveState(&config.State{
		Role:    "serve",
		Entry:   address.EntryURL(entryHost, flagPublic),
		Control: address.EntryURL(entryHost, ctrl),
		TokenFP: config.Fingerprint(token),
		Version: version,
		PID:     os.Getpid(),
	})

	fmt.Printf(`
──────────────────────────────────────────────────
 中继已启动  %s

 入口地址（手机 Orca 填这个）
   %s

 token（连电脑时用，仅显示这一次）
   %s

 指纹: %s
 证书指纹: %s
`, version, address.EntryURL(entryHost, publicPort), token, config.Fingerprint(token), certFp)

	fmt.Printf(`
 下一步
   电脑上执行:
     tp connect --token %s --host %s

   手机浏览器（4G）打开验证外部可达:
     %s

 配对链接（含控制端口 + 证书指纹，防中间人）:
   %s

 ⚠ 记得在云厂商安全组放行 TCP: %d-%d（连续三端口）
   （控制通道 / Orca 服务 / 健康端点）
──────────────────────────────────────────────────

配对二维码（手机相机直接扫）:
%s
`, token, entryHost, address.HealthURL(entryHost, healthPort), tokenx.PairURL(entryHost, ctrl, token, certFp, publicPort), ctrl, healthPort, address.RenderQR(tokenx.PairURL(entryHost, ctrl, token, certFp, publicPort)))

	if ipErr != nil {
		fmt.Printf("⚠ 未检测到公网出口 IPv4（%v），上面地址按 %s 显示。\n", ipErr, entryHost)
	}
	if cgnat {
		fmt.Println("⚠ 本机网卡存在 100.64.x.x 地址：你可能处于 CGNAT，外部无法连入（TP-5002）。")
		fmt.Println("  请把这个中继装到有公网 IP 的 VPS 上，家宽机器不适合做入口。")
	}

	// 阻塞到 Ctrl-C / SIGTERM（docker stop / systemctl stop），退出时 defer Stop 清理临时目录并杀子进程（#2）。
	<-ctx.Done()
	fmt.Println("\n正在停止…")
	return nil
}

func localEntryHint(bind string) string {
	if bind != "" && bind != "0.0.0.0" && bind != "::" {
		return bind
	}
	return "127.0.0.1"
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// jsonMarshal 供调试用（保持 import 干净）。
var _ = json.Marshal
