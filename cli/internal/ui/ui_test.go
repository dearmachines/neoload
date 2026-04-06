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
