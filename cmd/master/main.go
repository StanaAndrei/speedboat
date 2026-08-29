package main

import (
	"fmt"
	"github.com/alecthomas/kong"
)

type CLI struct {
	Run struct {
		ProjectDir string `short:"p" help:"Path to project directory containing project.yaml" default:"."`
		NatsURL    string `help:"NATS server endpoint" default:"nats://localhost:4222" env:"NATS_URL"`
		Registry   string `help:"Docker registry URL" default:"localhost:5000" env:"DOCKER_REGISTRY"`
		NoBuild    bool   `help:"Skip image building step" default:"false"`
	} `cmd:"" help:"Build image, load tasks, and start orchestrator."`

	Build struct {
		ProjectDir string `short:"p" help:"Path to project directory" default:"."`
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

	switch ctx.Command() {
	case "run":
		fmt.Println("Running orchestrator...")
		// Invoke Orchestrator Lifecycle
	case "build":
		fmt.Println("Building artifacts... %s", cli.Build.ProjectDir)
		// Invoke Builder Only

	}
}