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
