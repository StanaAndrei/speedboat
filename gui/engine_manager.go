package main

import (
	"context"
	"speedboat/internal/runtime"
)

// EngineCapabilities describes the available runtime engines and features on this host.
type EngineCapabilities struct {
	Platform           string `json:"platform"`              // "linux", "darwin", "windows"
	HeavyModeSupported bool   `json:"heavy_mode_supported"`  // true on Linux, false for now on others
	EngineName         string `json:"engine_name"`           // e.g. "Apptainer", "WSL2 / Docker", "Colima"
	IsInstalled        bool   `json:"is_installed"`
	BinaryPath         string `json:"binary_path"`
	Version            string `json:"version"`
	CanAutoInstall     bool   `json:"can_auto_install"`      // true on Linux for 1-click install
	GPUAvailable       bool   `json:"gpu_available"`
	GPUVendor          string `json:"gpu_vendor"`            // "NVIDIA", "AMD", "None"
	StatusMessage      string `json:"status_message"`
}

// ProgressCallback is invoked during engine installation to notify UI.
type ProgressCallback func(percent float64, message string)

// HeavyEngineManager manages the lifecycle and detection of container runtimes.
type HeavyEngineManager interface {
	GetCapabilities() EngineCapabilities
	Install(ctx context.Context, onProgress ProgressCallback) error
	CreateEngine(enableGPU bool) (runtime.Engine, error)
}
