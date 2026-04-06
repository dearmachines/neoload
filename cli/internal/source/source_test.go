package source

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		repo    string
		skill   string
		ref     string
	}{
		{
			name:  "valid",
			input: "anthropic/skills@xlsx",
			repo:  "anthropic/skills",
			skill: "xlsx",
		},
		{
			name:  "valid with hyphen",
			input: "my-org/my-repo@my-skill",
			repo:  "my-org/my-repo",
			skill: "my-skill",
		},
		{
			name:  "valid with ref tag",
			input: "o/r@skill:v1.0",
			repo:  "o/r",
			skill: "skill",
			ref:   "v1.0",
		},
		{
			name:  "valid with ref branch",
			input: "o/r@skill:main",
			repo:  "o/r",
			skill: "skill",
			ref:   "main",
		},
		{
			name:  "valid with ref commit SHA",
			input: "o/r@skill:abc123def456789012345678901234567890abcd",
			repo:  "o/r",
			skill: "skill",
			ref:   "abc123def456789012345678901234567890abcd",
		},
		{
			name:    "missing @",
			input:   "anthropic/skills",
			wantErr: true,
		},
		{
			name:    "multiple @",
			input:   "anthropic/skills@xlsx@extra",
			repo:    "anthropic/skills",
			skill:   "xlsx@extra",
			wantErr: false, // skill may contain @ since we use SplitN(s, "@", 2)
		},
		{
			name:    "missing owner",
			input:   "/skills@xlsx",
			wantErr: true,
		},
		{
			name:    "missing repo name",
			input:   "anthropic/@xlsx",
			wantErr: true,
		},
		{
			name:    "single segment repo",
			input:   "anthropic@xlsx",
			wantErr: true,
		},
		{
			name:    "empty skill",
			input:   "anthropic/skills@",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "empty ref after colon",
			input:   "o/r@skill:",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			if got.Repo != tt.repo {
				t.Errorf("Repo = %q, want %q", got.Repo, tt.repo)
			}
			if got.Skill != tt.skill {
				t.Errorf("Skill = %q, want %q", got.Skill, tt.skill)
			}
			if got.Ref != tt.ref {
				t.Errorf("Ref = %q, want %q", got.Ref, tt.ref)
			}
		})
	}
}

func TestSourceMethods(t *testing.T) {
	s := &Source{Repo: "anthropic/skills", Skill: "xlsx"}

	if got := s.Owner(); got != "anthropic" {
		t.Errorf("Owner() = %q, want %q", got, "anthropic")
	}
	if got := s.RepoName(); got != "skills" {
		t.Errorf("RepoName() = %q, want %q", got, "skills")
	}
	if got := s.String(); got != "anthropic/skills@xlsx" {
		t.Errorf("String() = %q, want %q", got, "anthropic/skills@xlsx")
	}
}

func TestSourceStringWithRef(t *testing.T) {
	s := &Source{Repo: "o/r", Skill: "sk", Ref: "v1.0"}
	if got := s.String(); got != "o/r@sk:v1.0" {
		t.Errorf("String() = %q, want %q", got, "o/r@sk:v1.0")
	}
}
