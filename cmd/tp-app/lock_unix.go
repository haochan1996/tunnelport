//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// acquire 单实例锁（Unix 文件锁 flock）。
func acquire() (bool, func(), func() bool) {
	home, _ := os.UserHomeDir()
	_ = os.MkdirAll(filepath.Join(home, ".tunnelport"), 0o700)
	f, err := os.OpenFile(filepath.Join(home, ".tunnelport", "app.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return true, func() {}, func() bool { return false }
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return false, writeShow, func() bool { return false }
	}
	return true, func() {}, pollShow
}
