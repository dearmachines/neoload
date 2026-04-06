package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"testing/fstest"
)

// SkillInfo describes a skill found in a repository's skills/ directory.
type SkillInfo struct {
	Name        string
	Description string
}

// Client downloads and resolves skills from GitHub.
type Client interface {
	ResolveSkill(ctx context.Context, owner, repo, skill, ref string) (*ResolvedSkill, error)
	ListSkills(ctx context.Context, owner, repo, ref string) ([]SkillInfo, error)
}

// ResolvedSkill holds the result of resolving a skill from GitHub.
type ResolvedSkill struct {
	CommitSHA string
	Files     fs.FS // rooted at the skill directory
}

// HTTPClient is the real GitHub API client.
type HTTPClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// New creates a new HTTPClient. token may be empty for unauthenticated access.
func New(token string) *HTTPClient {
	return &HTTPClient{
		baseURL: "https://api.github.com",
		token:   token,
		http: &http.Client{
			// Strip Authorization on redirect (e.g. to S3 presigned URLs).
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > 0 {
					req.Header.Del("Authorization")
				}
				return nil
			},
		},
	}
}

func (c *HTTPClient) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.http.Do(req)
}

func (c *HTTPClient) defaultBranch(ctx context.Context, owner, repo string) (string, error) {
	resp, err := c.get(ctx, fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, repo))
	if err != nil {
		return "", fmt.Errorf("fetch repository info: %w", err)
	}
	defer resp.Body.Close()

	if rle := checkRateLimit(resp, c.token != ""); rle != nil {
		return "", rle
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		return "", &SkillError{
			Kind:    ErrRepoNotFound,
			Repo:    owner + "/" + repo,
			Message: fmt.Sprintf("repository %s/%s not found", owner, repo),
		}
	case http.StatusOK:
	default:
		return "", fmt.Errorf("fetch repository info: unexpected status %d", resp.StatusCode)
	}

	var r struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("decode repository info: %w", err)
	}
	return r.DefaultBranch, nil
}

func (c *HTTPClient) commitSHA(ctx context.Context, owner, repo, ref string) (string, error) {
	resp, err := c.get(ctx, fmt.Sprintf("%s/repos/%s/%s/commits/%s", c.baseURL, owner, repo, ref))
	if err != nil {
		return "", fmt.Errorf("fetch commit: %w", err)
	}
	defer resp.Body.Close()

	if rle := checkRateLimit(resp, c.token != ""); rle != nil {
		return "", rle
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch commit: unexpected status %d", resp.StatusCode)
	}

	var r struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("decode commit: %w", err)
	}
	return r.SHA, nil
}

func (c *HTTPClient) downloadZip(ctx context.Context, owner, repo, sha string) ([]byte, error) {
	resp, err := c.get(ctx, fmt.Sprintf("%s/repos/%s/%s/zipball/%s", c.baseURL, owner, repo, sha))
	if err != nil {
		return nil, fmt.Errorf("download archive: %w", err)
	}
	defer resp.Body.Close()

	if rle := checkRateLimit(resp, c.token != ""); rle != nil {
		return nil, rle
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download archive: unexpected status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// isFullSHA returns true if s looks like a 40-character hex commit SHA.
func isFullSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// ResolveSkill resolves a skill from GitHub. If ref is empty, the default branch
// is used. If ref is a full 40-char hex SHA, the commit lookup is skipped.
func (c *HTTPClient) ResolveSkill(ctx context.Context, owner, repo, skill, ref string) (*ResolvedSkill, error) {
	var sha string
	var err error

	switch {
	case isFullSHA(ref):
		// Full SHA provided — skip both defaultBranch and commitSHA lookups.
		sha = ref
	case ref != "":
		// Ref provided (tag, branch, short SHA) — skip defaultBranch, resolve commit.
		sha, err = c.commitSHA(ctx, owner, repo, ref)
		if err != nil {
			return nil, err
		}
	default:
		// No ref — resolve default branch then commit.
		branch, err := c.defaultBranch(ctx, owner, repo)
		if err != nil {
			return nil, err
		}
		sha, err = c.commitSHA(ctx, owner, repo, branch)
		if err != nil {
			return nil, err
		}
	}

	skillFS, err := c.resolveViaContents(ctx, owner, repo, skill, sha)
	if err != nil {
		// Fallback to zip on server errors (5xx).
		var se *SkillError
		var rle *RateLimitError
		if errors.As(err, &se) || errors.As(err, &rle) {
			return nil, err
		}
		if isServerError(err) {
			skillFS, err = c.resolveViaZip(ctx, owner, repo, skill, sha)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	if _, err := fs.Stat(skillFS, "SKILL.md"); err != nil {
		return nil, &SkillError{
			Kind:    ErrSkillInvalid,
			Skill:   skill,
			Repo:    owner + "/" + repo,
			Message: fmt.Sprintf("skill %q is missing SKILL.md", skill),
		}
	}

	return &ResolvedSkill{CommitSHA: sha, Files: skillFS}, nil
}

// contentsEntry represents a single item from the GitHub Contents API response.
type contentsEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Type     string `json:"type"` // "file" or "dir"
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
}

// resolveViaContents fetches the skill directory using the GitHub Contents API.
func (c *HTTPClient) resolveViaContents(ctx context.Context, owner, repo, skill, sha string) (fs.FS, error) {
	memFS := make(fstest.MapFS)
	if err := c.fetchContents(ctx, owner, repo, "skills/"+skill, sha, "", memFS); err != nil {
		return nil, err
	}
	if len(memFS) == 0 {
		return nil, &SkillError{
			Kind:    ErrSkillNotFound,
			Skill:   skill,
			Message: fmt.Sprintf("skill %q not found in repository", skill),
		}
	}
	return memFS, nil
}

// fetchContents recursively fetches directory contents from the GitHub Contents API.
func (c *HTTPClient) fetchContents(ctx context.Context, owner, repo, path, ref, relPrefix string, memFS fstest.MapFS) error {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", c.baseURL, owner, repo, path, ref)
	resp, err := c.get(ctx, url)
	if err != nil {
		return fmt.Errorf("fetch contents: %w", err)
	}
	defer resp.Body.Close()

	if rle := checkRateLimit(resp, c.token != ""); rle != nil {
		return rle
	}

	if resp.StatusCode == http.StatusNotFound {
		return &SkillError{
			Kind:    ErrSkillNotFound,
			Skill:   strings.TrimPrefix(path, "skills/"),
			Message: fmt.Sprintf("skill %q not found in repository", strings.TrimPrefix(path, "skills/")),
		}
	}

	if resp.StatusCode >= 500 {
		return &serverError{status: resp.StatusCode}
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch contents: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read contents response: %w", err)
	}

	// Try parsing as array (directory listing) first.
	var entries []contentsEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		// Try single file object.
		var single contentsEntry
		if err := json.Unmarshal(body, &single); err != nil {
			return fmt.Errorf("decode contents response: %w", err)
		}
		entries = []contentsEntry{single}
	}

	for _, entry := range entries {
		name := entry.Name
		if relPrefix != "" {
			name = relPrefix + "/" + entry.Name
		}

		switch entry.Type {
		case "file":
			data, err := base64.StdEncoding.DecodeString(
				strings.ReplaceAll(entry.Content, "\n", ""),
			)
			if err != nil {
				return fmt.Errorf("decode %s: %w", name, err)
			}
			memFS[name] = &fstest.MapFile{Data: data, Mode: 0644}
		case "dir":
			if err := c.fetchContents(ctx, owner, repo, path+"/"+entry.Name, ref, name, memFS); err != nil {
				return err
			}
		}
	}

	return nil
}

// resolveViaZip is the fallback path using the zip archive download.
func (c *HTTPClient) resolveViaZip(ctx context.Context, owner, repo, skill, sha string) (fs.FS, error) {
	zipData, err := c.downloadZip(ctx, owner, repo, sha)
	if err != nil {
		return nil, err
	}
	return extractSkill(zipData, skill)
}

// serverError is an internal sentinel for 5xx responses triggering zip fallback.
type serverError struct{ status int }

func (e *serverError) Error() string {
	return fmt.Sprintf("server error: status %d", e.status)
}

func isServerError(err error) bool {
	var se *serverError
	return errors.As(err, &se)
}

// extractSkill extracts files under skills/<skill>/ from the GitHub zip archive
// and returns an fs.FS rooted at the skill directory.
func extractSkill(zipData []byte, skill string) (fs.FS, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}

	// GitHub archives have a top-level directory like "owner-repo-sha/".
	// We strip that first component then look for skills/<skill>/.
	prefix := "skills/" + skill + "/"
	memFS := make(fstest.MapFS)

	for _, f := range zr.File {
		// Strip the top-level directory component.
		idx := strings.Index(f.Name, "/")
		if idx < 0 {
			continue
		}
		relPath := f.Name[idx+1:]

		if !strings.HasPrefix(relPath, prefix) {
			continue
		}
		skillRelPath := relPath[len(prefix):]
		if skillRelPath == "" || f.FileInfo().IsDir() {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", f.Name, err)
		}
		data, readErr := io.ReadAll(rc)
		rc.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", f.Name, readErr)
		}

		memFS[skillRelPath] = &fstest.MapFile{
			Data: data,
			Mode: f.Mode(),
		}
	}

	if len(memFS) == 0 {
		return nil, &SkillError{
			Kind:    ErrSkillNotFound,
			Skill:   skill,
			Message: fmt.Sprintf("skill %q not found in repository", skill),
		}
	}

	return memFS, nil
}

// ListSkills lists all skills in a repository's skills/ directory.
// For each skill, it fetches SKILL.md and extracts the first paragraph as description.
func (c *HTTPClient) ListSkills(ctx context.Context, owner, repo, ref string) ([]SkillInfo, error) {
	var sha string
	var err error

	switch {
	case isFullSHA(ref):
		sha = ref
	case ref != "":
		sha, err = c.commitSHA(ctx, owner, repo, ref)
		if err != nil {
			return nil, err
		}
	default:
		branch, err := c.defaultBranch(ctx, owner, repo)
		if err != nil {
			return nil, err
		}
		sha, err = c.commitSHA(ctx, owner, repo, branch)
		if err != nil {
			return nil, err
		}
	}

	// Fetch the skills/ directory listing.
	url := fmt.Sprintf("%s/repos/%s/%s/contents/skills?ref=%s", c.baseURL, owner, repo, sha)
	resp, err := c.get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer resp.Body.Close()

	if rle := checkRateLimit(resp, c.token != ""); rle != nil {
		return nil, rle
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no skills/ directory found in %s/%s", owner, repo)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list skills: unexpected status %d", resp.StatusCode)
	}

	var entries []contentsEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode skills listing: %w", err)
	}

	var skills []SkillInfo
	for _, e := range entries {
		if e.Type != "dir" {
			continue
		}
		desc := c.fetchSkillDescription(ctx, owner, repo, e.Name, sha)
		skills = append(skills, SkillInfo{Name: e.Name, Description: desc})
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

// fetchSkillDescription fetches SKILL.md for a skill and extracts a short description.
// Returns empty string on any error (best-effort).
func (c *HTTPClient) fetchSkillDescription(ctx context.Context, owner, repo, skill, sha string) string {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/skills/%s/SKILL.md?ref=%s", c.baseURL, owner, repo, skill, sha)
	resp, err := c.get(ctx, url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var entry contentsEntry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return ""
	}

	data, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(entry.Content, "\n", ""))
	if err != nil {
		return ""
	}

	return extractDescription(string(data))
}

// extractDescription extracts the first non-heading, non-empty line from markdown content.
// Skips YAML frontmatter (--- delimited blocks at the start of the file).
func extractDescription(content string) string {
	lines := strings.Split(content, "\n")
	i := 0

	// Skip YAML frontmatter.
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		i++
		for i < len(lines) && strings.TrimSpace(lines[i]) != "---" {
			i++
		}
		if i >= len(lines) {
			return "" // unclosed frontmatter
		}
		i++ // skip closing ---
	}

	for ; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}
