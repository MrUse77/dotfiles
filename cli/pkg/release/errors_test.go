package release

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestReleaseErrorSentinels proves the config-release lifecycle sentinels exist,
// are non-nil, and each carries a distinct non-empty message. The verify phase
// depends on these sentinels being classifiable via errors.Is.
func TestReleaseErrorSentinels(t *testing.T) {
	sentinels := []error{
		ErrLockContended,
		ErrArtifactRejected,
		ErrIndeterminateJournal,
		ErrNoPreviousIdentity,
		ErrOfflineArtifactMissing,
		ErrUnboundForce,
	}
	seen := make(map[string]bool)
	for _, sentinel := range sentinels {
		if sentinel == nil {
			t.Fatalf("sentinel is nil")
		}
		msg := sentinel.Error()
		if strings.TrimSpace(msg) == "" {
			t.Fatalf("sentinel %v has an empty message", sentinel)
		}
		if seen[msg] {
			t.Fatalf("duplicate sentinel message %q", msg)
		}
		seen[msg] = true
	}
}

// TestReleaseErrorSentinels_WrappedIdentity proves callers can classify a
// wrapped sentinel with errors.Is instead of string matching.
func TestReleaseErrorSentinels_WrappedIdentity(t *testing.T) {
	if !errors.Is(fmt.Errorf("context: %w", ErrLockContended), ErrLockContended) {
		t.Fatal("errors.Is failed on wrapped ErrLockContended")
	}
	if errors.Is(ErrArtifactRejected, ErrLockContended) {
		t.Fatal("distinct sentinels must not compare equal")
	}
}

// TestInvalidConfigVersionError proves the typed rejection error used by
// ParseConfigVersion carries the offending value and a stable message.
func TestInvalidConfigVersionError(t *testing.T) {
	err := &InvalidConfigVersionError{Value: "latest"}
	if err.Value != "latest" {
		t.Fatalf("Value = %q, want latest", err.Value)
	}
	if msg := err.Error(); !strings.Contains(msg, "latest") {
		t.Fatalf("Error() = %q, want it to name the rejected value", msg)
	}
}
