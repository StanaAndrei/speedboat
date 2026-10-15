//go:build linux

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"

	speedboatRuntime "speedboat/internal/runtime"
)

type linuxHeavyEngineManager struct{}

func NewHeavyEngineManager() HeavyEngineManager {
	return &linuxHeavyEngineManager{}
}

func getSpeedboatInstallDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.TempDir()
	}
	return filepath.Join(home, ".local", "share", "speedboat")
}

func ensureRelocatableSymlinks(destDir string) {
	usrDir := filepath.Join(destDir, "usr")
	if fi, err := os.Stat(usrDir); err == nil && fi.IsDir() {
		// Apptainer expects configuration at PREFIX/etc (destDir/usr/etc)
		usrEtc := filepath.Join(usrDir, "etc")
		if _, err := os.Lstat(usrEtc); err != nil {
			_ = os.Symlink("../etc", usrEtc)
		}
		// Apptainer expects session directory at PREFIX/var (destDir/usr/var)
		usrVar := filepath.Join(usrDir, "var")
		if _, err := os.Lstat(usrVar); err != nil {
			_ = os.Symlink("../var", usrVar)
		}
	}

	// Ensure destDir/bin/apptainer is symlinked to ../usr/bin/apptainer
	realBin := filepath.Join(destDir, "usr", "bin", "apptainer")
	if fi, err := os.Stat(realBin); err == nil && (fi.Mode()&0111 != 0) {
		binDir := filepath.Join(destDir, "bin")
		_ = os.MkdirAll(binDir, 0755)
		linkBin := filepath.Join(binDir, "apptainer")
		if _, err := os.Lstat(linkBin); err != nil {
			_ = os.Symlink(realBin, linkBin)
		}
		_ = os.Chmod(realBin, 0755)
	}
}

func findApptainerBinary() (string, string) {
	destDir := getSpeedboatInstallDir()
	ensureRelocatableSymlinks(destDir)

	candidates := []string{
		filepath.Join(destDir, "bin", "apptainer"),
		filepath.Join(destDir, "usr", "bin", "apptainer"),
	}

	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && (fi.Mode()&0111 != 0) {
			out, err := exec.Command(p, "version").Output()
			if err == nil {
				return p, strings.TrimSpace(string(out))
			}
		}
	}

	if p, err := exec.LookPath("apptainer"); err == nil {
		out, err := exec.Command(p, "version").Output()
		if err == nil {
			return p, strings.TrimSpace(string(out))
		}
	}

	if p, err := exec.LookPath("singularity"); err == nil {
		out, err := exec.Command(p, "version").Output()
		if err == nil {
			return p, strings.TrimSpace(string(out))
		}
	}

	return "", ""
}

func (m *linuxHeavyEngineManager) GetCapabilities() EngineCapabilities {
	binPath, version := findApptainerBinary()
	hasGPU, gpuVendor := speedboatRuntime.DetectHostGPU()

	caps := EngineCapabilities{
		Platform:           "linux",
		HeavyModeSupported: true,
		EngineName:         "Apptainer",
		GPUAvailable:       hasGPU,
		GPUVendor:          gpuVendor,
	}

	if binPath != "" {
		caps.IsInstalled = true
		caps.BinaryPath = binPath
		caps.Version = version
		caps.CanAutoInstall = false
		caps.StatusMessage = fmt.Sprintf("Apptainer %s ready (%s)", version, binPath)
	} else {
		caps.IsInstalled = false
		caps.CanAutoInstall = true
		caps.StatusMessage = "Apptainer container runtime not detected. Click below for 1-click install."
	}

	return caps
}

func (m *linuxHeavyEngineManager) Install(ctx context.Context, onProgress ProgressCallback) error {
	destDir := getSpeedboatInstallDir()
	if err := os.MkdirAll(filepath.Join(destDir, "bin"), 0755); err != nil {
		return fmt.Errorf("failed to create destination dir: %w", err)
	}

	arch := "amd64"
	if goruntime.GOARCH == "arm64" {
		arch = "arm64"
	}

	debVersion := "1.3.6"
	debURL := fmt.Sprintf("https://github.com/apptainer/apptainer/releases/download/v%s/apptainer_%s_%s.deb", debVersion, debVersion, arch)

	if onProgress != nil {
		onProgress(5, fmt.Sprintf("Downloading Apptainer v%s (%s)...", debVersion, arch))
	}

	tmpDeb := filepath.Join(os.TempDir(), fmt.Sprintf("apptainer_%s.deb", debVersion))
	defer os.Remove(tmpDeb)

	req, err := http.NewRequestWithContext(ctx, "GET", debURL, nil)
	if err != nil {
		return fmt.Errorf("invalid download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download server returned HTTP %d for %s", resp.StatusCode, debURL)
	}

	out, err := os.Create(tmpDeb)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	totalSize := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			out.Close()
			return ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				out.Close()
				return writeErr
			}
			downloaded += int64(n)
			if totalSize > 0 && onProgress != nil {
				pct := 5.0 + (float64(downloaded)/float64(totalSize))*70.0
				onProgress(pct, fmt.Sprintf("Downloading Apptainer... (%.1f MB / %.1f MB)", float64(downloaded)/(1024*1024), float64(totalSize)/(1024*1024)))
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			out.Close()
			return readErr
		}
	}
	out.Close()

	if onProgress != nil {
		onProgress(80, "Extracting Apptainer into user directory...")
	}

	// Extract debian package to destDir
	dpkgDeb, err := exec.LookPath("dpkg-deb")
	if err == nil {
		cmd := exec.CommandContext(ctx, dpkgDeb, "-x", tmpDeb, destDir)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("dpkg-deb extraction failed: %w (stderr: %s)", err, stderr.String())
		}
	} else {
		// Fallback: tar / ar extraction
		arCmd, err := exec.LookPath("ar")
		if err != nil {
			return fmt.Errorf("neither dpkg-deb nor ar found on host to unpack .deb package")
		}
		cmd := exec.CommandContext(ctx, arCmd, "x", tmpDeb)
		cmd.Dir = os.TempDir()
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("ar extraction failed: %w", err)
		}
		dataTar := filepath.Join(os.TempDir(), "data.tar.gz")
		if _, err := os.Stat(dataTar); os.IsNotExist(err) {
			dataTar = filepath.Join(os.TempDir(), "data.tar.xz")
		}
		defer os.Remove(dataTar)
		tarCmd := exec.CommandContext(ctx, "tar", "-xf", dataTar, "-C", destDir)
		if err := tarCmd.Run(); err != nil {
			return fmt.Errorf("tar extraction failed: %w", err)
		}
	}

	// Ensure relocatable symlinks (usr/etc -> ../etc, usr/var -> ../var, bin/apptainer -> ../usr/bin/apptainer)
	ensureRelocatableSymlinks(destDir)

	if onProgress != nil {
		onProgress(95, "Verifying Apptainer installation...")
	}

	realBin := filepath.Join(destDir, "usr", "bin", "apptainer")
	linkBin := filepath.Join(destDir, "bin", "apptainer")

	outVer, err := exec.CommandContext(ctx, linkBin, "version").Output()
	if err != nil {
		outVer, err = exec.CommandContext(ctx, realBin, "version").Output()
		if err != nil {
			return fmt.Errorf("installed binary failed verification: %w", err)
		}
	}

	if onProgress != nil {
		onProgress(100, fmt.Sprintf("Apptainer %s installed successfully!", strings.TrimSpace(string(outVer))))
	}

	return nil
}

func (m *linuxHeavyEngineManager) CreateEngine(enableGPU bool) (speedboatRuntime.Engine, error) {
	binPath, _ := findApptainerBinary()
	if binPath == "" {
		return nil, fmt.Errorf("apptainer binary not found; please install it first")
	}

	eng := speedboatRuntime.NewApptainerEngineCustom(binPath)
	eng.SetGPUEnabled(enableGPU)
	return eng, nil
}
