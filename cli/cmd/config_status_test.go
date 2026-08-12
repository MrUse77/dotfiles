package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/release"
)

func TestConfigStatus_ReportsLegacyUnknownWithoutPreviousIdentity(t *testing.T) {
	t.Parallel()

	artifacts := filepath.Join(t.TempDir(), "artifacts")
	operations := newConfigRuntime(configRuntimeDependencies{
		paths:   configPaths{lock: "/state/lock", state: "/state/state.json", artifacts: artifacts},
		lock:    &stubConfigLock{},
		journal: &stubConfigJournal{outcome: release.JournalOutcomeCommitted},
		readState: func(string) (*release.State, error) {
			return nil, os.ErrNotExist
		},
	})

	var out bytes.Buffer
	if err := operations.Status(t.Context(), &out, configStatusRequest{}); err != nil {
		t.Fatalf("status: %v", err)
	}
	printed := out.String()
	for _, want := range []string{"current: legacy/unknown", "retained artifacts: 0", "orphan artifacts: 0", "journal: clean"} {
		if !strings.Contains(printed, want) {
			t.Fatalf("status output %q does not contain %q", printed, want)
		}
	}
	if strings.Contains(printed, "previous:") {
		t.Fatalf("legacy status unexpectedly reports previous identity: %q", printed)
	}
}

func TestConfigStatus_ReportsVerifiedRetentionAndOrphansAsJSON(t *testing.T) {
	t.Parallel()

	artifacts := filepath.Join(t.TempDir(), "artifacts")
	current := release.Identity{Tag: "config-v2.0.0", Digest: strings.Repeat("a", 64)}
	previous := release.Identity{Tag: "config-v1.0.0", Digest: strings.Repeat("b", 64)}
	orphan := strings.Repeat("c", 64)
	for _, digest := range []string{current.Digest, previous.Digest, orphan} {
		if err := os.MkdirAll(filepath.Join(artifacts, digest), 0o755); err != nil {
			t.Fatalf("create artifact %s: %v", digest, err)
		}
	}
	operations := newConfigRuntime(configRuntimeDependencies{
		paths:   configPaths{lock: "/state/lock", state: "/state/state.json", artifacts: artifacts},
		lock:    &stubConfigLock{},
		journal: &stubConfigJournal{outcome: release.JournalOutcomeCommitted},
		readState: func(string) (*release.State, error) {
			return &release.State{Current: &current, Previous: &previous, LastCompletedRunID: "run-2"}, nil
		},
	})

	var out bytes.Buffer
	if err := operations.Status(t.Context(), &out, configStatusRequest{JSON: true}); err != nil {
		t.Fatalf("status JSON: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode status JSON %q: %v", out.String(), err)
	}
	if got["current"] != current.Tag || got["current_digest"] != current.Digest || got["previous"] != previous.Tag || got["previous_digest"] != previous.Digest {
		t.Fatalf("status identities = %#v", got)
	}
	if got["retention_count"] != float64(2) || got["orphan_count"] != float64(1) || got["journal"] != "clean" {
		t.Fatalf("status retention = %#v", got)
	}
}

func TestConfigStatus_DisclosesUnresolvedJournalWithoutRecoveryMutation(t *testing.T) {
	t.Parallel()

	recoveryCalls := 0
	journal := &stubConfigJournal{
		outcome: release.JournalOutcomeUncommitted,
		records: []release.JournalRecord{
			{OpID: "run-crash", Phase: release.JournalOpStart, Tag: "config-v2.0.0", Digest: strings.Repeat("d", 64)},
			{OpID: "run-crash", Phase: release.JournalPrepared},
		},
	}
	operations := newConfigRuntime(configRuntimeDependencies{
		paths:   configPaths{lock: "/state/lock", state: "/state/state.json", artifacts: filepath.Join(t.TempDir(), "artifacts")},
		lock:    &stubConfigLock{},
		journal: journal,
		readState: func(string) (*release.State, error) {
			return &release.State{}, nil
		},
		recoverJournal: func() error {
			recoveryCalls++
			return nil
		},
	})

	var out bytes.Buffer
	if err := operations.Status(t.Context(), &out, configStatusRequest{}); err != nil {
		t.Fatalf("status unresolved journal: %v", err)
	}
	if !strings.Contains(out.String(), "journal: unresolved (uncommitted)") {
		t.Fatalf("status output = %q, want unresolved journal", out.String())
	}
	if recoveryCalls != 0 || len(journal.appends) != 0 {
		t.Fatalf("status mutated recovery state: recovery calls=%d appends=%d", recoveryCalls, len(journal.appends))
	}
}
