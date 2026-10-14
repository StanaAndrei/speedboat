package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

type WasmEngine struct{}

func NewWasmEngine() *WasmEngine {
	return &WasmEngine{}
}

func (w *WasmEngine) Name() string {
	return "wasm"
}

func (w *WasmEngine) IsAvailable() bool {
	return true
}

func (w *WasmEngine) PullImage(ctx context.Context, image string) error {
	// For WASM, images are .wasm local files or cache paths
	return nil
}

func (w *WasmEngine) Execute(ctx context.Context, wasmPath string, input []byte) ([]byte, error) {
	if wasmPath == "" {
		return nil, fmt.Errorf("wasm path cannot be empty")
	}

	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read wasm file at %s: %w", wasmPath, err)
	}

	return w.ExecuteBytes(ctx, wasmBytes, input)
}

func (w *WasmEngine) ExecuteBytes(ctx context.Context, wasmBytes []byte, input []byte) ([]byte, error) {
	r := wazero.NewRuntime(ctx)
	defer func() {
		_ = r.Close(ctx)
	}()

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	compiled, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to compile wasm module: %w", err)
	}
	defer func() {
		_ = compiled.Close(ctx)
	}()

	var stdout, stderr bytes.Buffer
	modConfig := wazero.NewModuleConfig().
		WithName("").
		WithStdin(bytes.NewReader(input)).
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithArgs("wasi_app")

	mod, err := r.InstantiateModule(ctx, compiled, modConfig)
	if err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() == 0 {
			// WASI proc_exit with code 0 is a clean success
		} else {
			return nil, fmt.Errorf("wasm execution failed: %w\nstderr: %s", err, strings.TrimSpace(stderr.String()))
		}
	} else {
		_ = mod.Close(ctx)
	}

	return stdout.Bytes(), nil
}
