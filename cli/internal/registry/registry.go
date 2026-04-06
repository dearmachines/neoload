package registry

// Agent describes an AI agent's skill directory conventions.
type Agent struct {
	Name           string `json:"name"`
	LocalMarker    string `json:"local_marker"`     // directory name that signals this agent is active (e.g. ".claude")
	LocalSkillDir  string `json:"local_skill_dir"`  // relative path for project-local skills
	GlobalSkillDir string `json:"global_skill_dir"` // path for user-global skills (~ prefix for home dir)
}

// DefaultAgents is the built-in list of supported agent targets.
var DefaultAgents = []Agent{
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
	{
		Name:           "codex",
		LocalMarker:    ".agents",
		LocalSkillDir:  ".agents/skills",
		GlobalSkillDir: "~/.agents/skills",
	},
}
