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
neoload add anthropic/skills:xlsx
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

### `neoload add <owner>/<repo>:<skill>`

Install a skill from GitHub into every detected agent directory.

```bash
neoload add anthropic/skills:xlsx
```

```
neoload add anthropic/skills:xlsx
  -g, --global     install to user-level agent directories (~/.claude/skills, etc.)
      --dry-run    print what would be installed without writing files
      --force      overwrite an existing skill without prompting
      --token      GitHub API token for private repos or higher rate limits (default: $GITHUB_TOKEN)
```

Examples:

```bash
# Install into all local agents
neoload add anthropic/skills:xlsx

# Preview without writing
neoload add anthropic/skills:xlsx --dry-run

# Install globally for all agents
neoload add anthropic/skills:xlsx -g

# Overwrite an existing install
neoload add anthropic/skills:xlsx --force
```

#### Private GitHub repositories

Private repositories are supported through GitHub API token authentication. Use a
fine-grained personal access token with read access to the repository contents.
Pass it with `--token`, or set `GITHUB_TOKEN`:

```bash
GITHUB_TOKEN=github_pat_xxx neoload add owner/private-repo:skill
# or
neoload add owner/private-repo:skill --token github_pat_xxx
```

Only GitHub API token auth is supported; SSH/git clone URLs and GitHub Enterprise
custom hosts are not supported. If the token is missing or lacks access, GitHub
may return a `404`/`403` and neoload will report the repository or skill as
unavailable.

### `neoload list`

List installed skills and their pinned commit.

```bash
neoload list
```

```
SKILL                    COMMIT   INSTALLED   TARGETS
anthropic/skills:xlsx    a1b2c3d  2026-04-06  claude, opencode
```

```bash
neoload list
  -g, --global    list globally installed skills
```

### `neoload remove <owner>/<repo>:<skill>`

Remove an installed skill and its lock file entry.

```bash
neoload remove anthropic/skills:xlsx
```

```bash
neoload remove anthropic/skills:xlsx
  -g, --global    remove from user-level agent directories
      --dry-run   print what would be removed without deleting files
```

---

## How it works

```
owner/repo:skill[@ref]
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

## Dependencies

3 direct, 14 indirect — split between third-party and Go extended stdlib.

### Third-party

| Module | Type | Purpose |
|--------|------|---------|
| [cobra](https://github.com/spf13/cobra) | direct | CLI framework (commands, flags, help) |
| [lipgloss](https://github.com/charmbracelet/lipgloss) | direct | Terminal styling and table output |
| pflag | indirect | Flag parsing (cobra) |
| mousetrap | indirect | Windows signal handling (cobra) |
| colorprofile | indirect | Terminal color profile detection (lipgloss) |
| x/ansi | indirect | ANSI escape sequence handling (lipgloss) |
| x/cellbuf | indirect | Cell-based terminal buffer (lipgloss) |
| x/term (charm) | indirect | Terminal capabilities (lipgloss) |
| termenv | indirect | Terminal environment detection (lipgloss) |
| go-colorful | indirect | Color manipulation (lipgloss) |
| go-isatty | indirect | TTY detection (lipgloss) |
| go-runewidth | indirect | Character width calculation (lipgloss) |
| go-osc52 | indirect | OSC52 clipboard support (lipgloss) |
| uniseg | indirect | Unicode segmentation (lipgloss) |
| terminfo | indirect | Terminal info database (lipgloss) |

### Go extended stdlib (`golang.org/x`)

| Module | Type | Purpose |
|--------|------|---------|
| [x/term](https://pkg.go.dev/golang.org/x/term) | direct | Terminal width detection |
| x/sys | indirect | Low-level OS primitives (x/term) |

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
