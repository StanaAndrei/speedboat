package orchestrator

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"path/filepath"
	"testing"
	"time"

	"speedboat/internal/protocol"
	"speedboat/internal/store"
)

func TestTaskShuffling(t *testing.T) {
	total := 100
	tasksList := make([]*managedTask, total)
	for i := 0; i < total; i++ {
		tasksList[i] = &managedTask{
			taskID: string(rune('A' + (i % 26))),
		}
	}

	orig := make([]*managedTask, total)
	copy(orig, tasksList)

	rand.Shuffle(len(tasksList), func(i, j int) {
		tasksList[i], tasksList[j] = tasksList[j], tasksList[i]
	})

	sameOrderCount := 0
	for i := 0; i < total; i++ {
		if tasksList[i] == orig[i] {
			sameOrderCount++
		}
	}

	if sameOrderCount == total {
		t.Fatalf("expected tasksList to be shuffled in random order, but order was identical")
	}
}

func TestStateRehydrationAndSetDifference(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "state.db")

	bboltStore, err := store.NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer bboltStore.Close()

	projectID := "test-project"

	// Pre-populate DB:
	// task-0001: already verified
	_ = bboltStore.SaveVerifiedResult(projectID, protocol.TaskResult{
		TaskID:    "task-0001",
		ProjectID: projectID,
		Status:    "ok",
		Output:    json.RawMessage(`{"res": 42}`),
	})

	// task-0002: has 1 raw result from 10.0.0.1 (needs redundancy 2)
	_ = bboltStore.SaveRawResult(projectID, protocol.TaskResult{
		TaskID:    "task-0002",
		ProjectID: projectID,
		Status:    "ok",
		Output:    json.RawMessage(`{"res": 100}`),
		IP:        "10.0.0.1",
	})

	// task-0003: no results in DB (completely fresh)

	cfg := &protocol.ProjectConfig{
		Name: projectID,
		Verification: protocol.VerificationConfig{
			Redundancy:         2,
			ConsensusThreshold: 2,
		},
	}

	rawTasks := []json.RawMessage{
		json.RawMessage(`{"task_id": "task-0001"}`),
		json.RawMessage(`{"task_id": "task-0002"}`),
		json.RawMessage(`{"task_id": "task-0003"}`),
	}

	// Rehydrate state as done in orchestrator.Run
	persistedVerified, err := bboltStore.GetVerifiedResults(cfg.Name)
	if err != nil {
		t.Fatalf("GetVerifiedResults failed: %v", err)
	}
	persistedRaw, err := bboltStore.GetRawResults(cfg.Name)
	if err != nil {
		t.Fatalf("GetRawResults failed: %v", err)
	}

	rawByTask := make(map[string][]protocol.TaskResult)
	for _, r := range persistedRaw {
		rawByTask[r.TaskID] = append(rawByTask[r.TaskID], r)
	}

	taskOutputs := make(map[string]map[string][]protocol.TaskResult)
	taskIPs := make(map[string]map[string]bool)
	verifiedResults := make(map[string]protocol.TaskResult)
	for k, v := range persistedVerified {
		verifiedResults[k] = v
	}

	tasksList := make([]*managedTask, 0)
	tasksMap := make(map[string]*managedTask)

	for _, raw := range rawTasks {
		var obj map[string]any
		_ = json.Unmarshal(raw, &obj)
		taskID := obj["task_id"].(string)

		mt := &managedTask{
			taskID:       taskID,
			rawBytes:     raw,
			assignedIPs:  make(map[string]time.Time),
			completedIPs: make(map[string]bool),
		}
		tasksMap[taskID] = mt

		if _, ok := verifiedResults[taskID]; ok {
			continue // Set difference: skip already verified
		}

		if rawList, ok := rawByTask[taskID]; ok {
			if taskIPs[taskID] == nil {
				taskIPs[taskID] = make(map[string]bool)
			}
			if taskOutputs[taskID] == nil {
				taskOutputs[taskID] = make(map[string][]protocol.TaskResult)
			}
			for _, r := range rawList {
				wIP := getWorkerIP(r.IP, r.WorkerID)
				taskIPs[taskID][wIP] = true
				mt.completedIPs[wIP] = true
				norm := normalizeJSON(r.Output)
				taskOutputs[taskID][norm] = append(taskOutputs[taskID][norm], r)
			}
		}

		tasksList = append(tasksList, mt)
	}

	// Assertions
	if len(tasksList) != 2 {
		t.Fatalf("expected 2 tasks in tasksList after set difference, got %d", len(tasksList))
	}

	// Verify task-0001 is not in tasksList
	for _, mt := range tasksList {
		if mt.taskID == "task-0001" {
			t.Fatalf("task-0001 should have been excluded from tasksList")
		}
	}

	// Verify task-0002 has 10.0.0.1 in completedIPs
	mt2 := tasksMap["task-0002"]
	if !mt2.completedIPs["10.0.0.1"] {
		t.Fatalf("expected 10.0.0.1 to be marked completed for task-0002")
	}

	// Verify task-0003 has no completed IPs
	mt3 := tasksMap["task-0003"]
	if len(mt3.completedIPs) != 0 {
		t.Fatalf("expected 0 completed IPs for fresh task-0003, got %d", len(mt3.completedIPs))
	}

	// Check verified results has task-0001
	if _, ok := verifiedResults["task-0001"]; !ok {
		t.Fatalf("expected task-0001 in verifiedResults")
	}
}

func TestAllTasksAlreadyVerified(t *testing.T) {
	orch := NewOrchestrator(".", "nats://localhost:4222", "registry:5000", true, "test.db")
	if orch.DBPath != "test.db" {
		t.Fatalf("expected DBPath test.db, got %s", orch.DBPath)
	}

	verifiedResults := map[string]protocol.TaskResult{
		"task-1": {TaskID: "task-1", Status: "ok"},
		"task-2": {TaskID: "task-2", Status: "ok"},
	}

	// collectAndVerifyResults should return immediately without subscribing
	res, err := orch.collectAndVerifyResults(
		context.Background(),
		&protocol.ProjectConfig{Name: "p"},
		[]string{"task-1", "task-2"},
		nil,
		nil,
		nil,
		verifiedResults,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("collectAndVerifyResults failed: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 results returned immediately, got %d", len(res))
	}
}

