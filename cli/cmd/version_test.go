package cmd

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })

	Version = "v0.1.0"
	cmd := versionCmd
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runVersion(cmd, nil); err != nil {
		t.Fatalf("runVersion() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "moonarch v0.1.0") {
		t.Errorf("output = %q, want moonarch v0.1.0", got)
	}
	if !strings.Contains(got, runtime.GOOS) {
		t.Errorf("output = %q, want GOOS %s", got, runtime.GOOS)
	}
}
