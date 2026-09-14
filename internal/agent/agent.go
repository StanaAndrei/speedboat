package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

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

	// 3. Mk pull subscriber on workqueue
	durableName := fmt.Sprintf("speedboat-workers-%s", a.ProjectID)
	subj := transport.TasksSubject(a.ProjectID)
	sub, err := js.PullSubscribe(subj, durableName)
	if err != nil {
		return fmt.Errorf("failed to subscribe to JetStream queue on %s: %w", subj, err)
	}
	defer sub.Unsubscribe()

	fmt.Printf("⚡ Ready! Listening for work on '%s'...\n", subj)

	tasksProcessed := 0
	pulledAny := false

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\n🛑 Agent shutting down gracefully...")
			return nil
		default:
		}

		if a.MaxTasks > 0 && tasksProcessed >= a.MaxTasks {
			fmt.Printf("🏁 Reached max task limit (%d tasks). Exiting.\n", a.MaxTasks)
			return nil
		}

		// Fetch 1 task message from JetStream WorkQueue
		msgs, err := sub.Fetch(1, nats.MaxWait(2*time.Second))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				if a.Once && pulledAny {
					fmt.Println("🏁 WorkQueue empty (--once specified). Exiting.")
					return nil
				}
				continue // Wait and retry
			}
			time.Sleep(500 * time.Millisecond) // Transient error or reconnection
			continue
		}

		if len(msgs) == 0 {
			continue
		}

		msg := msgs[0]
		pulledAny = true

		if err := a.processTaskMessage(ctx, msg); err != nil {
			fmt.Printf("❌ [Worker %s] Task processing error: %v\n", a.WorkerID, err)
		} else {
			tasksProcessed++
		}
	}
}

func (a *Agent) processTaskMessage(ctx context.Context, msg *nats.Msg) error {
	return nil
}
