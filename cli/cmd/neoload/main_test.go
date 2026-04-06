package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"neoload/internal/github"
	"neoload/internal/lock"
	"neoload/internal/ui"
)

// ─── fake GitHub client ───────────────────────────────────────────────────────

type fakeGHClient struct {
	sha    string
	files  fs.FS
	err    error
	skills []github.SkillInfo // for ListSkills
}

func (f *fakeGHClient) ResolveSkill(_ context.Context, _, _, _, _ string) (*github.ResolvedSkill, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &github.ResolvedSkill{CommitSHA: f.sha, Files: f.files}, nil
}

func (f *fakeGHClient) ListSkills(_ context.Context, _, _, _ string) ([]github.SkillInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.skills, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func makeTestSkillFS(files map[string]string) fstest.MapFS {
	mfs := make(fstest.MapFS)
	for name, content := range files {
		mfs[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return mfs
}

func captureOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	ui.Out = &buf
	ui.Err = &buf
	t.Cleanup(func() {
		ui.Out = os.Stdout
		ui.Err = os.Stderr
	})
	return &buf
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestRunAddSuccess(t *testing.T) {
	projectDir := t.TempDir()
	os.Mkdir(filepath.Join(projectDir, ".claude"), 0755)

	buf := captureOutput(t)

	client := &fakeGHClient{
		sha:   "aabbccddee112233445566778899001122334455",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# Test", "run.sh": "#!/bin/sh"}),
	}

	err := runAdd(context.Background(), projectDir, "o/r@myskill", false, false, false, 0, client)
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	// Files should be installed.
	destDir := filepath.Join(projectDir, ".claude/skills/myskill")
	if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not installed: %v", err)
	}

	// Lock file should exist.
	lockPath := filepath.Join(projectDir, ".neoload/skills.lock.json")
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file not written: %v", err)
	}

	// Output should contain commit SHA.
	if !strings.Contains(buf.String(), "aabbccd") {
		t.Errorf("output should mention commit SHA, got: %s", buf.String())
	}
}

func TestRunAddDryRun(t *testing.T) {
	projectDir := t.TempDir()
	os.Mkdir(filepath.Join(projectDir, ".claude"), 0755)

	buf := captureOutput(t)

	client := &fakeGHClient{
		sha:   "deadbeef11223344556677889900112233445566",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# SK"}),
	}

	err := runAdd(context.Background(), projectDir, "o/r@sk", false, true, false, 0, client)
	if err != nil {
		t.Fatalf("runAdd dry-run: %v", err)
	}

	// No files should be written.
	if _, err := os.Stat(filepath.Join(projectDir, ".claude/skills/sk")); err == nil {
		t.Error("dry-run should not write files")
	}

	if !strings.Contains(buf.String(), "[dry-run]") {
		t.Errorf("expected [dry-run] in output, got: %s", buf.String())
	}
}

func TestRunAddForceOverwrite(t *testing.T) {
	projectDir := t.TempDir()
	destDir := filepath.Join(projectDir, ".claude/skills/sk")
	os.MkdirAll(destDir, 0755)
	os.WriteFile(filepath.Join(destDir, "old.txt"), []byte("stale"), 0644)
	os.Mkdir(filepath.Join(projectDir, ".claude"), 0755)

	captureOutput(t)

	client := &fakeGHClient{
		sha:   "newsha1234",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# New"}),
	}

	err := runAdd(context.Background(), projectDir, "o/r@sk", false, false, true, 0, client)
	if err != nil {
		t.Fatalf("runAdd force: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "old.txt")); err == nil {
		t.Error("old.txt should have been replaced")
	}
	if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not found after force: %v", err)
	}
}

func TestRunAddExistingNoForce(t *testing.T) {
	projectDir := t.TempDir()
	destDir := filepath.Join(projectDir, ".claude/skills/sk")
	os.MkdirAll(destDir, 0755)
	os.Mkdir(filepath.Join(projectDir, ".claude"), 0755)

	buf := captureOutput(t)

	client := &fakeGHClient{
		sha:   "newsha",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# X"}),
	}

	err := runAdd(context.Background(), projectDir, "o/r@sk", false, false, false, 0, client)
	if err != nil {
		t.Fatalf("expected no error for already-installed skill, got %v", err)
	}
	if !strings.Contains(buf.String(), "already installed") {
		t.Errorf("expected 'already installed' message, got: %s", buf.String())
	}
}

func TestRunAddNewAgentTarget(t *testing.T) {
	projectDir := t.TempDir()
	// Start with claude already having the skill installed.
	os.MkdirAll(filepath.Join(projectDir, ".claude/skills/sk"), 0755)
	// opencode is a new agent directory.
	os.Mkdir(filepath.Join(projectDir, ".opencode"), 0755)

	buf := captureOutput(t)

	client := &fakeGHClient{
		sha:   "aabbccddee112233445566778899001122334455",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# Test"}),
	}

	err := runAdd(context.Background(), projectDir, "o/r@sk", false, false, false, 0, client)
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	// opencode should have the skill now.
	if _, err := os.Stat(filepath.Join(projectDir, ".opencode/skills/sk/SKILL.md")); err != nil {
		t.Errorf("skill not installed to opencode: %v", err)
	}

	// Lock file should record BOTH targets.
	lockPath := filepath.Join(projectDir, ".neoload/skills.lock.json")
	lf, err := lock.Read(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(lf.Installs) != 1 {
		t.Fatalf("expected 1 lock entry, got %d", len(lf.Installs))
	}
	entry := lf.Installs[0]
	if len(entry.InstalledTargets) != 2 {
		t.Errorf("expected 2 targets in lock, got %d: %v", len(entry.InstalledTargets), entry.InstalledTargets)
	}

	// Output should show the install, not "already installed".
	if !strings.Contains(buf.String(), "Installed") {
		t.Errorf("expected 'Installed' in output, got: %s", buf.String())
	}
}

func TestRunAddInvalidSource(t *testing.T) {
	captureOutput(t)
	err := runAdd(context.Background(), t.TempDir(), "bad-input", false, false, false, 0, nil)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected exitError{code:2}, got %v", err)
	}
}

func TestRunAddNoTargets(t *testing.T) {
	captureOutput(t)
	// Empty dir with no agent markers.
	err := runAdd(context.Background(), t.TempDir(), "o/r@sk", false, false, false, 0, nil)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 4 {
		t.Errorf("expected exitError{code:4}, got %v", err)
	}
}

func TestRunAddSkillNotFound(t *testing.T) {
	projectDir := t.TempDir()
	os.Mkdir(filepath.Join(projectDir, ".claude"), 0755)

	captureOutput(t)

	client := &fakeGHClient{err: &github.SkillError{
		Kind:    github.ErrSkillNotFound,
		Skill:   "missing",
		Message: "skill \"missing\" not found in repository",
	}}

	err := runAdd(context.Background(), projectDir, "o/r@missing", false, false, false, 0, client)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 3 {
		t.Errorf("expected exitError{code:3}, got %v", err)
	}
}

func TestRunAddNetworkError(t *testing.T) {
	projectDir := t.TempDir()
	os.Mkdir(filepath.Join(projectDir, ".claude"), 0755)

	captureOutput(t)

	client := &fakeGHClient{err: errors.New("dial tcp: connection refused")}

	err := runAdd(context.Background(), projectDir, "o/r@sk", false, false, false, 0, client)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 5 {
		t.Errorf("expected exitError{code:5}, got %v", err)
	}
}

func TestRunAddGlobal(t *testing.T) {
	captureOutput(t)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	// Clean up any files written to global dirs after the test.
	globalDirs := []string{
		filepath.Join(home, ".claude/skills/sk"),
		filepath.Join(home, ".opencode/skills/sk"),
		filepath.Join(home, ".agents/skills/sk"),
		filepath.Join(home, ".neoload/skills.lock.json"),
	}
	t.Cleanup(func() {
		for _, d := range globalDirs {
			os.RemoveAll(d)
		}
	})

	client := &fakeGHClient{
		sha:   "globalsha1234",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# G"}),
	}

	// Global mode installs to all agent directories; use force to avoid conflicts.
	err = runAdd(context.Background(), t.TempDir(), "o/r@sk", true, false, true, 0, client)
	if err != nil {
		t.Fatalf("runAdd global: %v", err)
	}
}

func TestNewVersionCmd(t *testing.T) {
	buf := captureOutput(t)
	cmd := newRootCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version cmd: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "neoload") {
		t.Errorf("version output should contain 'neoload', got: %s", out)
	}
	if !strings.Contains(out, runtime.GOOS) {
		t.Errorf("version output should contain OS, got: %s", out)
	}
	if !strings.Contains(out, runtime.GOARCH) {
		t.Errorf("version output should contain arch, got: %s", out)
	}
}

func TestExitError(t *testing.T) {
	inner := errors.New("inner error")
	ee := exit(3, inner)

	if ee.Error() != "inner error" {
		t.Errorf("Error() = %q, want %q", ee.Error(), "inner error")
	}
	if ee.Unwrap() != inner {
		t.Errorf("Unwrap() did not return inner error")
	}
	if ee.code != 3 {
		t.Errorf("code = %d, want 3", ee.code)
	}
}

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()
	if cmd.Use != "neoload" {
		t.Errorf("Use = %q, want %q", cmd.Use, "neoload")
	}
	// Should have an "add" subcommand.
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "add <owner>/<repo>@<skill>" {
			found = true
		}
	}
	if !found {
		t.Error("newRootCmd should have an 'add' subcommand")
	}
}

func TestNewAddCmd(t *testing.T) {
	cmd := newAddCmd()

	// Verify all required flags exist.
	for _, name := range []string{"global", "dry-run", "force", "token"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s not registered", name)
		}
	}

	// Verify -g shorthand.
	if cmd.Flags().ShorthandLookup("g") == nil {
		t.Error("flag -g shorthand not registered")
	}
}

func TestNewAddCmdMissingArg(t *testing.T) {
	captureOutput(t)
	cmd := newRootCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"add"}) // no argument
	err := cmd.Execute()
	// Should return an exitError with code 2.
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected exitError{code:2} for missing arg, got %v", err)
	}
}

func TestRunListEmpty(t *testing.T) {
	buf := captureOutput(t)

	err := runList(t.TempDir(), false, false)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	if !strings.Contains(buf.String(), "No skills installed") {
		t.Errorf("expected 'No skills installed', got: %s", buf.String())
	}
}

func TestRunListWithEntries(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".neoload", "skills.lock.json")

	lf := &lock.File{Version: 1, Installs: []lock.Install{
		{
			Source:           "anthropic/skills@xlsx",
			ResolvedCommit:   "aabbccddee",
			InstalledTargets: []string{"/project/.claude/skills/xlsx"},
			UpdatedAt:        time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC),
		},
	}}
	if err := lock.Write(lockPath, lf); err != nil {
		t.Fatal(err)
	}

	buf := captureOutput(t)

	if err := runList(dir, false, false); err != nil {
		t.Fatalf("runList: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "anthropic/skills@xlsx") {
		t.Errorf("expected skill name in output, got: %s", out)
	}
	if !strings.Contains(out, "aabbccd") {
		t.Errorf("expected short SHA in output, got: %s", out)
	}
	if !strings.Contains(out, "SKILL") {
		t.Errorf("expected table header in output, got: %s", out)
	}
}

func TestRunListWithAgentNames(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".neoload", "skills.lock.json")

	lf := &lock.File{Version: 1, Installs: []lock.Install{
		{
			Source:              "o/r@sk",
			ResolvedCommit:      "aabbccd",
			InstalledTargets:    []string{"/x/.claude/skills/sk"},
			InstalledAgentNames: []string{"claude"},
			UpdatedAt:           time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC),
		},
	}}
	lock.Write(lockPath, lf)

	buf := captureOutput(t)
	if err := runList(dir, false, false); err != nil {
		t.Fatalf("runList: %v", err)
	}
	if !strings.Contains(buf.String(), "claude") {
		t.Errorf("expected agent name 'claude' in output, got: %s", buf.String())
	}
}

func TestRunListFallbackAgentNames(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".neoload", "skills.lock.json")

	// Old lock file without InstalledAgentNames.
	lf := &lock.File{Version: 1, Installs: []lock.Install{
		{
			Source:           "o/r@sk",
			ResolvedCommit:   "aabbccd",
			InstalledTargets: []string{"/x/.claude/skills/sk"},
			UpdatedAt:        time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC),
		},
	}}
	lock.Write(lockPath, lf)

	buf := captureOutput(t)
	if err := runList(dir, false, false); err != nil {
		t.Fatalf("runList: %v", err)
	}
	if !strings.Contains(buf.String(), "claude") {
		t.Errorf("expected fallback agent name 'claude' in output, got: %s", buf.String())
	}
}

func TestRunListGlobal(t *testing.T) {
	buf := captureOutput(t)

	// Global with no lock file should print "No skills installed."
	err := runList(t.TempDir(), true, false)
	if err != nil {
		t.Fatalf("runList global: %v", err)
	}
	if !strings.Contains(buf.String(), "No skills installed") {
		t.Errorf("expected 'No skills installed', got: %s", buf.String())
	}
}

func TestRunListJSONEmpty(t *testing.T) {
	buf := captureOutput(t)

	err := runList(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("runList --json: %v", err)
	}

	var entries []listEntry
	if err := json.Unmarshal(buf.Bytes(), &entries); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(entries) != 0 {
		t.Errorf("expected empty array, got %d entries", len(entries))
	}
}

func TestRunListJSONPopulated(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".neoload", "skills.lock.json")

	now := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	lf := &lock.File{Version: 1, Installs: []lock.Install{
		{
			Source:           "anthropic/skills@xlsx",
			ResolvedCommit:   "aabbccddee",
			InstalledTargets: []string{"/project/.claude/skills/xlsx"},
			InstalledAt:      now,
			UpdatedAt:        now,
			CLIVersion:       "0.1.0",
		},
	}}
	if err := lock.Write(lockPath, lf); err != nil {
		t.Fatal(err)
	}

	buf := captureOutput(t)

	if err := runList(dir, false, true); err != nil {
		t.Fatalf("runList --json: %v", err)
	}

	out := buf.String()

	// Should be valid JSON.
	var entries []listEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Source != "anthropic/skills@xlsx" {
		t.Errorf("Source = %q, want %q", e.Source, "anthropic/skills@xlsx")
	}
	if e.Commit != "aabbccddee" {
		t.Errorf("Commit = %q, want %q", e.Commit, "aabbccddee")
	}
	if e.CLIVersion != "0.1.0" {
		t.Errorf("CLIVersion = %q, want %q", e.CLIVersion, "0.1.0")
	}
	if len(e.Targets) != 1 || e.Targets[0] != "/project/.claude/skills/xlsx" {
		t.Errorf("Targets = %v, want [/project/.claude/skills/xlsx]", e.Targets)
	}

	// No ANSI escape sequences.
	if strings.Contains(out, "\033[") {
		t.Error("JSON output should not contain ANSI escape sequences")
	}
}

func TestNewListCmd(t *testing.T) {
	cmd := newListCmd()
	if cmd.Flags().Lookup("global") == nil {
		t.Error("flag --global not registered")
	}
	if cmd.Flags().Lookup("json") == nil {
		t.Error("flag --json not registered")
	}
}

// ─── remove ───────────────────────────────────────────────────────────────────

// setupInstalledSkill creates a project directory with a skill installed on
// disk and a matching lock file entry. Returns the project dir.
func setupInstalledSkill(t *testing.T, repo, skill string) string {
	t.Helper()
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".claude"), 0755)

	skillDir := filepath.Join(dir, ".claude/skills", skill)
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# "+skill), 0644)

	lockPath := filepath.Join(dir, ".neoload", "skills.lock.json")
	lf := &lock.File{Version: 1, Installs: []lock.Install{
		{
			Scope:            "local",
			Source:           repo + "@" + skill,
			Repo:             repo,
			Skill:            skill,
			ResolvedCommit:   "abc123",
			InstalledTargets: []string{skillDir},
			InstalledAt:      time.Now().UTC(),
			UpdatedAt:        time.Now().UTC(),
		},
	}}
	if err := lock.Write(lockPath, lf); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunRemoveSuccess(t *testing.T) {
	dir := setupInstalledSkill(t, "o/r", "myskill")
	skillDir := filepath.Join(dir, ".claude/skills/myskill")

	buf := captureOutput(t)

	if err := runRemove(dir, "o/r@myskill", false, false); err != nil {
		t.Fatalf("runRemove: %v", err)
	}

	// Skill directory should be gone.
	if _, err := os.Stat(skillDir); err == nil {
		t.Error("skill directory should have been removed")
	}

	// Lock entry should be removed.
	lf, _ := lock.Read(filepath.Join(dir, ".neoload/skills.lock.json"))
	if len(lf.Installs) != 0 {
		t.Errorf("expected 0 lock entries after remove, got %d", len(lf.Installs))
	}

	if !strings.Contains(buf.String(), "myskill") {
		t.Errorf("output should mention skill name, got: %s", buf.String())
	}
}

func TestRunRemoveDryRun(t *testing.T) {
	dir := setupInstalledSkill(t, "o/r", "myskill")
	skillDir := filepath.Join(dir, ".claude/skills/myskill")

	buf := captureOutput(t)

	if err := runRemove(dir, "o/r@myskill", false, true); err != nil {
		t.Fatalf("runRemove dry-run: %v", err)
	}

	// Skill directory should still exist.
	if _, err := os.Stat(skillDir); err != nil {
		t.Error("dry-run should not remove skill directory")
	}

	// Lock should be unchanged.
	lf, _ := lock.Read(filepath.Join(dir, ".neoload/skills.lock.json"))
	if len(lf.Installs) != 1 {
		t.Errorf("dry-run should not modify lock file")
	}

	if !strings.Contains(buf.String(), "[dry-run]") {
		t.Errorf("expected [dry-run] in output, got: %s", buf.String())
	}
}

func TestRunRemoveNotInstalled(t *testing.T) {
	captureOutput(t)
	// No lock file exists — skill was never installed.
	err := runRemove(t.TempDir(), "o/r@sk", false, false)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 3 {
		t.Errorf("expected exitError{code:3}, got %v", err)
	}
}

func TestRunRemoveInvalidSource(t *testing.T) {
	captureOutput(t)
	err := runRemove(t.TempDir(), "bad-input", false, false)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected exitError{code:2}, got %v", err)
	}
}

func TestNewRemoveCmd(t *testing.T) {
	cmd := newRemoveCmd()
	for _, name := range []string{"global", "dry-run"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s not registered", name)
		}
	}
	if cmd.Flags().ShorthandLookup("g") == nil {
		t.Error("flag -g shorthand not registered")
	}
}

func TestNewRootCmdHasRemove(t *testing.T) {
	cmd := newRootCmd()
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "remove" {
			found = true
		}
	}
	if !found {
		t.Error("newRootCmd should have a 'remove' subcommand")
	}
}

func TestTypedErrors(t *testing.T) {
	t.Run("SkillError detected via errors.As", func(t *testing.T) {
		se := &github.SkillError{Kind: github.ErrRepoNotFound, Message: "not found"}
		wrapped := fmt.Errorf("resolve: %w", se)

		var target *github.SkillError
		if !errors.As(wrapped, &target) {
			t.Fatal("errors.As should find wrapped SkillError")
		}
		if target.Kind != github.ErrRepoNotFound {
			t.Errorf("Kind = %v, want ErrRepoNotFound", target.Kind)
		}
	})

	t.Run("RateLimitError is not SkillError", func(t *testing.T) {
		rle := &github.RateLimitError{Limit: 60, HasToken: false}
		var se *github.SkillError
		if errors.As(rle, &se) {
			t.Error("RateLimitError should not satisfy SkillError")
		}
	})

	t.Run("RateLimitError detected via errors.As", func(t *testing.T) {
		rle := &github.RateLimitError{Limit: 60, HasToken: false}
		wrapped := fmt.Errorf("fetch: %w", rle)

		var target *github.RateLimitError
		if !errors.As(wrapped, &target) {
			t.Fatal("errors.As should find wrapped RateLimitError")
		}
	})

	t.Run("plain error is neither", func(t *testing.T) {
		err := errors.New("dial tcp: connection refused")
		var se *github.SkillError
		var rle *github.RateLimitError
		if errors.As(err, &se) {
			t.Error("plain error should not satisfy SkillError")
		}
		if errors.As(err, &rle) {
			t.Error("plain error should not satisfy RateLimitError")
		}
	})
}

// ─── update ───────────────────────────────────────────────────────────────────

func TestRunUpdateSuccess(t *testing.T) {
	dir := setupInstalledSkill(t, "o/r", "myskill")
	buf := captureOutput(t)

	client := &fakeGHClient{
		sha:   "newsha1234567890",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# Updated"}),
	}

	err := runUpdate(context.Background(), dir, "o/r@myskill", false, false, client)
	if err != nil {
		t.Fatalf("runUpdate: %v", err)
	}

	// Lock should have new SHA.
	lf, _ := lock.Read(filepath.Join(dir, ".neoload/skills.lock.json"))
	if len(lf.Installs) != 1 {
		t.Fatalf("expected 1 install, got %d", len(lf.Installs))
	}
	if lf.Installs[0].ResolvedCommit != "newsha1234567890" {
		t.Errorf("commit = %q, want %q", lf.Installs[0].ResolvedCommit, "newsha1234567890")
	}

	out := buf.String()
	if !strings.Contains(out, "Updated") {
		t.Errorf("expected 'Updated' in output, got: %s", out)
	}
}

func TestRunUpdateAlreadyCurrent(t *testing.T) {
	dir := setupInstalledSkill(t, "o/r", "myskill")
	buf := captureOutput(t)

	// Return same SHA as installed ("abc123").
	client := &fakeGHClient{
		sha:   "abc123",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# Same"}),
	}

	err := runUpdate(context.Background(), dir, "o/r@myskill", false, false, client)
	if err != nil {
		t.Fatalf("runUpdate: %v", err)
	}

	if !strings.Contains(buf.String(), "already up to date") {
		t.Errorf("expected 'already up to date', got: %s", buf.String())
	}
}

func TestRunUpdateDryRun(t *testing.T) {
	dir := setupInstalledSkill(t, "o/r", "myskill")
	buf := captureOutput(t)

	client := &fakeGHClient{
		sha:   "newsha999",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# New"}),
	}

	err := runUpdate(context.Background(), dir, "o/r@myskill", false, true, client)
	if err != nil {
		t.Fatalf("runUpdate dry-run: %v", err)
	}

	// Lock should still have old SHA.
	lf, _ := lock.Read(filepath.Join(dir, ".neoload/skills.lock.json"))
	if lf.Installs[0].ResolvedCommit != "abc123" {
		t.Error("dry-run should not change lock file")
	}

	if !strings.Contains(buf.String(), "[dry-run]") {
		t.Errorf("expected [dry-run] in output, got: %s", buf.String())
	}
}

func TestRunUpdateNotInstalled(t *testing.T) {
	captureOutput(t)
	err := runUpdate(context.Background(), t.TempDir(), "o/r@missing", false, false, nil)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 3 {
		t.Errorf("expected exitError{code:3}, got %v", err)
	}
}

func TestRunUpdateAllMultiple(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".claude"), 0755)

	// Install two skills.
	skill1Dir := filepath.Join(dir, ".claude/skills/sk1")
	skill2Dir := filepath.Join(dir, ".claude/skills/sk2")
	os.MkdirAll(skill1Dir, 0755)
	os.MkdirAll(skill2Dir, 0755)
	os.WriteFile(filepath.Join(skill1Dir, "SKILL.md"), []byte("# sk1"), 0644)
	os.WriteFile(filepath.Join(skill2Dir, "SKILL.md"), []byte("# sk2"), 0644)

	lockPath := filepath.Join(dir, ".neoload", "skills.lock.json")
	now := time.Now().UTC()
	lf := &lock.File{Version: 1, Installs: []lock.Install{
		{Scope: "local", Source: "a/b@sk1", Repo: "a/b", Skill: "sk1", ResolvedCommit: "old1", InstalledTargets: []string{skill1Dir}, InstalledAt: now, UpdatedAt: now},
		{Scope: "local", Source: "a/b@sk2", Repo: "a/b", Skill: "sk2", ResolvedCommit: "old2", InstalledTargets: []string{skill2Dir}, InstalledAt: now, UpdatedAt: now},
	}}
	lock.Write(lockPath, lf)

	buf := captureOutput(t)

	callCount := 0
	client := &fakeGHClient{
		sha:   "new123",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# New"}),
	}
	// Use a counting wrapper to verify both skills are checked.
	countClient := &countingClient{inner: client, count: &callCount}

	err := runUpdateAll(context.Background(), dir, false, false, countClient)
	if err != nil {
		t.Fatalf("runUpdateAll: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 ResolveSkill calls, got %d", callCount)
	}

	out := buf.String()
	if !strings.Contains(out, "Updated") {
		t.Errorf("expected 'Updated' in output, got: %s", out)
	}
}

func TestRunUpdateAllEmpty(t *testing.T) {
	buf := captureOutput(t)

	err := runUpdateAll(context.Background(), t.TempDir(), false, false, nil)
	if err != nil {
		t.Fatalf("runUpdateAll empty: %v", err)
	}
	if !strings.Contains(buf.String(), "No skills installed") {
		t.Errorf("expected 'No skills installed', got: %s", buf.String())
	}
}

func TestNewUpdateCmd(t *testing.T) {
	cmd := newUpdateCmd()
	for _, name := range []string{"global", "dry-run", "all", "token"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s not registered", name)
		}
	}
}

func TestRunUpdateInvalidSource(t *testing.T) {
	captureOutput(t)
	err := runUpdate(context.Background(), t.TempDir(), "bad", false, false, nil)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected exitError{code:2}, got %v", err)
	}
}

func TestRunUpdateSkillError(t *testing.T) {
	dir := setupInstalledSkill(t, "o/r", "myskill")
	captureOutput(t)

	client := &fakeGHClient{err: &github.SkillError{
		Kind:    github.ErrRepoNotFound,
		Message: "not found",
	}}

	err := runUpdate(context.Background(), dir, "o/r@myskill", false, false, client)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 3 {
		t.Errorf("expected exitError{code:3}, got %v", err)
	}
}

func TestRunUpdateNetworkError(t *testing.T) {
	dir := setupInstalledSkill(t, "o/r", "myskill")
	captureOutput(t)

	client := &fakeGHClient{err: errors.New("connection refused")}

	err := runUpdate(context.Background(), dir, "o/r@myskill", false, false, client)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 5 {
		t.Errorf("expected exitError{code:5}, got %v", err)
	}
}

func TestRunUpdateAllDryRun(t *testing.T) {
	dir := setupInstalledSkill(t, "o/r", "myskill")
	buf := captureOutput(t)

	client := &fakeGHClient{
		sha:   "newsha999",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# New"}),
	}

	err := runUpdateAll(context.Background(), dir, false, true, client)
	if err != nil {
		t.Fatalf("runUpdateAll dry-run: %v", err)
	}

	// Lock should still have old SHA.
	lf, _ := lock.Read(filepath.Join(dir, ".neoload/skills.lock.json"))
	if lf.Installs[0].ResolvedCommit != "abc123" {
		t.Error("dry-run should not change lock file")
	}

	if !strings.Contains(buf.String(), "[dry-run]") {
		t.Errorf("expected [dry-run] in output, got: %s", buf.String())
	}
}

func TestRunUpdateAllSkipErrors(t *testing.T) {
	dir := setupInstalledSkill(t, "o/r", "myskill")
	buf := captureOutput(t)

	// Client returns error — should skip, not fail.
	client := &fakeGHClient{err: errors.New("network error")}

	err := runUpdateAll(context.Background(), dir, false, false, client)
	if err != nil {
		t.Fatalf("runUpdateAll should not fail on skip: %v", err)
	}

	if !strings.Contains(buf.String(), "skip") {
		t.Errorf("expected 'skip' in output, got: %s", buf.String())
	}
}

func TestRunUpdateAllAlreadyCurrent(t *testing.T) {
	dir := setupInstalledSkill(t, "o/r", "myskill")
	buf := captureOutput(t)

	client := &fakeGHClient{
		sha:   "abc123",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# Same"}),
	}

	err := runUpdateAll(context.Background(), dir, false, false, client)
	if err != nil {
		t.Fatalf("runUpdateAll: %v", err)
	}
	if !strings.Contains(buf.String(), "already up to date") {
		t.Errorf("expected 'already up to date', got: %s", buf.String())
	}
}

func TestNewUpdateCmdMutuallyExclusive(t *testing.T) {
	captureOutput(t)
	cmd := newRootCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"update", "--all", "o/r@sk"})
	err := cmd.Execute()
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected exitError{code:2} for --all + arg, got %v", err)
	}
}

func TestNewUpdateCmdMissingArg(t *testing.T) {
	captureOutput(t)
	cmd := newRootCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"update"})
	err := cmd.Execute()
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected exitError{code:2} for missing arg, got %v", err)
	}
}

func TestRunUpdateGlobal(t *testing.T) {
	captureOutput(t)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	// Set up a global lock entry.
	lockPath := filepath.Join(home, ".neoload", "skills.lock.json")
	globalSkillDir := filepath.Join(home, ".claude/skills/gsk")
	os.MkdirAll(globalSkillDir, 0755)
	os.WriteFile(filepath.Join(globalSkillDir, "SKILL.md"), []byte("# gsk"), 0644)

	lf := &lock.File{Version: 1, Installs: []lock.Install{
		{Scope: "global", Source: "o/r@gsk", Repo: "o/r", Skill: "gsk", ResolvedCommit: "old123",
			InstalledTargets: []string{globalSkillDir}, InstalledAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
	}}
	lock.Write(lockPath, lf)

	t.Cleanup(func() {
		os.RemoveAll(globalSkillDir)
		os.Remove(lockPath)
	})

	client := &fakeGHClient{
		sha:   "new456",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# Updated"}),
	}

	err = runUpdate(context.Background(), t.TempDir(), "o/r@gsk", true, false, client)
	if err != nil {
		t.Fatalf("runUpdate global: %v", err)
	}

	lf2, _ := lock.Read(lockPath)
	for _, inst := range lf2.Installs {
		if inst.Skill == "gsk" && inst.ResolvedCommit != "new456" {
			t.Errorf("expected updated commit, got %s", inst.ResolvedCommit)
		}
	}
}

func TestNewRootCmdHasUpdate(t *testing.T) {
	cmd := newRootCmd()
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "update" {
			found = true
		}
	}
	if !found {
		t.Error("newRootCmd should have an 'update' subcommand")
	}
}

// countingClient wraps a github.Client and counts calls.
type countingClient struct {
	inner github.Client
	count *int
}

func (c *countingClient) ResolveSkill(ctx context.Context, owner, repo, skill, ref string) (*github.ResolvedSkill, error) {
	*c.count++
	return c.inner.ResolveSkill(ctx, owner, repo, skill, ref)
}

func (c *countingClient) ListSkills(ctx context.Context, owner, repo, ref string) ([]github.SkillInfo, error) {
	return c.inner.ListSkills(ctx, owner, repo, ref)
}

// ─── timeout ──────────────────────────────────────────────────────────────────

// blockingClient blocks until the context is cancelled.
type blockingClient struct{}

func (b *blockingClient) ResolveSkill(ctx context.Context, _, _, _, _ string) (*github.ResolvedSkill, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingClient) ListSkills(ctx context.Context, _, _, _ string) ([]github.SkillInfo, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestRunAddTimeout(t *testing.T) {
	projectDir := t.TempDir()
	os.Mkdir(filepath.Join(projectDir, ".claude"), 0755)
	captureOutput(t)

	err := runAdd(context.Background(), projectDir, "o/r@sk", false, false, false, time.Millisecond, &blockingClient{})
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 5 {
		t.Errorf("expected exitError{code:5}, got %v", err)
	}
	if !strings.Contains(ee.Error(), "timed out") {
		t.Errorf("expected timeout message, got: %s", ee.Error())
	}
}

func TestNewAddCmdTimeoutFlag(t *testing.T) {
	cmd := newAddCmd()
	if cmd.Flags().Lookup("timeout") == nil {
		t.Error("flag --timeout not registered")
	}
}

func TestCountFiles(t *testing.T) {
	mfs := makeTestSkillFS(map[string]string{
		"a.md":   "a",
		"b.md":   "b",
		"c/d.sh": "d",
	})
	if n := countFiles(mfs); n != 3 {
		t.Errorf("countFiles = %d, want 3", n)
	}
}

// ─── search ──────────────────────────────────────────────────────────────────

func TestRunSearchSuccess(t *testing.T) {
	buf := captureOutput(t)

	client := &fakeGHClient{
		skills: []github.SkillInfo{
			{Name: "csv", Description: "Parse CSV data."},
			{Name: "xlsx", Description: "Read and write Excel files."},
		},
	}

	err := runSearch(context.Background(), "anthropic/skills", "", 0, false, client)
	if err != nil {
		t.Fatalf("runSearch: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "xlsx") {
		t.Errorf("expected 'xlsx' in output, got: %s", out)
	}
	if !strings.Contains(out, "csv") {
		t.Errorf("expected 'csv' in output, got: %s", out)
	}
	if !strings.Contains(out, "Read and write Excel files.") {
		t.Errorf("expected description in output, got: %s", out)
	}
	if !strings.Contains(out, "SKILL") {
		t.Errorf("expected table header in output, got: %s", out)
	}
}

func TestRunSearchEmpty(t *testing.T) {
	buf := captureOutput(t)

	client := &fakeGHClient{skills: []github.SkillInfo{}}

	err := runSearch(context.Background(), "o/r", "", 0, false, client)
	if err != nil {
		t.Fatalf("runSearch: %v", err)
	}
	if !strings.Contains(buf.String(), "No skills found") {
		t.Errorf("expected 'No skills found', got: %s", buf.String())
	}
}

func TestRunSearchJSON(t *testing.T) {
	buf := captureOutput(t)

	client := &fakeGHClient{
		skills: []github.SkillInfo{
			{Name: "xlsx", Description: "Excel stuff."},
		},
	}

	err := runSearch(context.Background(), "o/r", "", 0, true, client)
	if err != nil {
		t.Fatalf("runSearch --json: %v", err)
	}

	var entries []searchEntry
	if err := json.Unmarshal(buf.Bytes(), &entries); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "xlsx" {
		t.Errorf("Name = %q, want %q", entries[0].Name, "xlsx")
	}
	if entries[0].Description != "Excel stuff." {
		t.Errorf("Description = %q", entries[0].Description)
	}
	if entries[0].Source != "o/r@xlsx" {
		t.Errorf("Source = %q, want %q", entries[0].Source, "o/r@xlsx")
	}
	// No ANSI escape sequences.
	if strings.Contains(buf.String(), "\033[") {
		t.Error("JSON output should not contain ANSI escape sequences")
	}
}

func TestRunSearchJSONEmpty(t *testing.T) {
	buf := captureOutput(t)

	client := &fakeGHClient{skills: []github.SkillInfo{}}

	err := runSearch(context.Background(), "o/r", "", 0, true, client)
	if err != nil {
		t.Fatalf("runSearch --json empty: %v", err)
	}

	var entries []searchEntry
	if err := json.Unmarshal(buf.Bytes(), &entries); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty array, got %d entries", len(entries))
	}
}

func TestRunSearchInvalidRepo(t *testing.T) {
	captureOutput(t)
	err := runSearch(context.Background(), "bad-input", "", 0, false, nil)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected exitError{code:2}, got %v", err)
	}
}

func TestRunSearchRepoNotFound(t *testing.T) {
	captureOutput(t)

	client := &fakeGHClient{err: &github.SkillError{
		Kind:    github.ErrRepoNotFound,
		Repo:    "o/r",
		Message: "repository o/r not found",
	}}

	err := runSearch(context.Background(), "o/r", "", 0, false, client)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 3 {
		t.Errorf("expected exitError{code:3}, got %v", err)
	}
}

func TestRunSearchNetworkError(t *testing.T) {
	captureOutput(t)

	client := &fakeGHClient{err: errors.New("connection refused")}

	err := runSearch(context.Background(), "o/r", "", 0, false, client)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 5 {
		t.Errorf("expected exitError{code:5}, got %v", err)
	}
}

func TestRunSearchTimeout(t *testing.T) {
	captureOutput(t)
	err := runSearch(context.Background(), "o/r", "", time.Millisecond, false, &blockingClient{})
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 5 {
		t.Errorf("expected exitError{code:5}, got %v", err)
	}
	if !strings.Contains(ee.Error(), "timed out") {
		t.Errorf("expected timeout message, got: %s", ee.Error())
	}
}

func TestNewSearchCmd(t *testing.T) {
	cmd := newSearchCmd()
	for _, name := range []string{"token", "json", "timeout"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s not registered", name)
		}
	}
}

func TestNewSearchCmdMissingArg(t *testing.T) {
	captureOutput(t)
	cmd := newRootCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"search"})
	err := cmd.Execute()
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected exitError{code:2} for missing arg, got %v", err)
	}
}

func TestNewRootCmdHasSearch(t *testing.T) {
	cmd := newRootCmd()
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "search" {
			found = true
		}
	}
	if !found {
		t.Error("newRootCmd should have a 'search' subcommand")
	}
}
