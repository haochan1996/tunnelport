//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hiddenCmd 拉起子进程且不弹控制台黑窗（CREATE_NO_WINDOW）。
func hiddenCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return c
}
