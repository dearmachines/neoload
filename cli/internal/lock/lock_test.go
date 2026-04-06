package lock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadNonExistent(t *testing.T) {
	lf, err := Read(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lf.Version != fileVersion {
		t.Errorf("Version = %d, want %d", lf.Version, fileVersion)
	}
	if len(lf.Installs) != 0 {
		t.Errorf("expected empty installs, got %d", len(lf.Installs))
	}
}

func TestWriteRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "skills.lock.json")
	now := time.Now().UTC().Truncate(time.Second)

	lf := &File{
		Version: fileVersion,
		Installs: []Install{
			{
				Scope:            "local",
				Source:           "anthropic/skills@xlsx",
				Repo:             "anthropic/skills",
				Skill:            "xlsx",
				ResolvedCommit:   "abc123",
				InstalledTargets: []string{"/project/.claude/skills/xlsx"},
				InstalledAt:      now,
				UpdatedAt:        now,
				CLIVersion:       "0.1.0",
			},
		},
	}

	if err := Write(path, lf); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Version != fileVersion {
		t.Errorf("Version = %d, want %d", got.Version, fileVersion)
	}
	if len(got.Installs) != 1 {
		t.Fatalf("expected 1 install, got %d", len(got.Installs))
	}
	inst := got.Installs[0]
	if inst.Repo != "anthropic/skills" {
		t.Errorf("Repo = %q, want %q", inst.Repo, "anthropic/skills")
	}
	if inst.ResolvedCommit != "abc123" {
		t.Errorf("ResolvedCommit = %q, want %q", inst.ResolvedCommit, "abc123")
	}
	if !inst.InstalledAt.Equal(now) {
		t.Errorf("InstalledAt = %v, want %v", inst.InstalledAt, now)
	}
}

func TestUpsertNew(t *testing.T) {
	lf := &File{Version: fileVersion}
	entry := Install{
		Scope: "local",
		Repo:  "anthropic/skills",
		Skill: "xlsx",
	}
	Upsert(lf, entry)

	if len(lf.Installs) != 1 {
		t.Fatalf("expected 1 install, got %d", len(lf.Installs))
	}
}

func TestUpsertExistingPreservesInstalledAt(t *testing.T) {
	original := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	lf := &File{
		Version: fileVersion,
		Installs: []Install{
			{
				Scope:       "local",
				Repo:        "anthropic/skills",
				Skill:       "xlsx",
				InstalledAt: original,
				UpdatedAt:   original,
			},
		},
	}

	updated := time.Now().UTC()
	Upsert(lf, Install{
		Scope:          "local",
		Repo:           "anthropic/skills",
		Skill:          "xlsx",
		ResolvedCommit: "newsha",
		InstalledAt:    updated,
		UpdatedAt:      updated,
	})

	if len(lf.Installs) != 1 {
		t.Fatalf("expected 1 install after upsert, got %d", len(lf.Installs))
	}
	inst := lf.Installs[0]
	if !inst.InstalledAt.Equal(original) {
		t.Errorf("InstalledAt should be preserved as %v, got %v", original, inst.InstalledAt)
	}
	if !inst.UpdatedAt.Equal(updated) {
		t.Errorf("UpdatedAt should be %v, got %v", updated, inst.UpdatedAt)
	}
	if inst.ResolvedCommit != "newsha" {
		t.Errorf("ResolvedCommit = %q, want %q", inst.ResolvedCommit, "newsha")
	}
}

func TestReadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(path, []byte("{not valid json"), 0644)
	_, err := Read(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestUpsertMultipleScopes(t *testing.T) {
	lf := &File{Version: fileVersion}
	Upsert(lf, Install{Scope: "local", Repo: "r", Skill: "s"})
	Upsert(lf, Install{Scope: "global", Repo: "r", Skill: "s"})

	if len(lf.Installs) != 2 {
		t.Fatalf("expected 2 installs for different scopes, got %d", len(lf.Installs))
	}
}
