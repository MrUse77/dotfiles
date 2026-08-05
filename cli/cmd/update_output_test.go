package cmd

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestUpdateReporter_NonTTY(t *testing.T) {
	var out bytes.Buffer
	reporter := newUpdateReporter(&out, func(io.Writer) bool { return false })

	reporter.Start(StageRelease)
	reporter.Complete(StageResult{Stage: StageRelease, Status: StageSucceeded, Code: "resolved", Detail: "resolved v1.1.0"})
	reporter.Start(StageBinary)
	reporter.Complete(StageResult{Stage: StageBinary, Status: StageSucceeded, Code: "replaced", Detail: "replaced"})

	got := out.String()
	if strings.Contains(got, "\x1b") {
		t.Fatalf("non-TTY output contains ANSI escapes")
	}
	for _, want := range []string{
		"stage=release status=running",
		"stage=release status=success",
		"code=resolved",
		"detail=\"resolved v1.1.0\"",
		"stage=binary status=running",
		"stage=binary status=success",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestUpdateReporter_NonTTYIsDeterministic(t *testing.T) {
	results := []StageResult{
		{Stage: StageRelease, Status: StageSucceeded, Detail: "resolved v1.1.0"},
		{Stage: StageBinary, Status: StageSkipped, Code: "already-current", Detail: "binary already up to date"},
		{Stage: StageRepository, Status: StageSucceeded, Detail: "reconciled at v1.1.0"},
		{Stage: StageConfiguration, Status: StageSucceeded, Detail: "configuration reapplied"},
	}
	var first, second bytes.Buffer
	for _, out := range []*bytes.Buffer{&first, &second} {
		reporter := newUpdateReporter(out, func(io.Writer) bool { return false })
		for _, r := range results {
			reporter.Start(r.Stage)
			reporter.Complete(r)
		}
	}
	if first.String() != second.String() {
		t.Fatalf("non-TTY output is not deterministic:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}
}

func TestUpdateReporter_NonTTYFailureOrdering(t *testing.T) {
	var out bytes.Buffer
	reporter := newUpdateReporter(&out, func(io.Writer) bool { return false })
	reporter.Start(StageRelease)
	reporter.Complete(StageResult{Stage: StageRelease, Status: StageFailed, Code: "transport", Detail: "network error", Err: errors.New("network error")})
	got := out.String()
	if !strings.Contains(got, "stage=release status=failed") {
		t.Fatalf("missing failed status:\n%s", got)
	}
}

func TestUpdateReporter_TTY(t *testing.T) {
	var out bytes.Buffer
	reporter := newUpdateReporter(&out, func(io.Writer) bool { return true })
	reporter.Start(StageRelease)
	reporter.Complete(StageResult{Stage: StageRelease, Status: StageSucceeded, Detail: "resolved v1.1.0"})
	reporter.Start(StageBinary)
	reporter.Complete(StageResult{Stage: StageBinary, Status: StageSkipped, Code: "already-current", Detail: "binary already up to date"})

	got := out.String()
	if !strings.Contains(got, "[release]") {
		t.Fatalf("TTY output missing [release]:\n%s", got)
	}
	if !strings.Contains(got, "[binary]") {
		t.Fatalf("TTY output missing [binary]:\n%s", got)
	}
}

func TestUpdateReporter_FailedStageTTY(t *testing.T) {
	var out bytes.Buffer
	reporter := newUpdateReporter(&out, func(io.Writer) bool { return true })
	reporter.Complete(StageResult{Stage: StageConfiguration, Status: StageFailed, Detail: "configuration failed"})
	got := out.String()
	if !strings.Contains(got, "failed") {
		t.Fatalf("TTY output missing failed label:\n%s", got)
	}
}

func TestIsTTY(t *testing.T) {
	if isTTY(&bytes.Buffer{}) {
		t.Fatalf("bytes.Buffer should not be a TTY")
	}
}

func TestQuoteValue(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		{"has spaces", `"has spaces"`},
		{"has\ttab", `hastab`},
		{"has\nnewline", "hasnewline"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := quoteValue(tt.in); got != tt.want {
				t.Fatalf("quoteValue(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
