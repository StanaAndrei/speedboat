package orchestrator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"speedboat/internal/orchestrator/web"
)

func TestWebHandler(t *testing.T) {
	tmpDir := t.TempDir()
	wasmFile := filepath.Join(tmpDir, "test.wasm")
	wasmContent := []byte("\x00asm\x01\x00\x00\x00")
	if err := os.WriteFile(wasmFile, wasmContent, 0644); err != nil {
		t.Fatal(err)
	}

	handler := web.Handler("test-project", "ws://localhost:8080", wasmFile)

	t.Run("GET / serves index.html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
			t.Errorf("expected text/html, got %s", rec.Header().Get("Content-Type"))
		}
		if !strings.Contains(rec.Body.String(), "Speedboat Grid") {
			t.Errorf("expected body to contain 'Speedboat Grid'")
		}
	})

	t.Run("GET /worker.js serves worker script", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/worker.js", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "application/javascript") {
			t.Errorf("expected application/javascript, got %s", rec.Header().Get("Content-Type"))
		}
		if !strings.Contains(rec.Body.String(), "executeWASI") {
			t.Errorf("expected body to contain 'executeWASI'")
		}
	})

	t.Run("GET /info returns project metadata", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/info", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
		var info web.InfoResponse
		if err := json.NewDecoder(rec.Body).Decode(&info); err != nil {
			t.Fatalf("failed to decode json: %v", err)
		}
		if info.Project != "test-project" {
			t.Errorf("expected project 'test-project', got %q", info.Project)
		}
		if info.NatsWsURL != "ws://localhost:8080" {
			t.Errorf("expected nats ws url 'ws://localhost:8080', got %q", info.NatsWsURL)
		}
	})

	t.Run("GET /payload.wasm serves wasm bytes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/payload.wasm", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
		if rec.Header().Get("Content-Type") != "application/wasm" {
			t.Errorf("expected application/wasm, got %s", rec.Header().Get("Content-Type"))
		}
		if rec.Body.String() != string(wasmContent) {
			t.Errorf("payload content mismatch")
		}
	})

	t.Run("OPTIONS / returns CORS headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("expected CORS header *")
		}
	})
}
