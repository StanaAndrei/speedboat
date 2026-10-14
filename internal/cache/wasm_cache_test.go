package cache

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWasmCache(t *testing.T) {
	tmpDir := t.TempDir()
	cache := &WasmCache{BaseDir: tmpDir}

	dummyWasm := []byte("\x00asm\x01\x00\x00\x00")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/payload.wasm" {
			w.Header().Set("Content-Type", "application/wasm")
			_, _ = w.Write(dummyWasm)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	ctx := context.Background()
	projectID := "test-project"

	// 1. Fetch from server
	data, path, err := cache.GetOrFetch(ctx, projectID, server.URL+"/payload.wasm")
	if err != nil {
		t.Fatalf("unexpected error fetching wasm: %v", err)
	}
	if string(data) != string(dummyWasm) {
		t.Errorf("expected downloaded data to match dummy wasm")
	}
	if filepath.Base(path) != "test-project.wasm" {
		t.Errorf("unexpected cached filename: %s", path)
	}

	// 2. Fetch again - should come from local cache without needing server
	server.Close()
	cachedData, cachedPath, err := cache.GetOrFetch(ctx, projectID, "http://invalid-server-that-is-closed")
	if err != nil {
		t.Fatalf("failed retrieving from cache: %v", err)
	}
	if string(cachedData) != string(dummyWasm) {
		t.Errorf("cached data mismatch")
	}
	if cachedPath != path {
		t.Errorf("cached path mismatch: got %s, want %s", cachedPath, path)
	}

	// 3. Clear cache
	if err := cache.Clear(projectID); err != nil {
		t.Fatalf("failed to clear cache: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected cached file to be removed")
	}
}
