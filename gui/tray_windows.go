//go:build windows

package main

import (
	"runtime"
)

func (tm *TrayManager) startPlatform() {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		if err := tm.initTray(); err != nil {
			return
		}

		_ = tm.tray.Run()
	}()
}
