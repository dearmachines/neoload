```

  █   █  █████   ███   █      ███    ███   ████
  ██  █  █      █   █  █     █   █  █   █  █   █
  █ █ █  ████   █   █  █     █   █  █████  █   █
  █  ██  █      █   █  █     █   █  █   █  █   █
  █   █  █████   ███   █████  ███   █   █  ████

         i n s t a l l   s k i l l s .   b e c o m e .
```

> *"I know kung fu."*
> — Neo, after a 10-second skill upload

**neoload** uploads skills directly into your AI agents.
No sparring. No training montage. Just `neoload add` and it's done.

```bash
neoload add anthropic/skills@xlsx
```

```
Resolving anthropic/skills@xlsx...
Resolved commit: a1b2c3d

Installed anthropic/skills@xlsx
  commit:  a1b2c3d4e5f6...
  files:   12
  targets:
    /your/project/.claude/skills/xlsx
    /your/project/.opencode/skills/xlsx
```

**Agent:** I know xlsx.
**You:** [Show me.](https://www.youtube.com/watch?v=0YhJxJZOWBw)

---

## What is neoload?

The Matrix has agents. Your projects have agents too — Claude, OpenCode, Codex.
They're powerful, but they only know what they've been taught.

**neoload** is the Operator. It finds skills in GitHub repositories and loads
them straight into every agent running in your project.

One command. All agents. Instantly skilled.

---

## Install

```bash
go install neoload/cmd/neoload@latest
```

Or build from source:

```bash
just build   # → bin/neoload
```

---

## Usage

### `neoload add`

```bash
# Load a skill into all detected local agents
neoload add anthropic/skills@xlsx

# Preview the upload without writing anything
neoload add anthropic/skills@xlsx --dry-run

# Load into all agents, everywhere (global install)
neoload add anthropic/skills@xlsx -g

# Overwrite an existing skill without prompting
neoload add anthropic/skills@xlsx --force
```

Set `GITHUB_TOKEN` to avoid rate limits on private repositories.

### `neoload list`

```bash
# What skills does this project know?
neoload list

# What do you know globally?
neoload list -g
```

---

## How it works

```
owner/repo@skill
```

neoload resolves the repository's default branch, pins the exact commit SHA,
downloads the archive, and extracts `skills/<skill>/`. Every agent directory
found in your project receives a copy.

The loaded commit is recorded in a lock file so your team loads the exact same
version.

```
.skills/skills.lock.json   ← local
~/.skills/skills.lock.json ← global
```

---

## Supported agents

neoload detects agents by looking for their config directories:

| Agent    | Marker      | Skill directory         |
|----------|-------------|-------------------------|
| Claude   | `.claude`   | `.claude/skills/`       |
| OpenCode | `.opencode` | `.opencode/skills/`     |
| Codex    | `.agents`   | `.agents/skills/`       |

---

## Exit codes

| Code | Meaning                             |
|------|-------------------------------------|
| 0    | Upload complete                     |
| 2    | Invalid input                       |
| 3    | Skill not found / missing `SKILL.md`|
| 4    | No agents detected in this project  |
| 5    | Network, IO, or permission error    |

---

## Development

```bash
just test     # run all tests
just cover    # coverage report (target: 80%+)
just build    # compile the binary
just vet      # go vet
just tidy     # go mod tidy
just clean    # remove artifacts
```

---

*"He's beginning to believe."*
