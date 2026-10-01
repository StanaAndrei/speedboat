package store

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"speedboat/internal/protocol"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// RawResultRecord represents a raw result received from an agent worker.
type RawResultRecord struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	ProjectID string    `gorm:"size:255;not null;uniqueIndex:idx_raw_unique,priority:1;index:idx_raw_project"`
	TaskID    string    `gorm:"size:255;not null;uniqueIndex:idx_raw_unique,priority:2;index:idx_raw_project_task"`
	WorkerID  string    `gorm:"size:255;not null;default:'';uniqueIndex:idx_raw_unique,priority:3"`
	IP        string    `gorm:"size:64;not null;default:'';uniqueIndex:idx_raw_unique,priority:4"`
	Status    string    `gorm:"size:32"`
	Output    string    `gorm:"type:text"`
	Error     string    `gorm:"type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// TableName specifies table name for RawResultRecord.
func (RawResultRecord) TableName() string {
	return "raw_results"
}

// VerifiedResultRecord represents a consensus-verified result for a task.
type VerifiedResultRecord struct {
	ProjectID  string    `gorm:"size:255;not null;primaryKey"`
	TaskID     string    `gorm:"size:255;not null;primaryKey"`
	Status     string    `gorm:"size:32"`
	Output     string    `gorm:"type:text"`
	Error      string    `gorm:"type:text"`
	WorkerID   string    `gorm:"size:255;default:''"`
	IP         string    `gorm:"size:64;default:''"`
	VerifiedAt time.Time `gorm:"autoCreateTime;autoUpdateTime"`
}

// TableName specifies table name for VerifiedResultRecord.
func (VerifiedResultRecord) TableName() string {
	return "verified_results"
}

// SQLStore implements the Store interface using GORM across SQLite, PostgreSQL, and MySQL.
type SQLStore struct {
	db      *gorm.DB
	dialect string
}

// DetectDialect inspects the connection target string and returns the driver name and normalized DSN.
func DetectDialect(target string) (driver string, dsn string) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "sqlite", "speedboat.db"
	}

	// PostgreSQL
	if strings.HasPrefix(trimmed, "postgres://") || strings.HasPrefix(trimmed, "postgresql://") {
		return "postgres", trimmed
	}

	// MySQL
	if strings.HasPrefix(trimmed, "mysql://") {
		u, err := url.Parse(trimmed)
		if err == nil {
			user := u.User.Username()
			pass, _ := u.User.Password()
			auth := user
			if pass != "" {
				auth = fmt.Sprintf("%s:%s", user, pass)
			}
			host := u.Host
			if host == "" {
				host = "localhost:3306"
			}
			dbName := strings.TrimPrefix(u.Path, "/")
			query := u.Query()
			if !query.Has("parseTime") {
				query.Set("parseTime", "true")
			}
			qStr := query.Encode()
			formattedDSN := fmt.Sprintf("%s@tcp(%s)/%s", auth, host, dbName)
			if qStr != "" {
				formattedDSN = fmt.Sprintf("%s?%s", formattedDSN, qStr)
			}
			return "mysql", formattedDSN
		}
		return "mysql", strings.TrimPrefix(trimmed, "mysql://")
	}
	if strings.HasPrefix(trimmed, "mysql:") {
		return "mysql", strings.TrimPrefix(trimmed, "mysql:")
	}
	if strings.Contains(trimmed, "@tcp(") {
		return "mysql", trimmed
	}

	// SQLite
	sqlitePath := trimmed
	if strings.HasPrefix(sqlitePath, "sqlite://") {
		sqlitePath = strings.TrimPrefix(sqlitePath, "sqlite://")
	}
	return "sqlite", sqlitePath
}

// NewSQLStore creates a new SQLStore connected to target (file path or connection string).
func NewSQLStore(target string) (*SQLStore, error) {
	driver, dsn := DetectDialect(target)

	var dialector gorm.Dialector
	switch driver {
	case "postgres":
		dialector = postgres.Open(dsn)
	case "mysql":
		dialector = mysql.Open(dsn)
	case "sqlite":
		if dsn != ":memory:" {
			dir := filepath.Dir(dsn)
			if dir != "" && dir != "." {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return nil, fmt.Errorf("failed to create directory for sqlite database: %w", err)
				}
			}
		}
		dialector = sqlite.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}

	gormCfg := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	}

	db, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection (%s): %w", driver, err)
	}

	// Configure SQLite specific pragmas for concurrency
	if driver == "sqlite" {
		sqlDB, err := db.DB()
		if err == nil {
			_, _ = sqlDB.Exec("PRAGMA journal_mode = WAL;")
			_, _ = sqlDB.Exec("PRAGMA busy_timeout = 5000;")
		}
	}

	// Auto-migrate schema tables
	if err := db.AutoMigrate(&RawResultRecord{}, &VerifiedResultRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate database schema: %w", err)
	}

	return &SQLStore{
		db:      db,
		dialect: driver,
	}, nil
}

// NewStore is a generic constructor returning the Store interface.
func NewStore(target string) (Store, error) {
	return NewSQLStore(target)
}

// NewBboltStore is provided for backwards compatibility and forwards to NewSQLStore.
func NewBboltStore(target string) (Store, error) {
	return NewSQLStore(target)
}

// SaveRawResult stores or updates a raw result received from a worker.
func (s *SQLStore) SaveRawResult(projectID string, res protocol.TaskResult) error {
	outputStr := string(res.Output)
	record := RawResultRecord{
		ProjectID: projectID,
		TaskID:    res.TaskID,
		WorkerID:  res.WorkerID,
		IP:        res.IP,
		Status:    res.Status,
		Output:    outputStr,
		Error:     res.Error,
		CreatedAt: time.Now(),
	}

	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "project_id"},
			{Name: "task_id"},
			{Name: "worker_id"},
			{Name: "ip"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"status", "output", "error", "created_at"}),
	}).Create(&record).Error
}

// GetRawResults retrieves all raw task results recorded for the project.
func (s *SQLStore) GetRawResults(projectID string) ([]protocol.TaskResult, error) {
	var records []RawResultRecord
	if err := s.db.Where("project_id = ?", projectID).Order("id asc").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve raw results: %w", err)
	}

	results := make([]protocol.TaskResult, len(records))
	for i, r := range records {
		var out json.RawMessage
		if r.Output != "" {
			out = json.RawMessage(r.Output)
		}
		results[i] = protocol.TaskResult{
			TaskID:    r.TaskID,
			ProjectID: r.ProjectID,
			Status:    r.Status,
			Output:    out,
			Error:     r.Error,
			WorkerID:  r.WorkerID,
			IP:        r.IP,
		}
	}
	return results, nil
}

// SaveVerifiedResult stores or updates a verified consensus result for a task.
func (s *SQLStore) SaveVerifiedResult(projectID string, res protocol.TaskResult) error {
	outputStr := string(res.Output)
	record := VerifiedResultRecord{
		ProjectID:  projectID,
		TaskID:     res.TaskID,
		Status:     res.Status,
		Output:     outputStr,
		Error:      res.Error,
		WorkerID:   res.WorkerID,
		IP:         res.IP,
		VerifiedAt: time.Now(),
	}

	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "project_id"},
			{Name: "task_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"status", "output", "error", "worker_id", "ip", "verified_at"}),
	}).Create(&record).Error
}

// GetVerifiedResults retrieves all verified results for the project, keyed by TaskID.
func (s *SQLStore) GetVerifiedResults(projectID string) (map[string]protocol.TaskResult, error) {
	var records []VerifiedResultRecord
	if err := s.db.Where("project_id = ?", projectID).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve verified results: %w", err)
	}

	results := make(map[string]protocol.TaskResult, len(records))
	for _, r := range records {
		var out json.RawMessage
		if r.Output != "" {
			out = json.RawMessage(r.Output)
		}
		results[r.TaskID] = protocol.TaskResult{
			TaskID:    r.TaskID,
			ProjectID: r.ProjectID,
			Status:    r.Status,
			Output:    out,
			Error:     r.Error,
			WorkerID:  r.WorkerID,
			IP:        r.IP,
		}
	}
	return results, nil
}

// Close closes the underlying SQL database connection pool.
func (s *SQLStore) Close() error {
	if s.db == nil {
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
