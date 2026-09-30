package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"speedboat/internal/orchestrator"
	"syscall"

	"github.com/alecthomas/kong"
)

type CLI struct {
	Run struct {
		ProjectDir string `short:"p" help:"Path to project directory containing project.yaml" default:"."`
		NatsURL    string `help:"NATS server endpoint" default:"nats://localhost:4222" env:"NATS_URL"`
		Registry   string `help:"Docker registry URL" default:"localhost:5000" env:"DOCKER_REGISTRY"`
		NoBuild    bool   `help:"Skip image building step" default:"false"`
		DB         string `help:"Path to bbolt database file" default:"speedboat.db" env:"DB_PATH"`
		SendDup    bool   `name:"send-dup" help:"Allow orchestrator to send redundant tasks to the same agent IP" default:"false"`
	} `cmd:"" help:"Build image, load tasks, and start orchestrator."`

	Build struct {
		ProjectDir string `short:"p" help:"Path to project directory" default:"."`
		Registry   string `help:"Docker registry URL" default:"localhost:5000" env:"DOCKER_REGISTRY"`
	} `cmd:"" help:"Bake Docker/WASM artifacts without starting the grid."`
}

func run() {

}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("speedboat-master"),
		kong.Description("The master orchestrator for Speedboat, responsible for building and running tasks."),
		kong.UsageOnError(),
	)

	sigCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	switch ctx.Command() {
	case "run":
		orch := orchestrator.NewOrchestrator(
			cli.Run.ProjectDir,
			cli.Run.NatsURL,
			cli.Run.Registry,
			cli.Run.NoBuild,
			cli.Run.DB,
			cli.Run.SendDup,
		)
		if err := orch.Run(sigCtx); err != nil {
			fmt.Fprintf(os.Stderr, "Master orchestrator error: %v\n", err)
			os.Exit(1)
		}
		// Invoke Orchestrator Lifecycle
	case "build":
		fmt.Printf("Building artifacts... %s\n", cli.Build.ProjectDir)
		// Invoke Builder Only

	}
}
