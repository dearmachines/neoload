package targets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"neoload/internal/registry"
)

// Target is a resolved install destination for one agent.
type Target struct {
	AgentName string
	SkillsDir string // absolute path
}

// Detect returns install targets based on mode.
// In global mode it returns targets for all known agents.
// In local mode it scans cwd for agent marker directories.
func Detect(cwd string, agents []registry.Agent, global bool) ([]Target, error) {
	if global {
		return globalTargets(agents)
	}
	return localTargets(cwd, agents)
}

func globalTargets(agents []registry.Agent) ([]Target, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	targets := make([]Target, 0, len(agents))
	for _, a := range agents {
		dir := strings.Replace(a.GlobalSkillDir, "~", home, 1)
		targets = append(targets, Target{AgentName: a.Name, SkillsDir: dir})
	}
	return targets, nil
}

func localTargets(cwd string, agents []registry.Agent) ([]Target, error) {
	var targets []Target
	for _, a := range agents {
		marker := filepath.Join(cwd, a.LocalMarker)
		if _, err := os.Stat(marker); err == nil {
			targets = append(targets, Target{
				AgentName: a.Name,
				SkillsDir: filepath.Join(cwd, a.LocalSkillDir),
			})
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no agent directories found in %s; use -g for global install", cwd)
	}
	return targets, nil
}
