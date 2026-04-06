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
	if got := buf.String(); !strings.Contains(got, "error: bad thing") {
		t.Errorf("Error output = %q, want to contain %q", got, "error: bad thing")
	}
}
