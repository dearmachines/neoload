# neoload

```

  █   █  █████   ███   █      ███    ███   ████
  ██  █  █      █   █  █     █   █  █   █  █   █
  █ █ █  ████   █   █  █     █   █  █████  █   █
  █  ██  █      █   █  █     █   █  █   █  █   █
  █   █  █████   ███   █████  ███   █   █  ████

```

> **Agent:** I know xlsx.

> **You:** [Show me.](https://www.youtube.com/watch?v=0YhJxJZOWBw)

**neoload** uploads skills directly into your AI agents.

No sparring. No training montage. Just `neoload add` and it's done.

```bash
neoload add anthropic/skills@xlsx
```

---

## What is neoload?

The Matrix has agents. Your projects have agents too — Claude, OpenCode, Codex.
They're powerful, but they only know what they've been taught.

**neoload** is the Operator. It finds skills in GitHub repositories and loads
them straight into every agent running in your project.

One command. All agents. Instantly skilled.

---

## Install

**Download a binary** from the [latest release](https://github.com/dearmachines/neoload/releases/latest),
make it executable, and move it to your `$PATH`:

```bash
# macOS arm64 example
curl -L https://github.com/dearmachines/neoload/releases/latest/download/neoload-darwin-arm64 -o neoload
chmod +x neoload
mv neoload /usr/local/bin/
```

Available binaries: `linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`, `windows-amd64.exe`

**Build with Go** (requires Go 1.22+):

```bash
git clone https://github.com/dearmachines/neoload.git
cd neoload
go install ./cli/cmd/neoload
```

**Build with just**:

```bash
git clone https://github.com/dearmachines/neoload.git
cd neoload
just build   # → bin/neoload
```

---

## Commands

### `neoload add <owner>/<repo>@<skill>`

Install a skill from GitHub into every detected agent directory.

```bash
neoload add anthropic/skills@xlsx
```

```
neoload add anthropic/skills@xlsx
  -g, --global     install to user-level agent directories (~/.claude/skills, etc.)
      --dry-run    print what would be installed without writing files
      --force      overwrite an existing skill without prompting
      --token      GitHub API token (default: $GITHUB_TOKEN)
```

Examples:

```bash
# Install into all local agents
neoload add anthropic/skills@xlsx

# Preview without writing
neoload add anthropic/skills@xlsx --dry-run

# Install globally for all agents
neoload add anthropic/skills@xlsx -g

# Overwrite an existing install
neoload add anthropic/skills@xlsx --force
```

### `neoload list`

List installed skills and their pinned commit.

```bash
neoload list
```

```
SKILL                    COMMIT   INSTALLED   TARGETS
anthropic/skills@xlsx    a1b2c3d  2026-04-06  claude, opencode
```

```bash
neoload list
  -g, --global    list globally installed skills
```

### `neoload remove <owner>/<repo>@<skill>`

Remove an installed skill and its lock file entry.

```bash
neoload remove anthropic/skills@xlsx
```

```bash
neoload remove anthropic/skills@xlsx
  -g, --global    remove from user-level agent directories
      --dry-run   print what would be removed without deleting files
```

---

## How it works

```
owner/repo@skill
```

neoload resolves the repository's default branch, pins the exact commit SHA,
downloads the archive, and extracts `skills/<skill>/`. Every agent directory
found in your project receives a copy.

The pinned commit is recorded in a lock file — commit it so your team always
loads the exact same version.

```
.neoload/skills.lock.json   ← local
~/.neoload/skills.lock.json ← global
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

| Code | Meaning                              |
|------|--------------------------------------|
| 0    | Success                              |
| 2    | Invalid input                        |
| 3    | Skill not found / not installed      |
| 4    | No agents detected in this project   |
| 5    | Network, IO, or permission error     |

---

## Development

```bash
just test     # run all tests
just cover    # coverage report (target: 80%+)
just build    # compile the binary
just install  # install to $GOPATH/bin
just vet      # go vet
just tidy     # go mod tidy
just clean    # remove artifacts
```
