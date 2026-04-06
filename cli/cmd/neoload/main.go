package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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

var version = "dev"

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
			ui.Error("%v", ee.err)
			os.Exit(ee.code)
		}
		ui.Error("%v", err)
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
	root.AddCommand(newRemoveCmd())
	root.AddCommand(newUpdateCmd())
	root.AddCommand(newSearchCmd())
	root.AddCommand(newVersionCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			ui.Info("neoload %s (%s, %s/%s)", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		},
	}
}

func newAddCmd() *cobra.Command {
	var (
		global  bool
		dryRun  bool
		force   bool
		token   string
		timeout time.Duration
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
			return runAdd(cmd.Context(), cwd, args[0], global, dryRun, force, timeout, client)
		},
	}

	cmd.Flags().BoolVarP(&global, "global", "g", false, "install to user-level agent directories")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be installed without writing files")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing destination skill directories")
	cmd.Flags().StringVar(&token, "token", os.Getenv("GITHUB_TOKEN"), "GitHub API token ($GITHUB_TOKEN)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "timeout for GitHub API calls")

	return cmd
}

func newListCmd() *cobra.Command {
	var (
		global  bool
		jsonOut bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return exit(5, fmt.Errorf("cannot determine working directory: %w", err))
			}
			return runList(cwd, global, jsonOut)
		},
	}

	cmd.Flags().BoolVarP(&global, "global", "g", false, "list globally installed skills")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON array")

	return cmd
}

// listEntry is the stable JSON output struct for `neoload list --json`.
type listEntry struct {
	Source      string   `json:"source"`
	Commit      string   `json:"commit"`
	InstalledAt string   `json:"installed_at"`
	UpdatedAt   string   `json:"updated_at"`
	Targets     []string `json:"targets"`
	CLIVersion  string   `json:"cli_version"`
}

func runList(cwd string, global, jsonOut bool) error {
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

	if jsonOut {
		entries := make([]listEntry, 0, len(lf.Installs))
		for _, inst := range lf.Installs {
			entries = append(entries, listEntry{
				Source:      inst.Source,
				Commit:      inst.ResolvedCommit,
				InstalledAt: inst.InstalledAt.Format(time.RFC3339),
				UpdatedAt:   inst.UpdatedAt.Format(time.RFC3339),
				Targets:     inst.InstalledTargets,
				CLIVersion:  inst.CLIVersion,
			})
		}
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return exit(5, fmt.Errorf("marshal JSON: %w", err))
		}
		fmt.Fprintln(ui.Out, string(data))
		return nil
	}

	if len(lf.Installs) == 0 {
		ui.Info("No skills installed.")
		return nil
	}

	headers := []string{"SKILL", "COMMIT", "INSTALLED", "TARGETS"}
	var rows [][]string
	for _, inst := range lf.Installs {
		agents := agentNames(inst.InstalledTargets)
		if len(inst.InstalledAgentNames) > 0 {
			agents = strings.Join(inst.InstalledAgentNames, ", ")
		}
		rows = append(rows, []string{
			inst.Source,
			shortSHA(inst.ResolvedCommit),
			inst.UpdatedAt.Format("2006-01-02"),
			agents,
		})
	}
	ui.Table(headers, rows)

	return nil
}

func newRemoveCmd() *cobra.Command {
	var (
		global bool
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   "remove <owner>/<repo>@<skill>",
		Short: "Remove an installed skill",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exit(2, fmt.Errorf("expected one argument: owner/repo@skill"))
			}
			cwd, err := os.Getwd()
			if err != nil {
				return exit(5, fmt.Errorf("cannot determine working directory: %w", err))
			}
			return runRemove(cwd, args[0], global, dryRun)
		},
	}

	cmd.Flags().BoolVarP(&global, "global", "g", false, "remove from user-level agent directories")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be removed without deleting files")

	return cmd
}

func runRemove(cwd, arg string, global, dryRun bool) error {
	src, err := source.Parse(arg)
	if err != nil {
		return exit(2, err)
	}

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
		return exit(5, err)
	}

	// Find the matching entry.
	idx := -1
	for i, inst := range lf.Installs {
		if inst.Scope == scope && inst.Repo == src.Repo && inst.Skill == src.Skill {
			idx = i
			break
		}
	}
	if idx == -1 {
		return exit(3, fmt.Errorf("skill %q is not installed", src.String()))
	}

	entry := lf.Installs[idx]

	if dryRun {
		ui.Info("[dry-run] Would remove %s", src.String())
		for _, t := range entry.InstalledTargets {
			ui.Info("  %s", t)
		}
		return nil
	}

	// Remove each installed target directory.
	for _, t := range entry.InstalledTargets {
		if err := os.RemoveAll(t); err != nil {
			return exit(5, fmt.Errorf("remove %s: %w", t, err))
		}
	}

	// Remove entry from lock file and persist.
	lf.Installs = append(lf.Installs[:idx], lf.Installs[idx+1:]...)
	if err := lock.Write(lockPath, lf); err != nil {
		return exit(5, fmt.Errorf("write lock file: %w", err))
	}

	ui.Success("Removed %s", src.String())
	for _, t := range entry.InstalledTargets {
		ui.Info("  %s", t)
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
	timeout time.Duration,
	ghClient github.Client,
) error {
	// 1. Parse source string.
	src, err := source.Parse(arg)
	if err != nil {
		return exit(2, err)
	}

	// 2. Detect install targets.
	agents, err := registry.LoadAgents()
	if err != nil {
		return exit(5, err)
	}
	tgts, err := targets.Detect(cwd, agents, global)
	if err != nil {
		return exit(4, err)
	}

	// 3. Resolve and download skill from GitHub.
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	spin := ui.StartSpinner(fmt.Sprintf("Resolving %s...", src.String()))

	resolved, err := ghClient.ResolveSkill(ctx, src.Owner(), src.RepoName(), src.Skill, src.Ref)
	spin.Stop()

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return exit(5, fmt.Errorf("operation timed out after %s", timeout))
		}
		var se *github.SkillError
		var rle *github.RateLimitError
		switch {
		case errors.As(err, &se):
			return exit(3, err)
		case errors.As(err, &rle):
			return exit(5, err)
		default:
			return exit(5, err)
		}
	}

	ui.Success("Resolved %s (%s)", src.String(), shortSHA(resolved.CommitSHA))

	// 4. Dry-run: show what would happen without writing.
	if dryRun {
		fileCount := countFiles(resolved.Files)
		fmt.Fprintln(ui.Out)
		ui.Info("[dry-run] Would install %s (%s)", src.String(), shortSHA(resolved.CommitSHA))
		ui.Info("  files:   %d", fileCount)
		ui.Info("  targets:")
		for _, t := range tgts {
			ui.Info("    %s  (%s)", filepath.Join(t.SkillsDir, src.Skill), t.AgentName)
		}
		return nil
	}

	// 5. Install to each target.
	installedPaths, skippedPaths, fileCount, installErr := install.Install(resolved.Files, tgts, src.Skill, force)

	// 6. Write lock file — merge newly installed + already-existing (skipped) targets.
	allTargets := append(installedPaths, skippedPaths...)
	if len(allTargets) > 0 {
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
			Scope:               scope,
			Source:              src.String(),
			Repo:                src.Repo,
			Skill:               src.Skill,
			PinnedRef:           src.Ref,
			ResolvedCommit:      resolved.CommitSHA,
			InstalledTargets:    allTargets,
			InstalledAgentNames: matchAgentNames(tgts, allTargets, src.Skill),
			InstalledAt:         now,
			UpdatedAt:           now,
			CLIVersion:          version,
		})

		if err := lock.Write(lockPath, lf); err != nil {
			return exit(5, fmt.Errorf("write lock file: %w", err))
		}

		// 7. Print success summary.
		if len(installedPaths) > 0 {
			fmt.Fprintln(ui.Out)
			ui.Success("Installed %s@%s", src.Repo, src.Skill)
			ui.Info("  commit:  %s", resolved.CommitSHA)
			ui.Info("  files:   %d", fileCount)
			ui.Info("  targets:")
			for _, p := range installedPaths {
				ui.Info("    %s", p)
			}
		}
		if len(skippedPaths) > 0 && len(installedPaths) == 0 {
			ui.Info("Skill %s is already installed for all detected agents.", src.String())
		}
	}

	if installErr != nil {
		return exit(5, installErr)
	}

	return nil
}

func newUpdateCmd() *cobra.Command {
	var (
		global bool
		dryRun bool
		all    bool
		token  string
	)

	cmd := &cobra.Command{
		Use:   "update [<owner>/<repo>@<skill>]",
		Short: "Update installed skills to latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			if all && len(args) > 0 {
				return exit(2, fmt.Errorf("--all and positional argument are mutually exclusive"))
			}
			if !all && len(args) != 1 {
				return exit(2, fmt.Errorf("expected one argument: owner/repo@skill (or use --all)"))
			}
			cwd, err := os.Getwd()
			if err != nil {
				return exit(5, fmt.Errorf("cannot determine working directory: %w", err))
			}
			client := github.New(token)
			if all {
				return runUpdateAll(cmd.Context(), cwd, global, dryRun, client)
			}
			return runUpdate(cmd.Context(), cwd, args[0], global, dryRun, client)
		},
	}

	cmd.Flags().BoolVarP(&global, "global", "g", false, "update in user-level agent directories")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be updated without writing files")
	cmd.Flags().BoolVar(&all, "all", false, "update all installed skills")
	cmd.Flags().StringVar(&token, "token", os.Getenv("GITHUB_TOKEN"), "GitHub API token ($GITHUB_TOKEN)")

	return cmd
}

func runUpdate(
	ctx context.Context,
	cwd string,
	arg string,
	global, dryRun bool,
	ghClient github.Client,
) error {
	src, err := source.Parse(arg)
	if err != nil {
		return exit(2, err)
	}

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
		return exit(5, err)
	}

	idx := -1
	for i, inst := range lf.Installs {
		if inst.Scope == scope && inst.Repo == src.Repo && inst.Skill == src.Skill {
			idx = i
			break
		}
	}
	if idx == -1 {
		return exit(3, fmt.Errorf("skill %q is not installed", src.String()))
	}

	entry := lf.Installs[idx]

	spin := ui.StartSpinner(fmt.Sprintf("Checking %s...", src.String()))
	resolved, err := ghClient.ResolveSkill(ctx, src.Owner(), src.RepoName(), src.Skill, src.Ref)
	spin.Stop()

	if err != nil {
		var se *github.SkillError
		if errors.As(err, &se) {
			return exit(3, err)
		}
		return exit(5, err)
	}

	if resolved.CommitSHA == entry.ResolvedCommit {
		ui.Success("%s is already up to date (%s)", src.String(), shortSHA(resolved.CommitSHA))
		return nil
	}

	if dryRun {
		ui.Info("[dry-run] Would update %s: %s → %s",
			src.String(), shortSHA(entry.ResolvedCommit), shortSHA(resolved.CommitSHA))
		return nil
	}

	agents, err := registry.LoadAgents()
	if err != nil {
		return exit(5, err)
	}
	tgts, err := targets.Detect(cwd, agents, global)
	if err != nil {
		return exit(4, err)
	}

	installedPaths, _, fileCount, err := install.Install(resolved.Files, tgts, src.Skill, true)
	if err != nil {
		return exit(5, err)
	}

	now := time.Now().UTC()
	lock.Upsert(lf, lock.Install{
		Scope:               scope,
		Source:              src.String(),
		Repo:                src.Repo,
		Skill:               src.Skill,
		PinnedRef:           src.Ref,
		ResolvedCommit:      resolved.CommitSHA,
		InstalledTargets:    installedPaths,
		InstalledAgentNames: matchAgentNames(tgts, installedPaths, src.Skill),
		InstalledAt:         now,
		UpdatedAt:           now,
		CLIVersion:          version,
	})

	if err := lock.Write(lockPath, lf); err != nil {
		return exit(5, fmt.Errorf("write lock file: %w", err))
	}

	fmt.Fprintln(ui.Out)
	ui.Success("Updated %s: %s → %s",
		src.String(), shortSHA(entry.ResolvedCommit), shortSHA(resolved.CommitSHA))
	ui.Info("  files:   %d", fileCount)
	ui.Info("  targets:")
	for _, p := range installedPaths {
		ui.Info("    %s", p)
	}

	return nil
}

func runUpdateAll(
	ctx context.Context,
	cwd string,
	global, dryRun bool,
	ghClient github.Client,
) error {
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
		return exit(5, err)
	}

	var entries []lock.Install
	for _, inst := range lf.Installs {
		if inst.Scope == scope {
			entries = append(entries, inst)
		}
	}

	if len(entries) == 0 {
		ui.Info("No skills installed to update.")
		return nil
	}

	updated := 0
	for _, entry := range entries {
		src, err := source.Parse(entry.Source)
		if err != nil {
			ui.Error("skip %s: %v", entry.Source, err)
			continue
		}

		spin := ui.StartSpinner(fmt.Sprintf("Checking %s...", src.String()))
		resolved, err := ghClient.ResolveSkill(ctx, src.Owner(), src.RepoName(), src.Skill, src.Ref)
		spin.Stop()

		if err != nil {
			ui.Error("skip %s: %v", src.String(), err)
			continue
		}

		if resolved.CommitSHA == entry.ResolvedCommit {
			ui.Success("%s is already up to date", src.String())
			continue
		}

		if dryRun {
			ui.Info("[dry-run] Would update %s: %s → %s",
				src.String(), shortSHA(entry.ResolvedCommit), shortSHA(resolved.CommitSHA))
			updated++
			continue
		}

		agents, err := registry.LoadAgents()
		if err != nil {
			return exit(5, err)
		}
		tgts, err := targets.Detect(cwd, agents, global)
		if err != nil {
			return exit(4, err)
		}

		installedPaths, _, _, err := install.Install(resolved.Files, tgts, src.Skill, true)
		if err != nil {
			ui.Error("update %s: %v", src.String(), err)
			continue
		}

		now := time.Now().UTC()
		lock.Upsert(lf, lock.Install{
			Scope:               scope,
			Source:              src.String(),
			Repo:                src.Repo,
			Skill:               src.Skill,
			PinnedRef:           src.Ref,
			ResolvedCommit:      resolved.CommitSHA,
			InstalledTargets:    installedPaths,
			InstalledAgentNames: matchAgentNames(tgts, installedPaths, src.Skill),
			InstalledAt:         now,
			UpdatedAt:           now,
			CLIVersion:          version,
		})

		updated++
		ui.Success("Updated %s: %s → %s",
			src.String(), shortSHA(entry.ResolvedCommit), shortSHA(resolved.CommitSHA))
	}

	if updated > 0 && !dryRun {
		if err := lock.Write(lockPath, lf); err != nil {
			return exit(5, fmt.Errorf("write lock file: %w", err))
		}
	}

	return nil
}

func newSearchCmd() *cobra.Command {
	var (
		token   string
		jsonOut bool
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "search <owner>/<repo>",
		Short: "List available skills in a GitHub repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exit(2, fmt.Errorf("expected one argument: owner/repo"))
			}
			client := github.New(token)
			return runSearch(cmd.Context(), args[0], "", timeout, jsonOut, client)
		},
	}

	cmd.Flags().StringVar(&token, "token", os.Getenv("GITHUB_TOKEN"), "GitHub API token ($GITHUB_TOKEN)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON array")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "timeout for GitHub API calls")

	return cmd
}

// searchEntry is the stable JSON output struct for `neoload search --json`.
type searchEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

func runSearch(
	ctx context.Context,
	repoArg string,
	ref string,
	timeout time.Duration,
	jsonOut bool,
	ghClient github.Client,
) error {
	// Validate owner/repo format.
	parts := strings.SplitN(repoArg, "/", 3)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return exit(2, fmt.Errorf("invalid repository %q: must be owner/repo", repoArg))
	}
	owner, repo := parts[0], parts[1]

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var spin *ui.Spinner
	if !jsonOut {
		spin = ui.StartSpinner(fmt.Sprintf("Searching %s...", repoArg))
	}
	skills, err := ghClient.ListSkills(ctx, owner, repo, ref)
	if spin != nil {
		spin.Stop()
	}

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return exit(5, fmt.Errorf("operation timed out after %s", timeout))
		}
		var se *github.SkillError
		var rle *github.RateLimitError
		switch {
		case errors.As(err, &se):
			return exit(3, err)
		case errors.As(err, &rle):
			return exit(5, err)
		default:
			return exit(5, err)
		}
	}

	if jsonOut {
		entries := make([]searchEntry, 0, len(skills))
		for _, sk := range skills {
			entries = append(entries, searchEntry{
				Name:        sk.Name,
				Description: sk.Description,
				Source:      repoArg + "@" + sk.Name,
			})
		}
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return exit(5, fmt.Errorf("marshal JSON: %w", err))
		}
		fmt.Fprintln(ui.Out, string(data))
		return nil
	}

	if len(skills) == 0 {
		ui.Info("No skills found in %s.", repoArg)
		return nil
	}

	headers := []string{"SKILL", "DESCRIPTION"}
	var rows [][]string
	for _, sk := range skills {
		rows = append(rows, []string{sk.Name, sk.Description})
	}
	ui.Table(headers, rows)

	return nil
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// matchAgentNames correlates installed paths back to target agent names.
func matchAgentNames(tgts []targets.Target, installedPaths []string, skill string) []string {
	pathSet := make(map[string]bool, len(installedPaths))
	for _, p := range installedPaths {
		pathSet[p] = true
	}
	var names []string
	for _, t := range tgts {
		if pathSet[filepath.Join(t.SkillsDir, skill)] {
			names = append(names, t.AgentName)
		}
	}
	return names
}

// agentNames extracts agent names from installed target paths.
// Backward-compat fallback for lock files without InstalledAgentNames.
func agentNames(paths []string) string {
	var names []string
	for _, t := range paths {
		parent := filepath.Base(filepath.Dir(filepath.Dir(t)))
		names = append(names, strings.TrimPrefix(parent, "."))
	}
	return strings.Join(names, ", ")
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
