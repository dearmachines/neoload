package registry

// Agent describes an AI agent's skill directory conventions.
type Agent struct {
	Name           string
	LocalMarker    string // directory name that signals this agent is active (e.g. ".claude")
	LocalSkillDir  string // relative path for project-local skills
	GlobalSkillDir string // path for user-global skills (~ prefix for home dir)
}

// Agents is the list of supported agent targets.
var Agents = []Agent{
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
