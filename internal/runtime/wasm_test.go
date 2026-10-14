package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWasmEngineExecution(t *testing.T) {
	eng := NewWasmEngine()
	if eng.Name() != "wasm" {
		t.Fatalf("expected engine name 'wasm', got %q", eng.Name())
	}
	if !eng.IsAvailable() {
		t.Fatalf("expected wazero engine to always report available")
	}

	// Find logic.wasm
	wasmPath := filepath.Join("..", "..", "examples", "prime-cnt", "logic.wasm")
	if _, err := os.Stat(wasmPath); err != nil {
		t.Skipf("skipping test: %s not found", wasmPath)
	}

	ctx := context.Background()
	taskInput := []byte(`{"task_id":"test-wazero","start":1,"end":20}`)

	out, err := eng.Execute(ctx, wasmPath, taskInput)
	if err != nil {
		t.Fatalf("wasm execution failed: %v", err)
	}

	outStr := string(out)
	if !strings.Contains(outStr, `"count": 8`) && !strings.Contains(outStr, `"count":8`) {
		t.Errorf("expected count 8 for primes between 1 and 20, got: %s", outStr)
	}
}
