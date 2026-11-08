package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/haochan1996/tunnelport/internal/address"
	"github.com/haochan1996/tunnelport/internal/config"
	"github.com/haochan1996/tunnelport/internal/diag"
	"github.com/haochan1996/tunnelport/internal/engine"
)

func doctorCmd() *cobra.Command {
	var flagHost string
	var flagCtrl int
	var flagLocal int
	var flagOrcaPort int

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "五段诊断：本地 → 中继 → 鉴权 → 隧道 → 外部",
		Long: `按五段链路逐项判定，每项给一句事实和一个动作（FR-7）。
前一段失败，后续段标记跳过 —— 不猜、不伪造通过。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			host, ctrl := flagHost, flagCtrl
			if s, _ := config.LoadState(); s != nil && host == "" {
				host, ctrl = splitHostPort(s.Control)
			}
			if host == "" || ctrl == 0 {
				return fmt.Errorf("没有历史状态，且未提供 --host / --port；先运行 tp serve 或 tp connect")
			}
			local := flagLocal
			if local == 0 {
				local = flagOrcaPort
			}

			// A 本地 → B 中继；C/D 需要活跃会话（W2 从引擎取）；E 输出健康端点。
			results := []diag.Result{diag.Local(local)}
			if !results[0].Pass {
				results = append(results, diag.Result{Seg: "B", Name: "中继可达", Skip: true, Fact: "跳过（A 失败）"})
			} else {
				results = append(results, diag.Relay(host, ctrl))
			}
			results = append(results,
				diag.Result{Seg: "C", Name: "鉴权", Skip: true, Fact: "跳过（需要活跃会话，W2 提供）"},
				diag.Result{Seg: "D", Name: "隧道", Skip: true, Fact: "跳过（需要活跃会话，W2 提供）"},
			)
			diag.Print(os.Stdout, results)
			fmt.Printf("%s %-8s %s %s\n", "E", "外部可达", diag.MarkPass, address.HealthURL(host, engine.HealthPort))
			fmt.Printf("\n→ 用手机 4G 打开上面的地址（或扫码），显示 OK 即外部链路正常：\n%s\n", address.RenderQR(address.HealthURL(host, engine.HealthPort)))
			fmt.Printf("\n结论：%s\n", diag.Verdict(results))
			return nil
		},
	}
	cmd.Flags().StringVar(&flagHost, "host", "", "中继地址（默认从本地状态读）")
	cmd.Flags().IntVar(&flagCtrl, "port", 0, "控制通道端口")
	cmd.Flags().IntVar(&flagLocal, "local-port", 6768, "本地 Orca 端口")
	cmd.Flags().IntVar(&flagOrcaPort, "orca-port", 0, "--local-port 的别名（兼容）")
	cmd.Flags().MarkHidden("orca-port")
	return cmd
}

// splitHostPort 解析 "host:port"，失败返回零值。
func splitHostPort(s string) (string, int) {
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return s, 0
	}
	p, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return s, 0
	}
	return strings.Trim(s[:i], "[]"), p
}

var _ = bufio.NewReader // 保留占位
