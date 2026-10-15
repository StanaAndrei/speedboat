package main

import (
	"testing"
)

func TestEngineManagerCapabilities(t *testing.T) {
	mgr := NewHeavyEngineManager()
	caps := mgr.GetCapabilities()

	t.Logf("Platform: %s", caps.Platform)
	t.Logf("HeavyModeSupported: %v", caps.HeavyModeSupported)
	t.Logf("EngineName: %s", caps.EngineName)
	t.Logf("IsInstalled: %v", caps.IsInstalled)
	t.Logf("BinaryPath: %s", caps.BinaryPath)
	t.Logf("Version: %s", caps.Version)
	t.Logf("GPUAvailable: %v (Vendor: %s)", caps.GPUAvailable, caps.GPUVendor)

	if caps.HeavyModeSupported && caps.EngineName == "" {
		t.Errorf("expected non-empty engine name when heavy mode is supported")
	}
}

func TestInstallApptainerFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping install flow in short mode")
	}

	mgr := NewHeavyEngineManager()
	capsBefore := mgr.GetCapabilities()
	t.Logf("Before install: IsInstalled=%v, Path=%s", capsBefore.IsInstalled, capsBefore.BinaryPath)

	err := mgr.Install(t.Context(), func(pct float64, msg string) {
		t.Logf("[Install Progress %.0f%%] %s", pct, msg)
	})
	if err != nil {
		t.Fatalf("Install() failed: %v", err)
	}

	capsAfter := mgr.GetCapabilities()
	t.Logf("After install: IsInstalled=%v, Version=%s, Path=%s", capsAfter.IsInstalled, capsAfter.Version, capsAfter.BinaryPath)

	if !capsAfter.IsInstalled {
		t.Fatalf("expected Apptainer to be reported as installed after Install()")
	}
	if capsAfter.Version == "" {
		t.Fatalf("expected non-empty version after Install()")
	}
}
