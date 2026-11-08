package main

import (
	"os"
	"path/filepath"
)

// writeShow 二次启动：写唤起命令后退出（主实例轮询 showFile 唤起窗口）。
func writeShow() {
	home, _ := os.UserHomeDir()
	_ = os.WriteFile(showFile(home), []byte("show"), 0o600)
}

// pollShow 主实例轮询唤起命令。
func pollShow() bool {
	home, _ := os.UserHomeDir()
	f := showFile(home)
	if _, err := os.Stat(f); err == nil {
		_ = os.Remove(f)
		return true
	}
	return false
}

func showFile(home string) string {
	_ = os.MkdirAll(filepath.Join(home, ".tunnelport"), 0o700)
	return filepath.Join(home, ".tunnelport", "show.cmd")
}
