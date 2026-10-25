//go:build windows

package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func findNvidiaSmiWindows() string {
	if p, err := exec.LookPath("nvidia-smi.exe"); err == nil {
		return p
	}
	if p, err := exec.LookPath("nvidia-smi"); err == nil {
		return p
	}

	candidates := []string{
		`C:\Windows\System32\nvidia-smi.exe`,
		`C:\Program Files\NVIDIA Corporation\NVSMI\nvidia-smi.exe`,
	}
	sysRoot := os.Getenv("SystemRoot")
	if sysRoot != "" {
		candidates = append(candidates, filepath.Join(sysRoot, "System32", "nvidia-smi.exe"))
	}
	progFiles := os.Getenv("ProgramFiles")
	if progFiles != "" {
		candidates = append(candidates, filepath.Join(progFiles, "NVIDIA Corporation", "NVSMI", "nvidia-smi.exe"))
	}

	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

// HasNvidiaGPU checks for the presence of NVIDIA GPUs and drivers on Windows.
func HasNvidiaGPU() bool {
	if smi := findNvidiaSmiWindows(); smi != "" {
		out, err := exec.Command(smi, "-L").Output()
		if err == nil && len(out) > 0 {
			return true
		}
	}

	// Fallback check via powershell querying WMI Video Controller
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		"Get-CimInstance Win32_VideoController | Select-Object -ExpandProperty Name").Output()
	if err == nil && strings.Contains(strings.ToUpper(string(out)), "NVIDIA") {
		return true
	}
	return false
}

// HasAmdGPU checks for the presence of AMD GPUs on Windows.
func HasAmdGPU() bool {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		"Get-CimInstance Win32_VideoController | Select-Object -ExpandProperty Name").Output()
	if err == nil {
		upper := strings.ToUpper(string(out))
		if strings.Contains(upper, "AMD") || strings.Contains(upper, "RADEON") {
			return true
		}
	}
	return false
}

// DetectHostGPU returns whether an accelerator is detected and its vendor on Windows.
func DetectHostGPU() (bool, string) {
	if HasNvidiaGPU() {
		return true, "NVIDIA"
	}
	if HasAmdGPU() {
		return true, "AMD"
	}
	return false, "None"
}
