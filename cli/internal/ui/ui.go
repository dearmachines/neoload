package ui

import (
	"fmt"
	"io"
	"os"
)

// Out and Err are the output writers; override in tests.
var Out io.Writer = os.Stdout
var Err io.Writer = os.Stderr

// Info prints an informational message to stdout.
func Info(format string, args ...any) {
	fmt.Fprintf(Out, format+"\n", args...)
}

// Success prints a success message to stdout.
func Success(format string, args ...any) {
	fmt.Fprintf(Out, format+"\n", args...)
}

// Error prints an error message to stderr.
func Error(format string, args ...any) {
	fmt.Fprintf(Err, "error: "+format+"\n", args...)
}
