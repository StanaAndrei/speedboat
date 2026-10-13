package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"

	"speedboat/internal/protocol"
)

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

func (o *Orchestrator) runReducer(
	ctx context.Context,
	cfg *protocol.ProjectConfig,
	taskOrder []string,
	verifiedResults map[string]protocol.TaskResult,
) ([]byte, error) {
	scriptPath := cfg.Reducer.Source
	if !filepath.IsAbs(scriptPath) {
		scriptPath = filepath.Join(o.ProjectDir, scriptPath)
	}

	runtimeBin := cfg.Reducer.Runtime
	if runtimeBin == "" {
		runtimeBin = "python3"
	}

	orderedList := make([]protocol.TaskResult, 0, len(taskOrder))
	for _, taskID := range taskOrder {
		if res, ok := verifiedResults[taskID]; ok {
			orderedList = append(orderedList, res)
		}
	}

	batch := protocol.ReducerBatch{
		ProjectID: cfg.Name,
		Results:   orderedList,
	}
	batchBytes, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reducer batch: %w", err)
	}

	cmd := exec.CommandContext(ctx, runtimeBin, scriptPath)
	cmd.Dir = o.ProjectDir
	cmd.Stdin = bytes.NewReader(batchBytes)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("reducer execution failed: %w\nstderr: %s\nstdout: %s",
			err, stderr.String(), stdout.String())
	}

	return stdout.Bytes(), nil
}
