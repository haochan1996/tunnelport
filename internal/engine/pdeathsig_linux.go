//go:build linux

package engine

import (
	"os/exec"
	"syscall"
)

// setPDeathsig Linux：父进程死亡时内核直接回收 rathole，避免孤儿占端口。
func setPDeathsig(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
