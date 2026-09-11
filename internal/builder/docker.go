package builder

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"speedboat/internal/protocol"
	"strings"
)

func BuildAndPushImage(ctx context.Context, projectDir string, containerCfg protocol.ContainerConfig, registry string) (string, error) {
	dockerfilePath := containerCfg.Dockerfile
	if !filepath.IsAbs(dockerfilePath) {
		dockerfilePath = filepath.Join(projectDir, dockerfilePath)
	}

	primaryTag := containerCfg.ImageTag
	if primaryTag == "" {
		primaryTag = "speedboat-task:latest"
	}

	var tags []string
	var targetTag string
	if registry != "" { // Clean trailing slashes from registry
		reg := strings.TrimRight(registry, "/")
		targetTag = fmt.Sprintf("%s/%s", reg, primaryTag)
		tags = []string{targetTag, primaryTag}
	} else {
		targetTag = primaryTag
		tags = []string{primaryTag}
	}

	if err := BuildImage(ctx, projectDir, dockerfilePath, tags); err != nil {
		return "", fmt.Errorf("failed to build image: %w", err)
	}
	if registry != "" {
		if err := PushImage(ctx, targetTag); err != nil {
			return "", fmt.Errorf("failed to push image: %w", err)
		}
	}
	return targetTag, nil
}

// BuildImage executes docker build with the specified tags and Dockerfile
func BuildImage(ctx context.Context, contextDir string, dockerfilePath string, tags []string) error {
	args := []string{"build"}
	for _, tag := range tags {
		args = append(args, "-t", tag)
	}
	args = append(args, "-f", dockerfilePath, contextDir)

	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running 'docker %s': %w\nstderr: %s\nstdout: %s",
			strings.Join(args, " "), err, stderr.String(), stdout.String())
	}

	return nil
}

func PushImage(ctx context.Context, tag string) error { // PushImage executes docker push for the given tag
	cmd := exec.CommandContext(ctx, "docker", "push", tag)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running 'docker push %s': %w\nstderr: %s\nstdout: %s",
			tag, err, stderr.String(), stdout.String())
	}

	return nil
}
