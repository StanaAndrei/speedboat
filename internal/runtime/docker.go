package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type DockerEngine struct {
	binaryPath string
	GPUEnabled bool
}

func NewDockerEngine() *DockerEngine {
	return NewDockerEngineCustom("")
}

func NewDockerEngineCustom(customBin string) *DockerEngine {
	bin := customBin
	if bin == "" {
		for _, name := range []string{"docker", "docker.exe", "podman", "podman.exe"} {
			if p, err := exec.LookPath(name); err == nil {
				bin = p
				break
			}
		}
	}
	return &DockerEngine{
		binaryPath: bin,
	}
}

func (d *DockerEngine) SetGPUEnabled(enabled bool) {
	d.GPUEnabled = enabled
}

func (d *DockerEngine) GetBinaryPath() string {
	return d.binaryPath
}

func (d *DockerEngine) Name() string {
	if strings.Contains(strings.ToLower(d.binaryPath), "podman") {
		return "podman"
	}
	return "docker"
}

func (d *DockerEngine) getBin() string {
	if d.binaryPath != "" {
		return d.binaryPath
	}
	return "docker"
}

func (d *DockerEngine) IsAvailable() bool {
	bin := d.getBin()
	cmd := exec.Command(bin, "info")
	return cmd.Run() == nil
}

func (d *DockerEngine) PullImage(ctx context.Context, image string) error {
	if image == "" {
		return fmt.Errorf("empty image name")
	}

	bin := d.getBin()

	// check if image already exists locally
	inspectCmd := exec.CommandContext(ctx, bin, "image", "inspect", image)
	if err := inspectCmd.Run(); err == nil {
		return nil
	}

	cmd := exec.CommandContext(ctx, bin, "pull", image)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pull docker image %s: %w (stderr: %s)", image, err, stderr.String())
	}

	return nil
}

func (d *DockerEngine) Execute(ctx context.Context, image string, input []byte) ([]byte, error) {
	if image == "" {
		return nil, fmt.Errorf("cannot execute task: image name is empty")
	}

	bin := d.getBin()

	// Run container with standard stream contract: stdin -> container -> stdout
	// -i: interactive (keep stdin open)
	// --rm: automatically clean up container after exit
	// --network none: sandbox security isolation
	args := []string{"run", "-i", "--rm", "--network", "none"}
	if d.GPUEnabled {
		args = append(args, "--gpus", "all")
	}
	args = append(args, image)

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	_ = time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("docker execution failed for image %s: %w\nstderr: %s\nstdout: %s",
			image, err, strings.TrimSpace(stderr.String()), strings.TrimSpace(stdout.String()))
	}

	return stdout.Bytes(), nil
}
