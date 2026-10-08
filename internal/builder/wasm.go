package builder

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"speedboat/internal/protocol"
	"strings"
	"time"
)

type WasmMode string

const (
	WasmModeWASI WasmMode = "wasi"
)

type WasmBuildOptions struct {
	ProjectDir string
	Mode       WasmMode
	OutputPath string
}

// BuildWasm compiles a scientific project to a WebAssembly module using wasi-sdk.
func BuildWasm(ctx context.Context, opts WasmBuildOptions) (string, error) {
	if opts.ProjectDir == "" {
		opts.ProjectDir = "."
	}

	absProjectDir, err := filepath.Abs(opts.ProjectDir)
	if err != nil {
		return "", fmt.Errorf("invalid project directory %q: %w", opts.ProjectDir, err)
	}

	cfg, err := protocol.LoadProjectConfig(absProjectDir)
	if err != nil {
		return "", fmt.Errorf("failed to load project config: %w", err)
	}

	resolvedOutput := ResolveOutputPath(absProjectDir, opts.OutputPath)
	absOutput, err := filepath.Abs(resolvedOutput)
	if err != nil {
		return "", fmt.Errorf("invalid output path %q: %w", resolvedOutput, err)
	}

	mode := opts.Mode
	if mode == "" {
		mode = WasmModeWASI
	}

	switch mode {
	case WasmModeWASI:
		if err := BuildWASI(ctx, absProjectDir, cfg, absOutput); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unknown wasm mode: %q (expected %q)", mode, WasmModeWASI)
	}

	fi, err := os.Stat(absOutput)
	if err != nil {
		return "", fmt.Errorf("build reported success but output wasm not found: %w", err)
	}
	if fi.Size() == 0 {
		return "", fmt.Errorf("built wasm file is empty (0 bytes)")
	}

	return absOutput, nil
}

// ResolveOutputPath determines the final destination path for the .wasm artifact.
func ResolveOutputPath(projectDir, outputPath string) string {
	if outputPath == "" {
		return filepath.Join(projectDir, "logic.wasm")
	}

	if strings.HasSuffix(outputPath, string(filepath.Separator)) || strings.HasSuffix(outputPath, "/") {
		return filepath.Join(outputPath, "logic.wasm")
	}

	if fi, err := os.Stat(outputPath); err == nil && fi.IsDir() {
		return filepath.Join(outputPath, "logic.wasm")
	}

	if !filepath.IsAbs(outputPath) {
		return filepath.Join(projectDir, outputPath)
	}

	return outputPath
}

// ResolveSourceFile finds the C/C++ source file for direct WASI cross-compilation.
func ResolveSourceFile(projectDir string, cfg *protocol.ProjectConfig) (string, error) {
	if cfg != nil && cfg.Wasm.Source != "" {
		path := cfg.Wasm.Source
		if !filepath.IsAbs(path) {
			path = filepath.Join(projectDir, path)
		}
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			return path, nil
		}
		return "", fmt.Errorf("specified wasm source %q not found in project", cfg.Wasm.Source)
	}

	candidates := []string{
		"logic.cpp",
		"logic.cc",
		"logic.c",
		"main.cpp",
		"main.cc",
		"main.c",
	}

	for _, c := range candidates {
		path := filepath.Join(projectDir, c)
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			return path, nil
		}
	}

	return "", fmt.Errorf("no C/C++ source file (e.g., logic.cpp) found in %s for WASI compilation", projectDir)
}

// BuildWASI compiles C/C++ source directly to WASI using an ephemeral compiler container.
func BuildWASI(ctx context.Context, projectDir string, cfg *protocol.ProjectConfig, absOutputPath string) error {
	srcFile, err := ResolveSourceFile(projectDir, cfg)
	if err != nil {
		return err
	}

	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute project directory: %w", err)
	}

	relSrcPath, err := filepath.Rel(absProjectDir, srcFile)
	if err != nil {
		relSrcPath = filepath.Base(srcFile)
	}

	compiler := "/opt/wasi-sdk/bin/clang++"
	isC := strings.HasSuffix(srcFile, ".c")
	if isC {
		compiler = "/opt/wasi-sdk/bin/clang"
	}

	tempWasmName := fmt.Sprintf(".build_tmp_%d.wasm", time.Now().UnixNano())
	tempWasmPath := filepath.Join(absProjectDir, tempWasmName)
	defer os.Remove(tempWasmPath)

	args := []string{"run", "--rm"}
	if uid := os.Getuid(); uid >= 0 {
		args = append(args, "--user", fmt.Sprintf("%d:%d", uid, os.Getgid()))
	}
	args = append(args, "-v", fmt.Sprintf("%s:/src", absProjectDir), "-w", "/src")

	wasiImage := os.Getenv("SPEEDBOAT_WASI_IMAGE")
	if wasiImage == "" {
		wasiImage = "ghcr.io/webassembly/wasi-sdk:latest"
	}
	args = append(args, wasiImage, compiler, "--target=wasm32-wasip1")
	if !isC {
		args = append(args, "-fno-exceptions")
	}
	args = append(args, "-O3", relSrcPath, "-o", tempWasmName)

	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to compile WASI with docker: %w\nstderr: %s\nstdout: %s",
			err, stderr.String(), stdout.String())
	}

	if err := os.MkdirAll(filepath.Dir(absOutputPath), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	if err := moveOrCopyFile(tempWasmPath, absOutputPath); err != nil {
		return fmt.Errorf("failed to place built wasm file at destination: %w", err)
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func moveOrCopyFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}
