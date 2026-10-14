package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"speedboat/internal/protocol"
	"speedboat/internal/transport"

	"github.com/nats-io/nats.go"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

type WorkerMetrics struct {
	TasksCompleted   int64 `json:"tasks_completed"`
	TotalComputeMs   int64 `json:"total_compute_ms"`
	ActiveThreads    int   `json:"active_threads"`
	AverageLatencyMs int64 `json:"average_latency_ms"`
}

type WorkerCallbacks struct {
	OnLog     func(level, message string)
	OnStatus  func(state, text string)
	OnMetrics func(metrics WorkerMetrics)
}

type WorkerPool struct {
	projectID  string
	natsURL    string
	workerBase string
	ip         string
	wasmBytes  []byte
	callbacks  WorkerCallbacks

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.Mutex
	isPaused  bool
	targetCap int
	activeCap int

	nc       *nats.Conn
	wRuntime wazero.Runtime
	compiled wazero.CompiledModule

	tasksCompleted atomic.Int64
	totalComputeMs atomic.Int64
}

func NewWorkerPool(
	projectID, natsURL, workerBase, ip string,
	wasmBytes []byte,
	concurrency int,
	callbacks WorkerCallbacks,
) *WorkerPool {
	if concurrency < 1 {
		concurrency = 1
	}
	return &WorkerPool{
		projectID:  projectID,
		natsURL:    natsURL,
		workerBase: workerBase,
		ip:         ip,
		wasmBytes:  wasmBytes,
		targetCap:  concurrency,
		callbacks:  callbacks,
	}
}

func (p *WorkerPool) Start(parentCtx context.Context) error {
	p.ctx, p.cancel = context.WithCancel(parentCtx)

	p.log("info", fmt.Sprintf("🌐 Connecting to NATS cluster at %s...", p.natsURL))
	nc, err := nats.Connect(p.natsURL, nats.Timeout(5*time.Second))
	if err != nil {
		return fmt.Errorf("failed connecting to NATS (%s): %w", p.natsURL, err)
	}
	p.nc = nc
	p.log("success", "✓ Connected to NATS cluster")

	p.log("info", fmt.Sprintf("⚙️ Compiling WASM module in-memory with wazero JIT (%d KB)...", len(p.wasmBytes)/1024))
	r := wazero.NewRuntime(p.ctx)
	p.wRuntime = r
	wasi_snapshot_preview1.MustInstantiate(p.ctx, r)

	compiled, err := r.CompileModule(p.ctx, p.wasmBytes)
	if err != nil {
		_ = r.Close(p.ctx)
		nc.Close()
		return fmt.Errorf("wazero JIT compilation failed: %w", err)
	}
	p.compiled = compiled
	p.log("success", "✓ WASM module JIT compiled to native code")

	p.setStatus("idle", fmt.Sprintf("Running with %d worker thread(s)", p.targetCap))

	p.mu.Lock()
	for i := 1; i <= p.targetCap; i++ {
		p.activeCap++
		p.wg.Add(1)
		go p.workerLoop(i)
	}
	p.mu.Unlock()

	p.emitMetrics()
	return nil
}

func (p *WorkerPool) Pause() {
	p.mu.Lock()
	p.isPaused = true
	p.mu.Unlock()
	p.log("warn", "⏸️ Computation paused")
	p.setStatus("paused", "Paused")
}

func (p *WorkerPool) Resume() {
	p.mu.Lock()
	p.isPaused = false
	p.mu.Unlock()
	p.log("info", "▶️ Computation resumed")
	p.setStatus("idle", "Waiting for tasks")
}

func (p *WorkerPool) SetConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	old := p.targetCap
	p.targetCap = n

	if n > old {
		// Scale up
		diff := n - old
		for i := 0; i < diff; i++ {
			p.activeCap++
			workerNum := old + i + 1
			p.wg.Add(1)
			go p.workerLoop(workerNum)
		}
		p.log("info", fmt.Sprintf("⚡ Scaled worker threads up to %d", n))
	} else if n < old {
		p.log("info", fmt.Sprintf("📉 Scaling worker threads down to %d (current active will finish tasks)", n))
	}

	p.emitMetrics()
}

func (p *WorkerPool) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()

	if p.compiled != nil {
		_ = p.compiled.Close(context.Background())
	}
	if p.wRuntime != nil {
		_ = p.wRuntime.Close(context.Background())
	}
	if p.nc != nil {
		p.nc.Close()
	}

	p.setStatus("idle", "Stopped")
	p.log("info", "🛑 All workers stopped")
}

func (p *WorkerPool) workerLoop(workerNum int) {
	defer p.wg.Done()
	defer func() {
		p.mu.Lock()
		p.activeCap--
		p.mu.Unlock()
		p.emitMetrics()
	}()

	workerID := fmt.Sprintf("%s-w%d", p.workerBase, workerNum)
	reqSubj := transport.TaskRequestSubject(p.projectID)
	lastCompletedTaskID := ""

	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		p.mu.Lock()
		paused := p.isPaused
		target := p.targetCap
		p.mu.Unlock()

		if paused {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// Check if we should scale down
		if workerNum > target {
			return
		}

		req := protocol.TaskRequest{
			ProjectID:       p.projectID,
			WorkerID:        workerID,
			IP:              p.ip,
			CompletedTaskID: lastCompletedTaskID,
		}
		reqBytes, _ := json.Marshal(req)

		respMsg, err := p.nc.Request(reqSubj, reqBytes, 2*time.Second)
		if err != nil {
			time.Sleep(400 * time.Millisecond)
			continue
		}

		var resp protocol.TaskResponse
		if err := json.Unmarshal(respMsg.Data, &resp); err != nil {
			time.Sleep(400 * time.Millisecond)
			continue
		}

		if resp.Status == "completed" {
			p.log("success", "🏁 Project completed! All tasks finished.")
			p.setStatus("completed", "Project Completed")
			return
		}

		if resp.Status == "blacklisted" {
			p.log("err", "🚫 Worker blacklisted by master.")
			p.setStatus("paused", "Blacklisted")
			return
		}

		if resp.Status == "no_tasks" {
			time.Sleep(1 * time.Second)
			continue
		}

		if resp.Status == "ok" && len(resp.Task) > 0 {
			var task protocol.Task
			if err := json.Unmarshal(resp.Task, &task); err != nil {
				continue
			}

			taskID := task.TaskID
			if taskID == "" {
				taskID = "task"
			}

			p.setStatus("computing", fmt.Sprintf("Thread %d: Crunching %s...", workerNum, taskID))
			start := time.Now()

			var stdout, stderr bytes.Buffer
			modConfig := wazero.NewModuleConfig().
				WithName("").
				WithStdin(bytes.NewReader(resp.Task)).
				WithStdout(&stdout).
				WithStderr(&stderr).
				WithArgs("wasi_app")

			mod, execErr := p.wRuntime.InstantiateModule(p.ctx, p.compiled, modConfig)
			if execErr != nil {
				if exitErr, ok := execErr.(*sys.ExitError); ok && exitErr.ExitCode() == 0 {
					execErr = nil
				}
			} else {
				_ = mod.Close(p.ctx)
			}

			duration := time.Since(start)
			durationMs := duration.Milliseconds()

			var taskResult protocol.TaskResult
			if execErr != nil {
				p.log("err", fmt.Sprintf("❌ [%s] Task %s failed: %v", workerID, taskID, execErr))
				taskResult = protocol.TaskResult{
					TaskID:    taskID,
					ProjectID: p.projectID,
					Status:    "error",
					Error:     execErr.Error(),
					WorkerID:  workerID,
					IP:        p.ip,
				}
			} else {
				// Parse stdout
				if err := json.Unmarshal(stdout.Bytes(), &taskResult); err != nil {
					taskResult = protocol.TaskResult{
						TaskID:    taskID,
						ProjectID: p.projectID,
						Status:    "ok",
						Output:    json.RawMessage(stdout.Bytes()),
						WorkerID:  workerID,
						IP:        p.ip,
					}
				} else {
					if taskResult.TaskID == "" {
						taskResult.TaskID = taskID
					}
					taskResult.ProjectID = p.projectID
					taskResult.WorkerID = workerID
					taskResult.IP = p.ip
					if taskResult.Status == "" {
						taskResult.Status = "ok"
					}
				}
				p.log("success", fmt.Sprintf("📤 [%s] Task %s completed in %dms. Publishing result...", workerID, taskID, durationMs))
			}

			resBytes, err := json.Marshal(taskResult)
			if err == nil {
				_ = p.nc.Publish(transport.ResultsSubject(p.projectID), resBytes)
				_ = p.nc.Flush()
			}

			if execErr == nil {
				lastCompletedTaskID = taskID
				p.tasksCompleted.Add(1)
				p.totalComputeMs.Add(durationMs)
				p.emitMetrics()
			}
			p.setStatus("idle", fmt.Sprintf("Running with %d worker thread(s)", p.targetCap))
		}
	}
}

func (p *WorkerPool) emitMetrics() {
	completed := p.tasksCompleted.Load()
	totalMs := p.totalComputeMs.Load()
	avg := int64(0)
	if completed > 0 {
		avg = totalMs / completed
	}

	p.mu.Lock()
	active := p.activeCap
	p.mu.Unlock()

	if p.callbacks.OnMetrics != nil {
		p.callbacks.OnMetrics(WorkerMetrics{
			TasksCompleted:   completed,
			TotalComputeMs:   totalMs,
			ActiveThreads:    active,
			AverageLatencyMs: avg,
		})
	}
}

func (p *WorkerPool) log(level, msg string) {
	if p.callbacks.OnLog != nil {
		p.callbacks.OnLog(level, msg)
	}
}

func (p *WorkerPool) setStatus(state, text string) {
	if p.callbacks.OnStatus != nil {
		p.callbacks.OnStatus(state, text)
	}
}
