//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// hiddenCmd 拉起子进程且不弹控制台黑窗（CREATE_NO_WINDOW）。
func hiddenCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return c
}

// writePS 脚本落临时 .ps1（末尾自删）。不能用 -Command 传内联脚本：
// Go 的参数转义把脚本内双引号变成 \"，PowerShell/cmd 不认这种转义——
// 重启/快捷方式/自启脚本被搅碎的根因（v1.0.25）。
func writePS(script string) (string, error) {
	f, err := os.CreateTemp("", "tpps*.ps1")
	if err != nil {
		return "", err
	}
	selfdel := "\nRemove-Item -LiteralPath $MyInvocation.MyCommand.Path -ErrorAction SilentlyContinue\n"
	_, _ = f.WriteString("\ufeff" + script + selfdel) // BOM：PowerShell 5.1 中文不乱码
	f.Close()
	return f.Name(), nil
}

// runPS 同步执行 PowerShell 脚本。
func runPS(script string) error {
	p, err := writePS(script)
	if err != nil {
		return err
	}
	out, err := hiddenCmd("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", p).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// startPS 异步执行（脚本自删；调用方可立刻退出）。
func startPS(script string) error {
	p, err := writePS(script)
	if err != nil {
		return err
	}
	return hiddenCmd("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", p).Start()
}
