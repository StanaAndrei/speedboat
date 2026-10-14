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

	sqlStore, err := store.NewSQLStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer sqlStore.Close()

	projectID := "test-project"

	// Pre-populate DB:
	// task-0001: already verified
	_ = sqlStore.SaveVerifiedResult(projectID, protocol.TaskResult{
		TaskID:    "task-0001",
		ProjectID: projectID,
		Status:    "ok",
		Output:    json.RawMessage(`{"res": 42}`),
	})

	// task-0002: has 1 raw result from 10.0.0.1 (needs redundancy 2)
	_ = sqlStore.SaveRawResult(projectID, protocol.TaskResult{
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
	persistedVerified, err := sqlStore.GetVerifiedResults(cfg.Name)
	if err != nil {
		t.Fatalf("GetVerifiedResults failed: %v", err)
	}
	persistedRaw, err := sqlStore.GetRawResults(cfg.Name)
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

func TestNewOrchestratorSendDup(t *testing.T) {
	orchDefault := NewOrchestrator(".", "nats://localhost:4222", "registry:5000", true, "test.db")
	if orchDefault.SendDup {
		t.Fatalf("expected SendDup to default to false, got true")
	}

	orchFalse := NewOrchestrator(".", "nats://localhost:4222", "registry:5000", true, "test.db", false)
	if orchFalse.SendDup {
		t.Fatalf("expected SendDup to be false, got true")
	}

	orchTrue := NewOrchestrator(".", "nats://localhost:4222", "registry:5000", true, "test.db", true)
	if !orchTrue.SendDup {
		t.Fatalf("expected SendDup to be true, got false")
	}
}

func TestDispatcherSendDupSelection(t *testing.T) {
	redundancy := 2
	workerIP := "192.168.1.10"
	workerKey := "192.168.1.10-w1"

	selectTask := func(sendDup bool, tasksList []*managedTask, verifiedResults map[string]protocol.TaskResult) *managedTask {
		return selectTaskForWorker(tasksList, verifiedResults, workerIP, workerKey, sendDup, redundancy)
	}

	mt := &managedTask{
		taskID:         "task-0001",
		rawBytes:       []byte(`{"task_id":"task-0001"}`),
		assignedIPs:    make(map[string]time.Time),
		completedIPs:   make(map[string]bool),
		completedCount: 0,
	}
	tasksList := []*managedTask{mt}
	verifiedResults := make(map[string]protocol.TaskResult)

	// Round 1: First assignment to fresh worker
	sel := selectTask(false, tasksList, verifiedResults)
	if sel == nil || sel.taskID != "task-0001" {
		t.Fatalf("expected task-0001 to be selected on round 1")
	}

	// Worker completes round 1
	mt.completedIPs[workerIP] = true
	mt.completedCount = 1

	// Round 2 with sendDup = false (Default): should NOT send duplicate task to same workerIP
	selDefault := selectTask(false, tasksList, verifiedResults)
	if selDefault != nil {
		t.Fatalf("expected nil when sendDup is false and worker already completed the task, got %s", selDefault.taskID)
	}

	// Round 2 with sendDup = true: SHOULD send duplicate task to same workerIP
	selDup := selectTask(true, tasksList, verifiedResults)
	if selDup == nil || selDup.taskID != "task-0001" {
		t.Fatalf("expected task-0001 to be selected when sendDup is true, got %v", selDup)
	}

	// Worker completes round 2
	mt.completedCount = 2

	// Round 3 with sendDup = true: redundancy (2) reached, should NOT select task again
	selLimit := selectTask(true, tasksList, verifiedResults)
	if selLimit != nil {
		t.Fatalf("expected nil when redundancy is reached even with sendDup=true, got %s", selLimit.taskID)
	}
}

func TestConsensusVerificationSendDup(t *testing.T) {
	reqRedundancy := 2
	threshold := 2
	workerIP := "192.168.1.10"
	taskID := "task-0001"

	simulateConsensus := func(sendDup bool, results []protocol.TaskResult) bool {
		taskIPs := make(map[string]bool)
		taskOutputs := make(map[string][]protocol.TaskResult)
		consensusAchieved := false

		for _, res := range results {
			if !sendDup && taskIPs[workerIP] {
				// duplicate result from same IP ignored when sendDup is false
				continue
			}
			taskIPs[workerIP] = true

			norm := normalizeJSON(res.Output)
			taskOutputs[norm] = append(taskOutputs[norm], res)

			matchCount := len(taskOutputs[norm])
			distinctIPCount := len(taskIPs)
			totalCount := 0
			for _, matching := range taskOutputs {
				totalCount += len(matching)
			}

			if sendDup {
				if totalCount >= reqRedundancy && matchCount >= threshold {
					consensusAchieved = true
					break
				}
			} else {
				if distinctIPCount >= reqRedundancy && matchCount >= threshold {
					consensusAchieved = true
					break
				}
			}
		}

		return consensusAchieved
	}

	resultsFromSameIP := []protocol.TaskResult{
		{
			TaskID:    taskID,
			ProjectID: "test-proj",
			Status:    "ok",
			Output:    json.RawMessage(`{"count": 42}`),
			WorkerID:  "192.168.1.10-w1",
			IP:        workerIP,
		},
		{
			TaskID:    taskID,
			ProjectID: "test-proj",
			Status:    "ok",
			Output:    json.RawMessage(`{"count": 42}`),
			WorkerID:  "192.168.1.10-w1",
			IP:        workerIP,
		},
	}

	// When sendDup is false: consensus should NOT be reached with only 1 distinct IP
	if simulateConsensus(false, resultsFromSameIP) {
		t.Fatalf("expected consensus to fail when sendDup is false and results come from same IP")
	}

	// When sendDup is true: consensus SHOULD be reached because 2 matching results arrived
	if !simulateConsensus(true, resultsFromSameIP) {
		t.Fatalf("expected consensus to succeed when sendDup is true and results come from same IP")
	}
}

func TestTrapTaskDispatchAfterConsensus(t *testing.T) {
	redundancy := 3
	worker1 := "192.168.1.11"
	worker2 := "192.168.1.12"
	worker3 := "192.168.1.13"

	selectTask := func(workerIP string, tasksList []*managedTask, verifiedResults map[string]protocol.TaskResult) (*managedTask, bool) {
		isEligible := func(mt *managedTask) bool {
			if mt.completedIPs[workerIP] {
				return false
			}
			activeOther := 0
			for otherIP, ts := range mt.assignedIPs {
				if otherIP != workerIP && time.Since(ts) < 60*time.Second && !mt.completedIPs[otherIP] {
					activeOther++
				}
			}
			return len(mt.completedIPs)+activeOther < redundancy
		}

		// Pass 1: Unverified tasks
		for _, mt := range tasksList {
			if _, verified := verifiedResults[mt.taskID]; !verified {
				if !mt.completedIPs[workerIP] && isEligible(mt) {
					return mt, false
				}
			}
		}

		// Pass 2: Verified tasks as traps
		for _, mt := range tasksList {
			if _, verified := verifiedResults[mt.taskID]; verified {
				if !mt.completedIPs[workerIP] && isEligible(mt) {
					return mt, true
				}
			}
		}
		return nil, false
	}

	mt := &managedTask{
		taskID:       "task-0001",
		rawBytes:     []byte(`{"task_id":"task-0001"}`),
		assignedIPs:  make(map[string]time.Time),
		completedIPs: make(map[string]bool),
	}
	tasksList := []*managedTask{mt}
	verifiedResults := make(map[string]protocol.TaskResult)

	// Worker 1 executes task
	sel, isTrap := selectTask(worker1, tasksList, verifiedResults)
	if sel == nil || isTrap {
		t.Fatalf("expected task-0001 as unverified task for worker1")
	}
	mt.completedIPs[worker1] = true
	mt.completedCount = 1

	// Worker 2 executes task
	sel, isTrap = selectTask(worker2, tasksList, verifiedResults)
	if sel == nil || isTrap {
		t.Fatalf("expected task-0001 as unverified task for worker2")
	}
	mt.completedIPs[worker2] = true
	mt.completedCount = 2

	// Consensus reached! 2 out of 3 agreed.
	verifiedResults["task-0001"] = protocol.TaskResult{
		TaskID: "task-0001",
		Status: "ok",
		Output: json.RawMessage(`{"res": 42}`),
	}

	// Worker 3 requests work: should get task-0001 as a TRAP task!
	sel, isTrap = selectTask(worker3, tasksList, verifiedResults)
	if sel == nil || !isTrap {
		t.Fatalf("expected task-0001 to be selected as TRAP task for worker3, got sel=%v, isTrap=%v", sel, isTrap)
	}

	// Worker 3 completes trap
	mt.completedIPs[worker3] = true
	mt.completedCount = 3

	// Now all 3 redundant runs are complete: no more workers should get the task
	sel, _ = selectTask("192.168.1.14", tasksList, verifiedResults)
	if sel != nil {
		t.Fatalf("expected nil task when redundancy limit (3) is reached, got %s", sel.taskID)
	}
}

func TestCheatingDetectedOnTrapTask(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "trap_test.db")
	sqlStore, err := store.NewSQLStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer sqlStore.Close()

	orch := NewOrchestrator(".", "nats://localhost:4222", "registry:5000", true, dbPath)
	orch.Store = sqlStore

	verifiedResults := map[string]protocol.TaskResult{
		"task-0001": {
			TaskID: "task-0001",
			Status: "ok",
			Output: json.RawMessage(`{"count": 42}`),
		},
	}

	tasksMap := map[string]*managedTask{
		"task-0001": {
			taskID:      "task-0001",
			assignedIPs: map[string]time.Time{"10.0.0.99": time.Now()},
		},
	}

	// Honest worker submits matching output
	honestRes := protocol.TaskResult{
		TaskID: "task-0001",
		Status: "ok",
		Output: json.RawMessage(`{"count": 42}`),
		IP:     "10.0.0.50",
	}
	normHonest := normalizeJSON(honestRes.Output)
	expectedNorm := normalizeJSON(verifiedResults["task-0001"].Output)
	if normHonest != expectedNorm {
		t.Fatalf("expected honest output to match")
	}

	// Cheating worker submits fake output
	cheatRes := protocol.TaskResult{
		TaskID: "task-0001",
		Status: "ok",
		Output: json.RawMessage(`{"count": 999}`),
		IP:     "10.0.0.99",
	}
	normCheat := normalizeJSON(cheatRes.Output)
	if normCheat != expectedNorm {
		orch.blacklistIP(cheatRes.IP, "Trap output mismatch", tasksMap)
	}

	// Verify blacklisting
	if !orch.isIPBlacklisted("10.0.0.99") {
		t.Fatalf("expected cheating IP 10.0.0.99 to be blacklisted in memory")
	}
	isBL, err := sqlStore.IsIPBlacklisted("10.0.0.99")
	if err != nil || !isBL {
		t.Fatalf("expected cheating IP 10.0.0.99 to be blacklisted in DB")
	}
	if orch.isIPBlacklisted("10.0.0.50") {
		t.Fatalf("honest IP 10.0.0.50 should not be blacklisted")
	}
}

func TestMinorityCheaterBlacklistedBeforeConsensus(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "minority_test.db")
	sqlStore, err := store.NewSQLStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer sqlStore.Close()

	orch := NewOrchestrator(".", "nats://localhost:4222", "registry:5000", true, dbPath)
	orch.Store = sqlStore

	taskID := "task-0001"
	threshold := 2
	taskOutputs := make(map[string][]protocol.TaskResult)

	// Step 1: Worker 1 (10.0.0.1) submits correct answer 42
	res1 := protocol.TaskResult{TaskID: taskID, Status: "ok", Output: json.RawMessage(`{"val": 42}`), IP: "10.0.0.1"}
	taskOutputs[normalizeJSON(res1.Output)] = append(taskOutputs[normalizeJSON(res1.Output)], res1)

	// Step 2: Worker 2 (10.0.0.2) submits fake answer 999
	res2 := protocol.TaskResult{TaskID: taskID, Status: "ok", Output: json.RawMessage(`{"val": 999}`), IP: "10.0.0.2"}
	taskOutputs[normalizeJSON(res2.Output)] = append(taskOutputs[normalizeJSON(res2.Output)], res2)

	// Check: Neither has 2 votes. No consensus yet, no blacklisting yet.
	if len(taskOutputs[normalizeJSON(res1.Output)]) >= threshold || len(taskOutputs[normalizeJSON(res2.Output)]) >= threshold {
		t.Fatalf("unexpected early consensus")
	}
	if orch.isIPBlacklisted("10.0.0.1") || orch.isIPBlacklisted("10.0.0.2") {
		t.Fatalf("neither worker should be blacklisted before consensus")
	}

	// Step 3: Worker 3 (10.0.0.3) submits answer 42
	res3 := protocol.TaskResult{TaskID: taskID, Status: "ok", Output: json.RawMessage(`{"val": 42}`), IP: "10.0.0.3"}
	consensusNorm := normalizeJSON(res3.Output)
	taskOutputs[consensusNorm] = append(taskOutputs[consensusNorm], res3)

	// Now answer 42 has 2 votes >= threshold 2. Consensus reached!
	if len(taskOutputs[consensusNorm]) < threshold {
		t.Fatalf("expected consensus on 42")
	}

	// Minority resolution: any other submitted output is in the minority and blacklisted
	for normOut, list := range taskOutputs {
		if normOut != consensusNorm {
			for _, badRes := range list {
				orch.blacklistIP(badRes.IP, "Minority divergence from consensus")
			}
		}
	}

	// Verify Worker 2 (10.0.0.2) is blacklisted
	if !orch.isIPBlacklisted("10.0.0.2") {
		t.Fatalf("expected minority worker 10.0.0.2 to be blacklisted in memory")
	}
	isBL, err := sqlStore.IsIPBlacklisted("10.0.0.2")
	if err != nil || !isBL {
		t.Fatalf("expected minority worker 10.0.0.2 to be blacklisted in DB")
	}

	// Verify Worker 1 and Worker 3 are NOT blacklisted
	if orch.isIPBlacklisted("10.0.0.1") {
		t.Fatalf("worker 1 should not be blacklisted")
	}
	if orch.isIPBlacklisted("10.0.0.3") {
		t.Fatalf("worker 3 should not be blacklisted")
	}
}

func TestLoadProjectConfigIterations(t *testing.T) {
	cfg, err := protocol.LoadProjectConfig("../../examples/prime-cnt")
	if err != nil {
		t.Fatalf("failed to load prime-cnt project config: %v", err)
	}
	if cfg.Verification.Iterations != 5 {
		t.Fatalf("expected iterations to be 5, got %d", cfg.Verification.Iterations)
	}
}

func TestRedundancyEscalationMultiplication(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "escalate_test.db")
	sqlStore, err := store.NewSQLStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer sqlStore.Close()

	orch := NewOrchestrator(".", "nats://localhost:4222", "registry:5000", true, dbPath)
	orch.Store = sqlStore

	baseRedundancy := 3
	baseThreshold := 2
	maxIterations := 3

	taskID := "task-0001"
	mt := &managedTask{
		taskID:       taskID,
		rawBytes:     []byte(`{"task_id":"task-0001"}`),
		assignedIPs:  make(map[string]time.Time),
		completedIPs: make(map[string]bool),
		iteration:    1,
	}

	taskOutputs := make(map[string][]protocol.TaskResult)

	// Step 1: Iteration 1 (R=3, C=2). Three workers return different outputs: A, B, C
	outputs := []string{`{"val": "A"}`, `{"val": "B"}`, `{"val": "C"}`}
	workers := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}

	for i := 0; i < 3; i++ {
		res := protocol.TaskResult{
			TaskID: taskID, Status: "ok", Output: json.RawMessage(outputs[i]), IP: workers[i],
		}
		norm := normalizeJSON(res.Output)
		taskOutputs[norm] = append(taskOutputs[norm], res)
		mt.completedIPs[workers[i]] = true
		mt.completedCount++
	}

	// Check: All 3 completed, none reached threshold 2 * 1 = 2
	effThresh1 := baseThreshold * mt.iteration
	effRed1 := baseRedundancy * mt.iteration
	for _, m := range taskOutputs {
		if len(m) >= effThresh1 {
			t.Fatalf("unexpected consensus in iteration 1")
		}
	}
	if mt.completedCount < effRed1 {
		t.Fatalf("expected completedCount == 3")
	}

	// Escalate to iteration 2!
	if mt.iteration < maxIterations {
		mt.iteration++
	}
	if mt.iteration != 2 {
		t.Fatalf("expected iteration to escalate to 2, got %d", mt.iteration)
	}

	// Effective redundancy is now 3 * 2 = 6, consensus threshold is 2 * 2 = 4
	effThresh2 := baseThreshold * mt.iteration
	effRed2 := baseRedundancy * mt.iteration
	if effThresh2 != 4 || effRed2 != 6 {
		t.Fatalf("expected effThreshold=4, effRedundancy=6, got %d and %d", effThresh2, effRed2)
	}

	// Task should now be eligible for 3 more workers (up to 6 completions)
	isEligible := func(workerIP string) bool {
		return !mt.completedIPs[workerIP] && mt.completedCount < effRed2
	}
	if !isEligible("10.0.0.4") {
		t.Fatalf("expected new worker 10.0.0.4 to be eligible in iteration 2")
	}

	// Step 2: In iteration 2, workers 4, 5, 6 all return output "A"
	newWorkers := []string{"10.0.0.4", "10.0.0.5", "10.0.0.6"}
	for _, w := range newWorkers {
		res := protocol.TaskResult{
			TaskID: taskID, Status: "ok", Output: json.RawMessage(`{"val": "A"}`), IP: w,
		}
		norm := normalizeJSON(res.Output)
		taskOutputs[norm] = append(taskOutputs[norm], res)
		mt.completedIPs[w] = true
		mt.completedCount++
	}

	// Now output "A" has 1 (from iter 1) + 3 (from iter 2) = 4 votes!
	normA := normalizeJSON(json.RawMessage(`{"val": "A"}`))
	if len(taskOutputs[normA]) < effThresh2 {
		t.Fatalf("expected consensus on A with 4 votes, got %d", len(taskOutputs[normA]))
	}

	// Consensus achieved on A! Resolve minorities across ALL iterations:
	for norm, list := range taskOutputs {
		if norm != normA {
			for _, badRes := range list {
				orch.blacklistIP(badRes.IP, "Minority divergence in escalation", map[string]*managedTask{taskID: mt})
			}
		}
	}

	// Worker 2 (B) and Worker 3 (C) from iteration 1 MUST be blacklisted!
	if !orch.isIPBlacklisted("10.0.0.2") {
		t.Fatalf("expected worker 2 (10.0.0.2) to be blacklisted")
	}
	if !orch.isIPBlacklisted("10.0.0.3") {
		t.Fatalf("expected worker 3 (10.0.0.3) to be blacklisted")
	}

	// Worker 1 (A from iter 1) and Workers 4, 5, 6 (A from iter 2) must NOT be blacklisted!
	if orch.isIPBlacklisted("10.0.0.1") || orch.isIPBlacklisted("10.0.0.4") {
		t.Fatalf("honest workers who agreed with consensus should not be blacklisted")
	}
}

type mockEngine struct {
	output []byte
	err    error
}

func (m *mockEngine) Name() string                                              { return "docker" }
func (m *mockEngine) IsAvailable() bool                                         { return true }
func (m *mockEngine) PullImage(ctx context.Context, image string) error         { return nil }
func (m *mockEngine) Execute(ctx context.Context, image string, input []byte) ([]byte, error) {
	return m.output, m.err
}

func TestMasterLocalExecutionFallbackAndBlacklist(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "fallback_test.db")
	sqlStore, err := store.NewSQLStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer sqlStore.Close()

	orch := NewOrchestrator(".", "nats://localhost:4222", "registry:5000", true, dbPath)
	orch.Store = sqlStore
	orch.Config = &protocol.ProjectConfig{Name: "prime-counter"}

	// Inject mock docker engine that returns ground truth {"val": 42}
	orch.Engine = &mockEngine{
		output: []byte(`{"val": 42}`),
	}

	taskID := "task-0001"
	mt := &managedTask{
		taskID:       taskID,
		rawBytes:     []byte(`{"task_id":"task-0001"}`),
		assignedIPs:  make(map[string]time.Time),
		completedIPs: make(map[string]bool),
		iteration:    1,
	}

	// 3 workers disagree:
	// Worker 1: {"val": 42} (matches true math)
	// Worker 2: {"val": 999} (liar)
	// Worker 3: {"val": 888} (liar)
	taskOutputs := make(map[string][]protocol.TaskResult)
	res1 := protocol.TaskResult{TaskID: taskID, Status: "ok", Output: json.RawMessage(`{"val": 42}`), IP: "10.0.0.1"}
	res2 := protocol.TaskResult{TaskID: taskID, Status: "ok", Output: json.RawMessage(`{"val": 999}`), IP: "10.0.0.2"}
	res3 := protocol.TaskResult{TaskID: taskID, Status: "ok", Output: json.RawMessage(`{"val": 888}`), IP: "10.0.0.3"}
	taskOutputs[normalizeJSON(res1.Output)] = append(taskOutputs[normalizeJSON(res1.Output)], res1)
	taskOutputs[normalizeJSON(res2.Output)] = append(taskOutputs[normalizeJSON(res2.Output)], res2)
	taskOutputs[normalizeJSON(res3.Output)] = append(taskOutputs[normalizeJSON(res3.Output)], res3)

	// Max iterations is 1, so iteration 1 exhaustion triggers local fallback
	localRes, err := orch.executeTaskLocally(context.Background(), mt.rawBytes, "prime-counter:latest")
	if err != nil {
		t.Fatalf("executeTaskLocally failed: %v", err)
	}

	truthNorm := normalizeJSON(localRes.Output)
	if truthNorm != `{"val":42}` {
		t.Fatalf("unexpected local truth: %s", truthNorm)
	}

	// Blacklist any workers whose output differed from local truth
	for norm, list := range taskOutputs {
		if norm != truthNorm {
			for _, badRes := range list {
				orch.blacklistIP(badRes.IP, "Diverged from master local ground truth", map[string]*managedTask{taskID: mt})
			}
		}
	}

	// Worker 2 and 3 MUST be blacklisted
	if !orch.isIPBlacklisted("10.0.0.2") {
		t.Fatalf("expected liar worker 10.0.0.2 to be blacklisted")
	}
	if !orch.isIPBlacklisted("10.0.0.3") {
		t.Fatalf("expected liar worker 10.0.0.3 to be blacklisted")
	}
	isBL, err := sqlStore.IsIPBlacklisted("10.0.0.2")
	if err != nil || !isBL {
		t.Fatalf("expected liar worker 10.0.0.2 to be blacklisted in DB")
	}

	// Worker 1 agreed with local truth: must NOT be blacklisted
	if orch.isIPBlacklisted("10.0.0.1") {
		t.Fatalf("worker 1 should not be blacklisted")
	}
}

func TestMultiThreadSameIPNoDuplicateTask(t *testing.T) {
	redundancy := 2
	workerIP := "192.168.1.138"
	thread1 := "192.168.1.138-rand-w1"
	thread2 := "192.168.1.138-rand-w2"

	mt1 := &managedTask{
		taskID:       "task-0001",
		rawBytes:     []byte(`{"task_id":"task-0001"}`),
		assignedIPs:  make(map[string]time.Time),
		completedIPs: make(map[string]bool),
	}
	mt2 := &managedTask{
		taskID:       "task-0002",
		rawBytes:     []byte(`{"task_id":"task-0002"}`),
		assignedIPs:  make(map[string]time.Time),
		completedIPs: make(map[string]bool),
	}
	tasksList := []*managedTask{mt1, mt2}
	verifiedResults := make(map[string]protocol.TaskResult)

	// Thread 1 asks for a task -> gets task-0001
	sel1 := selectTaskForWorker(tasksList, verifiedResults, workerIP, thread1, false, redundancy)
	if sel1 == nil || sel1.taskID != "task-0001" {
		t.Fatalf("expected thread 1 to get task-0001, got %v", sel1)
	}
	sel1.assignedIPs[thread1] = time.Now()

	// Thread 2 (same IP) asks for a task -> must NOT get task-0001! Must get task-0002!
	sel2 := selectTaskForWorker(tasksList, verifiedResults, workerIP, thread2, false, redundancy)
	if sel2 == nil || sel2.taskID != "task-0002" {
		t.Fatalf("expected thread 2 to get task-0002, got %v", sel2)
	}
	sel2.assignedIPs[thread2] = time.Now()

	// Thread 3 asks for a task -> no more tasks left for this IP (both in flight by this IP)
	thread3 := "192.168.1.138-rand-w3"
	sel3 := selectTaskForWorker(tasksList, verifiedResults, workerIP, thread3, false, redundancy)
	if sel3 != nil {
		t.Fatalf("expected nil when all tasks are currently in flight by same IP, got %s", sel3.taskID)
	}
}





