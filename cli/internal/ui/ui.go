package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Out and Err are the output writers; override in tests.
var Out io.Writer = os.Stdout
var Err io.Writer = os.Stderr

func outRenderer() *lipgloss.Renderer {
	return lipgloss.NewRenderer(Out)
}

func errRenderer() *lipgloss.Renderer {
	return lipgloss.NewRenderer(Err)
}

// Info prints an informational message to stdout.
func Info(format string, args ...any) {
	fmt.Fprintf(Out, format+"\n", args...)
}

// Success prints a success message with a green ✓ prefix.
func Success(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	r := outRenderer()
	prefix := r.NewStyle().Foreground(lipgloss.Color("2")).Bold(true).Render("✓")
	fmt.Fprintf(Out, "%s %s\n", prefix, msg)
}

// Error prints an error message with a red ✗ prefix to stderr.
func Error(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	r := errRenderer()
	prefix := r.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render("✗")
	fmt.Fprintf(Err, "%s %s\n", prefix, msg)
}

// Table renders formatted columns with a styled header.
func Table(headers []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}

	// Calculate column widths.
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print header.
	r := outRenderer()
	headerStyle := r.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
	var hdr string
	for i, h := range headers {
		if i > 0 {
			hdr += "  "
		}
		hdr += fmt.Sprintf("%-*s", widths[i], h)
	}
	fmt.Fprintln(Out, headerStyle.Render(hdr))

	// Print rows.
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(Out, "  ")
			}
			if i < len(widths) {
				fmt.Fprintf(Out, "%-*s", widths[i], cell)
			}
		}
		fmt.Fprintln(Out)
	}
}

// Spinner displays an animated spinner in TTY mode.
type Spinner struct {
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// StartSpinner begins a spinner with the given message.
// In non-TTY mode, prints the message as static text.
func StartSpinner(msg string) *Spinner {
	s := &Spinner{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	if isTTY(Out) {
		go s.animate(msg)
	} else {
		Info(msg)
		close(s.done)
	}
	return s
}

// Stop ends the spinner animation and clears the line.
func (s *Spinner) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
	<-s.done
}

func (s *Spinner) animate(msg string) {
	defer close(s.done)
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	r := outRenderer()
	spinStyle := r.NewStyle().Foreground(lipgloss.Color("5"))

	for {
		select {
		case <-s.stop:
			fmt.Fprintf(Out, "\r\033[K")
			return
		case <-ticker.C:
			fmt.Fprintf(Out, "\r%s %s", spinStyle.Render(frames[i%len(frames)]), msg)
			i++
		}
	}
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
