package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type DockerEngine struct{}

func NewDockerEngine() *DockerEngine {
	return &DockerEngine{}
}

func (d *DockerEngine) Name() string {
	return "docker"
}

func (d *DockerEngine) IsAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

func (d *DockerEngine) PullImage(ctx context.Context, image string) error {
	if image == "" {
		return fmt.Errorf("empty image name")
	}

	// check if image already exists locally
	inspectCmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	if err := inspectCmd.Run(); err == nil {
		return nil
	}

	cmd := exec.CommandContext(ctx, "docker", "pull", image)
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

	// Run container with standard stream contract: stdin -> container -> stdout
	// -i: interactive (keep stdin open)
	// --rm: automatically clean up container after exit
	// --network none: sandbox security isolation
	args := []string{"run", "-i", "--rm", "--network", "none", image}

	cmd := exec.CommandContext(ctx, "docker", args...)
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
