package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestThemePhase_PreservesValidRelativeSelection(t *testing.T) {
	t.Parallel()

	current := newThemeFixture(t, "./tokyo-night")
	phase, err := prepareThemePhase(current, []string{"tokyo-night", "catppuccin"}, "")
	if err != nil {
		t.Fatalf("prepare theme phase: %v", err)
	}
	if err := phase.Commit(); err != nil {
		t.Fatalf("commit theme phase: %v", err)
	}
	assertThemeLink(t, current, "./tokyo-night")
}

func TestThemePhase_MissingBundleRequiresReplacement(t *testing.T) {
	t.Parallel()

	current := newThemeFixture(t, "retired-theme")
	if _, err := prepareThemePhase(current, []string{"tokyo-night"}, ""); err == nil {
		t.Fatal("expected unavailable selection to fail")
	}
	assertThemeLink(t, current, "retired-theme")
}

func TestThemePhase_ReplacementRewritesAndRollsBackRelativeLink(t *testing.T) {
	t.Parallel()

	current := newThemeFixture(t, "retired-theme")
	phase, err := prepareThemePhase(current, []string{"tokyo-night"}, "tokyo-night")
	if err != nil {
		t.Fatalf("prepare theme replacement: %v", err)
	}
	if err := phase.Commit(); err != nil {
		t.Fatalf("commit theme replacement: %v", err)
	}
	assertThemeLink(t, current, "tokyo-night")

	if err := phase.Rollback(); err != nil {
		t.Fatalf("rollback theme replacement: %v", err)
	}
	assertThemeLink(t, current, "retired-theme")
}

func TestThemePhase_RejectsEscapingLinkWithoutMutation(t *testing.T) {
	t.Parallel()

	current := newThemeFixture(t, "../outside")
	if _, err := prepareThemePhase(current, []string{"tokyo-night"}, ""); err == nil {
		t.Fatal("expected escaping selection to fail")
	}
	assertThemeLink(t, current, "../outside")
}

func TestThemePhase_ExplicitReplacementRepairsAndRestoresInvalidFileSelection(t *testing.T) {
	t.Parallel()

	themesRoot := filepath.Join(t.TempDir(), ".local", "share", "moonarch", "themes")
	if err := os.MkdirAll(themesRoot, 0o755); err != nil {
		t.Fatalf("create themes root: %v", err)
	}
	current := filepath.Join(themesRoot, "current")
	if err := os.WriteFile(current, []byte("invalid user state"), 0o600); err != nil {
		t.Fatalf("write invalid current selection: %v", err)
	}

	phase, err := prepareThemePhase(current, []string{"tokyo-night"}, "tokyo-night")
	if err != nil {
		t.Fatalf("prepare explicit replacement: %v", err)
	}
	if err := phase.Commit(); err != nil {
		t.Fatalf("commit explicit replacement: %v", err)
	}
	assertThemeLink(t, current, "tokyo-night")
	if err := phase.Rollback(); err != nil {
		t.Fatalf("rollback explicit replacement: %v", err)
	}
	content, err := os.ReadFile(current)
	if err != nil {
		t.Fatalf("read restored invalid selection: %v", err)
	}
	if string(content) != "invalid user state" {
		t.Fatalf("restored invalid selection = %q", content)
	}
	info, err := os.Stat(current)
	if err != nil {
		t.Fatalf("stat restored invalid selection: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("restored invalid selection mode = %#o, want 0600", info.Mode().Perm())
	}
}

func newThemeFixture(t *testing.T, target string) string {
	t.Helper()
	themesRoot := filepath.Join(t.TempDir(), ".local", "share", "moonarch", "themes")
	if err := os.MkdirAll(themesRoot, 0o755); err != nil {
		t.Fatalf("create themes root: %v", err)
	}
	current := filepath.Join(themesRoot, "current")
	if err := os.Symlink(target, current); err != nil {
		t.Fatalf("create current theme link: %v", err)
	}
	return current
}

func assertThemeLink(t *testing.T, current, want string) {
	t.Helper()
	got, err := os.Readlink(current)
	if err != nil {
		t.Fatalf("read current theme link: %v", err)
	}
	if got != want {
		t.Fatalf("current theme link = %q, want %q", got, want)
	}
}
