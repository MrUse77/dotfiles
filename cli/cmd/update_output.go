package cmd

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
)

// ttyDetector reports whether the supplied writer is connected to a terminal.
type ttyDetector func(io.Writer) bool

func isTTY(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && (isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd()))
}

// updateReporter implements StageReporter for both TTY and non-TTY outputs.
type updateReporter struct {
	out io.Writer
	tty bool
}

// newUpdateReporter returns a reporter for the given output and detector.
func newUpdateReporter(out io.Writer, detectTTY ttyDetector) StageReporter {
	return &updateReporter{out: out, tty: detectTTY(out)}
}

func (r *updateReporter) Start(stage UpdateStage) {
	if r.tty {
		fmt.Fprintf(r.out, "[%s] resolving...\n", stage)
		return
	}
	fmt.Fprintf(r.out, "stage=%s status=running\n", stage)
}

func (r *updateReporter) Complete(result StageResult) {
	if r.tty {
		r.completeTTY(result)
		return
	}
	r.completeNonTTY(result)
}

func (r *updateReporter) completeTTY(result StageResult) {
	label := fmt.Sprintf("[%s]", result.Stage)
	switch result.Status {
	case StageSucceeded:
		fmt.Fprintf(r.out, "%s %s\n", label, result.Detail)
	case StageSkipped:
		fmt.Fprintf(r.out, "%s skipped (%s)\n", label, result.Code)
	case StageFailed:
		fmt.Fprintf(r.out, "%s failed: %s\n", label, result.Detail)
	}
}

func (r *updateReporter) completeNonTTY(result StageResult) {
	parts := []string{
		fmt.Sprintf("stage=%s", result.Stage),
		fmt.Sprintf("status=%s", result.Status),
	}
	if result.Code != "" {
		parts = append(parts, fmt.Sprintf("code=%s", quoteValue(result.Code)))
	}
	if result.Detail != "" {
		parts = append(parts, fmt.Sprintf("detail=%s", quoteValue(result.Detail)))
	}
	if result.Rollback != "" {
		parts = append(parts, fmt.Sprintf("rollback=%s", result.Rollback))
	}
	if result.Err != nil {
		parts = append(parts, fmt.Sprintf("error=%s", quoteValue(result.Err.Error())))
	}
	fmt.Fprintln(r.out, strings.Join(parts, " "))
}

func quoteValue(v string) string {
	v = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, v)
	if strings.ContainsAny(v, " \t\n\r\"") {
		return strconv.Quote(v)
	}
	return v
}
