//go:build windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	speedboatRuntime "speedboat/internal/runtime"
)

type windowsHeavyEngineManager struct{}

func NewHeavyEngineManager() HeavyEngineManager {
	return &windowsHeavyEngineManager{}
}

func findWSLBinary() string {
	candidates := []string{"wsl.exe", "wsl"}
	for _, name := range candidates {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}

	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	direct := filepath.Join(sysRoot, "System32", "wsl.exe")
	if fi, err := os.Stat(direct); err == nil && !fi.IsDir() {
		return direct
	}
	return ""
}

func findDockerBinary() (string, string) {
	for _, name := range []string{"docker.exe", "docker"} {
		if p, err := exec.LookPath(name); err == nil {
			out, err := exec.Command(p, "version", "--format", "{{.Server.Version}}").Output()
			if err == nil {
				return p, strings.TrimSpace(speedboatRuntime.DecodeWindowsOutput(out))
			}
			outSimple, errSimple := exec.Command(p, "--version").Output()
			if errSimple == nil {
				return p, strings.TrimSpace(speedboatRuntime.DecodeWindowsOutput(outSimple))
			}
			return p, ""
		}
	}
	return "", ""
}

func findPodmanBinary() (string, string) {
	for _, name := range []string{"podman.exe", "podman"} {
		if p, err := exec.LookPath(name); err == nil {
			out, err := exec.Command(p, "--version").Output()
			if err == nil {
				return p, strings.TrimSpace(speedboatRuntime.DecodeWindowsOutput(out))
			}
			return p, ""
		}
	}
	return "", ""
}

func isDistroInstalled(wslBin, distro string) bool {
	if wslBin == "" {
		return false
	}
	out, err := exec.Command(wslBin, "-l", "-q").Output()
	if err != nil {
		return false
	}
	decoded := speedboatRuntime.DecodeWindowsOutput(out)
	for _, line := range strings.Split(decoded, "\n") {
		clean := strings.TrimSpace(line)
		clean = strings.Trim(clean, "\r\x00")
		if strings.EqualFold(clean, distro) {
			return true
		}
	}
	return false
}

func isDistroReady(wslBin, distro string) (bool, string) {
	if !isDistroInstalled(wslBin, distro) {
		return false, ""
	}
	out, err := exec.Command(wslBin, "-d", distro, "-u", "root", "--", "podman", "--version").Output()
	if err == nil {
		return true, strings.TrimSpace(speedboatRuntime.DecodeWindowsOutput(out))
	}
	// Fallback check: crun
	outCrun, errCrun := exec.Command(wslBin, "-d", distro, "-u", "root", "--", "crun", "--version").Output()
	if errCrun == nil {
		firstLine := strings.Split(strings.TrimSpace(speedboatRuntime.DecodeWindowsOutput(outCrun)), "\n")[0]
		return true, firstLine
	}
	return false, ""
}

func ensureWSLConfig() error {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	cfgPath := filepath.Join(home, ".wslconfig")

	content, err := os.ReadFile(cfgPath)
	if os.IsNotExist(err) {
		defaultCfg := "[wsl2]\nmemory=4GB\nprocessors=4\nswap=2GB\n"
		return os.WriteFile(cfgPath, []byte(defaultCfg), 0644)
	} else if err == nil {
		str := string(content)
		if !strings.Contains(strings.ToLower(str), "memory=") {
			// Append memory limit to avoid host starvation
			newCfg := strings.TrimSpace(str) + "\n\n[wsl2]\nmemory=4GB\nprocessors=4\n"
			return os.WriteFile(cfgPath, []byte(newCfg), 0644)
		}
	}
	return nil
}

func (m *windowsHeavyEngineManager) GetCapabilities() EngineCapabilities {
	hasGPU, gpuVendor := speedboatRuntime.DetectHostGPU()

	// 1. Check native Docker Desktop
	dockerBin, dockerVer := findDockerBinary()
	if dockerBin != "" {
		cmd := exec.Command(dockerBin, "info")
		if cmd.Run() == nil {
			return EngineCapabilities{
				Platform:           "windows",
				HeavyModeSupported: true,
				EngineName:         "Docker",
				IsInstalled:        true,
				BinaryPath:         dockerBin,
				Version:            dockerVer,
				CanAutoInstall:     false,
				GPUAvailable:       hasGPU,
				GPUVendor:          gpuVendor,
				StatusMessage:      fmt.Sprintf("Docker Desktop ready (%s)", dockerVer),
			}
		}
	}

	// 2. Check native Podman
	podmanBin, podmanVer := findPodmanBinary()
	if podmanBin != "" {
		cmd := exec.Command(podmanBin, "info")
		if cmd.Run() == nil {
			return EngineCapabilities{
				Platform:           "windows",
				HeavyModeSupported: true,
				EngineName:         "Podman",
				IsInstalled:        true,
				BinaryPath:         podmanBin,
				Version:            podmanVer,
				CanAutoInstall:     false,
				GPUAvailable:       hasGPU,
				GPUVendor:          gpuVendor,
				StatusMessage:      fmt.Sprintf("Podman ready (%s)", podmanVer),
			}
		}
	}

	// 3. Check WSL2 with SpeedboatWorker
	wslBin := findWSLBinary()
	if wslBin != "" {
		if ready, ver := isDistroReady(wslBin, "SpeedboatWorker"); ready {
			return EngineCapabilities{
				Platform:           "windows",
				HeavyModeSupported: true,
				EngineName:         "WSL2 (SpeedboatWorker)",
				IsInstalled:        true,
				BinaryPath:         wslBin,
				Version:            ver,
				CanAutoInstall:     false,
				GPUAvailable:       hasGPU,
				GPUVendor:          gpuVendor,
				StatusMessage:      fmt.Sprintf("WSL2 SpeedboatWorker ready (%s)", ver),
			}
		}

		// WSL2 binary is available, but SpeedboatWorker distribution is not yet provisioned
		return EngineCapabilities{
			Platform:           "windows",
			HeavyModeSupported: true,
			EngineName:         "WSL2 (SpeedboatWorker)",
			IsInstalled:        false,
			BinaryPath:         wslBin,
			Version:            "",
			CanAutoInstall:     true,
			GPUAvailable:       hasGPU,
			GPUVendor:          gpuVendor,
			StatusMessage:      "WSL2 container environment not yet provisioned. Click below for 1-click automated setup.",
		}
	}

	// WSL2 not detected
	return EngineCapabilities{
		Platform:           "windows",
		HeavyModeSupported: true,
		EngineName:         "WSL2 / Docker",
		IsInstalled:        false,
		BinaryPath:         "",
		Version:            "",
		CanAutoInstall:     false,
		GPUAvailable:       hasGPU,
		GPUVendor:          gpuVendor,
		StatusMessage:      "Windows Subsystem for Linux (WSL2) or Docker Desktop required for Heavy Mode.",
	}
}

func (m *windowsHeavyEngineManager) Install(ctx context.Context, onProgress ProgressCallback) error {
	wslBin := findWSLBinary()
	if wslBin == "" {
		return fmt.Errorf("wsl.exe was not found. Please enable Windows Subsystem for Linux via 'Turn Windows features on or off'")
	}

	if onProgress != nil {
		onProgress(5, "Checking Windows Subsystem for Linux (WSL2) status...")
	}

	// 1. Verify WSL status
	statusCmd := exec.CommandContext(ctx, wslBin, "--status")
	statusOut, statusErr := statusCmd.CombinedOutput()
	decodedStatus := strings.ToLower(speedboatRuntime.DecodeWindowsOutput(statusOut))

	if statusErr != nil {
		if strings.Contains(decodedStatus, "virtual") || strings.Contains(decodedStatus, "bios") {
			return fmt.Errorf("hardware virtualization (VT-x / AMD-V) is disabled in BIOS/firmware. Please enable it in BIOS or continue in Lite Mode (Wazero)")
		}
		// Attempt to run wsl --install --no-distribution
		if onProgress != nil {
			onProgress(10, "Attempting to enable WSL kernel components...")
		}
		instCmd := exec.CommandContext(ctx, wslBin, "--install", "--no-distribution")
		_ = instCmd.Run()
	}

	// 2. Configure .wslconfig
	if onProgress != nil {
		onProgress(18, "Configuring host resource limits in .wslconfig (4GB memory cap)...")
	}
	_ = ensureWSLConfig()

	// 3. Download minimal Alpine rootfs (~3.3 MB)
	alpineURL := os.Getenv("SPEEDBOAT_ALPINE_URL")
	if alpineURL == "" {
		alpineURL = "https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/x86_64/alpine-minirootfs-3.20.3-x86_64.tar.gz"
	}

	if onProgress != nil {
		onProgress(25, "Downloading minimal Alpine Linux rootfs (~3.3 MB)...")
	}

	tmpTar := filepath.Join(os.TempDir(), "speedboat-alpine-minirootfs.tar.gz")
	defer os.Remove(tmpTar)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, alpineURL, nil)
	if err != nil {
		return fmt.Errorf("invalid download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Fallback mirror
		fallbackURL := "https://dl-cdn.alpinelinux.org/alpine/v3.19/releases/x86_64/alpine-minirootfs-3.19.1-x86_64.tar.gz"
		reqFb, fbErr := http.NewRequestWithContext(ctx, http.MethodGet, fallbackURL, nil)
		if fbErr == nil {
			resp, err = http.DefaultClient.Do(reqFb)
		}
	}
	if err != nil {
		return fmt.Errorf("failed to download Alpine rootfs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP %d while downloading Alpine rootfs", resp.StatusCode)
	}

	tarFile, err := os.Create(tmpTar)
	if err != nil {
		return fmt.Errorf("failed to create temporary file for rootfs: %w", err)
	}

	totalSize := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			tarFile.Close()
			return ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := tarFile.Write(buf[:n]); writeErr != nil {
				tarFile.Close()
				return writeErr
			}
			downloaded += int64(n)
			if totalSize > 0 && onProgress != nil {
				pct := 25.0 + (float64(downloaded)/float64(totalSize))*40.0
				onProgress(pct, fmt.Sprintf("Downloading Alpine rootfs... (%.1f MB / %.1f MB)",
					float64(downloaded)/(1024*1024), float64(totalSize)/(1024*1024)))
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			tarFile.Close()
			return readErr
		}
	}
	tarFile.Close()

	// 4. Prepare WSL target directory
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, _ := os.UserHomeDir()
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	wslDir := filepath.Join(localAppData, "Speedboat", "wsl")
	if err := os.MkdirAll(wslDir, 0755); err != nil {
		return fmt.Errorf("failed to create WSL directory (%s): %w", wslDir, err)
	}

	// 5. Unregister previous instance if present
	if isDistroInstalled(wslBin, "SpeedboatWorker") {
		_ = exec.CommandContext(ctx, wslBin, "--unregister", "SpeedboatWorker").Run()
	}

	// 6. Import into WSL2
	if onProgress != nil {
		onProgress(70, "Importing SpeedboatWorker distribution into WSL2...")
	}

	importCmd := exec.CommandContext(ctx, wslBin, "--import", "SpeedboatWorker", wslDir, tmpTar, "--version", "2")
	var importStderr bytes.Buffer
	importCmd.Stderr = &importStderr
	if err := importCmd.Run(); err != nil {
		// Retry without --version 2 if default version is already 2
		importCmdFallback := exec.CommandContext(ctx, wslBin, "--import", "SpeedboatWorker", wslDir, tmpTar)
		if fbErr := importCmdFallback.Run(); fbErr != nil {
			return fmt.Errorf("wsl --import failed: %w (stderr: %s)",
				err, strings.TrimSpace(speedboatRuntime.DecodeWindowsOutput(importStderr.Bytes())))
		}
	}

	// 7. Install podman, crun, ca-certificates, curl inside Alpine WSL
	if onProgress != nil {
		onProgress(80, "Installing container packages (podman, crun) inside WSL2...")
	}

	_ = exec.CommandContext(ctx, wslBin, "-d", "SpeedboatWorker", "-u", "root", "--", "apk", "update").Run()

	addCmd := exec.CommandContext(ctx, wslBin, "-d", "SpeedboatWorker", "-u", "root", "--",
		"apk", "add", "--no-cache", "podman", "crun", "ca-certificates", "curl")
	var addStderr bytes.Buffer
	addCmd.Stderr = &addStderr
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("failed installing container tools in WSL2: %w (stderr: %s)",
			err, strings.TrimSpace(speedboatRuntime.DecodeWindowsOutput(addStderr.Bytes())))
	}

	// 8. Configure container engine inside WSL (cgroupfs for rootless/systemless container runtimes)
	if onProgress != nil {
		onProgress(92, "Configuring container engine inside WSL2...")
	}
	cfgCmd := exec.CommandContext(ctx, wslBin, "-d", "SpeedboatWorker", "-u", "root", "--",
		"sh", "-c", "mkdir -p /etc/containers && printf '[engine]\\ncgroup_manager = \"cgroupfs\"\\nevents_logger = \"file\"\\n' > /etc/containers/containers.conf")
	_ = cfgCmd.Run()

	// 9. Verify installation
	if onProgress != nil {
		onProgress(95, "Verifying WSL container engine...")
	}

	verCmd := exec.CommandContext(ctx, wslBin, "-d", "SpeedboatWorker", "-u", "root", "--", "podman", "--version")
	outVer, err := verCmd.Output()
	if err != nil {
		return fmt.Errorf("container runtime verification in WSL failed: %w", err)
	}

	verStr := strings.TrimSpace(speedboatRuntime.DecodeWindowsOutput(outVer))
	if onProgress != nil {
		onProgress(100, fmt.Sprintf("WSL2 SpeedboatWorker ready (%s)!", verStr))
	}

	return nil
}

func (m *windowsHeavyEngineManager) CreateEngine(enableGPU bool) (speedboatRuntime.Engine, error) {
	// 1. Docker Desktop
	dockerBin, _ := findDockerBinary()
	if dockerBin != "" {
		if exec.Command(dockerBin, "info").Run() == nil {
			eng := speedboatRuntime.NewDockerEngineCustom(dockerBin)
			eng.SetGPUEnabled(enableGPU)
			return eng, nil
		}
	}

	// 2. Podman
	podmanBin, _ := findPodmanBinary()
	if podmanBin != "" {
		if exec.Command(podmanBin, "info").Run() == nil {
			eng := speedboatRuntime.NewDockerEngineCustom(podmanBin)
			eng.SetGPUEnabled(enableGPU)
			return eng, nil
		}
	}

	// 3. WSL2 SpeedboatWorker
	wslBin := findWSLBinary()
	if wslBin != "" && isDistroInstalled(wslBin, "SpeedboatWorker") {
		eng := speedboatRuntime.NewWSLEngineCustom("SpeedboatWorker", wslBin)
		eng.SetGPUEnabled(enableGPU)
		return eng, nil
	}

	return nil, fmt.Errorf("no container engine found on Windows; please click Install to provision WSL2 SpeedboatWorker, or launch Docker Desktop")
}
