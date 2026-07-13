package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

func TestInstallDiscovererIsReadOnlyAndIncludesManagedTargets(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".config"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".zshrc"), []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := (installDiscoverer{}).Discover(repo, home, plan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %d, want config and root file", len(targets))
	}
	after, err := os.ReadDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("planning mutated repository entries: before=%d after=%d", len(before), len(after))
	}
}

func TestInstallDevModeHasExplicitUnsupportedPlanningResult(t *testing.T) {
	if !strings.Contains(ErrDevModeUnsupported.Error(), "not supported") {
		t.Fatalf("unexpected dev-mode result: %v", ErrDevModeUnsupported)
	}
}

func TestPrintExecutionReportIncludesFingerprintAndRetainedBackup(t *testing.T) {
	var out bytes.Buffer
	printExecutionReport(&out, &report.ExecutionReport{
		Fingerprint: "abc",
		ManagedTargets: []report.TargetOutcome{{
			Destination: "/home/user/.zshrc", Status: report.TargetMutated, BackupPath: "/backup/zshrc",
		}},
		ExternalActions: []report.ActionOutcome{{Description: "install packages", Status: report.ActionCompleted}},
	})
	for _, want := range []string{"abc", "/home/user/.zshrc", "/backup/zshrc", "install packages"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report output missing %q: %s", want, out.String())
		}
	}
}
