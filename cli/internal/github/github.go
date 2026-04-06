package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"testing/fstest"
)

// Client downloads and resolves skills from GitHub.
type Client interface {
	ResolveSkill(ctx context.Context, owner, repo, skill string) (*ResolvedSkill, error)
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

	switch resp.StatusCode {
	case http.StatusNotFound:
		return "", fmt.Errorf("repository %s/%s not found", owner, repo)
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download archive: unexpected status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// ResolveSkill resolves the default branch, pins a commit SHA, downloads the
// zip archive, extracts the requested skill directory, and validates SKILL.md.
func (c *HTTPClient) ResolveSkill(ctx context.Context, owner, repo, skill string) (*ResolvedSkill, error) {
	branch, err := c.defaultBranch(ctx, owner, repo)
	if err != nil {
		return nil, err
	}

	sha, err := c.commitSHA(ctx, owner, repo, branch)
	if err != nil {
		return nil, err
	}

	zipData, err := c.downloadZip(ctx, owner, repo, sha)
	if err != nil {
		return nil, err
	}

	skillFS, err := extractSkill(zipData, skill)
	if err != nil {
		return nil, err
	}

	if _, err := fs.Stat(skillFS, "SKILL.md"); err != nil {
		return nil, fmt.Errorf("skill %q is missing SKILL.md", skill)
	}

	return &ResolvedSkill{CommitSHA: sha, Files: skillFS}, nil
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
		return nil, fmt.Errorf("skill %q not found in repository", skill)
	}

	return memFS, nil
}
