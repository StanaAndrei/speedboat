package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"speedboat/internal/protocol"
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
	IdleTimeout       int

	nc     *nats.Conn
	js     nats.JetStreamContext
	engine runtime.Engine
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				if ipNet.IP.To4() != nil {
					return ipNet.IP.String()
				}
			}
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				return ipNet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

func NewAgent(projectID, natsURL, runtimePref, workerID string, maxTasks int, once bool, idleTimeout int) *Agent {
	if workerID == "" {
		ip := getLocalIP()
		if ip == "" {
			ip = "127.0.0.1"
		}
		randBytes := make([]byte, 4)
		_, _ = rand.Read(randBytes)
		workerID = fmt.Sprintf("%s-%s", ip, hex.EncodeToString(randBytes))
	}

	return &Agent{
		ProjectID:         projectID,
		NatsURL:           natsURL,
		RuntimePreference: runtimePref,
		WorkerID:          workerID,
		MaxTasks:          maxTasks,
		Once:              once,
		IdleTimeout:       idleTimeout,
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
	lastWorkTime := time.Now()

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

		if a.IdleTimeout > 0 && time.Since(lastWorkTime) >= time.Duration(a.IdleTimeout)*time.Second {
			fmt.Printf("🏁 Idle timeout reached (%ds without receiving work). Exiting.\n", a.IdleTimeout)
			return nil
		}

		fetchWait := 2 * time.Second
		if a.IdleTimeout > 0 {
			remaining := time.Duration(a.IdleTimeout)*time.Second - time.Since(lastWorkTime)
			if remaining <= 0 {
				fmt.Printf("🏁 Idle timeout reached (%ds without receiving work). Exiting.\n", a.IdleTimeout)
				return nil
			}
			if remaining < fetchWait {
				fetchWait = remaining
			}
		}

		// Fetch 1 task message from JetStream WorkQueue
		msgs, err := sub.Fetch(1, nats.MaxWait(fetchWait))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				if a.Once && pulledAny {
					fmt.Println("🏁 WorkQueue empty (--once specified). Exiting.")
					return nil
				}
				if a.IdleTimeout > 0 && time.Since(lastWorkTime) >= time.Duration(a.IdleTimeout)*time.Second {
					fmt.Printf("🏁 Idle timeout reached (%ds without receiving work). Exiting.\n", a.IdleTimeout)
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
		lastWorkTime = time.Now()

		if err := a.processTaskMessage(ctx, msg); err != nil {
			fmt.Printf("❌ [Worker %s] Task processing error: %v\n", a.WorkerID, err)
		} else {
			tasksProcessed++
		}
		lastWorkTime = time.Now()
	}
}

func (a *Agent) processTaskMessage(ctx context.Context, msg *nats.Msg) error {
	var task protocol.Task
	if err := json.Unmarshal(msg.Data, &task); err != nil {
		_ = msg.Ack()
		return fmt.Errorf("unmarshal task JSON failed: %w", err)
	}

	taskID := task.TaskID
	if taskID == "" {
		taskID = "unknown"
	}

	image := task.Image
	if image == "" {
		// Fallback to project ID tag if not present in task
		image = fmt.Sprintf("%s:latest", a.ProjectID)
	}

	startTime := time.Now()
	fmt.Printf("📥 [Worker %s] Fetched task %s (image: %s)\n", a.WorkerID, taskID, image)

	// Ensure container image is available
	if err := a.engine.PullImage(ctx, image); err != nil {
		fmt.Printf("⚠️  [Worker %s] Image pull warning: %v\n", a.WorkerID, err)
	}

	// Execute container logic via stdin -> stdout
	stdoutBytes, execErr := a.engine.Execute(ctx, image, msg.Data)
	duration := time.Since(startTime)

	var taskResult protocol.TaskResult
	if execErr != nil {
		fmt.Printf("❌ [Worker %s] Task %s failed after %v: %v\n", a.WorkerID, taskID, duration, execErr)
		taskResult = protocol.TaskResult{
			TaskID:    taskID,
			ProjectID: a.ProjectID,
			Status:    "error",
			Error:     execErr.Error(),
			WorkerID:  a.WorkerID,
		}
	} else {
		// Try parsing stdout as standard TaskResult JSON
		if err := json.Unmarshal(stdoutBytes, &taskResult); err != nil {
			// If stdout is arbitrary JSON or raw output, wrap it
			taskResult = protocol.TaskResult{
				TaskID:    taskID,
				ProjectID: a.ProjectID,
				Status:    "ok",
				Output:    json.RawMessage(stdoutBytes),
				WorkerID:  a.WorkerID,
			}
		} else {
			if taskResult.TaskID == "" {
				taskResult.TaskID = taskID
			}
			taskResult.ProjectID = a.ProjectID
			taskResult.WorkerID = a.WorkerID
			if taskResult.Status == "" {
				taskResult.Status = "ok"
			}
		}
		fmt.Printf("📤 [Worker %s] Task %s completed in %v. Publishing result...\n", a.WorkerID, taskID, duration)
	}

	// Publish result to results.<project_id>
	resBytes, err := json.Marshal(taskResult)
	if err != nil {
		return fmt.Errorf("failed to marshal task result: %w", err)
	}

	resultsSubj := transport.ResultsSubject(a.ProjectID)
	if err := a.nc.Publish(resultsSubj, resBytes); err != nil {
		return fmt.Errorf("failed to publish result to %s: %w", resultsSubj, err)
	}

	// JetStream ACK - deletes message from WorkQueue
	if err := msg.Ack(); err != nil {
		return fmt.Errorf("failed to ACK JetStream task message: %w", err)
	}

	return nil
}
