//go:build windows

package main

import "golang.org/x/sys/windows"

// acquire 单实例锁（Windows 命名互斥体）。
// 返回 isPrimary、signalShow（二次启动唤起主窗口）、showRequested（主实例轮询）。
func acquire() (bool, func(), func() bool) {
	name, _ := windows.UTF16PtrFromString("tunnelport-single-instance")
	h, err := windows.CreateMutex(nil, false, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		// 必须关掉句柄：否则本进程的句柄把互斥体"钉住"，旧进程退出后锁仍在，
		// --restart 的等锁重试永远失败（v1.0.21 根修"点击重启没起来"）
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
		return false, writeShow, func() bool { return false }
	}
	// 主实例持有句柄至进程退出（互斥体随进程存活）
	return true, func() {}, pollShow
}
