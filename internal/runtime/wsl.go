package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode/utf16"
)

// DecodeWindowsOutput converts UTF-16LE or UTF-8 output from Windows tools to a clean Go string.
func DecodeWindowsOutput(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		return decodeUTF16LE(b[2:])
	}
	if len(b) >= 4 && b[1] == 0 && b[3] == 0 {
		return decodeUTF16LE(b)
	}
	return string(b)
}

func decodeUTF16LE(b []byte) string {
	u16 := make([]uint16, len(b)/2)
	for i := 0; i < len(u16); i++ {
		u16[i] = uint16(b[i*2]) | (uint16(b[i*2+1]) << 8)
	}
	return string(utf16.Decode(u16))
}

// WSLEngine executes container workloads inside a Linux WSL2 distribution on Windows.
type WSLEngine struct {
	distroName string
	wslPath    string
	GPUEnabled bool
	cmdRunner  func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewWSLEngine creates an engine targeting the specified distribution (defaults to SpeedboatWorker).
func NewWSLEngine(distro string) *WSLEngine {
	return NewWSLEngineCustom(distro, "")
}

// NewWSLEngineCustom creates an engine with a custom WSL binary and distribution name.
func NewWSLEngineCustom(distro, customWSL string) *WSLEngine {
	bin := customWSL
	if bin == "" {
		for _, name := range []string{"wsl.exe", "wsl"} {
			if p, err := exec.LookPath(name); err == nil {
				bin = p
				break
			}
		}
		if bin == "" {
			bin = "wsl.exe"
		}
	}
	if distro == "" {
		distro = "SpeedboatWorker"
	}
	return &WSLEngine{
		distroName: distro,
		wslPath:    bin,
		cmdRunner:  exec.CommandContext,
	}
}

// SetCmdRunner allows injecting a command builder for unit testing without live WSL.
func (w *WSLEngine) SetCmdRunner(runner func(ctx context.Context, name string, args ...string) *exec.Cmd) {
	w.cmdRunner = runner
}

func (w *WSLEngine) Name() string {
	return "wsl2"
}

func (w *WSLEngine) DistroName() string {
	return w.distroName
}

func (w *WSLEngine) GetBinaryPath() string {
	return w.wslPath
}

func (w *WSLEngine) SetGPUEnabled(enabled bool) {
	w.GPUEnabled = enabled
}

func (w *WSLEngine) runCmd(ctx context.Context, args ...string) *exec.Cmd {
	if w.cmdRunner != nil {
		return w.cmdRunner(ctx, w.wslPath, args...)
	}
	return exec.CommandContext(ctx, w.wslPath, args...)
}

// IsAvailable checks if the WSL binary is present and the targeted distro responds.
func (w *WSLEngine) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := w.runCmd(ctx, "-d", w.distroName, "-u", "root", "--", "echo", "speedboat-ready")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	decoded := DecodeWindowsOutput(out)
	return strings.Contains(decoded, "speedboat-ready")
}

// DetectContainerTool checks what container management binary is available inside the distro.
func (w *WSLEngine) DetectContainerTool(ctx context.Context) string {
	for _, tool := range []string{"podman", "docker", "apptainer", "crun"} {
		cmd := w.runCmd(ctx, "-d", w.distroName, "-u", "root", "--", "which", tool)
		if err := cmd.Run(); err == nil {
			return tool
		}
	}
	return "podman"
}

// BuildPullArgs constructs the command-line arguments to pull an image inside WSL.
func (w *WSLEngine) BuildPullArgs(tool, image string) []string {
	cleanImage := strings.TrimPrefix(image, "docker://")
	insecure := isInsecureRegistry(image)

	switch tool {
	case "docker":
		return []string{"-d", w.distroName, "-u", "root", "--", "docker", "pull", cleanImage}
	case "apptainer":
		args := []string{"-d", w.distroName, "-u", "root", "--", "apptainer", "-s", "pull", "--force"}
		if insecure {
			args = append(args, "--no-https")
		}
		args = append(args, cleanImage)
		return args
	default: // podman
		args := []string{"-d", w.distroName, "-u", "root", "--", "podman", "pull"}
		if insecure {
			args = append(args, "--tls-verify=false")
		}
		args = append(args, cleanImage)
		return args
	}
}

// BuildRunArgs constructs the command-line arguments to execute an image inside WSL.
func (w *WSLEngine) BuildRunArgs(tool, image string) []string {
	cleanImage := strings.TrimPrefix(image, "docker://")

	switch tool {
	case "docker":
		args := []string{"-d", w.distroName, "-u", "root", "--", "docker", "run", "-i", "--rm", "--network", "none"}
		if w.GPUEnabled {
			args = append(args, "--gpus", "all")
		}
		args = append(args, cleanImage)
		return args
	case "apptainer":
		args := []string{"-d", w.distroName, "-u", "root", "--", "apptainer", "-s", "run", "--contain", "--net", "--network=none"}
		if w.GPUEnabled {
			args = append(args, "--nv")
		}
		args = append(args, cleanImage)
		return args
	default: // podman
		args := []string{"-d", w.distroName, "-u", "root", "--", "podman", "run", "-i", "--rm", "--network", "none"}
		if w.GPUEnabled {
			// In WSL2, NVIDIA GPU device passthrough is available via DirectX /dev/dxg or nvidia CDI
			args = append(args, "--device", "/dev/dxg")
		}
		args = append(args, cleanImage)
		return args
	}
}

func (w *WSLEngine) PullImage(ctx context.Context, image string) error {
	if image == "" {
		return fmt.Errorf("empty image name")
	}

	tool := w.DetectContainerTool(ctx)
	args := w.BuildPullArgs(tool, image)

	cmd := w.runCmd(ctx, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("WSL container pull failed (%s): %w\nstderr: %s",
			tool, err, strings.TrimSpace(DecodeWindowsOutput(stderr.Bytes())))
	}
	return nil
}

func (w *WSLEngine) Execute(ctx context.Context, image string, input []byte) ([]byte, error) {
	if image == "" {
		return nil, fmt.Errorf("cannot execute task: image name is empty")
	}

	tool := w.DetectContainerTool(ctx)
	args := w.BuildRunArgs(tool, image)

	cmd := w.runCmd(ctx, args...)
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	_ = time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("WSL container execution failed (%s): %w\nstderr: %s\nstdout: %s",
			tool, err,
			strings.TrimSpace(DecodeWindowsOutput(stderr.Bytes())),
			strings.TrimSpace(DecodeWindowsOutput(stdout.Bytes())))
	}

	return stdout.Bytes(), nil
}
