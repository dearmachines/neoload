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
			input: "anthropic/skills:xlsx",
			repo:  "anthropic/skills",
			skill: "xlsx",
		},
		{
			name:  "valid with hyphen",
			input: "my-org/my-repo:my-skill",
			repo:  "my-org/my-repo",
			skill: "my-skill",
		},
		{
			name:  "valid with ref tag",
			input: "o/r:skill@v1.0",
			repo:  "o/r",
			skill: "skill",
			ref:   "v1.0",
		},
		{
			name:  "valid with ref branch",
			input: "o/r:skill@main",
			repo:  "o/r",
			skill: "skill",
			ref:   "main",
		},
		{
			name:  "valid with ref commit SHA",
			input: "o/r:skill@abc123def456789012345678901234567890abcd",
			repo:  "o/r",
			skill: "skill",
			ref:   "abc123def456789012345678901234567890abcd",
		},
		{
			name:  "valid with short commit SHA",
			input: "shadcn-ui/ui:shadcn@fc62d57",
			repo:  "shadcn-ui/ui",
			skill: "shadcn",
			ref:   "fc62d57",
		},
		{
			name:    "missing colon (no skill)",
			input:   "anthropic/skills",
			wantErr: true,
		},
		{
			name:    "missing owner",
			input:   "/skills:xlsx",
			wantErr: true,
		},
		{
			name:    "missing repo name",
			input:   "anthropic/:xlsx",
			wantErr: true,
		},
		{
			name:    "single segment (no slash)",
			input:   "anthropic:xlsx",
			wantErr: true,
		},
		{
			name:    "empty skill",
			input:   "anthropic/skills:",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "empty ref after @",
			input:   "o/r:skill@",
			wantErr: true,
		},
		{
			name:    "@ without colon",
			input:   "o/r@skill",
			wantErr: true,
		},
		{
			name:  "skill with dots",
			input: "o/r:skill.v2",
			repo:  "o/r",
			skill: "skill.v2",
		},
		{
			name:  "skill with underscores",
			input: "o/r:my_skill",
			repo:  "o/r",
			skill: "my_skill",
		},
		{
			name:  "owner with dots",
			input: "my.org/repo:skill",
			repo:  "my.org/repo",
			skill: "skill",
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
	if got := s.String(); got != "anthropic/skills:xlsx" {
		t.Errorf("String() = %q, want %q", got, "anthropic/skills:xlsx")
	}
}

func TestSourceStringWithRef(t *testing.T) {
	s := &Source{Repo: "o/r", Skill: "sk", Ref: "v1.0"}
	if got := s.String(); got != "o/r:sk@v1.0" {
		t.Errorf("String() = %q, want %q", got, "o/r:sk@v1.0")
	}
}

func TestSourceStringLocal(t *testing.T) {
	s := &Source{Repo: "/tmp/my-skills", Skill: "xlsx", Local: true}
	if got := s.String(); got != "local:/tmp/my-skills:xlsx" {
		t.Errorf("String() = %q, want %q", got, "local:/tmp/my-skills:xlsx")
	}
}

func TestParseLocal(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		path    string
		skill   string
	}{
		{
			name:  "absolute path",
			input: "/tmp/my-skills:xlsx",
			path:  "/tmp/my-skills",
			skill: "xlsx",
		},
		{
			name:  "relative path",
			input: "./my-skills:xlsx",
			path:  "./my-skills",
			skill: "xlsx",
		},
		{
			name:  "relative path without dot",
			input: "my-skills:xlsx",
			path:  "my-skills",
			skill: "xlsx",
		},
		{
			name:  "home-relative path",
			input: "~/projects/skills:my-skill",
			path:  "~/projects/skills",
			skill: "my-skill",
		},
		{
			name:  "path with colons uses last colon",
			input: "/tmp/a:b:skill",
			path:  "/tmp/a:b",
			skill: "skill",
		},
		{
			name:    "missing colon",
			input:   "/tmp/my-skills",
			wantErr: true,
		},
		{
			name:    "empty skill",
			input:   "/tmp/my-skills:",
			wantErr: true,
		},
		{
			name:    "empty path",
			input:   ":skill",
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
			got, err := ParseLocal(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseLocal(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLocal(%q) unexpected error: %v", tt.input, err)
			}
			if got.Repo != tt.path {
				t.Errorf("Repo = %q, want %q", got.Repo, tt.path)
			}
			if got.Skill != tt.skill {
				t.Errorf("Skill = %q, want %q", got.Skill, tt.skill)
			}
			if !got.Local {
				t.Error("Local should be true")
			}
		})
	}
}
