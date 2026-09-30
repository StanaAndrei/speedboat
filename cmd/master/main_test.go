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
