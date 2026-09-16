package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"speedboat/internal/runtime"
	"syscall"

	"speedboat/internal/agent"

	"github.com/alecthomas/kong"
)

type CLI struct {
	Run struct {
		ProjectID string `short:"p" help:"Project ID to subscribe to" default:"prime-counter" env:"PROJECT_ID"`
		NatsURL   string `help:"NATS server endpoint" default:"nats://localhost:4222" env:"NATS_URL"`
		Runtime   string `short:"r" help:"Execution runtime engine (auto, docker, apptainer, wasm)" default:"auto" env:"RUNTIME"`
		WorkerID  string `help:"Unique worker ID (defaults to host-random)" default:"" env:"WORKER_ID"`
		MaxTasks    int    `help:"Maximum number of tasks to execute (0 for unlimited)" default:"0"`
		Once        bool   `help:"Exit after task queue becomes empty" default:"false"`
		IdleTimeout int    `short:"t" help:"Time in seconds to wait without receiving work before exiting (0 for unlimited)" default:"30" env:"IDLE_TIMEOUT"`
	} `cmd:"" default:"1" help:"Run the volunteer agent worker."`

	Info struct {
		Runtime string `short:"r" help:"Execution runtime engine (auto, docker, apptainer, wasm)" default:"auto"`
	} `cmd:"" help:"Check agent environment and runtime capabilities."`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("speedboat-agent"),
		kong.Description("The volunteer client for Speedboat, responsible for executing tasks."),
		kong.UsageOnError(),
	)
	sigCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	switch ctx.Command() {
	case "run":
		ag := agent.NewAgent(
			cli.Run.ProjectID,
			cli.Run.NatsURL,
			cli.Run.Runtime,
			cli.Run.WorkerID,
			cli.Run.MaxTasks,
			cli.Run.Once,
			cli.Run.IdleTimeout,
		)
		if err := ag.Run(sigCtx); err != nil {
			fmt.Fprintf(os.Stderr, "Agent error: %v\n", err)
			os.Exit(1)
		}

	case "info":
		fmt.Println("🚤 Speedboat Agent System Capabilities:")
		eng, err := runtime.AutoDetectEngine(cli.Info.Runtime)
		if err != nil {
			fmt.Printf("   Runtime: None available (%v)\n", err)
		} else {
			fmt.Printf("   Runtime: %s (Available: %v)\n", eng.Name(), eng.IsAvailable())
		}
	}
}
