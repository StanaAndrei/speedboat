package runtime

import (
	"context"
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
