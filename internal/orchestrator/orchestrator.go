package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"speedboat/internal/builder"
	"speedboat/internal/protocol"
	"speedboat/internal/transport"

	"github.com/nats-io/nats.go"
)

type Orchestrator struct {
	ProjectDir string
	NatsURL    string
	Registry   string
	NoBuild    bool
	Timeout    time.Duration
	Config     *protocol.ProjectConfig
	nc         *nats.Conn
	js         nats.JetStreamContext
}

func NewOrchestrator(projectDir string, natsURL string, registry string, noBuild bool) *Orchestrator {
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		absDir = projectDir
	}
	return &Orchestrator{
		ProjectDir: absDir,
		NatsURL:    natsURL,
		Registry:   registry,
		NoBuild:    noBuild,
		Timeout:    0, // unlimited by default
	}
}

func (o *Orchestrator) Run(ctx context.Context) error {
	// 1. Load project config
	cfg, err := protocol.LoadProjectConfig(o.ProjectDir)
	if err != nil {
		return fmt.Errorf("failed to load project config: %w", err)
	}
	o.Config = cfg

	fmt.Printf("🚀 Speedboat Master: Project '%s' (v%s)\n", cfg.Name, cfg.Version)
	fmt.Printf("   Redundancy: %d | Consensus Threshold: %d\n",
		cfg.Verification.Redundancy, cfg.Verification.ConsensusThreshold)

	// 2. Build & push container img
	var imageTag string
	if !o.NoBuild {
		fmt.Printf("📦 Step 1/6: Baking Docker image from %s...\n", cfg.Container.Dockerfile)
		tag, err := builder.BuildAndPushImage(ctx, o.ProjectDir, cfg.Container, o.Registry)
		if err != nil {
			return fmt.Errorf("image baking failed: %w", err)
		}
		imageTag = tag
		fmt.Printf("   ✓ Successfully baked and pushed image: %s\n", imageTag)
	} else {
		if o.Registry != "" {
			imageTag = fmt.Sprintf("%s/%s", o.Registry, cfg.Container.ImageTag)
		} else {
			imageTag = cfg.Container.ImageTag
		}
		fmt.Printf("⏩ Step 1/6: Skipping build. Using image tag: %s\n", imageTag)
	}

	// 3. Connect to NATS + setup JetStream
	fmt.Printf("🔌 Step 2/6: Connecting to NATS at %s...\n", o.NatsURL)
	nc, js, err := transport.Connect(o.NatsURL)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	defer nc.Close()
	o.nc = nc
	o.js = js

	fmt.Printf("   ✓ Initializing JetStream WorkQueue stream for project '%s'...\n", cfg.Name)
	if _, err := transport.EnsureTaskStream(js, cfg.Name); err != nil {
		return fmt.Errorf("failed to ensure task stream: %w", err)
	}

	// 4. Run task generator
	fmt.Printf("⚙️  Step 3/6: Running generator script (%s %s)...\n", cfg.Generator.Runtime, cfg.Generator.Source)
	rawTasks, err := o.runGenerator(ctx, cfg)
	if err != nil {
		return fmt.Errorf("generator script failed: %w", err)
	}
	totalTasks := len(rawTasks)
	if totalTasks == 0 {
		return fmt.Errorf("generator produced 0 tasks")
	}
	fmt.Printf("   ✓ Generated %d unique tasks\n", totalTasks)

	return nil
}

func (o *Orchestrator) runGenerator(ctx context.Context, cfg *protocol.ProjectConfig) ([]json.RawMessage, error) {
	scriptPath := cfg.Generator.Source
	if !filepath.IsAbs(scriptPath) {
		scriptPath = filepath.Join(o.ProjectDir, scriptPath)
	}

	runtimeBin := cfg.Generator.Runtime
	if runtimeBin == "" {
		runtimeBin = "python3"
	}

	cmd := exec.CommandContext(ctx, runtimeBin, scriptPath)
	cmd.Dir = o.ProjectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("generator execution failed: %w\nstderr: %s", err, stderr.String())
	}
	var tasks []json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &tasks); err != nil {
		return nil, fmt.Errorf("generator output is not a valid JSON array: %w\noutput: %s", err, stdout.String())
	}
	return tasks, nil
}
