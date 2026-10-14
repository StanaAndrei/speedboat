package runtime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

func TestConcurrentWazeroInstantiation(t *testing.T) {
	wasmPath := filepath.Join("..", "..", "examples", "prime-cnt", "logic.wasm")
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("skipping: %v", err)
	}

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	compiled, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		t.Fatalf("failed compiling module: %v", err)
	}
	defer compiled.Close(ctx)

	var wg sync.WaitGroup
	concurrency := 8
	errorsCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerIdx int) {
			defer wg.Done()
			var stdout, stderr bytes.Buffer
			input := []byte(`{"task_id":"concurrent-test","start":1,"end":20}`)

			// Crucial: WithName("") for anonymous instantiation
			modConfig := wazero.NewModuleConfig().
				WithName("").
				WithStdin(bytes.NewReader(input)).
				WithStdout(&stdout).
				WithStderr(&stderr).
				WithArgs("wasi_app")

			mod, execErr := r.InstantiateModule(ctx, compiled, modConfig)
			if execErr != nil {
				if exitErr, ok := execErr.(*sys.ExitError); ok && exitErr.ExitCode() == 0 {
					execErr = nil
				}
			} else {
				_ = mod.Close(ctx)
			}

			if execErr != nil {
				errorsCh <- execErr
				return
			}

			if !strings.Contains(stdout.String(), `"count": 8`) && !strings.Contains(stdout.String(), `"count":8`) {
				errorsCh <- os.ErrInvalid
				return
			}
		}(i)
	}

	wg.Wait()
	close(errorsCh)

	for err := range errorsCh {
		t.Fatalf("concurrent instantiation failed: %v", err)
	}
}
