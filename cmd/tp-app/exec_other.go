//go:build !windows

package main

import "os/exec"

// hiddenCmd 非 Windows 平台无控制台概念，原样执行。
func hiddenCmd(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
