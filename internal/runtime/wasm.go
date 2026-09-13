package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type WasmEngine struct { // executes .wasm payloads using an embedded or external WASI runtime
	binaryPath string
}

func NewWasmEngine() *WasmEngine {
	for _, bin := range []string{"wasmtime", "wasmer", "wazero"} {
		if path, err := exec.LookPath(bin); err == nil {
			return &WasmEngine{binaryPath: path}
		}
	}
	return &WasmEngine{}
}

func (w *WasmEngine) Name() string {
	return "wasm"
}

func (w *WasmEngine) IsAvailable() bool {
	return w.binaryPath != ""
}

func (w *WasmEngine) PullImage(ctx context.Context, image string) error {
	// For WASM, images are .wasm files or ObjectStore keys
	return nil
}

func (w *WasmEngine) Execute(ctx context.Context, wasmPath string, input []byte) ([]byte, error) {
	if !w.IsAvailable() {
		return nil, fmt.Errorf("no WASM runtime engine (wasmtime, wasmer, or wazero) found")
	}

	cmd := exec.CommandContext(ctx, w.binaryPath, "run", wasmPath)
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("wasm execution failed: %w\nstderr: %s", err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}
