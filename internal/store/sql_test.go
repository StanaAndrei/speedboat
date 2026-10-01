package store

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"speedboat/internal/protocol"
)

func TestDetectDialect(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantDriver  string
		wantDSNPart string
	}{
		{
			name:        "empty defaults to speedboat.db sqlite",
			input:       "",
			wantDriver:  "sqlite",
			wantDSNPart: "speedboat.db",
		},
		{
			name:        "sqlite relative file path",
			input:       "my_experiment.db",
			wantDriver:  "sqlite",
			wantDSNPart: "my_experiment.db",
		},
		{
			name:        "sqlite scheme prefix",
			input:       "sqlite:///tmp/test.db",
			wantDriver:  "sqlite",
			wantDSNPart: "/tmp/test.db",
		},
		{
			name:        "sqlite in-memory",
			input:       ":memory:",
			wantDriver:  "sqlite",
			wantDSNPart: ":memory:",
		},
		{
			name:        "postgres URL",
			input:       "postgres://user:pass@localhost:5432/speedboat?sslmode=disable",
			wantDriver:  "postgres",
			wantDSNPart: "postgres://user:pass@localhost:5432/speedboat?sslmode=disable",
		},
		{
			name:        "postgresql URL",
			input:       "postgresql://user:pass@db.example.com:5432/speedboat",
			wantDriver:  "postgres",
			wantDSNPart: "postgresql://user:pass@db.example.com:5432/speedboat",
		},
		{
			name:        "mysql URL standard conversion",
			input:       "mysql://user:pass@127.0.0.1:3306/speedboat",
			wantDriver:  "mysql",
			wantDSNPart: "user:pass@tcp(127.0.0.1:3306)/speedboat?parseTime=true",
		},
		{
			name:        "mysql tcp format",
			input:       "user:pass@tcp(localhost:3306)/speedboat?parseTime=true",
			wantDriver:  "mysql",
			wantDSNPart: "user:pass@tcp(localhost:3306)/speedboat?parseTime=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver, dsn := DetectDialect(tt.input)
			if driver != tt.wantDriver {
				t.Errorf("DetectDialect(%q) driver = %v, want %v", tt.input, driver, tt.wantDriver)
			}
			if dsn != tt.wantDSNPart {
				t.Errorf("DetectDialect(%q) dsn = %v, want %v", tt.input, dsn, tt.wantDSNPart)
			}
		})
	}
}

func TestSQLStore(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewSQLStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create SQLStore: %v", err)
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

	// Update raw result (same worker and IP, new status or output)
	res1Updated := res1
	res1Updated.Output = json.RawMessage(`{"primes": [2, 3, 5, 7], "count": 4}`)
	if err := store.SaveRawResult(projectID, res1Updated); err != nil {
		t.Fatalf("SaveRawResult res1Updated failed: %v", err)
	}
	raw, err = store.GetRawResults(projectID)
	if err != nil {
		t.Fatalf("GetRawResults after update failed: %v", err)
	}
	if len(raw) != 2 {
		t.Fatalf("expected still 2 raw results after upsert, got %d", len(raw))
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

	// Upsert verified result
	res1VerifiedUpdate := res1
	res1VerifiedUpdate.Output = json.RawMessage(`{"primes": [2, 3, 5, 7], "count": 4}`)
	if err := store.SaveVerifiedResult(projectID, res1VerifiedUpdate); err != nil {
		t.Fatalf("SaveVerifiedResult update failed: %v", err)
	}
	verified, err = store.GetVerifiedResults(projectID)
	if err != nil {
		t.Fatalf("GetVerifiedResults failed: %v", err)
	}
	if len(verified) != 1 {
		t.Fatalf("expected 1 verified result after upsert, got %d", len(verified))
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

	reopenedStore, err := NewSQLStore(dbPath)
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

func TestSQLStoreInMemory(t *testing.T) {
	store, err := NewSQLStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory SQLStore: %v", err)
	}
	defer store.Close()

	res := protocol.TaskResult{
		TaskID:    "t-1",
		ProjectID: "p-1",
		Status:    "ok",
		Output:    json.RawMessage(`{"status": "done"}`),
	}
	if err := store.SaveVerifiedResult("p-1", res); err != nil {
		t.Fatalf("SaveVerifiedResult failed: %v", err)
	}

	verified, err := store.GetVerifiedResults("p-1")
	if err != nil {
		t.Fatalf("GetVerifiedResults failed: %v", err)
	}
	if len(verified) != 1 || verified["t-1"].TaskID != "t-1" {
		t.Fatalf("unexpected verified results: %+v", verified)
	}
}

func TestBlacklistedIPs(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_blacklist.db")

	store, err := NewSQLStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create SQLStore: %v", err)
	}

	// 1. Initially empty
	bl, err := store.GetBlacklistedIPs()
	if err != nil {
		t.Fatalf("GetBlacklistedIPs failed: %v", err)
	}
	if len(bl) != 0 {
		t.Fatalf("expected 0 blacklisted IPs, got %d", len(bl))
	}

	isBL, err := store.IsIPBlacklisted("192.168.1.100")
	if err != nil {
		t.Fatalf("IsIPBlacklisted failed: %v", err)
	}
	if isBL {
		t.Fatalf("expected 192.168.1.100 not to be blacklisted")
	}

	// 2. Blacklist IP
	if err := store.BlacklistIP("192.168.1.100", "Cheating detected on trap task"); err != nil {
		t.Fatalf("BlacklistIP failed: %v", err)
	}
	if err := store.BlacklistIP("10.0.0.50", "Minority result differing from consensus"); err != nil {
		t.Fatalf("BlacklistIP failed: %v", err)
	}

	isBL, err = store.IsIPBlacklisted("192.168.1.100")
	if err != nil || !isBL {
		t.Fatalf("expected 192.168.1.100 to be blacklisted, err: %v", err)
	}

	isBL, err = store.IsIPBlacklisted("10.0.0.50")
	if err != nil || !isBL {
		t.Fatalf("expected 10.0.0.50 to be blacklisted, err: %v", err)
	}

	isBL, err = store.IsIPBlacklisted("10.0.0.99")
	if err != nil || isBL {
		t.Fatalf("expected 10.0.0.99 not to be blacklisted")
	}

	bl, err = store.GetBlacklistedIPs()
	if err != nil {
		t.Fatalf("GetBlacklistedIPs failed: %v", err)
	}
	if len(bl) != 2 {
		t.Fatalf("expected 2 blacklisted IPs, got %d", len(bl))
	}

	// 3. Upsert / update reason on same IP (idempotent)
	if err := store.BlacklistIP("192.168.1.100", "Updated reason"); err != nil {
		t.Fatalf("BlacklistIP upsert failed: %v", err)
	}
	bl, err = store.GetBlacklistedIPs()
	if err != nil || len(bl) != 2 {
		t.Fatalf("expected still 2 blacklisted IPs, got %d", len(bl))
	}

	// 4. Persistence across close & reopen
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reopenedStore, err := NewSQLStore(dbPath)
	if err != nil {
		t.Fatalf("reopening store failed: %v", err)
	}
	defer reopenedStore.Close()

	isBL, err = reopenedStore.IsIPBlacklisted("192.168.1.100")
	if err != nil || !isBL {
		t.Fatalf("expected 192.168.1.100 to remain blacklisted after reopen")
	}
}
