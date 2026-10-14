package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveProjectURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/info" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(infoResponse{
				Project:   "server-project",
				NatsWsURL: "ws://server:8080",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	ctx := context.Background()

	t.Run("explicit query parameters", func(t *testing.T) {
		input := server.URL + "/?project=my-job&nats=ws://custom:9090"
		meta, err := ResolveProjectURL(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !meta.Reachable {
			t.Errorf("expected server to be reachable")
		}
		if meta.ProjectID != "my-job" {
			t.Errorf("expected project 'my-job', got %q", meta.ProjectID)
		}
		if meta.NatsWsURL != "ws://custom:9090" {
			t.Errorf("expected nats 'ws://custom:9090', got %q", meta.NatsWsURL)
		}
		if meta.PayloadURL != server.URL+"/payload.wasm" {
			t.Errorf("expected payload url %s/payload.wasm, got %s", server.URL, meta.PayloadURL)
		}
	})

	t.Run("fallback to /info when query parameters missing", func(t *testing.T) {
		input := server.URL
		meta, err := ResolveProjectURL(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !meta.Reachable {
			t.Errorf("expected server to be reachable")
		}
		if meta.ProjectID != "server-project" {
			t.Errorf("expected project 'server-project', got %q", meta.ProjectID)
		}
		if meta.NatsWsURL != "ws://server:8080" {
			t.Errorf("expected nats 'ws://server:8080', got %q", meta.NatsWsURL)
		}
	})

	t.Run("handles unreachable host gracefully", func(t *testing.T) {
		input := "http://127.0.0.1:59999/?project=offline"
		meta, err := ResolveProjectURL(ctx, input)
		if err != nil {
			t.Fatalf("unexpected fatal error: %v", err)
		}
		if meta.Reachable {
			t.Errorf("expected reachable to be false for dead port")
		}
		if meta.ProjectID != "offline" {
			t.Errorf("expected project 'offline', got %q", meta.ProjectID)
		}
	})
}
