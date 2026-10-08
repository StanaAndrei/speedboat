package builder

import (
	"os"
	"path/filepath"
	"speedboat/internal/protocol"
	"testing"
)

func TestResolveOutputPath(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("empty output path defaults to logic.wasm in projectDir", func(t *testing.T) {
		got := ResolveOutputPath(tmpDir, "")
		expected := filepath.Join(tmpDir, "logic.wasm")
		if got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	})

	t.Run("trailing slash treated as directory", func(t *testing.T) {
		got := ResolveOutputPath(tmpDir, filepath.Join(tmpDir, "dist")+"/")
		expected := filepath.Join(tmpDir, "dist", "logic.wasm")
		if got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	})

	t.Run("existing directory treated as destination directory", func(t *testing.T) {
		subDir := filepath.Join(tmpDir, "out")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatal(err)
		}
		got := ResolveOutputPath(tmpDir, subDir)
		expected := filepath.Join(subDir, "logic.wasm")
		if got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	})

	t.Run("specific file path preserved", func(t *testing.T) {
		target := filepath.Join(tmpDir, "custom.wasm")
		got := ResolveOutputPath(tmpDir, target)
		if got != target {
			t.Errorf("expected %q, got %q", target, got)
		}
	})

	t.Run("relative file path resolved against projectDir", func(t *testing.T) {
		got := ResolveOutputPath(tmpDir, "sub/calc.wasm")
		expected := filepath.Join(tmpDir, "sub/calc.wasm")
		if got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	})
}

func TestResolveSourceFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("error when no source files exist", func(t *testing.T) {
		_, err := ResolveSourceFile(tmpDir, nil)
		if err == nil {
			t.Fatalf("expected error when no source file found, got nil")
		}
	})

	t.Run("finds logic.cpp candidate", func(t *testing.T) {
		cppFile := filepath.Join(tmpDir, "logic.cpp")
		if err := os.WriteFile(cppFile, []byte("// test"), 0644); err != nil {
			t.Fatal(err)
		}
		got, err := ResolveSourceFile(tmpDir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != cppFile {
			t.Errorf("expected %q, got %q", cppFile, got)
		}
		_ = os.Remove(cppFile)
	})

	t.Run("uses custom config source if specified", func(t *testing.T) {
		customFile := filepath.Join(tmpDir, "compute.cc")
		if err := os.WriteFile(customFile, []byte("// compute"), 0644); err != nil {
			t.Fatal(err)
		}
		cfg := &protocol.ProjectConfig{
			Wasm: protocol.WasmConfig{
				Source: "compute.cc",
			},
		}
		got, err := ResolveSourceFile(tmpDir, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != customFile {
			t.Errorf("expected %q, got %q", customFile, got)
		}
		_ = os.Remove(customFile)
	})
}
