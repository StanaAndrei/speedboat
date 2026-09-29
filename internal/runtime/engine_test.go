package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestAutoDetectEngine(t *testing.T) {
	eng, err := AutoDetectEngine("auto")
	if err != nil {
		t.Fatalf("expected engine to be detected, got: %v", err)
	}

	if !eng.IsAvailable() {
		t.Errorf("detected engine %s reports not available", eng.Name())
	}
}

func TestDockerEngineAvailable(t *testing.T) {
	eng := NewDockerEngine()
	if eng.Name() != "docker" {
		t.Errorf("expected engine name 'docker', got '%s'", eng.Name())
	}
	if !eng.IsAvailable() {
		t.Log("docker daemon might not be running in this environment")
	}
}

func TestDockerEngineExecuteEmptyImage(t *testing.T) {
	eng := NewDockerEngine()
	_, err := eng.Execute(context.Background(), "", []byte("{}"))
	if err == nil {
		t.Errorf("expected error for empty image name, got nil")
	}
}

func TestApptainerEngineAvailable(t *testing.T) {
	eng := NewApptainerEngine()
	if eng.Name() != "apptainer" {
		t.Errorf("expected engine name 'apptainer', got '%s'", eng.Name())
	}
}

func TestApptainerEngineExecuteEmptyImage(t *testing.T) {
	eng := NewApptainerEngine()
	if !eng.IsAvailable() {
		t.Skip("apptainer not available on host")
	}
	_, err := eng.Execute(context.Background(), "", []byte("{}"))
	if err == nil {
		t.Errorf("expected error for empty image name, got nil")
	}
}

func TestApptainerSifPath(t *testing.T) {
	eng := NewApptainerEngine()
	path1 := eng.sifPath("/already/a/path.sif")
	if path1 != "/already/a/path.sif" {
		t.Errorf("expected /already/a/path.sif, got %s", path1)
	}

	path2 := eng.sifPath("localhost:5000/prime-counter:latest")
	if !strings.HasSuffix(path2, "localhost_5000_prime-counter_latest.sif") {
		t.Errorf("expected sanitized suffix, got %s", path2)
	}
}

func TestIsInsecureRegistry(t *testing.T) {
	tests := []struct {
		image    string
		expected bool
	}{
		{"localhost:5000/prime-counter:latest", true},
		{"docker://localhost:5000/prime-counter:latest", true},
		{"127.0.0.1:5000/image:v1", true},
		{"192.168.1.100:5000/image:v1", true},
		{"custom.registry.local:5000/image", true},
		{"docker.io/library/alpine:latest", false},
		{"alpine:latest", false},
	}

	for _, tt := range tests {
		got := isInsecureRegistry(tt.image)
		if got != tt.expected {
			t.Errorf("isInsecureRegistry(%q) = %v; want %v", tt.image, got, tt.expected)
		}
	}
}

func TestApptainerEnginePullAndExecute(t *testing.T) {
	eng := NewApptainerEngine()
	if !eng.IsAvailable() {
		t.Skip("apptainer not installed")
	}

	image := "localhost:5000/prime-counter:latest"
	ctx := context.Background()

	// Pull should succeed with --no-https automatically
	if err := eng.PullImage(ctx, image); err != nil {
		t.Skipf("skipping pull test (registry may not be reachable): %v", err)
	}

	// Execute should run the container and return output
	taskInput := []byte(`{"task_id":"test-apptainer","start":1,"end":20}`)
	out, err := eng.Execute(ctx, image, taskInput)
	if err != nil {
		if strings.Contains(err.Error(), "user namespace") || strings.Contains(err.Error(), "Function not implemented") {
			t.Skipf("skipping execute test (user namespaces not permitted in this environment): %v", err)
		}
		t.Fatalf("apptainer execute failed: %v", err)
	}

	if !strings.Contains(string(out), `"count": 8`) {
		t.Errorf("unexpected output: %s", string(out))
	}
}


