//go:build windows

package engine

import (
	"os/exec"
)

// setPDeathsig Windows 无等价的 Pdeathsig；tp 崩溃时 rathole 成为孤儿，
// 由 W2 的服务管理（任务计划/服务停止时按进程树终止）兜底。
func setPDeathsig(cmd *exec.Cmd) {
	// Windows: 交给 Job Object / 服务管理（W2）
	_ = cmd
}
