package targets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neoload/internal/registry"
)

var testAgents = []registry.Agent{
	{
		Name:           "claude",
		LocalMarker:    ".claude",
		LocalSkillDir:  ".claude/skills",
		GlobalSkillDir: "~/.claude/skills",
	},
	{
		Name:           "opencode",
		LocalMarker:    ".opencode",
		LocalSkillDir:  ".opencode/skills",
		GlobalSkillDir: "~/.opencode/skills",
	},
}

func TestDetectLocal(t *testing.T) {
	t.Run("finds marker", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, ".claude"), 0755); err != nil {
			t.Fatal(err)
		}

		tgts, err := Detect(dir, testAgents, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tgts) != 1 {
			t.Fatalf("expected 1 target, got %d", len(tgts))
		}
		if tgts[0].AgentName != "claude" {
			t.Errorf("AgentName = %q, want %q", tgts[0].AgentName, "claude")
		}
		want := filepath.Join(dir, ".claude/skills")
		if tgts[0].SkillsDir != want {
			t.Errorf("SkillsDir = %q, want %q", tgts[0].SkillsDir, want)
		}
	})

	t.Run("finds multiple markers", func(t *testing.T) {
		dir := t.TempDir()
		os.Mkdir(filepath.Join(dir, ".claude"), 0755)
		os.Mkdir(filepath.Join(dir, ".opencode"), 0755)

		tgts, err := Detect(dir, testAgents, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tgts) != 2 {
			t.Fatalf("expected 2 targets, got %d", len(tgts))
		}
	})

	t.Run("no markers returns error", func(t *testing.T) {
		dir := t.TempDir()
		_, err := Detect(dir, testAgents, false)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "no agent directories found") {
			t.Errorf("error %q should mention 'no agent directories found'", err)
		}
	})
}

func TestDetectGlobal(t *testing.T) {
	tgts, err := Detect("/any/cwd", testAgents, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tgts) != len(testAgents) {
		t.Fatalf("expected %d targets, got %d", len(testAgents), len(tgts))
	}

	home, _ := os.UserHomeDir()
	for _, tgt := range tgts {
		if strings.Contains(tgt.SkillsDir, "~") {
			t.Errorf("SkillsDir %q still contains ~", tgt.SkillsDir)
		}
		if !strings.HasPrefix(tgt.SkillsDir, home) {
			t.Errorf("SkillsDir %q does not start with home %q", tgt.SkillsDir, home)
		}
	}
}
