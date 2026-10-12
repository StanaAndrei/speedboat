package main

import (
	"testing"

	"github.com/alecthomas/kong"
)

func TestMasterCLISendDupFlag(t *testing.T) {
	t.Run("default is false when omitted", func(t *testing.T) {
		var cli CLI
		parser, err := kong.New(&cli)
		if err != nil {
			t.Fatalf("failed to create kong parser: %v", err)
		}
		_, err = parser.Parse([]string{"run"})
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if cli.Run.SendDup {
			t.Fatalf("expected SendDup to default to false, got true")
		}
	})

	t.Run("presence sets SendDup to true without value", func(t *testing.T) {
		var cli CLI
		parser, err := kong.New(&cli)
		if err != nil {
			t.Fatalf("failed to create kong parser: %v", err)
		}
		_, err = parser.Parse([]string{"run", "--send-dup"})
		if err != nil {
			t.Fatalf("unexpected parse error with --send-dup: %v", err)
		}
		if !cli.Run.SendDup {
			t.Fatalf("expected SendDup to be true when --send-dup is provided, got false")
		}
	})
}

func TestMasterCLIBuildWasmCommand(t *testing.T) {
	t.Run("default flags on build-wasm", func(t *testing.T) {
		var cli CLI
		parser, err := kong.New(&cli)
		if err != nil {
			t.Fatalf("failed to create kong parser: %v", err)
		}
		kctx, err := parser.Parse([]string{"build-wasm"})
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if kctx.Command() != "build-wasm" {
			t.Fatalf("expected command build-wasm, got %s", kctx.Command())
		}
		if cli.BuildWasm.ProjectDir != "." {
			t.Errorf("expected default project dir '.', got %q", cli.BuildWasm.ProjectDir)
		}
		if !cli.BuildWasm.WASI {
			t.Errorf("expected default WASI to be true, got false")
		}
		if cli.BuildWasm.Output != "" {
			t.Errorf("expected default Output to be empty, got %q", cli.BuildWasm.Output)
		}
	})

	t.Run("parse with explicit flags and custom output", func(t *testing.T) {
		var cli CLI
		parser, err := kong.New(&cli)
		if err != nil {
			t.Fatalf("failed to create kong parser: %v", err)
		}
		_, err = parser.Parse([]string{"build-wasm", "-p", "examples/prime-cnt", "--wasi", "-o", "dist/custom.wasm"})
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if cli.BuildWasm.ProjectDir != "examples/prime-cnt" {
			t.Errorf("expected project dir 'examples/prime-cnt', got %q", cli.BuildWasm.ProjectDir)
		}
		if !cli.BuildWasm.WASI {
			t.Errorf("expected WASI to be true, got false")
		}
		if cli.BuildWasm.Output != "dist/custom.wasm" {
			t.Errorf("expected output 'dist/custom.wasm', got %q", cli.BuildWasm.Output)
		}
	})
}

func TestMasterCLIRunWasmFlags(t *testing.T) {
	t.Run("default wasm flags when omitted", func(t *testing.T) {
		var cli CLI
		parser, err := kong.New(&cli)
		if err != nil {
			t.Fatalf("failed to create kong parser: %v", err)
		}
		_, err = parser.Parse([]string{"run"})
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if cli.Run.WasmPath != "" {
			t.Errorf("expected WasmPath to be empty, got %q", cli.Run.WasmPath)
		}
		if cli.Run.WebPort != 3000 {
			t.Errorf("expected WebPort to default to 3000, got %d", cli.Run.WebPort)
		}
		if cli.Run.NatsWsURL != "" {
			t.Errorf("expected NatsWsURL to be empty, got %q", cli.Run.NatsWsURL)
		}
	})

	t.Run("parse with custom wasm flags", func(t *testing.T) {
		var cli CLI
		parser, err := kong.New(&cli)
		if err != nil {
			t.Fatalf("failed to create kong parser: %v", err)
		}
		_, err = parser.Parse([]string{"run", "--wasm", "dist/logic.wasm", "--web-port", "8088", "--nats-ws", "ws://1.2.3.4:9222"})
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if cli.Run.WasmPath != "dist/logic.wasm" {
			t.Errorf("expected WasmPath to be 'dist/logic.wasm', got %q", cli.Run.WasmPath)
		}
		if cli.Run.WebPort != 8088 {
			t.Errorf("expected WebPort to be 8088, got %d", cli.Run.WebPort)
		}
		if cli.Run.NatsWsURL != "ws://1.2.3.4:9222" {
			t.Errorf("expected NatsWsURL to be 'ws://1.2.3.4:9222', got %q", cli.Run.NatsWsURL)
		}
	})
}


