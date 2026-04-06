package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
	sha   string
	files fs.FS
	err   error
}

func (f *fakeGHClient) ResolveSkill(_ context.Context, _, _, _ string) (*github.ResolvedSkill, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &github.ResolvedSkill{CommitSHA: f.sha, Files: f.files}, nil
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

	err := runAdd(context.Background(), projectDir, "o/r@myskill", false, false, false, client)
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

	err := runAdd(context.Background(), projectDir, "o/r@sk", false, true, false, client)
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

	err := runAdd(context.Background(), projectDir, "o/r@sk", false, false, true, client)
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

	captureOutput(t)

	client := &fakeGHClient{
		sha:   "newsha",
		files: makeTestSkillFS(map[string]string{"SKILL.md": "# X"}),
	}

	err := runAdd(context.Background(), projectDir, "o/r@sk", false, false, false, client)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 5 {
		t.Errorf("expected exitError{code:5}, got %v", err)
	}
}

func TestRunAddInvalidSource(t *testing.T) {
	captureOutput(t)
	err := runAdd(context.Background(), t.TempDir(), "bad-input", false, false, false, nil)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected exitError{code:2}, got %v", err)
	}
}

func TestRunAddNoTargets(t *testing.T) {
	captureOutput(t)
	// Empty dir with no agent markers.
	err := runAdd(context.Background(), t.TempDir(), "o/r@sk", false, false, false, nil)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 4 {
		t.Errorf("expected exitError{code:4}, got %v", err)
	}
}

func TestRunAddSkillNotFound(t *testing.T) {
	projectDir := t.TempDir()
	os.Mkdir(filepath.Join(projectDir, ".claude"), 0755)

	captureOutput(t)

	client := &fakeGHClient{err: errors.New("skill not found in repository")}

	err := runAdd(context.Background(), projectDir, "o/r@missing", false, false, false, client)
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

	err := runAdd(context.Background(), projectDir, "o/r@sk", false, false, false, client)
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
	err = runAdd(context.Background(), t.TempDir(), "o/r@sk", true, false, true, client)
	if err != nil {
		t.Fatalf("runAdd global: %v", err)
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

	err := runList(t.TempDir(), false)
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

	if err := runList(dir, false); err != nil {
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

func TestRunListGlobal(t *testing.T) {
	buf := captureOutput(t)

	// Global with no lock file should print "No skills installed."
	err := runList(t.TempDir(), true)
	if err != nil {
		t.Fatalf("runList global: %v", err)
	}
	if !strings.Contains(buf.String(), "No skills installed") {
		t.Errorf("expected 'No skills installed', got: %s", buf.String())
	}
}

func TestNewListCmd(t *testing.T) {
	cmd := newListCmd()
	if cmd.Flags().Lookup("global") == nil {
		t.Error("flag --global not registered")
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

func TestIsSkillError(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"repository testowner/testrepo not found", true},
		{"skill \"xlsx\" not found in archive", true},
		{"skill \"xlsx\" is missing SKILL.md", true},
		{"dial tcp: connection refused", false},
		{"unexpected status 500", false},
		{"", false},
	}
	for _, tt := range tests {
		var err error
		if tt.msg != "" {
			err = errors.New(tt.msg)
		}
		got := isSkillError(err)
		if got != tt.want {
			t.Errorf("isSkillError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
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
