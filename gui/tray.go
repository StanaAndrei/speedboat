package main

import (
	"context"
	_ "embed"
	"fmt"
	"sync"

	"github.com/gogpu/systray"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/trayicon.png
var trayIconBytes []byte

type TrayManager struct {
	app        *App
	ctx        context.Context
	tray       *systray.SystemTray
	statusItem *systray.MenuItem
	mu         sync.Mutex
	statusText string
}

func NewTrayManager(app *App) *TrayManager {
	return &TrayManager{
		app:        app,
		statusText: "Ready",
	}
}

func (tm *TrayManager) Start(ctx context.Context) {
	tm.mu.Lock()
	tm.ctx = ctx
	tm.mu.Unlock()

	tm.startPlatform()
}

func (tm *TrayManager) initTray() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tray := systray.New()
	if tray == nil {
		return fmt.Errorf("failed to create system tray")
	}
	tm.tray = tray

	if len(trayIconBytes) > 0 {
		tray.SetIcon(trayIconBytes)
		tray.SetTemplateIcon(trayIconBytes)
	}

	tooltip := "Speedboat - Volunteer Computing Agent"
	tray.SetTooltip(tooltip)

	menu := systray.NewMenu()

	menu.Add("Open Speedboat", func() {
		tm.ShowWindow()
	})

	menu.Add("Hide to Tray", func() {
		tm.HideWindow()
	})

	menu.AddSeparator()

	statusLabel := fmt.Sprintf("Status: %s", tm.statusText)
	tm.statusItem = menu.Add(statusLabel, func() {})
	if tm.statusItem != nil {
		tm.statusItem.SetDisabled(true)
	}

	menu.AddSeparator()

	menu.Add("Quit Speedboat", func() {
		tm.Quit()
	})

	tray.SetMenu(menu)

	tray.OnClick(func() {
		tm.ShowWindow()
	})

	tray.OnDoubleClick(func() {
		tm.ShowWindow()
	})

	tray.Show()
	return nil
}

func (tm *TrayManager) ShowWindow() {
	tm.mu.Lock()
	ctx := tm.ctx
	tm.mu.Unlock()

	if ctx != nil {
		go func() {
			wruntime.WindowShow(ctx)
			wruntime.WindowUnminimise(ctx)
			wruntime.Show(ctx)
		}()
	}
}

func (tm *TrayManager) HideWindow() {
	tm.mu.Lock()
	ctx := tm.ctx
	tm.mu.Unlock()

	if ctx != nil {
		go func() {
			wruntime.WindowHide(ctx)
		}()
	}
}

func (tm *TrayManager) UpdateStatus(state, text string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if text == "" {
		text = state
	}
	if text == "" {
		text = "Idle"
	}
	tm.statusText = text

	if tm.tray != nil {
		tm.tray.SetTooltip(fmt.Sprintf("Speedboat - %s", text))
	}
	if tm.statusItem != nil {
		tm.statusItem.SetLabel(fmt.Sprintf("Status: %s", text))
	}
}

func (tm *TrayManager) Quit() {
	tm.Shutdown()
	tm.mu.Lock()
	ctx := tm.ctx
	tm.mu.Unlock()

	if ctx != nil {
		go wruntime.Quit(ctx)
	}
}

func (tm *TrayManager) Shutdown() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.tray != nil {
		tm.tray.Remove()
		tm.tray = nil
		tm.statusItem = nil
	}
}
