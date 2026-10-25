//go:build !linux && !windows

package runtime

// HasNvidiaGPU is a stub on unsupported platforms.
func HasNvidiaGPU() bool {
	return false
}

// HasAmdGPU is a stub on unsupported platforms.
func HasAmdGPU() bool {
	return false
}

// DetectHostGPU returns whether an accelerator is detected and its vendor on other platforms.
func DetectHostGPU() (bool, string) {
	return false, "None"
}
