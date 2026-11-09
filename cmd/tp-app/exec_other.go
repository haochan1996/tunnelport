//go:build !windows

package main

import (
	"fmt"
	"os/exec"
)

// hiddenCmd 非 Windows 平台无控制台概念，原样执行。
func hiddenCmd(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// runPS 非 Windows 平台无 PowerShell。
func runPS(script string) error {
	return fmt.Errorf("仅 Windows 支持")
}

// startPS 非 Windows 平台无 PowerShell。
func startPS(script string) error {
	return fmt.Errorf("仅 Windows 支持")
}
