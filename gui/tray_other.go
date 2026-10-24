//go:build !windows && !linux && !darwin

package main

func (tm *TrayManager) startPlatform() {
	go func() {
		if err := tm.initTray(); err != nil {
			return
		}

		_ = tm.tray.Run()
	}()
}
