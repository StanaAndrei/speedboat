package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestTrayIconEmbedded(t *testing.T) {
	if len(trayIconBytes) == 0 {
		t.Fatal("expected trayIconBytes to be embedded and non-empty")
	}

	img, err := png.Decode(bytes.NewReader(trayIconBytes))
	if err != nil {
		t.Fatalf("trayIconBytes is not a valid PNG: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		t.Fatalf("invalid icon dimensions: %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestHideWindowOnCloseOption(t *testing.T) {
	app := NewApp()
	opts := getAppOptions(app)
	if opts == nil {
		t.Fatal("expected getAppOptions to return non-nil options")
	}

	if !opts.HideWindowOnClose {
		t.Errorf("expected HideWindowOnClose to be true, got %v", opts.HideWindowOnClose)
	}

	if opts.OnStartup == nil {
		t.Error("expected OnStartup to be configured")
	}

	if opts.OnShutdown == nil {
		t.Error("expected OnShutdown to be configured")
	}
}

func TestTrayManagerLifecycle(t *testing.T) {
	app := NewApp()
	tm := app.trayMgr
	if tm == nil {
		t.Fatal("expected app.trayMgr to be initialized")
	}

	// Initialize tray
	if err := tm.initTray(); err != nil {
		t.Logf("initTray returned err (normal in headless test environment): %v", err)
	}

	// Update status
	tm.UpdateStatus("computing", "Crunching numbers")
	if tm.statusText != "Crunching numbers" {
		t.Errorf("expected statusText to be 'Crunching numbers', got '%s'", tm.statusText)
	}

	tm.UpdateStatus("idle", "")
	if tm.statusText != "idle" {
		t.Errorf("expected statusText to be 'idle', got '%s'", tm.statusText)
	}

	// Window hide and show calls should not panic with nil ctx
	tm.ShowWindow()
	tm.HideWindow()

	// Shutdown should be idempotent
	tm.Shutdown()
	tm.Shutdown()

	if tm.tray != nil {
		t.Errorf("expected tm.tray to be nil after shutdown")
	}
}
