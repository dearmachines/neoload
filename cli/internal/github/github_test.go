package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testowner/testrepo":
			json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
		case "/repos/testowner/testrepo/commits/main":
			json.NewEncoder(w).Encode(map[string]string{"sha": sha})
		case "/repos/testowner/testrepo/contents/skills/myskill":
			json.NewEncoder(w).Encode([]map[string]string{
				{
					"name":     "SKILL.md",
					"path":     "skills/myskill/SKILL.md",
					"type":     "file",
					"encoding": "base64",
					"content":  base64.StdEncoding.EncodeToString([]byte("# MySkill")),
				},
				{
					"name":     "helper.sh",
					"path":     "skills/myskill/helper.sh",
					"type":     "file",
					"encoding": "base64",
					"content":  base64.StdEncoding.EncodeToString([]byte("#!/bin/sh\necho hi")),
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	resolved, err := c.ResolveSkill(context.Background(), owner, repo, skill, "")
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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r":
			json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
		case "/repos/o/r/commits/main":
			json.NewEncoder(w).Encode(map[string]string{"sha": sha})
		case "/repos/o/r/contents/skills/myskill":
			// Skill exists but lacks SKILL.md
			json.NewEncoder(w).Encode([]map[string]string{
				{
					"name":     "script.sh",
					"path":     "skills/myskill/script.sh",
					"type":     "file",
					"encoding": "base64",
					"content":  base64.StdEncoding.EncodeToString([]byte("#!/bin/sh")),
				},
			})
		}
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	_, err := c.ResolveSkill(context.Background(), "o", "r", "myskill", "")
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

	_, err := c.ResolveSkill(context.Background(), "o", "r", "skill", "")
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

	_, err := c.ResolveSkill(context.Background(), "o", "r", "skill", "")
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

	_, err := c.ResolveSkill(context.Background(), "o", "r", "skill", "")
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

func TestRateLimit403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	_, err := c.ResolveSkill(context.Background(), "o", "r", "sk", "")
	if err == nil {
		t.Fatal("expected error for rate-limited 403")
	}
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("expected *RateLimitError, got %T: %v", err, err)
	}
	if rle.Limit != 60 {
		t.Errorf("Limit = %d, want 60", rle.Limit)
	}
	if rle.HasToken {
		t.Error("HasToken should be false for unauthenticated client")
	}
	if !containsSubstring(rle.Error(), "--token") {
		t.Errorf("error message should suggest --token, got: %s", rle.Error())
	}
}

func TestRateLimit429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New("mytoken")
	c.baseURL = srv.URL

	_, err := c.ResolveSkill(context.Background(), "o", "r", "sk", "")
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("expected *RateLimitError for 429, got %T: %v", err, err)
	}
	if !rle.HasToken {
		t.Error("HasToken should be true for authenticated client")
	}
	if containsSubstring(rle.Error(), "--token") {
		t.Errorf("error message should not suggest --token when authenticated, got: %s", rle.Error())
	}
}

func TestNonRateLimit403(t *testing.T) {
	// 403 without rate-limit headers (e.g. private repo) should not be RateLimitError.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	_, err := c.ResolveSkill(context.Background(), "o", "r", "sk", "")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	var rle *RateLimitError
	if errors.As(err, &rle) {
		t.Error("non-rate-limit 403 should not return *RateLimitError")
	}
}

func TestSkillErrorTypes(t *testing.T) {
	t.Run("repo not found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer srv.Close()

		c := New("")
		c.baseURL = srv.URL

		_, err := c.ResolveSkill(context.Background(), "o", "r", "sk", "")
		var se *SkillError
		if !errors.As(err, &se) {
			t.Fatalf("expected *SkillError, got %T: %v", err, err)
		}
		if se.Kind != ErrRepoNotFound {
			t.Errorf("Kind = %v, want ErrRepoNotFound", se.Kind)
		}
	})

	t.Run("skill not found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/o/r":
				json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
			case "/repos/o/r/commits/main":
				json.NewEncoder(w).Encode(map[string]string{"sha": "abc123"})
			case "/repos/o/r/contents/skills/missing":
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		c := New("")
		c.baseURL = srv.URL

		_, err := c.ResolveSkill(context.Background(), "o", "r", "missing", "")
		var se *SkillError
		if !errors.As(err, &se) {
			t.Fatalf("expected *SkillError, got %T: %v", err, err)
		}
		if se.Kind != ErrSkillNotFound {
			t.Errorf("Kind = %v, want ErrSkillNotFound", se.Kind)
		}
	})

	t.Run("skill invalid (missing SKILL.md)", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/o/r":
				json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
			case "/repos/o/r/commits/main":
				json.NewEncoder(w).Encode(map[string]string{"sha": "abc123"})
			case "/repos/o/r/contents/skills/sk":
				json.NewEncoder(w).Encode([]map[string]string{
					{
						"name":     "script.sh",
						"path":     "skills/sk/script.sh",
						"type":     "file",
						"encoding": "base64",
						"content":  base64.StdEncoding.EncodeToString([]byte("#!/bin/sh")),
					},
				})
			}
		}))
		defer srv.Close()

		c := New("")
		c.baseURL = srv.URL

		_, err := c.ResolveSkill(context.Background(), "o", "r", "sk", "")
		var se *SkillError
		if !errors.As(err, &se) {
			t.Fatalf("expected *SkillError, got %T: %v", err, err)
		}
		if se.Kind != ErrSkillInvalid {
			t.Errorf("Kind = %v, want ErrSkillInvalid", se.Kind)
		}
	})

	t.Run("errors.As works on wrapped SkillError", func(t *testing.T) {
		se := &SkillError{Kind: ErrRepoNotFound, Message: "not found"}
		wrapped := fmt.Errorf("resolve: %w", se)
		var target *SkillError
		if !errors.As(wrapped, &target) {
			t.Fatal("errors.As should find wrapped SkillError")
		}
	})
}

// ─── Contents API tests ──────────────────────────────────────────────────────

// contentsHandler returns an httptest handler that serves repo metadata, commit,
// and Contents API responses for the given skill files.
func contentsHandler(sha string, skillFiles map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/o/r":
			json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
		case r.URL.Path == "/repos/o/r/commits/main":
			json.NewEncoder(w).Encode(map[string]string{"sha": sha})
		case strings.HasPrefix(r.URL.Path, "/repos/o/r/contents/"):
			path := strings.TrimPrefix(r.URL.Path, "/repos/o/r/contents/")
			// Build directory listing or single file.
			var entries []map[string]string
			isDirHit := false
			for name, content := range skillFiles {
				fullPath := "skills/sk/" + name
				if fullPath == path {
					// Single file request.
					json.NewEncoder(w).Encode(map[string]string{
						"name":     name,
						"path":     fullPath,
						"type":     "file",
						"encoding": "base64",
						"content":  base64.StdEncoding.EncodeToString([]byte(content)),
					})
					return
				}
				if strings.HasPrefix(fullPath, path+"/") {
					isDirHit = true
					relName := strings.TrimPrefix(fullPath, path+"/")
					// Only include direct children.
					if !strings.Contains(relName, "/") {
						entries = append(entries, map[string]string{
							"name":     relName,
							"path":     fullPath,
							"type":     "file",
							"encoding": "base64",
							"content":  base64.StdEncoding.EncodeToString([]byte(content)),
						})
					} else {
						// Subdirectory entry.
						dirName := relName[:strings.Index(relName, "/")]
						found := false
						for _, e := range entries {
							if e["name"] == dirName {
								found = true
								break
							}
						}
						if !found {
							entries = append(entries, map[string]string{
								"name": dirName,
								"path": path + "/" + dirName,
								"type": "dir",
							})
						}
					}
				}
			}
			if isDirHit {
				json.NewEncoder(w).Encode(entries)
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func TestContentsAPIHappyPath(t *testing.T) {
	files := map[string]string{
		"SKILL.md": "# Test Skill",
		"run.sh":   "#!/bin/sh\necho hello",
	}
	srv := httptest.NewServer(contentsHandler("abc123", files))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	resolved, err := c.ResolveSkill(context.Background(), "o", "r", "sk", "")
	if err != nil {
		t.Fatalf("ResolveSkill: %v", err)
	}
	if resolved.CommitSHA != "abc123" {
		t.Errorf("CommitSHA = %q, want %q", resolved.CommitSHA, "abc123")
	}
	if _, err := fs.Stat(resolved.Files, "SKILL.md"); err != nil {
		t.Errorf("SKILL.md not found: %v", err)
	}
	if _, err := fs.Stat(resolved.Files, "run.sh"); err != nil {
		t.Errorf("run.sh not found: %v", err)
	}
}

func TestContentsAPISubdirectory(t *testing.T) {
	files := map[string]string{
		"SKILL.md":         "# Test",
		"sub/nested.sh":    "#!/bin/sh",
		"sub/deep/file.md": "content",
	}
	srv := httptest.NewServer(contentsHandler("def456", files))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	resolved, err := c.ResolveSkill(context.Background(), "o", "r", "sk", "")
	if err != nil {
		t.Fatalf("ResolveSkill: %v", err)
	}
	for _, path := range []string{"SKILL.md", "sub/nested.sh", "sub/deep/file.md"} {
		if _, err := fs.Stat(resolved.Files, path); err != nil {
			t.Errorf("%s not found: %v", path, err)
		}
	}
}

func TestContentsAPIFallbackToZip(t *testing.T) {
	zipData := makeZip(map[string]string{
		"o-r-abc/skills/sk/SKILL.md": "# From zip",
	})
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/o/r":
			json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
		case r.URL.Path == "/repos/o/r/commits/main":
			json.NewEncoder(w).Encode(map[string]string{"sha": "abc123"})
		case strings.HasPrefix(r.URL.Path, "/repos/o/r/contents/"):
			callCount++
			w.WriteHeader(http.StatusInternalServerError) // 5xx → triggers fallback
		case r.URL.Path == "/repos/o/r/zipball/abc123":
			w.Write(zipData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	resolved, err := c.ResolveSkill(context.Background(), "o", "r", "sk", "")
	if err != nil {
		t.Fatalf("ResolveSkill with fallback: %v", err)
	}
	if _, err := fs.Stat(resolved.Files, "SKILL.md"); err != nil {
		t.Errorf("SKILL.md not found after zip fallback: %v", err)
	}
	if callCount == 0 {
		t.Error("Contents API should have been attempted before fallback")
	}
}

func TestCheckRateLimitNil(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}}
	if rle := checkRateLimit(resp, false); rle != nil {
		t.Errorf("expected nil for 200, got %v", rle)
	}
}

func TestRateLimitErrorMessage(t *testing.T) {
	rle := &RateLimitError{Limit: 60, Remaining: 0, HasToken: false}
	msg := rle.Error()
	if !strings.Contains(msg, "rate limit") {
		t.Errorf("expected 'rate limit' in message, got: %s", msg)
	}
	if !strings.Contains(msg, "--token") {
		t.Errorf("expected '--token' suggestion, got: %s", msg)
	}

	rle2 := &RateLimitError{Limit: 5000, HasToken: true}
	msg2 := rle2.Error()
	if strings.Contains(msg2, "--token") {
		t.Errorf("should not suggest --token when authenticated, got: %s", msg2)
	}
}

func TestServerErrorMessage(t *testing.T) {
	se := &serverError{status: 502}
	if se.Error() != "server error: status 502" {
		t.Errorf("unexpected message: %s", se.Error())
	}
}

func containsSubstring(s, sub string) bool {
	return strings.Contains(s, sub)
}

func TestIsFullSHA(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"abc123def456789012345678901234567890abcd", true},
		{"0000000000000000000000000000000000000000", true},
		{"abc123", false}, // too short
		{"ABC123DEF456789012345678901234567890ABCD", false}, // uppercase
		{"ghijklmnopqrstuvwxyz12345678901234567890", false}, // non-hex
		{"", false},
	}
	for _, tt := range tests {
		if got := isFullSHA(tt.input); got != tt.want {
			t.Errorf("isFullSHA(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestResolveSkillWithRef(t *testing.T) {
	// When ref is provided, should skip defaultBranch call and use ref directly.
	commitCalled := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/commits/v1.0"):
			commitCalled = true
			json.NewEncoder(w).Encode(map[string]string{"sha": "aaaa000000000000000000000000000000000000"})
		case strings.Contains(r.URL.Path, "/contents/skills/sk"):
			json.NewEncoder(w).Encode([]contentsEntry{
				{Name: "SKILL.md", Type: "file", Encoding: "base64", Content: base64.StdEncoding.EncodeToString([]byte("# SK"))},
			})
		default:
			// defaultBranch should NOT be called
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.Error(w, "unexpected", 500)
		}
	}))
	defer ts.Close()

	c := &HTTPClient{baseURL: ts.URL, http: ts.Client()}
	resolved, err := c.ResolveSkill(context.Background(), "o", "r", "sk", "v1.0")
	if err != nil {
		t.Fatal(err)
	}
	if !commitCalled {
		t.Error("commitSHA should have been called with ref")
	}
	if resolved.CommitSHA != "aaaa000000000000000000000000000000000000" {
		t.Errorf("SHA = %q, want aaaa...", resolved.CommitSHA)
	}
}

func TestResolveSkillWithFullSHA(t *testing.T) {
	fullSHA := "bbbb111111111111111111111111111111111111"
	// With a full SHA, should skip both defaultBranch and commitSHA.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/contents/skills/sk"):
			json.NewEncoder(w).Encode([]contentsEntry{
				{Name: "SKILL.md", Type: "file", Encoding: "base64", Content: base64.StdEncoding.EncodeToString([]byte("# SK"))},
			})
		default:
			t.Errorf("unexpected request (should skip defaultBranch/commitSHA): %s", r.URL.Path)
			http.Error(w, "unexpected", 500)
		}
	}))
	defer ts.Close()

	c := &HTTPClient{baseURL: ts.URL, http: ts.Client()}
	resolved, err := c.ResolveSkill(context.Background(), "o", "r", "sk", fullSHA)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.CommitSHA != fullSHA {
		t.Errorf("SHA = %q, want %q", resolved.CommitSHA, fullSHA)
	}
}

// ─── ListSkills tests ───────────────────────────────────────────────────────

func TestListSkillsHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r":
			json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
		case "/repos/o/r/commits/main":
			json.NewEncoder(w).Encode(map[string]string{"sha": "abc123"})
		case "/repos/o/r/contents/skills":
			json.NewEncoder(w).Encode([]contentsEntry{
				{Name: "xlsx", Type: "dir"},
				{Name: "csv", Type: "dir"},
				{Name: "README.md", Type: "file"}, // should be ignored
			})
		case "/repos/o/r/contents/skills/xlsx/SKILL.md":
			json.NewEncoder(w).Encode(contentsEntry{
				Name:     "SKILL.md",
				Type:     "file",
				Encoding: "base64",
				Content:  base64.StdEncoding.EncodeToString([]byte("# XLSX\n\nRead and write Excel files.")),
			})
		case "/repos/o/r/contents/skills/csv/SKILL.md":
			json.NewEncoder(w).Encode(contentsEntry{
				Name:     "SKILL.md",
				Type:     "file",
				Encoding: "base64",
				Content:  base64.StdEncoding.EncodeToString([]byte("# CSV\n\nParse CSV data.")),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	skills, err := c.ListSkills(context.Background(), "o", "r", "")
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	// Should be sorted by name.
	if skills[0].Name != "csv" || skills[1].Name != "xlsx" {
		t.Errorf("skills not sorted: %v", skills)
	}
	if skills[0].Description != "Parse CSV data." {
		t.Errorf("csv description = %q", skills[0].Description)
	}
	if skills[1].Description != "Read and write Excel files." {
		t.Errorf("xlsx description = %q", skills[1].Description)
	}
}

func TestListSkillsNoSkillsDir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r":
			json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
		case "/repos/o/r/commits/main":
			json.NewEncoder(w).Encode(map[string]string{"sha": "abc123"})
		case "/repos/o/r/contents/skills":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	_, err := c.ListSkills(context.Background(), "o", "r", "")
	if err == nil {
		t.Fatal("expected error when skills/ directory not found")
	}
}

func TestListSkillsRepoNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	_, err := c.ListSkills(context.Background(), "o", "r", "")
	if err == nil {
		t.Fatal("expected error for repo not found")
	}
	var se *SkillError
	if !errors.As(err, &se) || se.Kind != ErrRepoNotFound {
		t.Errorf("expected SkillError{ErrRepoNotFound}, got %v", err)
	}
}

func TestListSkillsMissingSkillMD(t *testing.T) {
	// A skill directory without SKILL.md should still appear but with empty description.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r":
			json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
		case "/repos/o/r/commits/main":
			json.NewEncoder(w).Encode(map[string]string{"sha": "abc123"})
		case "/repos/o/r/contents/skills":
			json.NewEncoder(w).Encode([]contentsEntry{
				{Name: "nomd", Type: "dir"},
			})
		case "/repos/o/r/contents/skills/nomd/SKILL.md":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	skills, err := c.ListSkills(context.Background(), "o", "r", "")
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "nomd" {
		t.Errorf("name = %q, want %q", skills[0].Name, "nomd")
	}
	if skills[0].Description != "" {
		t.Errorf("description should be empty, got %q", skills[0].Description)
	}
}

func TestListSkillsWithRef(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/commits/v2.0":
			json.NewEncoder(w).Encode(map[string]string{"sha": "ref123"})
		case "/repos/o/r/contents/skills":
			json.NewEncoder(w).Encode([]contentsEntry{
				{Name: "sk", Type: "dir"},
			})
		case "/repos/o/r/contents/skills/sk/SKILL.md":
			json.NewEncoder(w).Encode(contentsEntry{
				Name: "SKILL.md", Type: "file", Encoding: "base64",
				Content: base64.StdEncoding.EncodeToString([]byte("# SK\n\nA skill.")),
			})
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	skills, err := c.ListSkills(context.Background(), "o", "r", "v2.0")
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "sk" {
		t.Errorf("unexpected skills: %v", skills)
	}
}

func TestListSkillsRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r":
			json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
		case "/repos/o/r/commits/main":
			json.NewEncoder(w).Encode(map[string]string{"sha": "abc"})
		case "/repos/o/r/contents/skills":
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL

	_, err := c.ListSkills(context.Background(), "o", "r", "")
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Errorf("expected RateLimitError, got %T: %v", err, err)
	}
}

func TestExtractDescription(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"heading then paragraph", "# Title\n\nThis is the description.", "This is the description."},
		{"heading then paragraph no blank", "# Title\nThis is the description.", "This is the description."},
		{"no heading", "Just a paragraph.", "Just a paragraph."},
		{"empty", "", ""},
		{"heading only", "# Title", ""},
		{"heading with blank lines", "# Title\n\n\nDesc here.", "Desc here."},
		{"multiline paragraph takes first", "# T\n\nLine one.\nLine two.", "Line one."},
		{"frontmatter then heading", "---\nname: foo\n---\n\n# Title\n\nThe real description.", "The real description."},
		{"frontmatter then paragraph", "---\ntitle: bar\n---\n\nJust text.", "Just text."},
		{"frontmatter only", "---\nname: foo\n---", ""},
		{"unclosed frontmatter", "---\nname: foo\nno closing", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDescription(tt.content)
			if got != tt.want {
				t.Errorf("extractDescription(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}
