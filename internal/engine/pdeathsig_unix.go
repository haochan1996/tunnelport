//go:build linux || darwin

package engine

import (
	"os/exec"
	"syscall"
)

// setPDeathsig Linux 上给 rathole 子进程设置父进程死亡信号：
// tp 被强杀（kill -9 / OOM）时内核直接杀掉 rathole，避免孤儿进程继续占用端口
// （2026-11-08 稳定性审查 #3）。macOS 无 Pdeathsig，此文件仅为共享编译。
func setPDeathsig(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
