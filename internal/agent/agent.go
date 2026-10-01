package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	IP                string
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
	ip := getLocalIP()
	if ip == "" {
		ip = "127.0.0.1"
	}
	if workerID == "" {
		randBytes := make([]byte, 4)
		_, _ = rand.Read(randBytes)
		workerID = fmt.Sprintf("%s-%s", ip, hex.EncodeToString(randBytes))
	}

	return &Agent{
		ProjectID:         projectID,
		NatsURL:           natsURL,
		RuntimePreference: runtimePref,
		WorkerID:          workerID,
		IP:                ip,
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

	tasksProcessed := 0
	lastWorkTime := time.Now()
	lastCompletedTaskID := ""
	reqSubj := transport.TaskRequestSubject(a.ProjectID)

	fmt.Printf("⚡ Ready! Requesting work on '%s' (Worker: %s, IP: %s)...\n", reqSubj, a.WorkerID, a.IP)

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

		reqWait := 2 * time.Second
		if a.IdleTimeout > 0 {
			remaining := time.Duration(a.IdleTimeout)*time.Second - time.Since(lastWorkTime)
			if remaining <= 0 {
				fmt.Printf("🏁 Idle timeout reached (%ds without receiving work). Exiting.\n", a.IdleTimeout)
				return nil
			}
			if remaining < reqWait {
				reqWait = remaining
			}
		}

		req := protocol.TaskRequest{
			ProjectID:       a.ProjectID,
			WorkerID:        a.WorkerID,
			IP:              a.IP,
			CompletedTaskID: lastCompletedTaskID,
		}
		reqBytes, _ := json.Marshal(req)

		respMsg, err := nc.Request(reqSubj, reqBytes, reqWait)
		if err != nil {
			if a.IdleTimeout > 0 && time.Since(lastWorkTime) >= time.Duration(a.IdleTimeout)*time.Second {
				fmt.Printf("🏁 Idle timeout reached (%ds without receiving work). Exiting.\n", a.IdleTimeout)
				return nil
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}

		var resp protocol.TaskResponse
		if err := json.Unmarshal(respMsg.Data, &resp); err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if resp.Status == "completed" {
			fmt.Println("🏁 Project completed. Exiting.")
			return nil
		}

		if resp.Status == "blacklisted" {
			fmt.Printf("🚫 [Worker %s] Worker IP %s has been blacklisted by the master. Exiting.\n", a.WorkerID, a.IP)
			return nil
		}

		if resp.Status == "no_tasks" {
			if a.IdleTimeout > 0 && time.Since(lastWorkTime) >= time.Duration(a.IdleTimeout)*time.Second {
				fmt.Printf("🏁 Idle timeout reached (%ds without receiving work). Exiting.\n", a.IdleTimeout)
				return nil
			}
			time.Sleep(1 * time.Second)
			continue
		}

		if resp.Status == "ok" && len(resp.Task) > 0 {
			lastWorkTime = time.Now()
			taskID, err := a.processTaskData(ctx, resp.Task)
			if err != nil {
				fmt.Printf("❌ [Worker %s] Task processing error: %v\n", a.WorkerID, err)
			} else {
				tasksProcessed++
				lastCompletedTaskID = taskID
			}
			lastWorkTime = time.Now()
		}
	}
}

func (a *Agent) processTaskMessage(ctx context.Context, msg *nats.Msg) error {
	defer msg.Ack()
	_, err := a.processTaskData(ctx, msg.Data)
	return err
}

func (a *Agent) processTaskData(ctx context.Context, data []byte) (string, error) {
	var task protocol.Task
	if err := json.Unmarshal(data, &task); err != nil {
		return "", fmt.Errorf("unmarshal task JSON failed: %w", err)
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
	stdoutBytes, execErr := a.engine.Execute(ctx, image, data)
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
			IP:        a.IP,
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
				IP:        a.IP,
			}
		} else {
			if taskResult.TaskID == "" {
				taskResult.TaskID = taskID
			}
			taskResult.ProjectID = a.ProjectID
			taskResult.WorkerID = a.WorkerID
			taskResult.IP = a.IP
			if taskResult.Status == "" {
				taskResult.Status = "ok"
			}
		}
		fmt.Printf("📤 [Worker %s] Task %s completed in %v. Publishing result...\n", a.WorkerID, taskID, duration)
	}

	// Publish result to results.<project_id>
	resBytes, err := json.Marshal(taskResult)
	if err != nil {
		return taskID, fmt.Errorf("failed to marshal task result: %w", err)
	}

	resultsSubj := transport.ResultsSubject(a.ProjectID)
	if err := a.nc.Publish(resultsSubj, resBytes); err != nil {
		return taskID, fmt.Errorf("failed to publish result to %s: %w", resultsSubj, err)
	}
	_ = a.nc.Flush()

	return taskID, nil
}
