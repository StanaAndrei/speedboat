package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"speedboat/internal/runtime"
	"speedboat/internal/transport"

	"github.com/nats-io/nats.go"
)

type Agent struct {
	ProjectID         string
	NatsURL           string
	RuntimePreference string
	WorkerID          string
	MaxTasks          int
	Once              bool

	nc     *nats.Conn
	js     nats.JetStreamContext
	engine runtime.Engine
}

func NewAgent(projectID, natsURL, runtimePref, workerID string, maxTasks int, once bool) *Agent {
	if workerID == "" {
		host, _ := os.Hostname()
		if host == "" {
			host = "agent"
		}
		randBytes := make([]byte, 4)
		_, _ = rand.Read(randBytes)
		workerID = fmt.Sprintf("%s-%s", host, hex.EncodeToString(randBytes))
	}

	return &Agent{
		ProjectID:         projectID,
		NatsURL:           natsURL,
		RuntimePreference: runtimePref,
		WorkerID:          workerID,
		MaxTasks:          maxTasks,
		Once:              once,
	}
}

func (a *Agent) Run(ctx context.Context) error {
	// 1. init engine
	engine, err := runtime.AutoDetectEngine(a.RuntimePreference)
	if err != nil {
		return fmt.Errorf("failed to detect runtime engine: %w", err)
	}
	a.engine = engine

	fmt.Printf("🚤 Speedboat Volunteer Agent\n")
	fmt.Printf("   Worker ID: %s\n", a.WorkerID)
	fmt.Printf("   Target Project: %s\n", a.ProjectID)
	fmt.Printf("   Runtime Engine: %s\n", engine.Name())
	fmt.Printf("   Connecting to NATS: %s\n", a.NatsURL)

	// 2. connect to NATS
	nc, js, err := transport.Connect(a.NatsURL)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	defer nc.Close()
	a.nc = nc
	a.js = js
	return nil
}
