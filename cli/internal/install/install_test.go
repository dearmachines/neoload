package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"neoload/internal/targets"
)

func makeSkillFS(files map[string]string) fstest.MapFS {
	mfs := make(fstest.MapFS)
	for name, content := range files {
		mfs[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return mfs
}

func TestInstallSingleTarget(t *testing.T) {
	dir := t.TempDir()
	skillFS := makeSkillFS(map[string]string{
		"SKILL.md":  "# Test",
		"script.sh": "#!/bin/sh",
	})

	tgts := []targets.Target{
		{AgentName: "claude", SkillsDir: filepath.Join(dir, ".claude/skills")},
	}

	paths, count, err := Install(skillFS, tgts, "myskill", false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if count != 2 {
		t.Errorf("file count = %d, want 2", count)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 installed path, got %d", len(paths))
	}

	destDir := filepath.Join(dir, ".claude/skills/myskill")
	if paths[0] != destDir {
		t.Errorf("path = %q, want %q", paths[0], destDir)
	}

	// Verify files exist at destination.
	if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "script.sh")); err != nil {
		t.Errorf("script.sh not found: %v", err)
	}
}

func TestInstallMultipleTargets(t *testing.T) {
	dir := t.TempDir()
	skillFS := makeSkillFS(map[string]string{
		"SKILL.md": "# Test",
	})

	tgts := []targets.Target{
		{AgentName: "claude", SkillsDir: filepath.Join(dir, ".claude/skills")},
		{AgentName: "opencode", SkillsDir: filepath.Join(dir, ".opencode/skills")},
	}

	paths, count, err := Install(skillFS, tgts, "myskill", false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 installed paths, got %d", len(paths))
	}
	// count is per-target, so 2 targets × 1 file = 2
	if count != 2 {
		t.Errorf("total file count = %d, want 2", count)
	}
}

func TestInstallExistingNoForce(t *testing.T) {
	dir := t.TempDir()
	skillFS := makeSkillFS(map[string]string{"SKILL.md": "# Test"})

	destDir := filepath.Join(dir, ".claude/skills/myskill")
	os.MkdirAll(destDir, 0755)

	tgts := []targets.Target{
		{AgentName: "claude", SkillsDir: filepath.Join(dir, ".claude/skills")},
	}

	_, _, err := Install(skillFS, tgts, "myskill", false)
	if err == nil {
		t.Fatal("expected error when destination exists and force=false")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error %q should mention --force", err)
	}
}

func TestInstallExistingWithForce(t *testing.T) {
	dir := t.TempDir()
	destDir := filepath.Join(dir, ".claude/skills/myskill")
	os.MkdirAll(destDir, 0755)
	// Write a stale file that should be replaced.
	os.WriteFile(filepath.Join(destDir, "old.txt"), []byte("old"), 0644)

	skillFS := makeSkillFS(map[string]string{"SKILL.md": "# New"})

	tgts := []targets.Target{
		{AgentName: "claude", SkillsDir: filepath.Join(dir, ".claude/skills")},
	}

	_, _, err := Install(skillFS, tgts, "myskill", true)
	if err != nil {
		t.Fatalf("Install with force: %v", err)
	}

	// Old file should be gone.
	if _, err := os.Stat(filepath.Join(destDir, "old.txt")); err == nil {
		t.Error("old.txt should have been removed")
	}
	// New file should exist.
	if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not found: %v", err)
	}
}

func TestInstallNestedFiles(t *testing.T) {
	dir := t.TempDir()
	skillFS := makeSkillFS(map[string]string{
		"SKILL.md":        "# Test",
		"scripts/run.sh":  "#!/bin/sh",
		"assets/icon.png": "PNG",
	})

	tgts := []targets.Target{
		{AgentName: "claude", SkillsDir: filepath.Join(dir, ".claude/skills")},
	}

	_, count, err := Install(skillFS, tgts, "myskill", false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if count != 3 {
		t.Errorf("file count = %d, want 3", count)
	}

	destDir := filepath.Join(dir, ".claude/skills/myskill")
	if _, err := os.Stat(filepath.Join(destDir, "scripts/run.sh")); err != nil {
		t.Errorf("scripts/run.sh not found: %v", err)
	}
}
