//go:build windows

package main

import "golang.org/x/sys/windows"

// acquire 单实例锁（Windows 命名互斥体）。
// 返回 isPrimary、signalShow（二次启动唤起主窗口）、showRequested（主实例轮询）。
func acquire() (bool, func(), func() bool) {
	name, _ := windows.UTF16PtrFromString("tunnelport-single-instance")
	_, err := windows.CreateMutex(nil, false, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		return false, writeShow, func() bool { return false }
	}
	return true, func() {}, pollShow
}
