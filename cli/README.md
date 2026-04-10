# neoload CLI

Go CLI for installing agent skills from GitHub repositories. See the [project README](../README.md) for user-facing documentation.

## Package Layout

```
cmd/neoload/       CLI entry point and command wiring
internal/
  github/          GitHub API client (Contents API + zip fallback)
  install/         Atomic file copy to target directories
  lock/            Lock file persistence (skills.lock.json)
  registry/        Agent definitions (Claude, OpenCode, Codex)
  source/          Source string parsing (owner/repo:skill[@ref])
  targets/         Install target detection (local/global)
  ui/              Terminal output, spinner, table formatting
```

## Build

```bash
just build          # → bin/neoload
go build ./cmd/neoload
```

## Test

```bash
just test           # run all tests
just cover          # coverage report (target: 80%+)
just vet            # go vet
just tidy           # go mod tidy
```
