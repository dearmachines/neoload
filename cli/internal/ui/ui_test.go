package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestInfo(t *testing.T) {
	var buf bytes.Buffer
	Out = &buf
	t.Cleanup(func() { Out = nil })

	Info("hello %s", "world")
	if got := buf.String(); !strings.Contains(got, "hello world") {
		t.Errorf("Info output = %q, want to contain %q", got, "hello world")
	}
}

func TestSuccess(t *testing.T) {
	var buf bytes.Buffer
	Out = &buf
	t.Cleanup(func() { Out = nil })

	Success("done %d", 42)
	if got := buf.String(); !strings.Contains(got, "done 42") {
		t.Errorf("Success output = %q, want to contain %q", got, "done 42")
	}
}

func TestError(t *testing.T) {
	var buf bytes.Buffer
	Err = &buf
	t.Cleanup(func() { Err = nil })

	Error("bad %s", "thing")
	if got := buf.String(); !strings.Contains(got, "bad thing") {
		t.Errorf("Error output = %q, want to contain %q", got, "bad thing")
	}
	if got := buf.String(); !strings.Contains(got, "✗") {
		t.Errorf("Error output = %q, want to contain ✗ prefix", got)
	}
}

func TestTable(t *testing.T) {
	var buf bytes.Buffer
	Out = &buf
	t.Cleanup(func() { Out = nil })

	Table([]string{"NAME", "VALUE"}, [][]string{
		{"foo", "bar"},
		{"baz", "qux"},
	})

	out := buf.String()
	if !strings.Contains(out, "NAME") {
		t.Errorf("table should contain header, got: %s", out)
	}
	if !strings.Contains(out, "foo") {
		t.Errorf("table should contain data, got: %s", out)
	}
	// Should have bordered output.
	if !strings.Contains(out, "│") {
		t.Errorf("table should have vertical border chars, got: %s", out)
	}
	if !strings.Contains(out, "─") {
		t.Errorf("table should have horizontal border chars, got: %s", out)
	}
}

func TestTableSingleRow(t *testing.T) {
	var buf bytes.Buffer
	Out = &buf
	t.Cleanup(func() { Out = nil })

	Table([]string{"A", "B"}, [][]string{{"x", "y"}})
	out := buf.String()
	if !strings.Contains(out, "x") {
		t.Errorf("table should contain row data, got: %s", out)
	}
	if !strings.Contains(out, "│") {
		t.Errorf("table should have border chars, got: %s", out)
	}
}

func TestTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	Out = &buf
	t.Cleanup(func() { Out = nil })

	Table([]string{"A", "B"}, [][]string{})
	if buf.Len() != 0 {
		t.Errorf("empty table should produce no output, got: %s", buf.String())
	}
}

func TestRenderInlineMd(t *testing.T) {
	tests := []struct {
		name  string
		input string
		// We check that formatting markers are removed and text is preserved.
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:            "bold",
			input:           "Install **skills** now",
			wantContains:    []string{"Install", "skills", "now"},
			wantNotContains: []string{"**"},
		},
		{
			name:            "inline code",
			input:           "Use `neoload add` to install",
			wantContains:    []string{"Use", "neoload add", "to install"},
			wantNotContains: []string{"`"},
		},
		{
			name:            "italic",
			input:           "This is *important* info",
			wantContains:    []string{"This is", "important", "info"},
			wantNotContains: []string{"*important*"},
		},
		{
			name:            "link",
			input:           "See [docs](https://example.com) for more",
			wantContains:    []string{"See", "docs", "for more"},
			wantNotContains: []string{"](", "https://example.com"},
		},
		{
			name:         "plain text unchanged",
			input:        "No formatting here",
			wantContains: []string{"No formatting here"},
		},
		{
			name:         "empty string",
			input:        "",
			wantContains: []string{""},
		},
		{
			name:            "mixed formatting",
			input:           "**Invocation:** `/seo $1 $2` where `$1` is the command",
			wantContains:    []string{"Invocation:", "/seo $1 $2", "where", "is the command"},
			wantNotContains: []string{"**", "`"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderInlineMd(tt.input)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("RenderInlineMd(%q) = %q, want to contain %q", tt.input, got, want)
				}
			}
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(got, notWant) {
					t.Errorf("RenderInlineMd(%q) = %q, should not contain %q", tt.input, got, notWant)
				}
			}
		})
	}
}

func TestStartSpinnerNonTTY(t *testing.T) {
	var buf bytes.Buffer
	Out = &buf
	t.Cleanup(func() { Out = nil })

	// Out is *bytes.Buffer, not *os.File, so isTTY returns false.
	s := StartSpinner("loading...")
	s.Stop()

	out := buf.String()
	if !strings.Contains(out, "loading...") {
		t.Errorf("spinner should print static message for non-TTY, got: %s", out)
	}
}
