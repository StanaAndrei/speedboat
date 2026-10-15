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
