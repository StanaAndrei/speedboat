package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	"speedboat/internal/cache"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type SystemInfo struct {
	NumCPU         int    `json:"num_cpu"`
	DefaultThreads int    `json:"default_threads"`
	OS             string `json:"os"`
	WorkerID       string `json:"worker_id"`
	IP             string `json:"ip"`
}

type StartConfig struct {
	ProjectID   string `json:"project_id"`
	NatsURL     string `json:"nats_url"`
	PayloadURL  string `json:"payload_url"`
	Concurrency int    `json:"concurrency"`
}

type App struct {
	ctx      context.Context
	workerID string
	ip       string
	cache    *cache.WasmCache
	pool     *WorkerPool
	mu       sync.Mutex
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

func NewApp() *App {
	ip := getLocalIP()
	randBytes := make([]byte, 4)
	_, _ = rand.Read(randBytes)
	workerID := fmt.Sprintf("%s-%s", ip, hex.EncodeToString(randBytes))

	wasmCache, _ := cache.NewWasmCache()

	return &App{
		ip:       ip,
		workerID: workerID,
		cache:    wasmCache,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pool != nil {
		a.pool.Stop()
		a.pool = nil
	}
}

func (a *App) GetSystemInfo() SystemInfo {
	numCPU := runtime.NumCPU()
	defThreads := numCPU / 2
	if defThreads < 1 {
		defThreads = 1
	}

	return SystemInfo{
		NumCPU:         numCPU,
		DefaultThreads: defThreads,
		OS:             runtime.GOOS,
		WorkerID:       a.workerID,
		IP:             a.ip,
	}
}

func (a *App) ResolveProjectURL(urlStr string) (*ProjectMetadata, error) {
	return ResolveProjectURL(a.ctx, urlStr)
}

func (a *App) StartCompute(cfg StartConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.pool != nil {
		a.pool.Stop()
		a.pool = nil
	}

	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}

	a.emitLog("info", fmt.Sprintf("📦 Resolving WASM payload for project '%s'...", cfg.ProjectID))
	a.emitStatus("connecting", "Downloading payload...")

	wasmBytes, cachePath, err := a.cache.GetOrFetch(a.ctx, cfg.ProjectID, cfg.PayloadURL)
	if err != nil {
		a.emitLog("err", fmt.Sprintf("❌ Failed to obtain WASM payload: %v", err))
		a.emitStatus("paused", "Payload Error")
		return fmt.Errorf("failed to fetch wasm payload: %w", err)
	}

	a.emitLog("success", fmt.Sprintf("✓ Payload ready (cached at %s, size %d KB)", cachePath, len(wasmBytes)/1024))

	callbacks := WorkerCallbacks{
		OnLog: func(level, message string) {
			a.emitLog(level, message)
		},
		OnStatus: func(state, text string) {
			a.emitStatus(state, text)
		},
		OnMetrics: func(m WorkerMetrics) {
			a.emitMetrics(m)
		},
	}

	pool := NewWorkerPool(
		cfg.ProjectID,
		cfg.NatsURL,
		a.workerID,
		a.ip,
		wasmBytes,
		cfg.Concurrency,
		callbacks,
	)

	if err := pool.Start(a.ctx); err != nil {
		a.emitLog("err", fmt.Sprintf("❌ Failed to start worker pool: %v", err))
		a.emitStatus("paused", "Connection Error")
		return err
	}

	a.pool = pool
	return nil
}

func (a *App) PauseCompute() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pool != nil {
		a.pool.Pause()
	}
	return nil
}

func (a *App) ResumeCompute() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pool != nil {
		a.pool.Resume()
	}
	return nil
}

func (a *App) SetConcurrency(threads int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pool != nil {
		a.pool.SetConcurrency(threads)
	}
	return nil
}

func (a *App) StopCompute() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pool != nil {
		a.pool.Stop()
		a.pool = nil
	}
	a.emitStatus("idle", "Idle")
	a.emitLog("info", "Computation stopped by user")
	return nil
}

func (a *App) emitLog(level, msg string) {
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "log", map[string]string{
			"level": level,
			"msg":   msg,
			"time":  time.Now().Format("15:04:05"),
		})
	}
}

func (a *App) emitStatus(state, text string) {
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "status", map[string]string{
			"state": state,
			"text":  text,
		})
	}
}

func (a *App) emitMetrics(m WorkerMetrics) {
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "metrics", m)
	}
}
