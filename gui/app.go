package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"speedboat/internal/cache"
	speedboatRuntime "speedboat/internal/runtime"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type SystemInfo struct {
	NumCPU         int                `json:"num_cpu"`
	DefaultThreads int                `json:"default_threads"`
	OS             string             `json:"os"`
	WorkerID       string             `json:"worker_id"`
	IP             string             `json:"ip"`
	EngineCaps     EngineCapabilities `json:"engine_caps"`
}

type StartConfig struct {
	ProjectID   string `json:"project_id"`
	NatsURL     string `json:"nats_url"`
	PayloadURL  string `json:"payload_url"`
	Concurrency int    `json:"concurrency"`
	ComputeMode string `json:"compute_mode"` // "wasm" or "apptainer"
	EnableGPU   bool   `json:"enable_gpu"`
}

type App struct {
	ctx      context.Context
	workerID string
	ip       string
	cache    *cache.WasmCache
	heavyMgr HeavyEngineManager
	pool     *WorkerPool
	trayMgr  *TrayManager
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
	heavyMgr := NewHeavyEngineManager()

	app := &App{
		ip:       ip,
		workerID: workerID,
		cache:    wasmCache,
		heavyMgr: heavyMgr,
	}
	app.trayMgr = NewTrayManager(app)
	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if a.trayMgr != nil {
		a.trayMgr.Start(ctx)
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	if a.pool != nil {
		a.pool.Stop()
		a.pool = nil
	}
	a.mu.Unlock()

	if a.trayMgr != nil {
		a.trayMgr.Shutdown()
	}
}

func (a *App) GetSystemInfo() SystemInfo {
	numCPU := runtime.NumCPU()
	defThreads := numCPU / 2
	if defThreads < 1 {
		defThreads = 1
	}

	caps := a.heavyMgr.GetCapabilities()

	return SystemInfo{
		NumCPU:         numCPU,
		DefaultThreads: defThreads,
		OS:             runtime.GOOS,
		WorkerID:       a.workerID,
		IP:             a.ip,
		EngineCaps:     caps,
	}
}

func (a *App) GetEngineCapabilities() EngineCapabilities {
	return a.heavyMgr.GetCapabilities()
}

func (a *App) InstallHeavyEngine() error {
	a.emitLog("info", "🔧 Starting container engine installation...")
	err := a.heavyMgr.Install(a.ctx, func(percent float64, message string) {
		if a.ctx != nil {
			wruntime.EventsEmit(a.ctx, "install_progress", map[string]interface{}{
				"percent": percent,
				"message": message,
			})
		}
		a.emitLog("info", fmt.Sprintf("[Install] %s", message))
	})
	if err != nil {
		a.emitLog("err", fmt.Sprintf("❌ Installation failed: %v", err))
		return err
	}
	a.emitLog("success", "✓ Container engine ready")
	return nil
}

func (a *App) ResolveProjectURL(urlStr string) (*ProjectMetadata, error) {
	return ResolveProjectURL(a.ctx, urlStr)
}

func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	// Fallback heuristic: strip protocol and take before first colon/slash
	clean := strings.TrimPrefix(rawURL, "http://")
	clean = strings.TrimPrefix(clean, "https://")
	clean = strings.TrimPrefix(clean, "ws://")
	clean = strings.TrimPrefix(clean, "wss://")
	parts := strings.Split(clean, "/")
	hostPort := parts[0]
	if strings.Contains(hostPort, ":") {
		host, _, _ := net.SplitHostPort(hostPort)
		return host
	}
	return hostPort
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
	if cfg.ComputeMode == "" {
		cfg.ComputeMode = "wasm"
	}

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

	masterHost := extractHost(cfg.NatsURL)
	if masterHost == "" {
		masterHost = extractHost(cfg.PayloadURL)
	}

	var wasmBytes []byte
	var containerEng speedboatRuntime.Engine

	if cfg.ComputeMode == "apptainer" {
		a.emitLog("info", "🚀 Initializing Heavy Mode (Apptainer)...")
		eng, err := a.heavyMgr.CreateEngine(cfg.EnableGPU)
		if err != nil {
			a.emitLog("err", fmt.Sprintf("❌ Container engine error: %v", err))
			a.emitStatus("paused", "Engine Error")
			return err
		}
		containerEng = eng
		if cfg.EnableGPU {
			a.emitLog("info", "⚡ GPU acceleration requested for container execution")
		}
	} else {
		// Lite Mode (WASM)
		a.emitLog("info", fmt.Sprintf("📦 Resolving WASM payload for project '%s'...", cfg.ProjectID))
		a.emitStatus("connecting", "Downloading payload...")

		bytesRead, cachePath, err := a.cache.GetOrFetch(a.ctx, cfg.ProjectID, cfg.PayloadURL)
		if err != nil {
			a.emitLog("err", fmt.Sprintf("❌ Failed to obtain WASM payload: %v", err))
			a.emitStatus("paused", "Payload Error")
			return fmt.Errorf("failed to fetch wasm payload: %w", err)
		}
		wasmBytes = bytesRead
		a.emitLog("success", fmt.Sprintf("✓ Payload ready (cached at %s, size %d KB)", cachePath, len(wasmBytes)/1024))
	}

	pool := NewWorkerPool(
		cfg.ProjectID,
		cfg.NatsURL,
		a.workerID,
		a.ip,
		cfg.ComputeMode,
		masterHost,
		wasmBytes,
		containerEng,
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

func (a *App) MinimizeToTray() {
	if a.trayMgr != nil {
		a.trayMgr.HideWindow()
	} else if a.ctx != nil {
		wruntime.WindowHide(a.ctx)
	}
}

func (a *App) RestoreFromTray() {
	if a.trayMgr != nil {
		a.trayMgr.ShowWindow()
	} else if a.ctx != nil {
		wruntime.WindowShow(a.ctx)
		wruntime.WindowUnminimise(a.ctx)
		wruntime.Show(a.ctx)
	}
}

func (a *App) emitStatus(state, text string) {
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "status", map[string]string{
			"state": state,
			"text":  text,
		})
	}
	if a.trayMgr != nil {
		a.trayMgr.UpdateStatus(state, text)
	}
}

func (a *App) emitMetrics(m WorkerMetrics) {
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "metrics", m)
	}
}
