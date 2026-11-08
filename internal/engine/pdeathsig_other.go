//go:build !linux

package engine

import "os/exec"

// setPDeathsig 非 Linux 无 Pdeathsig：macOS/Windows 交由服务管理兜底（FR-5）。
func setPDeathsig(cmd *exec.Cmd) {
	_ = cmd
}
