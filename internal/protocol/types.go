package protocol

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ProjectConfig struct { // specification in project.yaml
	Name         string             `yaml:"name" json:"name"`
	Version      string             `yaml:"version" json:"version"`
	Container    ContainerConfig    `yaml:"container" json:"container"`
	Generator    ScriptConfig       `yaml:"generator" json:"generator"`
	Reducer      ScriptConfig       `yaml:"reducer" json:"reducer"`
	Verification VerificationConfig `yaml:"verification" json:"verification"`
}

type ContainerConfig struct { // container build & runtime settings
	Dockerfile string `yaml:"dockerfile" json:"dockerfile"`
	ImageTag   string `yaml:"image_tag" json:"image_tag"`
}

type ScriptConfig struct { // runtime and source for generator or reducer scripts
	Runtime string `yaml:"runtime" json:"runtime"`
	Source  string `yaml:"source" json:"source"`
}

type VerificationConfig struct {
	Redundancy         int `yaml:"redundancy" json:"redundancy"`
	ConsensusThreshold int `yaml:"consensus_threshold" json:"consensus_threshold"`
}

type ProjectAnnouncement struct {
	ProjectID          string `json:"project_id"`
	Name               string `json:"name"`
	Version            string `json:"version"`
	Image              string `json:"image"`
	NatsURL            string `json:"nats_url,omitempty"`
	TotalTasks         int    `json:"total_tasks,omitempty"`
	Redundancy         int    `json:"redundancy,omitempty"`
	ConsensusThreshold int    `json:"consensus_threshold,omitempty"`
	CreatedAt          int64  `json:"created_at,omitempty"`
}

func LoadProjectConfig(projectDir string) (*ProjectConfig, error) {
	candidates := []string{
		filepath.Join(projectDir, "project.yaml"),
		filepath.Join(projectDir, "project.yml"),
	}

	var data []byte
	var err error
	var foundPath string

	for _, path := range candidates {
		data, err = os.ReadFile(path)
		if err == nil {
			foundPath = path
			break
		}
	}

	if foundPath == "" {
		return nil, fmt.Errorf("project.yaml not found in directory: %s", projectDir)
	}

	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", foundPath, err)
	}

	// Apply sensible defaults
	if cfg.Name == "" {
		cfg.Name = filepath.Base(projectDir)
	}
	if cfg.Version == "" {
		cfg.Version = "1.0.0"
	}
	if cfg.Container.Dockerfile == "" {
		cfg.Container.Dockerfile = "./Dockerfile"
	}
	if cfg.Container.ImageTag == "" {
		cfg.Container.ImageTag = fmt.Sprintf("%s:latest", cfg.Name)
	}
	if cfg.Verification.Redundancy <= 0 {
		cfg.Verification.Redundancy = 1
	}
	if cfg.Verification.ConsensusThreshold <= 0 {
		cfg.Verification.ConsensusThreshold = 1
	}
	if cfg.Verification.ConsensusThreshold > cfg.Verification.Redundancy {
		cfg.Verification.ConsensusThreshold = cfg.Verification.Redundancy
	}
	return &cfg, nil
}
