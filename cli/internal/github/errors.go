package github

import (
	"fmt"
	"net/http"
	"time"
)

// SkillErrorKind distinguishes categories of skill resolution failures.
type SkillErrorKind int

const (
	ErrRepoNotFound  SkillErrorKind = iota // repository does not exist or is inaccessible
	ErrSkillNotFound                       // skill directory not present in repo
	ErrSkillInvalid                        // skill exists but is invalid (e.g. missing SKILL.md)
)

// SkillError represents a user-actionable error during skill resolution.
type SkillError struct {
	Kind    SkillErrorKind
	Skill   string
	Repo    string
	Message string
}

func (e *SkillError) Error() string { return e.Message }

// RateLimitError indicates the GitHub API rate limit has been exceeded.
type RateLimitError struct {
	Limit     int
	Remaining int
	ResetAt   time.Time
	HasToken  bool
}

func (e *RateLimitError) Error() string {
	msg := fmt.Sprintf("GitHub API rate limit exceeded (resets %s)", e.ResetAt.Format(time.RFC3339))
	if !e.HasToken {
		msg += "; authenticate with --token or $GITHUB_TOKEN for higher limits"
	}
	return msg
}

// checkRateLimit inspects an HTTP response for rate-limit signals.
// Returns a *RateLimitError for 403 with X-RateLimit-Remaining: 0 or 429.
// Returns nil if the response is not a rate-limit error.
func checkRateLimit(resp *http.Response, hasToken bool) *RateLimitError {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}

	remaining := resp.Header.Get("X-RateLimit-Remaining")
	if resp.StatusCode == http.StatusForbidden && remaining != "0" {
		return nil // non-rate-limit 403 (e.g. private repo)
	}

	rle := &RateLimitError{HasToken: hasToken}

	if v := resp.Header.Get("X-RateLimit-Limit"); v != "" {
		fmt.Sscanf(v, "%d", &rle.Limit)
	}
	if remaining != "" {
		fmt.Sscanf(remaining, "%d", &rle.Remaining)
	}
	if v := resp.Header.Get("X-RateLimit-Reset"); v != "" {
		var epoch int64
		fmt.Sscanf(v, "%d", &epoch)
		rle.ResetAt = time.Unix(epoch, 0).UTC()
	}

	return rle
}
