package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type ApptainerEngine struct {
	binaryPath string
}

func NewApptainerEngine() *ApptainerEngine {
	bin, err := exec.LookPath("apptainer")
	if err != nil {
		bin, _ = exec.LookPath("singularity")
	}
	return &ApptainerEngine{binaryPath: bin}
}

func (a *ApptainerEngine) Name() string {
	return "apptainer"
}

func (a *ApptainerEngine) IsAvailable() bool {
	return a.binaryPath != ""
}

func (a *ApptainerEngine) PullImage(ctx context.Context, image string) error {
	if !a.IsAvailable() {
		return fmt.Errorf("apptainer binary not found")
	}
	// Apptainer pulls automatically when using docker:// URI, but can pre-pull with pull command
	cmd := exec.CommandContext(ctx, a.binaryPath, "pull", "--disable-cache", "--force", fmt.Sprintf("docker://%s", image))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apptainer pull failed for %s: %w (stderr: %s)", image, err, stderr.String())
	}
	return nil
}

func (a *ApptainerEngine) Execute(ctx context.Context, image string, input []byte) ([]byte, error) {
	if !a.IsAvailable() {
		return nil, fmt.Errorf("apptainer binary not found")
	}

	imageURI := image
	if !strings.HasPrefix(imageURI, "docker://") && !strings.HasSuffix(imageURI, ".sif") {
		imageURI = fmt.Sprintf("docker://%s", image)
	}

	args := []string{"exec", "--contain", "--net", "--network=none", imageURI}
	cmd := exec.CommandContext(ctx, a.binaryPath, args...)
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("apptainer execution failed: %w\nstderr: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}
