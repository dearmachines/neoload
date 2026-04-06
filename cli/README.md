# neoload

Install agent skills from GitHub repositories into local or global agent directories.

```bash
neoload add anthropic/skills@xlsx
```

## Install

```bash
just build          # produces bin/neoload
```

Or install directly:

```bash
go install neoload/cmd/neoload@latest
```

## Usage

```
neoload add <owner>/<repo>@<skill> [flags]

Flags:
  -g, --global     install to user-level agent directories (~/.claude/skills, etc.)
      --dry-run    print what would be installed without writing files
      --force      overwrite existing skill directories without prompt
      --token      GitHub API token (default: $GITHUB_TOKEN)

neoload list [flags]

Flags:
  -g, --global     list globally installed skills
```

### Examples

```bash
# Install xlsx skill into all detected local agent directories
neoload add anthropic/skills@xlsx

# Preview without writing
neoload add anthropic/skills@xlsx --dry-run

# Install globally for all agents
neoload add anthropic/skills@xlsx -g

# Replace an existing install
neoload add anthropic/skills@xlsx --force

# List locally installed skills
neoload list

# List globally installed skills
neoload list -g
```

## Supported agents

Skills are installed into whichever agent directories are present in the current project:

| Agent     | Local marker | Local skill dir       | Global skill dir        |
|-----------|--------------|-----------------------|-------------------------|
| claude    | `.claude`    | `.claude/skills/`     | `~/.claude/skills/`     |
| opencode  | `.opencode`  | `.opencode/skills/`   | `~/.opencode/skills/`   |
| codex     | `.agents`    | `.agents/skills/`     | `~/.agents/skills/`     |

## Source format

```
owner/repo@skill
```

The skill maps to `skills/<skill>/` inside the repository. `SKILL.md` must exist in that directory.

## Lock file

Installed skills are tracked in a lock file that records the pinned commit SHA:

- Local: `<project>/.skills/skills.lock.json`
- Global: `~/.skills/skills.lock.json`

## Exit codes

| Code | Meaning                          |
|------|----------------------------------|
| 0    | Success                          |
| 2    | Invalid input                    |
| 3    | Skill not found / missing SKILL.md |
| 4    | No agent directories detected    |
| 5    | Network, IO, or permission error |

## Development

```bash
just test       # run all tests
just cover      # test with coverage summary
just vet        # run go vet
just tidy       # tidy go.mod
just clean      # remove build artifacts
```

Test coverage target: **80%** (currently ~83%).
