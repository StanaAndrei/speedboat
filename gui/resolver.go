package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProjectMetadata represents resolved connection and project parameters for the agent.
type ProjectMetadata struct {
	InputURL   string `json:"input_url"`
	ProjectID  string `json:"project_id"`
	NatsWsURL  string `json:"nats_ws_url"`
	PayloadURL string `json:"payload_url"`
	MasterHost string `json:"master_host"`
	Reachable  bool   `json:"reachable"`
	StatusText string `json:"status_text"`
}

type infoResponse struct {
	Project   string `json:"project"`
	NatsWsURL string `json:"nats_ws_url"`
}

// ResolveProjectURL parses a volunteer URL (e.g., http://host:3000/?project=foo&nats=ws://host:8080)
// or queries the master /info endpoint if query parameters are omitted.
func ResolveProjectURL(ctx context.Context, input string) (*ProjectMetadata, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}

	// Normalize missing protocol
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "http://" + raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL format: %w", err)
	}

	if parsed.Host == "" {
		return nil, fmt.Errorf("URL missing host")
	}

	baseURL := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
	q := parsed.Query()
	projectID := strings.TrimSpace(q.Get("project"))
	natsURL := strings.TrimSpace(q.Get("nats"))
	payloadURL := fmt.Sprintf("%s/payload.wasm", baseURL)

	meta := &ProjectMetadata{
		InputURL:   raw,
		ProjectID:  projectID,
		NatsWsURL:  natsURL,
		PayloadURL: payloadURL,
		MasterHost: parsed.Host,
		Reachable:  false,
		StatusText: "Connecting...",
	}

	// Probe master endpoint with short timeout
	client := &http.Client{Timeout: 3 * time.Second}
	infoURL := fmt.Sprintf("%s/info", baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, infoURL, nil)
	if err != nil {
		meta.StatusText = fmt.Sprintf("Error creating request: %v", err)
		return meta, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		meta.StatusText = fmt.Sprintf("Cannot reach master node (%v)", err)
		return meta, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		meta.Reachable = true
		meta.StatusText = "Online"

		var info infoResponse
		if err := json.NewDecoder(resp.Body).Decode(&info); err == nil {
			if meta.ProjectID == "" && info.Project != "" {
				meta.ProjectID = info.Project
			}
			if meta.NatsWsURL == "" && info.NatsWsURL != "" {
				meta.NatsWsURL = info.NatsWsURL
			}
		}
	} else {
		meta.Reachable = true
		meta.StatusText = fmt.Sprintf("Master node returned HTTP %d", resp.StatusCode)
	}

	// Fallback defaults if still empty
	if meta.ProjectID == "" {
		meta.ProjectID = "default-project"
	}
	if meta.NatsWsURL == "" {
		hostOnly := parsed.Hostname()
		meta.NatsWsURL = fmt.Sprintf("ws://%s:8080", hostOnly)
	}

	return meta, nil
}
