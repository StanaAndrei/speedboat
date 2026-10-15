package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type ApptainerEngine struct {
	binaryPath string
	cacheDir   string
	GPUEnabled bool
	mu         sync.Mutex
}

func NewApptainerEngine() *ApptainerEngine {
	return NewApptainerEngineCustom("")
}

func NewApptainerEngineCustom(customBin string) *ApptainerEngine {
	bin := customBin
	if bin == "" {
		var err error
		bin, err = exec.LookPath("apptainer")
		if err != nil {
			bin, _ = exec.LookPath("singularity")
		}
	}

	cacheDir := os.Getenv("SPEEDBOAT_SIF_DIR")
	if cacheDir == "" {
		userCache, err := os.UserCacheDir()
		if err != nil || userCache == "" {
			cacheDir = filepath.Join(os.TempDir(), "speedboat-sif")
		} else {
			cacheDir = filepath.Join(userCache, "speedboat", "sif")
		}
	}

	return &ApptainerEngine{
		binaryPath: bin,
		cacheDir:   cacheDir,
	}
}

func (a *ApptainerEngine) SetGPUEnabled(enabled bool) {
	a.GPUEnabled = enabled
}

func (a *ApptainerEngine) GetBinaryPath() string {
	return a.binaryPath
}

func (a *ApptainerEngine) Name() string {
	return "apptainer"
}

func (a *ApptainerEngine) IsAvailable() bool {
	return a.binaryPath != ""
}

// sifPath returns the destination path for an image.
// If the image is already a path ending in .sif, it is returned directly.
// Otherwise, a deterministic filename under a.cacheDir is produced.
func (a *ApptainerEngine) sifPath(image string) string {
	if strings.HasSuffix(image, ".sif") {
		return image
	}
	clean := strings.TrimPrefix(image, "docker://")
	var safe strings.Builder
	for _, r := range clean {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			safe.WriteRune(r)
		} else {
			safe.WriteRune('_')
		}
	}
	return filepath.Join(a.cacheDir, safe.String()+".sif")
}

func isInsecureRegistry(image string) bool {
	raw := strings.TrimPrefix(image, "docker://")
	parts := strings.SplitN(raw, "/", 2)
	host := parts[0]
	if strings.HasPrefix(host, "localhost") ||
		strings.HasPrefix(host, "127.0.0.1") ||
		strings.HasPrefix(host, "::1") ||
		strings.Contains(host, ":5000") ||
		strings.HasPrefix(host, "192.168.") ||
		strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "172.") ||
		os.Getenv("INSECURE_REGISTRY") == "true" ||
		os.Getenv("SPEEDBOAT_INSECURE") == "true" {
		return true
	}
	return false
}

func (a *ApptainerEngine) pullToPath(ctx context.Context, targetPath string, sourceURI string, noHTTPS bool) (string, error) {
	tmpPath := fmt.Sprintf("%s.tmp.%d", targetPath, os.Getpid())
	defer os.Remove(tmpPath)

	args := []string{"-s", "pull", "--force"}
	if noHTTPS {
		args = append(args, "--no-https")
	}
	args = append(args, tmpPath, sourceURI)

	cmd := exec.CommandContext(ctx, a.binaryPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stderr.String(), err
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		return "", fmt.Errorf("failed to save pulled SIF to %s: %w", targetPath, err)
	}

	return "", nil
}

func (a *ApptainerEngine) PullImage(ctx context.Context, image string) error {
	if !a.IsAvailable() {
		return fmt.Errorf("apptainer binary not found")
	}
	if image == "" {
		return fmt.Errorf("empty image name")
	}

	target := a.sifPath(image)

	// If image already exists locally and has content, skip pulling
	if fi, err := os.Stat(target); err == nil && fi.Size() > 0 {
		return nil
	}

	if strings.HasSuffix(image, ".sif") {
		// A local SIF was requested but does not exist
		return fmt.Errorf("SIF image file not found: %s", target)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Double check after lock
	if fi, err := os.Stat(target); err == nil && fi.Size() > 0 {
		return nil
	}

	if err := os.MkdirAll(a.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir %s: %w", a.cacheDir, err)
	}

	sourceURI := image
	if !strings.HasPrefix(sourceURI, "docker://") {
		sourceURI = fmt.Sprintf("docker://%s", image)
	}

	insecure := isInsecureRegistry(sourceURI)
	stderrStr, err := a.pullToPath(ctx, target, sourceURI, insecure)
	if err != nil {
		// If failed with HTTP-response-to-HTTPS error, retry with --no-https
		if !insecure && strings.Contains(stderrStr, "server gave HTTP response to HTTPS client") {
			stderrStr, err = a.pullToPath(ctx, target, sourceURI, true)
		} else if insecure && (strings.Contains(stderrStr, "tls") || strings.Contains(stderrStr, "https")) {
			// If guessed insecure but required HTTPS, retry without --no-https
			stderrStr, err = a.pullToPath(ctx, target, sourceURI, false)
		}
	}

	if err != nil {
		return fmt.Errorf("apptainer pull failed for %s: %w (stderr: %s)", image, err, strings.TrimSpace(stderrStr))
	}

	return nil
}

func HasNvidiaGPU() bool {
	for _, dev := range []string{"/dev/nvidia0", "/dev/nvidiactl"} {
		if _, err := os.Stat(dev); err == nil {
			return true
		}
	}
	return false
}

func HasAmdGPU() bool {
	if _, err := os.Stat("/dev/kfd"); err == nil {
		return true
	}
	return false
}

func DetectHostGPU() (bool, string) {
	if HasNvidiaGPU() {
		return true, "NVIDIA"
	}
	if HasAmdGPU() {
		return true, "AMD"
	}
	return false, "None"
}

func (a *ApptainerEngine) Execute(ctx context.Context, image string, input []byte) ([]byte, error) {
	if !a.IsAvailable() {
		return nil, fmt.Errorf("apptainer binary not found")
	}
	if image == "" {
		return nil, fmt.Errorf("cannot execute task: image name is empty")
	}

	target := a.sifPath(image)
	if fi, err := os.Stat(target); os.IsNotExist(err) || (err == nil && fi.Size() == 0) {
		if err := a.PullImage(ctx, image); err != nil {
			return nil, err
		}
	}

	// Run container with standard stream contract: stdin -> container -> stdout
	// -s: silent mode (suppresses internal apptainer info/warnings on stderr)
	// run: execute the container image's default entrypoint
	// --contain: isolate filesystem (/tmp, /var/tmp)
	// --net --network=none: sandbox network isolation
	args := []string{"-s", "run", "--contain", "--net", "--network=none"}
	if a.GPUEnabled {
		if HasNvidiaGPU() {
			args = append(args, "--nv")
		} else if HasAmdGPU() {
			args = append(args, "--rocm")
		}
	}
	args = append(args, target)

	cmd := exec.CommandContext(ctx, a.binaryPath, args...)
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("apptainer execution failed: %w\nstderr: %s\nstdout: %s",
			err, strings.TrimSpace(stderr.String()), strings.TrimSpace(stdout.String()))
	}

	return stdout.Bytes(), nil
}
