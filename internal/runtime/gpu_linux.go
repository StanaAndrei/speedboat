//go:build linux

package runtime

import (
	"os"
)

// HasNvidiaGPU checks for the presence of NVIDIA GPU devices on Linux.
func HasNvidiaGPU() bool {
	for _, dev := range []string{"/dev/nvidia0", "/dev/nvidiactl"} {
		if _, err := os.Stat(dev); err == nil {
			return true
		}
	}
	return false
}

// HasAmdGPU checks for the presence of AMD GPU devices on Linux.
func HasAmdGPU() bool {
	if _, err := os.Stat("/dev/kfd"); err == nil {
		return true
	}
	return false
}

// DetectHostGPU returns whether an accelerator is detected and its vendor on Linux.
func DetectHostGPU() (bool, string) {
	if HasNvidiaGPU() {
		return true, "NVIDIA"
	}
	if HasAmdGPU() {
		return true, "AMD"
	}
	return false, "None"
}
