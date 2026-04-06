package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LoadAgents returns the merged agent list from three sources (lowest to highest priority):
// built-in defaults < config file < NEOLOAD_AGENTS env var.
func LoadAgents() ([]Agent, error) {
	agents := make([]Agent, len(DefaultAgents))
	copy(agents, DefaultAgents)

	configAgents, err := loadConfigFile()
	if err != nil {
		return nil, err
	}
	agents = mergeAgents(agents, configAgents)

	envAgents, err := loadEnvAgents()
	if err != nil {
		return nil, err
	}
	agents = mergeAgents(agents, envAgents)

	return agents, nil
}

func configDir() string {
	if d := os.Getenv("NEOLOAD_CONFIG"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".neoload")
}

func loadConfigFile() ([]Agent, error) {
	dir := configDir()
	if dir == "" {
		return nil, nil
	}
	path := filepath.Join(dir, "agents.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read agents config: %w", err)
	}
	var agents []Agent
	if err := json.Unmarshal(data, &agents); err != nil {
		return nil, fmt.Errorf("parse agents config %s: %w", path, err)
	}
	return agents, nil
}

func loadEnvAgents() ([]Agent, error) {
	raw := os.Getenv("NEOLOAD_AGENTS")
	if raw == "" {
		return nil, nil
	}
	var agents []Agent
	if err := json.Unmarshal([]byte(raw), &agents); err != nil {
		return nil, fmt.Errorf("parse NEOLOAD_AGENTS: %w", err)
	}
	return agents, nil
}

// mergeAgents merges overrides into base. Agents with matching names replace
// existing entries; new names are appended.
func mergeAgents(base, overrides []Agent) []Agent {
	if len(overrides) == 0 {
		return base
	}
	result := make([]Agent, len(base))
	copy(result, base)

	for _, o := range overrides {
		found := false
		for i, b := range result {
			if b.Name == o.Name {
				result[i] = o
				found = true
				break
			}
		}
		if !found {
			result = append(result, o)
		}
	}
	return result
}
