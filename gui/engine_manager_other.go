//go:build !linux && !windows

package main

import (
	"context"
	"fmt"
	goruntime "runtime"

	speedboatRuntime "speedboat/internal/runtime"
)

type otherHeavyEngineManager struct{}

func NewHeavyEngineManager() HeavyEngineManager {
	return &otherHeavyEngineManager{}
}

func (m *otherHeavyEngineManager) GetCapabilities() EngineCapabilities {
	engineName := "Containers (Docker / WSL2)"
	if goruntime.GOOS == "darwin" {
		engineName = "Containers (Colima / Docker)"
	}

	return EngineCapabilities{
		Platform:           goruntime.GOOS,
		HeavyModeSupported: false,
		EngineName:         engineName,
		IsInstalled:        false,
		BinaryPath:         "",
		Version:            "",
		CanAutoInstall:     false,
		GPUAvailable:       false,
		GPUVendor:          "None",
		StatusMessage: fmt.Sprintf(
			"Heavy Mode (native containers) is currently optimized for Linux. On %s, please use Lite Mode (Wazero WASM) for direct computation.",
			goruntime.GOOS,
		),
	}
}

func (m *otherHeavyEngineManager) Install(ctx context.Context, onProgress ProgressCallback) error {
	return fmt.Errorf("one-click container installation is currently only supported on Linux")
}

func (m *otherHeavyEngineManager) CreateEngine(enableGPU bool) (speedboatRuntime.Engine, error) {
	return nil, fmt.Errorf("heavy mode containers are currently supported on Linux; use Lite Mode (Wazero WASM) on %s", goruntime.GOOS)
}
