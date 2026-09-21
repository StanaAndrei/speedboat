package store

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"speedboat/internal/protocol"
)

func TestBboltStore(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create BboltStore: %v", err)
	}

	projectID := "test-project"

	// 1. Initially empty
	raw, err := store.GetRawResults(projectID)
	if err != nil {
		t.Fatalf("GetRawResults failed: %v", err)
	}
	if len(raw) != 0 {
		t.Fatalf("expected 0 raw results, got %d", len(raw))
	}

	verified, err := store.GetVerifiedResults(projectID)
	if err != nil {
		t.Fatalf("GetVerifiedResults failed: %v", err)
	}
	if len(verified) != 0 {
		t.Fatalf("expected 0 verified results, got %d", len(verified))
	}

	// 2. Save raw results
	res1 := protocol.TaskResult{
		TaskID:    "task-0001",
		ProjectID: projectID,
		Status:    "ok",
		Output:    json.RawMessage(`{"primes": [2, 3, 5], "count": 3}`),
		WorkerID:  "worker-1",
		IP:        "192.168.1.10",
	}
	res2 := protocol.TaskResult{
		TaskID:    "task-0001",
		ProjectID: projectID,
		Status:    "ok",
		Output:    json.RawMessage(`{"primes": [2, 3, 5], "count": 3}`),
		WorkerID:  "worker-2",
		IP:        "192.168.1.11",
	}

	if err := store.SaveRawResult(projectID, res1); err != nil {
		t.Fatalf("SaveRawResult res1 failed: %v", err)
	}
	if err := store.SaveRawResult(projectID, res2); err != nil {
		t.Fatalf("SaveRawResult res2 failed: %v", err)
	}

	raw, err = store.GetRawResults(projectID)
	if err != nil {
		t.Fatalf("GetRawResults failed: %v", err)
	}
	if len(raw) != 2 {
		t.Fatalf("expected 2 raw results, got %d", len(raw))
	}

	// 3. Save verified result
	if err := store.SaveVerifiedResult(projectID, res1); err != nil {
		t.Fatalf("SaveVerifiedResult failed: %v", err)
	}

	verified, err = store.GetVerifiedResults(projectID)
	if err != nil {
		t.Fatalf("GetVerifiedResults failed: %v", err)
	}
	if len(verified) != 1 {
		t.Fatalf("expected 1 verified result, got %d", len(verified))
	}
	var expectedOut, actualOut any
	_ = json.Unmarshal(res1.Output, &expectedOut)
	_ = json.Unmarshal(verified["task-0001"].Output, &actualOut)
	expBytes, _ := json.Marshal(expectedOut)
	actBytes, _ := json.Marshal(actualOut)
	if string(expBytes) != string(actBytes) {
		t.Fatalf("expected output %s, got %s", string(expBytes), string(actBytes))
	}

	// 4. Project isolation
	otherRaw, err := store.GetRawResults("other-project")
	if err != nil {
		t.Fatalf("GetRawResults for other-project failed: %v", err)
	}
	if len(otherRaw) != 0 {
		t.Fatalf("expected 0 raw results for other-project, got %d", len(otherRaw))
	}

	// 5. Close and reopen to test persistence across crash/restart
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reopenedStore, err := NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("reopening store failed: %v", err)
	}
	defer reopenedStore.Close()

	reopenedVerified, err := reopenedStore.GetVerifiedResults(projectID)
	if err != nil {
		t.Fatalf("GetVerifiedResults on reopened store failed: %v", err)
	}
	if len(reopenedVerified) != 1 {
		t.Fatalf("expected 1 verified result on reopened store, got %d", len(reopenedVerified))
	}
	if reopenedVerified["task-0001"].TaskID != "task-0001" {
		t.Fatalf("unexpected task ID: %s", reopenedVerified["task-0001"].TaskID)
	}
}
