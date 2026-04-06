package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
)

// makeZip creates a zip archive with the given file entries (name -> content).
func makeZip(files map[string]string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, _ := w.Create(name)
		f.Write([]byte(content))
	}
	w.Close()
	return buf.Bytes()
}

func TestExtractSkill(t *testing.T) {
	t.Run("valid skill", func(t *testing.T) {
		zipData := makeZip(map[string]string{
			"owner-repo-abc1234/skills/xlsx/SKILL.md":  "# XLSX",
			"owner-repo-abc1234/skills/xlsx/script.sh": "#!/bin/sh",
			"owner-repo-abc1234/skills/other/SKILL.md": "# Other",
		})

		fsys, err := extractSkill(zipData, "xlsx")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// SKILL.md should be accessible at root
		if _, err := fs.Stat(fsys, "SKILL.md"); err != nil {
			t.Errorf("SKILL.md not found: %v", err)
		}

		// script.sh should be accessible
		if _, err := fs.Stat(fsys, "script.sh"); err != nil {
			t.Errorf("script.sh not found: %v", err)
		}

		// files from another skill should not leak
		if _, err := fs.Stat(fsys, "other/SKILL.md"); err == nil {
			t.Error("other/SKILL.md should not be present")
		}
	})

	t.Run("skill not found", func(t *testing.T) {
		zipData := makeZip(map[string]string{
			"owner-repo-abc/skills/other/SKILL.md": "# Other",
		})
		_, err := extractSkill(zipData, "xlsx")
		if err == nil {
			t.Fatal("expected error for missing skill")
		}
	})

	t.Run("invalid zip", func(t *testing.T) {
		_, err := extractSkill([]byte("not a zip"), "xlsx")
		if err == nil {
			t.Fatal("expected error for invalid zip")
		}
	})

	t.Run("empty zip", func(t *testing.T) {
		zipData := makeZip(map[string]string{})
		_, err := extractSkill(zipData, "xlsx")
		if err == nil {
			t.Fatal("expected error for empty zip / missing skill")
		}
	})
}

func TestResolveSkillHTTP(t *testing.T) {
	const (
		owner = "testowner"
		repo  = "testrepo"
		skill = "myskill"
		sha   = "aabbccdd1122334455667788990011aabbccddee"
	)

	zipData := makeZip(map[string]string{
		"testowner-testrepo-aabbccd/skills/myskill/SKILL.md": "# MySkill",
		"testowner-testrepo-aabbccd/skills/myskill/helper.sh": "#!/bin/sh\necho hi",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testowner/testrepo":
			json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
		case "/repos/testowner/testrepo/commits/main":
			json.NewEncoder(w).Encode(map[string]string{"sha": sha})
		case "/repos/testowner/testrepo/zipball/" + sha:
			w.Header().Set("Content-Type", "application/zip")
			w.Write(zipData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	resolved, err := c.ResolveSkill(context.Background(), owner, repo, skill)
	if err != nil {
		t.Fatalf("ResolveSkill: %v", err)
	}

	if resolved.CommitSHA != sha {
		t.Errorf("CommitSHA = %q, want %q", resolved.CommitSHA, sha)
	}

	if _, err := fs.Stat(resolved.Files, "SKILL.md"); err != nil {
		t.Errorf("SKILL.md not in resolved files: %v", err)
	}
}

func TestResolveSkillMissingSkillMD(t *testing.T) {
	const sha = "deadbeef"

	// Skill exists but lacks SKILL.md
	zipData := makeZip(map[string]string{
		"owner-repo-dead/skills/myskill/script.sh": "#!/bin/sh",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r":
			json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
		case "/repos/o/r/commits/main":
			json.NewEncoder(w).Encode(map[string]string{"sha": sha})
		case "/repos/o/r/zipball/" + sha:
			w.Write(zipData)
		}
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	_, err := c.ResolveSkill(context.Background(), "o", "r", "myskill")
	if err == nil {
		t.Fatal("expected error for missing SKILL.md")
	}
}

func TestResolveSkillRepoNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	_, err := c.ResolveSkill(context.Background(), "o", "r", "skill")
	if err == nil {
		t.Fatal("expected error for 404 repository")
	}
}

func TestResolveSkillCommitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r":
			json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	_, err := c.ResolveSkill(context.Background(), "o", "r", "skill")
	if err == nil {
		t.Fatal("expected error when commit fetch fails")
	}
}

func TestResolveSkillDownloadError(t *testing.T) {
	const sha = "abc123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r":
			json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
		case "/repos/o/r/commits/main":
			json.NewEncoder(w).Encode(map[string]string{"sha": sha})
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	_, err := c.ResolveSkill(context.Background(), "o", "r", "skill")
	if err == nil {
		t.Fatal("expected error when download fails")
	}
}

func TestDefaultBranchBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	_, err := c.defaultBranch(context.Background(), "o", "r")
	if err == nil {
		t.Fatal("expected error for bad JSON response")
	}
}

func TestNewCheckRedirectStripsAuth(t *testing.T) {
	// Verify that the HTTP client strips Authorization on redirect.
	c := New("mytoken")
	if c.http == nil {
		t.Fatal("http client is nil")
	}
	// Create a fake redirect chain to exercise CheckRedirect.
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	req.Header.Set("Authorization", "Bearer mytoken")
	via := []*http.Request{req}

	newReq, _ := http.NewRequest("GET", "http://s3.amazonaws.com/bucket", nil)
	newReq.Header.Set("Authorization", "Bearer mytoken")

	err := c.http.CheckRedirect(newReq, via)
	if err != nil {
		t.Fatalf("CheckRedirect returned error: %v", err)
	}
	if newReq.Header.Get("Authorization") != "" {
		t.Error("Authorization header should be stripped on redirect")
	}
}
