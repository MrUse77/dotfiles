package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/transaction"
	"github.com/MrUse77/dots-cli/pkg/release"
)

func TestConfigRollback_AbsentPreviousFailsBeforeMutationAndNetwork(t *testing.T) {
	t.Parallel()

	resolver := &stubConfigResolver{err: errors.New("network must remain offline")}
	events := []string{}
	operations := newConfigRuntime(configRuntimeDependencies{
		paths:    configPaths{lock: "/state/lock", state: "/state/state.json"},
		lock:     &stubConfigLock{},
		journal:  &stubConfigJournal{outcome: release.JournalOutcomeCommitted},
		resolver: resolver,
		readState: func(string) (*release.State, error) {
			return &release.State{Current: &release.Identity{Tag: "config-v2.0.0", Digest: strings.Repeat("b", 64)}}, nil
		},
		newTransaction: func(plan.InstallationPlan) configManagedTransaction {
			events = append(events, "transaction:new")
			return &stubConfigTransaction{events: &events, inventory: &transaction.Inventory{}}
		},
	})

	err := operations.Rollback(context.Background(), &bytes.Buffer{}, configRollbackRequest{Offline: true})
	if !errors.Is(err, release.ErrNoPreviousIdentity) {
		t.Fatalf("rollback error = %v, want ErrNoPreviousIdentity", err)
	}
	if resolver.calls != 0 {
		t.Fatalf("resolver calls = %d, want 0", resolver.calls)
	}
	if len(events) != 0 {
		t.Fatalf("mutation events = %v, want none", events)
	}
}

func TestConfigRollback_UsesRetainedArtifactOfflineAndSwapsIdentities(t *testing.T) {
	t.Parallel()

	events := []string{}
	digestA := strings.Repeat("a", 64)
	digestB := strings.Repeat("b", 64)
	previous := release.Identity{Tag: "config-v1.0.0", Digest: digestA}
	current := release.Identity{Tag: "config-v2.0.0", Digest: digestB}
	artifactRoot := filepath.Join(t.TempDir(), "artifacts", digestA)
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatalf("create retained artifact root: %v", err)
	}
	resolver := &stubConfigResolver{err: errors.New("network must remain offline")}
	cache := &recordingCache{events: &events, root: artifactRoot}
	configPlan, err := plan.NewInstallationPlan("run-rollback", nil)
	if err != nil {
		t.Fatalf("build rollback plan: %v", err)
	}
	var written *release.State
	operations := newConfigRuntime(configRuntimeDependencies{
		paths:      configPaths{home: t.TempDir(), lock: "/state/lock", state: "/state/state.json", themeCurrent: "/home/test/themes/current"},
		lock:       &stubConfigLock{},
		journal:    &stubConfigJournal{outcome: release.JournalOutcomeCommitted, events: &events},
		resolver:   resolver,
		cache:      cache,
		cliVersion: "v1.0.0",
		readState: func(string) (*release.State, error) {
			return &release.State{Current: &current, Previous: &previous, LastCompletedRunID: "run-current"}, nil
		},
		readManifest: func(path string) (release.Manifest, error) {
			events = append(events, "manifest")
			if path != filepath.Join(artifactRoot, release.ManifestFilename) {
				t.Fatalf("manifest path = %q", path)
			}
			return release.Manifest{SchemaVersion: "1", Catalog: []release.CatalogEntry{}}, nil
		},
		loadBaseline: func(*release.State) (configBaseline, error) { return nil, nil },
		buildPlan: func(root, _ string, _ release.Manifest, _ configBaseline, _ plan.StateReader) (plan.InstallationPlan, []string, error) {
			events = append(events, "plan")
			if root != artifactRoot {
				t.Fatalf("plan root = %q, want %q", root, artifactRoot)
			}
			return configPlan, []string{"tokyo-night"}, nil
		},
		prepareTheme: func(string, []string, string) (configThemeMutation, error) {
			return &stubThemeMutation{events: &events}, nil
		},
		newTransaction: func(plan.InstallationPlan) configManagedTransaction {
			return &stubConfigTransaction{events: &events, inventory: &transaction.Inventory{RunID: "run-rollback"}}
		},
		stateReader: mapStateReader{},
		writeState: func(state *release.State) error {
			written = state
			events = append(events, "state:capture")
			return nil
		},
	})

	var out bytes.Buffer
	if err := operations.Rollback(context.Background(), &out, configRollbackRequest{Offline: true}); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if resolver.calls != 0 {
		t.Fatalf("resolver calls = %d, want 0", resolver.calls)
	}
	if written == nil || written.Current == nil || *written.Current != previous || written.Previous == nil || *written.Previous != current {
		t.Fatalf("rollback state = %#v, want previous/current swapped", written)
	}
	if !strings.Contains(out.String(), "rolled back to config-v1.0.0") {
		t.Fatalf("rollback output = %q", out.String())
	}
	if len(events) == 0 || events[0] != "lookup" {
		t.Fatalf("rollback events = %v, want cache lookup first", events)
	}
}
