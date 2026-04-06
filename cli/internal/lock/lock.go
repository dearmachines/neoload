package lock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const fileVersion = 1

// File is the top-level lock file structure.
type File struct {
	Version  int       `json:"version"`
	Installs []Install `json:"installs"`
}

// Install records a single skill installation.
type Install struct {
	Scope            string    `json:"scope"`
	Source           string    `json:"source"`
	Repo             string    `json:"repo"`
	Skill            string    `json:"skill"`
	ResolvedCommit   string    `json:"resolved_commit"`
	InstalledTargets []string  `json:"installed_targets"`
	InstalledAt      time.Time `json:"installed_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	CLIVersion       string    `json:"cli_version"`
}

// Read reads the lock file at path. Returns an empty file if it does not exist.
func Read(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &File{Version: fileVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read lock file: %w", err)
	}
	var lf File
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse lock file: %w", err)
	}
	return &lf, nil
}

// Write atomically writes the lock file to path.
func Write(path string, lf *File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create lock directory: %w", err)
	}
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lock file: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("finalize lock file: %w", err)
	}
	return nil
}

// Upsert inserts or updates an install entry keyed by (scope, repo, skill).
// On update, InstalledAt is preserved from the existing record.
func Upsert(lf *File, entry Install) {
	for i, inst := range lf.Installs {
		if inst.Scope == entry.Scope && inst.Repo == entry.Repo && inst.Skill == entry.Skill {
			entry.InstalledAt = inst.InstalledAt
			lf.Installs[i] = entry
			return
		}
	}
	lf.Installs = append(lf.Installs, entry)
}
