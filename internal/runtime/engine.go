package runtime

import (
	"context"
	"fmt"
	"strings"
)

type Engine interface {
	Name() string
	IsAvailable() bool
	PullImage(ctx context.Context, image string) error
	Execute(ctx context.Context, image string, input []byte) ([]byte, error)
}

func AutoDetectEngine(preference string) (Engine, error) {
	pref := strings.ToLower(strings.TrimSpace(preference))

	dockerEng := NewDockerEngine()
	apptainerEng := NewApptainerEngine()
	wasmEng := NewWasmEngine()

	switch pref {
	case "apptainer", "singularity":
		if apptainerEng.IsAvailable() {
			return apptainerEng, nil
		}
		return nil, fmt.Errorf("apptainer runtime requested but apptainer is not available")

	case "docker":
		if dockerEng.IsAvailable() {
			return dockerEng, nil
		}
		return nil, fmt.Errorf("docker runtime requested but docker is not available")

	case "wasm", "wazero":
		if wasmEng.IsAvailable() {
			return wasmEng, nil
		}
		return nil, fmt.Errorf("wasm runtime requested but wasm runtime is not available")

	case "auto", "":
		if dockerEng.IsAvailable() {
			return dockerEng, nil
		}
		if apptainerEng.IsAvailable() {
			return apptainerEng, nil
		}
		if wasmEng.IsAvailable() {
			return wasmEng, nil
		}
		return nil, fmt.Errorf("no supported runtime engine (Docker, Apptainer, or WASM) found on host")

	default:
		return nil, fmt.Errorf("unknown runtime engine preference: %s", preference)
	}
}
