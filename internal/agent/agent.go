package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"speedboat/internal/runtime"

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
	return nil
}
