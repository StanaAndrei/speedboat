package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

//go:embed index.html styles.css worker.js
var Content embed.FS

type InfoResponse struct {
	Project   string `json:"project"`
	NatsWsURL string `json:"nats_ws_url"`
}

// Handler returns an http.Handler that serves the embedded web dashboard,
// the dedicated Web Worker script, and the target WASM file.
func Handler(projectID, natsWsURL, wasmFilePath string) http.Handler {
	mux := http.NewServeMux()

	// Serve index.html
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		data, err := Content.ReadFile("index.html")
		if err != nil {
			http.Error(w, "internal server error: missing index.html", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})

	// Serve styles.css
	mux.HandleFunc("/styles.css", func(w http.ResponseWriter, r *http.Request) {
		data, err := Content.ReadFile("styles.css")
		if err != nil {
			http.Error(w, "internal server error: missing styles.css", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(data)
	})

	// Serve worker.js
	mux.HandleFunc("/worker.js", func(w http.ResponseWriter, r *http.Request) {
		data, err := Content.ReadFile("worker.js")
		if err != nil {
			http.Error(w, "internal server error: missing worker.js", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write(data)
	})

	// Serve /payload.wasm
	mux.HandleFunc("/payload.wasm", func(w http.ResponseWriter, r *http.Request) {
		if wasmFilePath == "" {
			http.Error(w, "no wasm payload configured", http.StatusNotFound)
			return
		}
		data, err := os.ReadFile(wasmFilePath)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to read wasm payload: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/wasm")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		_, _ = w.Write(data)
	})

	// Serve /info
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(InfoResponse{
			Project:   projectID,
			NatsWsURL: natsWsURL,
		})
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
