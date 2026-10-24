//go:build darwin

package main

func (tm *TrayManager) startPlatform() {
	// On macOS, Wails runs the AppKit event loop ([NSApp run]) on the main thread.
	// We must not call tm.tray.Run() on a secondary goroutine because Cocoa forbids
	// running multiple or secondary [NSApp run] loops.
	// Initializing the tray and attaching the NSStatusItem is sufficient;
	// Wails' main event loop pumps the AppKit event queue and dispatches click/menu actions.
	if err := tm.initTray(); err != nil {
		return
	}
}
