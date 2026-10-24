package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

//go:embed all:frontend/dist
var assets embed.FS

func getAppOptions(app *App) *options.App {
	opts := &options.App{
		Title:  "Speedboat - Volunteer Computing Agent",
		Width:  1024,
		Height: 740,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour:  &options.RGBA{R: 9, G: 13, B: 22, A: 255},
		OnStartup:         app.startup,
		OnShutdown:        app.shutdown,
		HideWindowOnClose: true,
		Bind: []interface{}{
			app,
		},
	}

	if len(trayIconBytes) > 0 {
		opts.Linux = &linux.Options{
			Icon:        trayIconBytes,
			ProgramName: "speedboat-gui",
		}
	}

	return opts
}

func main() {
	app := NewApp()

	err := wails.Run(getAppOptions(app))
	if err != nil {
		println("Error:", err.Error())
	}
}
