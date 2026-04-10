package source

import (
	"fmt"
	"regexp"
	"strings"
)

// sourceRe matches owner/repo:skill with optional @ref.
var sourceRe = regexp.MustCompile(`^([^/]+)/([^/:]+):([^@]+?)(?:@(.+))?$`)

// Source represents a parsed owner/repo:skill[@ref] or local path:skill reference.
type Source struct {
	Repo  string // "owner/repo" for remote sources, absolute path for local sources
	Skill string // e.g. "xlsx"
	Ref   string // optional pinned ref (tag, branch, or commit SHA); empty means default branch
	Local bool   // true for local filesystem sources
}

// Parse parses a source string of the form "owner/repo:skill" or "owner/repo:skill@ref".
func Parse(s string) (*Source, error) {
	m := sourceRe.FindStringSubmatch(s)
	if m == nil {
		return nil, fmt.Errorf("invalid source %q: must be owner/repo:skill", s)
	}
	owner, repo, skill, ref := m[1], m[2], m[3], m[4]
	if owner == "" || repo == "" || skill == "" {
		return nil, fmt.Errorf("invalid source %q: must be owner/repo:skill", s)
	}
	return &Source{Repo: owner + "/" + repo, Skill: skill, Ref: ref}, nil
}

// Owner returns the repository owner.
func (s *Source) Owner() string {
	return strings.SplitN(s.Repo, "/", 2)[0]
}

// RepoName returns the repository name.
func (s *Source) RepoName() string {
	return strings.SplitN(s.Repo, "/", 2)[1]
}

// String returns the canonical source string.
func (s *Source) String() string {
	if s.Local {
		return "local:" + s.Repo + ":" + s.Skill
	}
	base := s.Repo + ":" + s.Skill
	if s.Ref != "" {
		return base + "@" + s.Ref
	}
	return base
}

// ParseLocal parses a local source string of the form "path:skill".
// The path is the directory containing a skills/<skill>/ structure.
func ParseLocal(s string) (*Source, error) {
	idx := strings.LastIndex(s, ":")
	if idx == -1 || idx == len(s)-1 {
		return nil, fmt.Errorf("invalid local source %q: must be path:skill", s)
	}
	path := s[:idx]
	skill := s[idx+1:]
	if path == "" {
		return nil, fmt.Errorf("invalid local source %q: path cannot be empty", s)
	}
	if skill == "" {
		return nil, fmt.Errorf("invalid local source %q: skill cannot be empty", s)
	}
	return &Source{Repo: path, Skill: skill, Local: true}, nil
}
