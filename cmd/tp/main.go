// tp — 把你的 VPS/NAS 变成 Orca 手机端的稳定入口。
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/haochan1996/tunnelport/internal/config"
	"github.com/haochan1996/tunnelport/internal/engine"
)

// version 由 CI 通过 ldflags 注入。
var version = "dev"

func init() {
	// 双击 exe 启动时 Cobra 的 mousetrap 会弹英文提示后退出——改成人话，并等待回车
	// （桌面用户应下载 tunnelport_windows_amd64.exe 图形界面版）
	cobra.MousetrapHelpText = `这是 tunnelport 命令行工具（tp.exe）。

想装桌面软件：请下载 tunnelport_windows_amd64.exe —— 图形界面，双击即用。
想用命令行：请打开 cmd.exe 或 PowerShell，进入本目录运行 tp --help。

按回车键退出...`
	cobra.MousetrapDisplayDuration = 0 // 0 = 等待回车，来得及读完
}

func main() {
	root := &cobra.Command{
		Use:   "tp",
		Short: "把你的 VPS 变成 Orca 手机端的稳定入口",
		Long: `tp — 自托管 TCP 隧道。

  serve    在有公网的机器上起中继，输出地址 / token / 二维码
  connect  在跑 Orca 的机器上连中继，映射本地 6768
  doctor   五段诊断：本地 → 中继 → 鉴权 → 隧道 → 外部
  status   查看当前状态`,
		SilenceUsage:      true,
		SilenceErrors:     true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		Version:           version,
	}
	root.SetVersionTemplate("tp {{.Version}}\n")

	root.AddCommand(serveCmd(), connectCmd(), statusCmd(), doctorCmd(), logsCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err.Error())
		os.Exit(1)
	}
}

// dockerMode 判断是否在容器内运行（FR-2.5：容器内 TP_PORT 必填）。
func dockerMode() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return os.Getenv("TP_DOCKER") == "1"
}

func mustStateDir() string {
	d, err := config.Dir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
	return d
}

// newEngine 按 --engine 选择实现：native（默认，自研，无杀软特征）/ rathole（逃生舱）。
func newEngine(name string) engine.Engine {
	switch name {
	case "rathole":
		return engine.NewRathole()
	default:
		return engine.NewNative()
	}
}
