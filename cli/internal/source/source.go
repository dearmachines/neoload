package source

import (
	"fmt"
	"strings"
)

// Source represents a parsed owner/repo@skill[:ref] reference.
type Source struct {
	Repo  string // "owner/repo"
	Skill string // e.g. "xlsx"
	Ref   string // optional pinned ref (tag, branch, or commit SHA); empty means default branch
}

// Parse parses a source string of the form "owner/repo@skill" or "owner/repo@skill:ref".
func Parse(s string) (*Source, error) {
	parts := strings.SplitN(s, "@", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid source %q: must be owner/repo@skill", s)
	}
	repo, skillRef := parts[0], parts[1]

	repoParts := strings.SplitN(repo, "/", 3)
	if len(repoParts) != 2 || repoParts[0] == "" || repoParts[1] == "" {
		return nil, fmt.Errorf("invalid source %q: repo must be owner/repo", s)
	}

	// Split skill and optional ref on ":".
	skill, ref, _ := strings.Cut(skillRef, ":")
	if skill == "" {
		return nil, fmt.Errorf("invalid source %q: skill name must not be empty", s)
	}
	if strings.Contains(skillRef, ":") && ref == "" {
		return nil, fmt.Errorf("invalid source %q: ref must not be empty when : is present", s)
	}

	return &Source{Repo: repo, Skill: skill, Ref: ref}, nil
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
	base := s.Repo + "@" + s.Skill
	if s.Ref != "" {
		return base + ":" + s.Ref
	}
	return base
}
