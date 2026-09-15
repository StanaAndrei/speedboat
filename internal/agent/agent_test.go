package agent

import (
	"net"
	"strings"
	"testing"
)

func TestNewAgentDefaultWorkerIDUsesIP(t *testing.T) {
	ag := NewAgent("test-proj", "nats://localhost:4222", "auto", "", 10, true)
	if ag == nil {
		t.Fatal("expected non-nil agent")
	}

	parts := strings.Split(ag.WorkerID, "-")
	if len(parts) < 2 {
		t.Fatalf("unexpected WorkerID format: %s", ag.WorkerID)
	}

	ipStr := parts[0]
	parsedIP := net.ParseIP(ipStr)
	if parsedIP == nil {
		t.Fatalf("expected WorkerID prefix to be a valid IP address, got: %s (WorkerID: %s)", ipStr, ag.WorkerID)
	}
}

func TestNewAgentCustomWorkerID(t *testing.T) {
	customID := "custom-worker-123"
	ag := NewAgent("test-proj", "nats://localhost:4222", "auto", customID, 10, true)
	if ag.WorkerID != customID {
		t.Fatalf("expected WorkerID to be %s, got: %s", customID, ag.WorkerID)
	}
}
