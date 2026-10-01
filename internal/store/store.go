package store

import (
	"speedboat/internal/protocol"
)

// Store defines the persistence operations required by the master orchestrator.
type Store interface {
	// SaveRawResult stores a raw result received from a worker.
	SaveRawResult(projectID string, res protocol.TaskResult) error

	// GetRawResults retrieves all raw task results recorded for the project.
	GetRawResults(projectID string) ([]protocol.TaskResult, error)

	// SaveVerifiedResult stores a verified consensus result for a task.
	SaveVerifiedResult(projectID string, res protocol.TaskResult) error

	// GetVerifiedResults retrieves all verified results for the project, keyed by TaskID.
	GetVerifiedResults(projectID string) (map[string]protocol.TaskResult, error)

	// BlacklistIP records an IP address as blacklisted with an optional reason and projectID.
	BlacklistIP(ip string, reason string) error

	// GetBlacklistedIPs retrieves all blacklisted IP addresses.
	GetBlacklistedIPs() ([]string, error)

	// IsIPBlacklisted checks if an IP is blacklisted.
	IsIPBlacklisted(ip string) (bool, error)

	// Close closes the underlying database.
	Close() error
}
