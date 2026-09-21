package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"speedboat/internal/protocol"

	"go.etcd.io/bbolt"
)

const (
	bucketRawResults      = "raw_results"
	bucketVerifiedResults = "verified_results"
)

// BboltStore implements the Store interface using go.etcd.io/bbolt.
type BboltStore struct {
	db *bbolt.DB
}

// NewBboltStore opens or creates a bbolt database at dbPath.
func NewBboltStore(dbPath string) (*BboltStore, error) {
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory for bbolt database: %w", err)
		}
	}

	opts := &bbolt.Options{
		Timeout: 2 * time.Second,
	}
	db, err := bbolt.Open(dbPath, 0600, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open bbolt database at %s: %w", dbPath, err)
	}

	return &BboltStore{db: db}, nil
}

// rawResultKey builds a deterministic key for a raw result within a task.
func rawResultKey(res protocol.TaskResult) string {
	worker := res.WorkerID
	if worker == "" {
		worker = res.IP
	} else if res.IP != "" {
		worker = fmt.Sprintf("%s_%s", res.IP, res.WorkerID)
	}
	return fmt.Sprintf("%s/%s", res.TaskID, worker)
}

// SaveRawResult stores a raw result received from a worker.
func (s *BboltStore) SaveRawResult(projectID string, res protocol.TaskResult) error {
	data, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("failed to marshal raw task result: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		projBucket, err := tx.CreateBucketIfNotExists([]byte(projectID))
		if err != nil {
			return fmt.Errorf("failed to get/create project bucket: %w", err)
		}

		rawBucket, err := projBucket.CreateBucketIfNotExists([]byte(bucketRawResults))
		if err != nil {
			return fmt.Errorf("failed to get/create raw_results bucket: %w", err)
		}

		key := []byte(rawResultKey(res))
		return rawBucket.Put(key, data)
	})
}

// GetRawResults retrieves all raw task results recorded for the project.
func (s *BboltStore) GetRawResults(projectID string) ([]protocol.TaskResult, error) {
	var results []protocol.TaskResult

	err := s.db.View(func(tx *bbolt.Tx) error {
		projBucket := tx.Bucket([]byte(projectID))
		if projBucket == nil {
			return nil
		}

		rawBucket := projBucket.Bucket([]byte(bucketRawResults))
		if rawBucket == nil {
			return nil
		}

		return rawBucket.ForEach(func(k, v []byte) error {
			var res protocol.TaskResult
			if err := json.Unmarshal(v, &res); err != nil {
				return fmt.Errorf("failed to unmarshal raw result %s: %w", string(k), err)
			}
			results = append(results, res)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// SaveVerifiedResult stores a verified consensus result for a task.
func (s *BboltStore) SaveVerifiedResult(projectID string, res protocol.TaskResult) error {
	data, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("failed to marshal verified task result: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		projBucket, err := tx.CreateBucketIfNotExists([]byte(projectID))
		if err != nil {
			return fmt.Errorf("failed to get/create project bucket: %w", err)
		}

		verifiedBucket, err := projBucket.CreateBucketIfNotExists([]byte(bucketVerifiedResults))
		if err != nil {
			return fmt.Errorf("failed to get/create verified_results bucket: %w", err)
		}

		return verifiedBucket.Put([]byte(res.TaskID), data)
	})
}

// GetVerifiedResults retrieves all verified results for the project, keyed by TaskID.
func (s *BboltStore) GetVerifiedResults(projectID string) (map[string]protocol.TaskResult, error) {
	results := make(map[string]protocol.TaskResult)

	err := s.db.View(func(tx *bbolt.Tx) error {
		projBucket := tx.Bucket([]byte(projectID))
		if projBucket == nil {
			return nil
		}

		verifiedBucket := projBucket.Bucket([]byte(bucketVerifiedResults))
		if verifiedBucket == nil {
			return nil
		}

		return verifiedBucket.ForEach(func(k, v []byte) error {
			var res protocol.TaskResult
			if err := json.Unmarshal(v, &res); err != nil {
				return fmt.Errorf("failed to unmarshal verified result %s: %w", string(k), err)
			}
			results[string(k)] = res
			return nil
		})
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// Close closes the bbolt database.
func (s *BboltStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
