// Package diag 实现 tp doctor 的五段诊断输出（FR-7）。
// 严格三段式：状态符号 + 一句事实 + 一个动作；前段失败后续跳过（FR-7.1/7.2）。
package diag

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/haochan1996/tunnelport-internal/internal/detect"
)

// Mark 状态符号。
const (
	MarkPass = "✔"
	MarkFail = "✘"
	MarkSkip = "–"
)

// Result 是一段检查的结论。
type Result struct {
	Seg    string // A–E
	Name   string // 本地服务 / 中继可达 / ...
	Pass   bool
	Skip   bool
	Fact   string // 一句事实
	Action string // 一个动作
}

// Local 检查 A 段：Orca 端口是否监听。
func Local(port int) Result {
	r := Result{Seg: "A", Name: "本地服务"}
	if detect.Listening(port) {
		r.Pass = true
		r.Fact = fmt.Sprintf("Orca 在 127.0.0.1:%d", port)
	} else {
		r.Fact = fmt.Sprintf("127.0.0.1:%d 没有进程监听", port)
		r.Action = "Orca 好像没启动，先打开 Orca"
	}
	return r
}

// Relay 检查 B 段：中继控制端口是否可达。
func Relay(host string, controlPort int) Result {
	r := Result{Seg: "B", Name: "中继可达"}
	rtt, err := detect.TCPRtt(host, controlPort, 3*time.Second)
	if err == nil {
		r.Pass = true
		r.Fact = fmt.Sprintf("%s:%d 可达（%dms）", host, controlPort, rtt.Milliseconds())
	} else {
		r.Fact = fmt.Sprintf("连不上 %s:%d", host, controlPort)
		r.Action = fmt.Sprintf("检查云厂商安全组 / 防火墙是否放行 TCP %d", controlPort)
	}
	return r
}

// Print 按五段格式输出到 w。
func Print(w io.Writer, results []Result) {
	for _, r := range results {
		mark, pad := MarkSkip, ""
		switch {
		case r.Skip:
			mark = MarkSkip
		case r.Pass:
			mark = MarkPass
		default:
			mark = MarkFail
		}
		line := fmt.Sprintf("%s %-8s %s %s", r.Seg, r.Name, mark, r.Fact)
		fmt.Fprintln(w, line)
		if r.Action != "" {
			pad = strings.Repeat(" ", 15)
			fmt.Fprintf(w, "%s→ %s\n", pad, r.Action)
		}
	}
}

// Verdict 根据结果生成一句话结论（第一失败段驱动）。
func Verdict(results []Result) string {
	for _, r := range results {
		if !r.Pass && !r.Skip {
			return r.Action
		}
	}
	return "五段全部通过，链路正常。"
}
