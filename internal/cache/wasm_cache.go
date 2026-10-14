package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WasmCache manages downloaded and local .wasm payloads across platforms.
type WasmCache struct {
	BaseDir string
}

// NewWasmCache returns a WasmCache configured with the OS user cache directory.
func NewWasmCache() (*WasmCache, error) {
	userCache, err := os.UserCacheDir()
	if err != nil {
		userCache = os.TempDir()
	}
	dir := filepath.Join(userCache, "speedboat", "wasm")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create wasm cache directory %s: %w", dir, err)
	}
	return &WasmCache{BaseDir: dir}, nil
}

// PathForProject returns the path to the cached wasm file for a given project.
func (c *WasmCache) PathForProject(projectID string) string {
	safeName := sanitizeProjectName(projectID)
	return filepath.Join(c.BaseDir, safeName+".wasm")
}

// GetOrFetch returns the cached wasm binary data and file path.
// If not found locally and a valid wasmURL is provided, it downloads and caches the file.
func (c *WasmCache) GetOrFetch(ctx context.Context, projectID, wasmURL string) ([]byte, string, error) {
	cachePath := c.PathForProject(projectID)

	// Check if already cached
	if fi, err := os.Stat(cachePath); err == nil && fi.Size() > 0 {
		data, err := os.ReadFile(cachePath)
		if err == nil && len(data) > 0 {
			return data, cachePath, nil
		}
	}

	if wasmURL == "" {
		return nil, "", fmt.Errorf("payload not found in cache and no wasmURL provided for project %s", projectID)
	}

	// Download via HTTP
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wasmURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("invalid wasm download request: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch wasm from %s: %w", wasmURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to download wasm payload, server returned HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed reading wasm payload body: %w", err)
	}

	// Atomic write
	randBytes := make([]byte, 4)
	_, _ = rand.Read(randBytes)
	tmpPath := fmt.Sprintf("%s.tmp.%s", cachePath, hex.EncodeToString(randBytes))

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return nil, "", fmt.Errorf("failed writing temporary wasm file: %w", err)
	}

	if err := os.Rename(tmpPath, cachePath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, "", fmt.Errorf("failed finalizing cached wasm file: %w", err)
	}

	return data, cachePath, nil
}

// Clear removes any cached wasm file for a given project.
func (c *WasmCache) Clear(projectID string) error {
	path := c.PathForProject(projectID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func sanitizeProjectName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "default"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return replacer.Replace(name)
}
