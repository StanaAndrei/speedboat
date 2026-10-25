//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsHeavyEngineManagerCapabilities(t *testing.T) {
	mgr := NewHeavyEngineManager()
	caps := mgr.GetCapabilities()

	t.Logf("Platform: %s", caps.Platform)
	t.Logf("HeavyModeSupported: %v", caps.HeavyModeSupported)
	t.Logf("EngineName: %s", caps.EngineName)
	t.Logf("IsInstalled: %v", caps.IsInstalled)
	t.Logf("BinaryPath: %s", caps.BinaryPath)
	t.Logf("CanAutoInstall: %v", caps.CanAutoInstall)
	t.Logf("StatusMessage: %s", caps.StatusMessage)

	if caps.Platform != "windows" {
		t.Errorf("expected platform 'windows', got %s", caps.Platform)
	}
	if !caps.HeavyModeSupported {
		t.Errorf("expected heavy mode to be supported on Windows")
	}
	if caps.EngineName == "" {
		t.Errorf("expected non-empty engine name on Windows")
	}
}

func TestEnsureWSLConfig(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("USERPROFILE")
	os.Setenv("USERPROFILE", tmpHome)
	defer os.Setenv("USERPROFILE", origHome)

	err := ensureWSLConfig()
	if err != nil {
		t.Fatalf("ensureWSLConfig failed: %v", err)
	}

	cfgPath := filepath.Join(tmpHome, ".wslconfig")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed reading created .wslconfig: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "[wsl2]") {
		t.Errorf("expected [wsl2] header in .wslconfig, got:\n%s", content)
	}
	if !strings.Contains(content, "memory=4GB") {
		t.Errorf("expected memory=4GB limit in .wslconfig, got:\n%s", content)
	}

	// Idempotency check: running ensureWSLConfig again shouldn't duplicate [wsl2]
	err = ensureWSLConfig()
	if err != nil {
		t.Fatalf("second ensureWSLConfig failed: %v", err)
	}

	data2, _ := os.ReadFile(cfgPath)
	if strings.Count(string(data2), "memory=4GB") != 1 {
		t.Errorf("expected memory=4GB to appear exactly once, got:\n%s", string(data2))
	}
}

func TestWindowsDistroInstalledCheck(t *testing.T) {
	// If wslBin is empty, should return false
	if isDistroInstalled("", "SpeedboatWorker") {
		t.Errorf("expected false for empty wslBin")
	}
}
