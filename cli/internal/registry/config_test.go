package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAgentsNoConfigNoEnv(t *testing.T) {
	t.Setenv("NEOLOAD_CONFIG", t.TempDir()) // empty dir, no agents.json
	t.Setenv("NEOLOAD_AGENTS", "")

	agents, err := LoadAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != len(DefaultAgents) {
		t.Errorf("expected %d agents, got %d", len(DefaultAgents), len(agents))
	}
	if agents[0].Name != "claude" {
		t.Errorf("first agent = %q, want %q", agents[0].Name, "claude")
	}
}

func TestLoadAgentsConfigAddsNew(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NEOLOAD_CONFIG", dir)
	t.Setenv("NEOLOAD_AGENTS", "")

	config := `[{"name":"cursor","local_marker":".cursor","local_skill_dir":".cursor/skills","global_skill_dir":"~/.cursor/skills"}]`
	os.WriteFile(filepath.Join(dir, "agents.json"), []byte(config), 0644)

	agents, err := LoadAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != len(DefaultAgents)+1 {
		t.Errorf("expected %d agents, got %d", len(DefaultAgents)+1, len(agents))
	}
	last := agents[len(agents)-1]
	if last.Name != "cursor" {
		t.Errorf("last agent = %q, want %q", last.Name, "cursor")
	}
}

func TestLoadAgentsConfigOverridesExisting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NEOLOAD_CONFIG", dir)
	t.Setenv("NEOLOAD_AGENTS", "")

	config := `[{"name":"claude","local_marker":".claude2","local_skill_dir":".claude2/skills","global_skill_dir":"~/.claude2/skills"}]`
	os.WriteFile(filepath.Join(dir, "agents.json"), []byte(config), 0644)

	agents, err := LoadAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != len(DefaultAgents) {
		t.Errorf("expected %d agents, got %d", len(DefaultAgents), len(agents))
	}
	if agents[0].LocalMarker != ".claude2" {
		t.Errorf("LocalMarker = %q, want %q", agents[0].LocalMarker, ".claude2")
	}
}

func TestLoadAgentsEnvOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NEOLOAD_CONFIG", dir)

	config := `[{"name":"claude","local_marker":".config-claude","local_skill_dir":".config-claude/skills","global_skill_dir":"~/.config-claude/skills"}]`
	os.WriteFile(filepath.Join(dir, "agents.json"), []byte(config), 0644)

	envJSON := `[{"name":"claude","local_marker":".env-claude","local_skill_dir":".env-claude/skills","global_skill_dir":"~/.env-claude/skills"}]`
	t.Setenv("NEOLOAD_AGENTS", envJSON)

	agents, err := LoadAgents()
	if err != nil {
		t.Fatal(err)
	}
	if agents[0].LocalMarker != ".env-claude" {
		t.Errorf("LocalMarker = %q, want %q (env should override config)", agents[0].LocalMarker, ".env-claude")
	}
}

func TestLoadAgentsInvalidConfigJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NEOLOAD_CONFIG", dir)
	t.Setenv("NEOLOAD_AGENTS", "")

	os.WriteFile(filepath.Join(dir, "agents.json"), []byte("{bad json"), 0644)

	_, err := LoadAgents()
	if err == nil {
		t.Fatal("expected error for invalid config JSON")
	}
}

func TestLoadAgentsInvalidEnvJSON(t *testing.T) {
	t.Setenv("NEOLOAD_CONFIG", t.TempDir())
	t.Setenv("NEOLOAD_AGENTS", "{bad json")

	_, err := LoadAgents()
	if err == nil {
		t.Fatal("expected error for invalid env JSON")
	}
}

func TestMergeAgentsEmpty(t *testing.T) {
	base := []Agent{{Name: "a"}}
	result := mergeAgents(base, nil)
	if len(result) != 1 || result[0].Name != "a" {
		t.Errorf("mergeAgents with nil overrides should return base")
	}
}
