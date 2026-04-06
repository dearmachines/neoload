package source

import (
	"fmt"
	"strings"
)

// Source represents a parsed owner/repo@skill reference.
type Source struct {
	Repo  string // "owner/repo"
	Skill string // e.g. "xlsx"
}

// Parse parses a source string of the form "owner/repo@skill".
func Parse(s string) (*Source, error) {
	parts := strings.SplitN(s, "@", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid source %q: must be owner/repo@skill", s)
	}
	repo, skill := parts[0], parts[1]

	repoParts := strings.SplitN(repo, "/", 3)
	if len(repoParts) != 2 || repoParts[0] == "" || repoParts[1] == "" {
		return nil, fmt.Errorf("invalid source %q: repo must be owner/repo", s)
	}
	if skill == "" {
		return nil, fmt.Errorf("invalid source %q: skill name must not be empty", s)
	}

	return &Source{Repo: repo, Skill: skill}, nil
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
	return s.Repo + "@" + s.Skill
}
