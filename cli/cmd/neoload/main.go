package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"neoload/internal/github"
	"neoload/internal/install"
	"neoload/internal/lock"
	"neoload/internal/registry"
	"neoload/internal/source"
	"neoload/internal/targets"
	"neoload/internal/ui"
)

const cliVersion = "0.1.0"

// exitError carries an exit code alongside its message so main can exit cleanly.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func exit(code int, err error) *exitError { return &exitError{code: code, err: err} }

func main() {
	cmd := newRootCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	if err := cmd.Execute(); err != nil {
		var ee *exitError
		if errors.As(err, &ee) {
			fmt.Fprintln(os.Stderr, "error:", ee.err)
			os.Exit(ee.code)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "neoload",
		Short: "Install and manage agent skills from GitHub",
	}
	root.AddCommand(newAddCmd())
	root.AddCommand(newListCmd())
	return root
}

func newAddCmd() *cobra.Command {
	var (
		global bool
		dryRun bool
		force  bool
		token  string
	)

	cmd := &cobra.Command{
		Use:   "add <owner>/<repo>@<skill>",
		Short: "Install a skill from GitHub into detected agent directories",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exit(2, fmt.Errorf("expected one argument: owner/repo@skill"))
			}
			cwd, err := os.Getwd()
			if err != nil {
				return exit(5, fmt.Errorf("cannot determine working directory: %w", err))
			}
			client := github.New(token)
			return runAdd(cmd.Context(), cwd, args[0], global, dryRun, force, client)
		},
	}

	cmd.Flags().BoolVarP(&global, "global", "g", false, "install to user-level agent directories")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be installed without writing files")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing destination skill directories")
	cmd.Flags().StringVar(&token, "token", os.Getenv("GITHUB_TOKEN"), "GitHub API token ($GITHUB_TOKEN)")

	return cmd
}

func newListCmd() *cobra.Command {
	var global bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return exit(5, fmt.Errorf("cannot determine working directory: %w", err))
			}
			return runList(cwd, global)
		},
	}

	cmd.Flags().BoolVarP(&global, "global", "g", false, "list globally installed skills")

	return cmd
}

func runList(cwd string, global bool) error {
	lockPath := filepath.Join(cwd, ".neoload", "skills.lock.json")
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return exit(5, fmt.Errorf("cannot determine home directory: %w", err))
		}
		lockPath = filepath.Join(home, ".neoload", "skills.lock.json")
	}

	lf, err := lock.Read(lockPath)
	if err != nil {
		return exit(5, err)
	}

	if len(lf.Installs) == 0 {
		ui.Info("No skills installed.")
		return nil
	}

	for _, inst := range lf.Installs {
		shortSHA := inst.ResolvedCommit
		if len(shortSHA) > 7 {
			shortSHA = shortSHA[:7]
		}
		ui.Info("%s  %s  %s", inst.Source, shortSHA, inst.UpdatedAt.Format("2006-01-02"))
		for _, t := range inst.InstalledTargets {
			ui.Info("  %s", t)
		}
	}

	return nil
}

// runAdd is the core logic for `neoload add`. Accepting cwd and a github.Client
// makes it testable without cobra or real network access.
func runAdd(
	ctx context.Context,
	cwd string,
	arg string,
	global, dryRun, force bool,
	ghClient github.Client,
) error {
	// 1. Parse source string.
	src, err := source.Parse(arg)
	if err != nil {
		return exit(2, err)
	}

	// 2. Detect install targets.
	tgts, err := targets.Detect(cwd, registry.Agents, global)
	if err != nil {
		return exit(4, err)
	}

	// 3. Resolve and download skill from GitHub.
	ui.Info("Resolving %s...", src.String())

	resolved, err := ghClient.ResolveSkill(ctx, src.Owner(), src.RepoName(), src.Skill)
	if err != nil {
		code := 5
		if isSkillError(err) {
			code = 3
		}
		return exit(code, err)
	}

	shortSHA := resolved.CommitSHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	ui.Info("Resolved commit: %s", shortSHA)

	// 4. Dry-run: show what would happen without writing.
	if dryRun {
		fileCount := countFiles(resolved.Files)
		ui.Info("\n[dry-run] Would install %s (%s)", src.String(), shortSHA)
		ui.Info("  files:   %d", fileCount)
		ui.Info("  targets:")
		for _, t := range tgts {
			ui.Info("    %s  (%s)", filepath.Join(t.SkillsDir, src.Skill), t.AgentName)
		}
		return nil
	}

	// 5. Install to each target.
	installedPaths, fileCount, err := install.Install(resolved.Files, tgts, src.Skill, force)
	if err != nil {
		return exit(5, err)
	}

	// 6. Write lock file.
	scope := "local"
	lockPath := filepath.Join(cwd, ".neoload", "skills.lock.json")
	if global {
		scope = "global"
		home, err := os.UserHomeDir()
		if err != nil {
			return exit(5, fmt.Errorf("cannot determine home directory: %w", err))
		}
		lockPath = filepath.Join(home, ".neoload", "skills.lock.json")
	}

	lf, err := lock.Read(lockPath)
	if err != nil {
		return exit(5, fmt.Errorf("read lock file: %w", err))
	}

	now := time.Now().UTC()
	lock.Upsert(lf, lock.Install{
		Scope:            scope,
		Source:           src.String(),
		Repo:             src.Repo,
		Skill:            src.Skill,
		ResolvedCommit:   resolved.CommitSHA,
		InstalledTargets: installedPaths,
		InstalledAt:      now,
		UpdatedAt:        now,
		CLIVersion:       cliVersion,
	})

	if err := lock.Write(lockPath, lf); err != nil {
		return exit(5, fmt.Errorf("write lock file: %w", err))
	}

	// 7. Print success summary.
	ui.Success("\nInstalled %s@%s", src.Repo, src.Skill)
	ui.Success("  commit:  %s", resolved.CommitSHA)
	ui.Success("  files:   %d", fileCount)
	ui.Success("  targets:")
	for _, p := range installedPaths {
		ui.Success("    %s", p)
	}

	return nil
}

// isSkillError returns true for errors that indicate a missing or invalid skill
// (exit code 3) rather than a transient network/IO failure (exit code 5).
func isSkillError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, kw := range []string{"not found", "missing SKILL.md", "not a valid skill"} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// countFiles walks an fs.FS and returns the number of regular files.
func countFiles(fsys fs.FS) int {
	n := 0
	fs.WalkDir(fsys, ".", func(_ string, d fs.DirEntry, err error) error { //nolint:errcheck
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}
