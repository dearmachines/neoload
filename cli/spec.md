# neoload CLI Specification (MVP)

## Purpose

`neoload` installs and manages agent skills from GitHub repositories into local or global agent skill directories.

Primary user flow:

```bash
neoload add anthropic/skills@xlsx
```

In this example:

- `anthropic/skills` is the source GitHub repository
- `xlsx` is the specific skill to install from `skills/xlsx/`

## Goals

- Install one skill from a GitHub source into one or more detected agent targets
- Support both local (project) and global installs
- Track installed skills and exact source commit SHA in a lock file
- Keep agent target mapping easy to extend via a single in-code config
- Achieve **at least 80% test coverage** for the CLI codebase

## Non-goals (MVP)

- Version ranges/tags in source syntax
- Dependency resolution between skills
- Auto-update all installed skills
- Publishing/installing plugins (skills only)

## Command Surface

### `neoload add <owner>/<repo>@<skill>`

Install a skill from GitHub into detected agent directories.

Flags:

- `-g, --global`: install to user-level agent directories (`$HOME/...`)
- `--dry-run`: print what would be installed without writing files
- `--force`: overwrite existing destination skill directories without prompt

Exit codes:

- `0`: success
- `2`: invalid input
- `3`: skill not found / invalid source content
- `4`: no install targets detected
- `5`: network, IO, or permission error

## Source Syntax

Input format:

```text
owner/repo@skill
```

Rules:

- Exactly one `@`
- Repository segment must look like `owner/repo`
- Skill segment maps to `skills/<skill>/` in source repo
- `skills/<skill>/SKILL.md` must exist

## Install Targets

Agent destinations are defined in a single registry config table in code.

Initial entries:

1. `claude`
   - local marker: `.claude`
   - local skill dir: `.claude/skills`
   - global skill dir: `~/.claude/skills`
2. `opencode`
   - local marker: `.opencode`
   - local skill dir: `.opencode/skills`
   - global skill dir: `~/.opencode/skills`
3. `codex`
   - local marker: `.agents`
   - local skill dir: `.agents/skills`
   - global skill dir: `~/.agents/skills`

Local mode behavior:

- Detect agent markers in the project/repo context
- Install to all matching local targets
- If none found, return exit code `4` with `Use -g for global install`

Global mode behavior:

- Install to all known global target roots
- Create missing directories as needed

## GitHub Resolution and Download

`add` must:

1. Resolve repository default branch
2. Resolve exact commit SHA used for install
3. Download archive at that commit
4. Extract only `skills/<skill>/`
5. Validate `SKILL.md`

The resolved commit SHA is persisted in the lock file.

## Lock File

Track what was installed, where, and from which commit.

Local lock path:

```text
<repo-root>/.skills/skills.lock.json
```

Global lock path:

```text
~/.skills/skills.lock.json
```

Schema (v1):

```json
{
  "version": 1,
  "installs": [
    {
      "scope": "local",
      "source": "anthropic/skills@xlsx",
      "repo": "anthropic/skills",
      "skill": "xlsx",
      "resolved_commit": "<sha>",
      "installed_targets": [
        "/abs/path/.claude/skills/xlsx",
        "/abs/path/.agents/skills/xlsx"
      ],
      "installed_at": "2026-04-04T19:12:00Z",
      "updated_at": "2026-04-04T19:12:00Z",
      "cli_version": "0.1.0"
    }
  ]
}
```

Behavior:

- Upsert by `(scope, repo, skill)`
- Write lock file only after all target installs succeed
- If install fails, do not update lock

## File Installation Semantics

- Copy full skill subtree (including references/scripts/assets)
- Use atomic strategy per target:
  - write into temp path
  - rename/move into final destination
- Existing skill destination:
  - default: require confirmation or fail in non-interactive mode
  - `--force`: replace directly

## User Output

Success output should include:

- source and skill name
- resolved commit SHA
- installed target paths
- file count copied

Error output should be actionable and specific.

## Proposed Go Layout

```text
cli/
  cmd/neoload/
  internal/source/
  internal/registry/
  internal/github/
  internal/targets/
  internal/install/
  internal/lock/
  internal/ui/
```

## Testing Requirements

Minimum total coverage target: **80%**.

Required tests:

- source parsing and validation
- target detection in local/global modes
- lock file create/read/upsert semantics
- installer atomic replacement behavior
- `--dry-run` and `--force` behavior
- failure paths (not found, missing `SKILL.md`, network/IO errors)

Suggested approach:

- table-driven unit tests for parsers and lock behavior
- filesystem tests using temp dirs for install logic
- integration-style tests for `add` command with mocked GitHub client

## Milestones

1. Bootstrap CLI skeleton and command wiring
2. Implement source parser + registry
3. Implement target detection
4. Implement GitHub fetch/extract abstraction
5. Implement installer + lock writer
6. Add tests to reach 80%+
7. Final polish for error messages and dry-run UX
