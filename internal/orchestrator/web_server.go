package orchestrator

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"speedboat/internal/orchestrator/web"
)

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

// startVolunteerServer launches an embedded HTTP server serving the WebAssembly browser volunteer worker
// if WasmPath is specified. It returns a cleanup function to gracefully shut down the server.
func (o *Orchestrator) startVolunteerServer() (func(), error) {
	if o.WasmPath == "" {
		return func() {}, nil
	}

	absWasmPath := o.WasmPath
	if !filepath.IsAbs(absWasmPath) {
		if _, err := os.Stat(absWasmPath); err == nil {
			absWasmPath, _ = filepath.Abs(absWasmPath)
		} else {
			absWasmPath = filepath.Join(o.ProjectDir, absWasmPath)
		}
	}
	if fi, err := os.Stat(absWasmPath); err != nil || fi.IsDir() {
		return nil, fmt.Errorf("wasm payload not found at %s: %w", absWasmPath, err)
	}

	localIP := getLocalIP()
	natsWsURL := o.NatsWsURL
	if natsWsURL == "" {
		natsWsURL = fmt.Sprintf("ws://%s:8080", localIP)
	}

	webPort := o.WebPort
	if webPort <= 0 {
		webPort = 3000
	}

	webHandler := web.Handler(o.Config.Name, natsWsURL, absWasmPath)
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", webPort),
		Handler: webHandler,
	}
	o.httpServer = server

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("⚠️  [Web] Volunteer server error: %v\n", err)
		}
	}()

	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}

	volunteerURL := fmt.Sprintf("http://%s:%d/?project=%s&nats=%s", localIP, webPort, o.Config.Name, natsWsURL)
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("🌐 [WASM Web Worker] Volunteer page available at:\n")
	fmt.Printf("👉 %s\n", volunteerURL)
	fmt.Println("--------------------------------------------------------------------------------")

	return cleanup, nil
}
