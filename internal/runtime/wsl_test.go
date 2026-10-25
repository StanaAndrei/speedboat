package runtime

import (
	"context"
	"testing"
)

func TestWSLEngineBasics(t *testing.T) {
	eng := NewWSLEngine("SpeedboatWorker")
	if eng.Name() != "wsl2" {
		t.Errorf("expected engine name 'wsl2', got %s", eng.Name())
	}
	if eng.DistroName() != "SpeedboatWorker" {
		t.Errorf("expected distro name 'SpeedboatWorker', got %s", eng.DistroName())
	}
	if eng.GetBinaryPath() == "" {
		t.Errorf("expected non-empty binary path")
	}
}

func TestWSLBuildRunArgs(t *testing.T) {
	eng := NewWSLEngine("SpeedboatWorker")

	// Podman without GPU
	podmanArgs := eng.BuildRunArgs("podman", "docker://localhost:5000/speedboat/prime:latest")
	expectedPrefix := []string{"-d", "SpeedboatWorker", "-u", "root", "--", "podman", "run", "-i", "--rm", "--network", "none"}
	for i, exp := range expectedPrefix {
		if podmanArgs[i] != exp {
			t.Errorf("expected arg %d to be %s, got %s", i, exp, podmanArgs[i])
		}
	}
	if podmanArgs[len(podmanArgs)-1] != "localhost:5000/speedboat/prime:latest" {
		t.Errorf("expected last arg to be stripped image name, got %s", podmanArgs[len(podmanArgs)-1])
	}

	// Podman with GPU
	eng.SetGPUEnabled(true)
	podmanGPUArgs := eng.BuildRunArgs("podman", "prime:latest")
	foundGPU := false
	for _, a := range podmanGPUArgs {
		if a == "/dev/dxg" {
			foundGPU = true
			break
		}
	}
	if !foundGPU {
		t.Errorf("expected GPU device flag in podman args when GPU is enabled")
	}

	// Docker with GPU
	dockerGPUArgs := eng.BuildRunArgs("docker", "prime:latest")
	foundDockerGPU := false
	for i, a := range dockerGPUArgs {
		if a == "--gpus" && i+1 < len(dockerGPUArgs) && dockerGPUArgs[i+1] == "all" {
			foundDockerGPU = true
			break
		}
	}
	if !foundDockerGPU {
		t.Errorf("expected --gpus all flag in docker args when GPU is enabled")
	}

	// Apptainer with GPU
	apptainerGPUArgs := eng.BuildRunArgs("apptainer", "prime:latest")
	foundApptainerGPU := false
	for _, a := range apptainerGPUArgs {
		if a == "--nv" {
			foundApptainerGPU = true
			break
		}
	}
	if !foundApptainerGPU {
		t.Errorf("expected --nv flag in apptainer args when GPU is enabled")
	}
}

func TestWSLBuildPullArgs(t *testing.T) {
	eng := NewWSLEngine("SpeedboatWorker")

	// Insecure registry (localhost)
	argsInsecure := eng.BuildPullArgs("podman", "localhost:5000/my-image:latest")
	foundInsecure := false
	for _, a := range argsInsecure {
		if a == "--tls-verify=false" {
			foundInsecure = true
			break
		}
	}
	if !foundInsecure {
		t.Errorf("expected --tls-verify=false for insecure localhost registry")
	}

	// Secure registry
	argsSecure := eng.BuildPullArgs("podman", "docker.io/library/alpine:latest")
	for _, a := range argsSecure {
		if a == "--tls-verify=false" {
			t.Errorf("did not expect --tls-verify=false for docker.io")
		}
	}
}

func TestDecodeWindowsOutput(t *testing.T) {
	// Standard ASCII / UTF-8
	rawASCII := []byte("Ubuntu\nSpeedboatWorker\n")
	if DecodeWindowsOutput(rawASCII) != "Ubuntu\nSpeedboatWorker\n" {
		t.Errorf("unexpected output for raw ASCII")
	}

	// UTF-16LE with BOM
	utf16BOM := []byte{0xFF, 0xFE, 'O', 0x00, 'K', 0x00}
	if DecodeWindowsOutput(utf16BOM) != "OK" {
		t.Errorf("expected 'OK' from UTF-16LE BOM, got %q", DecodeWindowsOutput(utf16BOM))
	}

	// UTF-16LE without BOM (null bytes alternating)
	utf16NoBOM := []byte{'W', 0x00, 'S', 0x00, 'L', 0x00}
	if DecodeWindowsOutput(utf16NoBOM) != "WSL" {
		t.Errorf("expected 'WSL' from UTF-16LE without BOM, got %q", DecodeWindowsOutput(utf16NoBOM))
	}
}

func TestWSLEngineEmptyImageValidation(t *testing.T) {
	eng := NewWSLEngine("SpeedboatWorker")
	if err := eng.PullImage(context.Background(), ""); err == nil {
		t.Errorf("expected error when pulling empty image")
	}
	if _, err := eng.Execute(context.Background(), "", []byte{}); err == nil {
		t.Errorf("expected error when executing empty image")
	}
}

func TestDockerEngineGPUFlags(t *testing.T) {
	eng := NewDockerEngine()
	eng.SetGPUEnabled(true)
	if !eng.GPUEnabled {
		t.Errorf("expected GPUEnabled to be true")
	}
}
