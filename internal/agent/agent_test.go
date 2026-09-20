package agent

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"speedboat/internal/protocol"
	"speedboat/internal/transport"

	"github.com/nats-io/nats.go"
)

func TestNewAgentDefaultWorkerIDUsesIP(t *testing.T) {
	ag := NewAgent("test-proj", "nats://localhost:4222", "auto", "", 10, true, 30)
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
	ag := NewAgent("test-proj", "nats://localhost:4222", "auto", customID, 10, true, 30)
	if ag.WorkerID != customID {
		t.Fatalf("expected WorkerID to be %s, got: %s", customID, ag.WorkerID)
	}
	if ag.IdleTimeout != 30 {
		t.Fatalf("expected IdleTimeout to be 30, got: %d", ag.IdleTimeout)
	}
}

func TestNewAgentIdleTimeout(t *testing.T) {
	ag := NewAgent("test-proj", "nats://localhost:4222", "auto", "test-worker", 5, false, 45)
	if ag.IdleTimeout != 45 {
		t.Fatalf("expected IdleTimeout to be 45, got: %d", ag.IdleTimeout)
	}
}

func TestAgentRunIdleTimeoutWithNoTasks(t *testing.T) {
	natsURL := "nats://localhost:4222"
	nc, err := nats.Connect(natsURL, nats.Timeout(1*time.Second))
	if err != nil {
		t.Skip("NATS server not available on localhost:4222, skipping idle timeout integration test")
	}
	defer nc.Close()

	projID := "test-idle-proj"
	reqSubj := transport.TaskRequestSubject(projID)

	sub, err := nc.Subscribe(reqSubj, func(msg *nats.Msg) {
		resp := protocol.TaskResponse{Status: "no_tasks"}
		respBytes, _ := json.Marshal(resp)
		_ = msg.Respond(respBytes)
	})
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	ag := NewAgent(projID, natsURL, "auto", "idle-worker-1", 0, false, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err = ag.Run(ctx)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("expected Run to return nil on idle timeout, got: %v", err)
	}
	if duration < 2*time.Second {
		t.Fatalf("expected agent to run for at least 2s idle timeout, finished in: %v", duration)
	}
	if duration > 4*time.Second {
		t.Fatalf("expected agent to stop around 2-3s idle timeout, took: %v", duration)
	}
}
