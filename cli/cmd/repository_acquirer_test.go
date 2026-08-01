package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGitRunner records the Git commands it receives and returns configured
// results for a deterministic test surface.
type fakeGitRunner struct {
	calls []gitCall
	err   error
}

type gitCall struct {
	Dir  string
	Args []string
}

func (f *fakeGitRunner) Run(ctx context.Context, dir string, stdout, stderr io.Writer, args ...string) error {
	f.calls = append(f.calls, gitCall{Dir: dir, Args: append([]string(nil), args...)})
	if f.err != nil {
		return f.err
	}
	return nil
}

func TestRepositoryRequest_FieldsAreExported(t *testing.T) {
	req := RepositoryRequest{Destination: "/dst", Ref: "main", URL: "https://example.com/repo.git"}
	if req.Destination != "/dst" || req.Ref != "main" || req.URL != "https://example.com/repo.git" {
		t.Fatalf("RepositoryRequest fields not retained: %+v", req)
	}
}

func TestRepositoryAcquisition_FieldsAreExported(t *testing.T) {
	acq := RepositoryAcquisition{Root: "/root", Destination: "/dst", Ref: "v1"}
	if acq.Root != "/root" || acq.Destination != "/dst" || acq.Ref != "v1" {
		t.Fatalf("RepositoryAcquisition fields not retained: %+v", acq)
	}
}

func TestRepositoryAcquirer_Interface(t *testing.T) {
	var _ RepositoryAcquirer = NewRepositoryAcquirer()
}

func TestPreflightRepositoryDestination_RejectsNonRepositoryDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "conflict"), 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "conflict")
	if err := os.WriteFile(filepath.Join(dest, "not-git"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := PreflightRepositoryDestination(dest)
	if got == nil || !strings.Contains(got.Error(), "ya existe pero no es un clon") {
		t.Fatalf("Preflight error = %v, want non-repository error", got)
	}
}

func TestPreflightRepositoryDestination_AcceptsMissingDestination(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "missing")
	if err := PreflightRepositoryDestination(dest); err != nil {
		t.Fatalf("Preflight error = %v, want nil", err)
	}
}

func TestPreflightRepositoryDestination_AcceptsUsableRepository(t *testing.T) {
	dest := t.TempDir()
	if err := os.Mkdir(filepath.Join(dest, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := PreflightRepositoryDestination(dest); err != nil {
		t.Fatalf("Preflight error = %v, want nil", err)
	}
}

func TestPreflightFailureOccursBeforeAnyRunnerCall(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "conflict")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &fakeGitRunner{}
	acquirer := NewRepositoryAcquirerWithRunner(runner)
	_, err := acquirer.Acquire(context.Background(), RepositoryRequest{Destination: dest}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "ya existe pero no es un clon") {
		t.Fatalf("Acquire error = %v, want preflight failure", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0", len(runner.calls))
	}
	if _, statErr := os.Stat(dest); statErr != nil {
		t.Fatalf("destination removed during preflight failure: %v", statErr)
	}
}

func TestAcquire_FrozenRefReachesGitSeam(t *testing.T) {
	dest := t.TempDir()
	dest = filepath.Join(dest, "clone")
	runner := &fakeGitRunner{}
	acquirer := NewRepositoryAcquirerWithRunner(runner)
	req := RepositoryRequest{Destination: dest, Ref: "v0.2.0", URL: "https://example.com/repo.git"}

	acq, err := acquirer.Acquire(context.Background(), req, io.Discard)
	if err != nil {
		t.Fatalf("Acquire error = %v", err)
	}
	if acq.Ref != "v0.2.0" || acq.Destination != dest || acq.Root != dest {
		t.Fatalf("unexpected acquisition: %+v", acq)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %v", runner.calls)
	}
	want := []string{"clone", "--recurse-submodules", "--branch", "v0.2.0", "https://example.com/repo.git", dest}
	if !sliceEqual(runner.calls[0].Args, want) {
		t.Fatalf("clone args = %v, want %v", runner.calls[0].Args, want)
	}
}

func TestAcquire_UsesRecursiveSubmodulesForClone(t *testing.T) {
	dest := t.TempDir()
	dest = filepath.Join(dest, "clone")
	runner := &fakeGitRunner{}
	acquirer := NewRepositoryAcquirerWithRunner(runner)

	_, err := acquirer.Acquire(context.Background(), RepositoryRequest{Destination: dest, Ref: "main", URL: "u"}, io.Discard)
	if err != nil {
		t.Fatalf("Acquire error = %v", err)
	}
	found := false
	for _, call := range runner.calls {
		if len(call.Args) > 0 && call.Args[0] == "clone" {
			for _, arg := range call.Args {
				if arg == "--recurse-submodules" {
					found = true
					break
				}
			}
		}
	}
	if !found {
		t.Fatalf("clone did not use --recurse-submodules: %v", runner.calls)
	}
}

func TestAcquire_UpdatesExistingRepository(t *testing.T) {
	dest := t.TempDir()
	if err := os.Mkdir(filepath.Join(dest, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeGitRunner{}
	acquirer := NewRepositoryAcquirerWithRunner(runner)

	_, err := acquirer.Acquire(context.Background(), RepositoryRequest{Destination: dest, Ref: "main", URL: "u"}, io.Discard)
	if err != nil {
		t.Fatalf("Acquire error = %v", err)
	}
	want := [][]string{
		{"fetch", "origin", "main"},
		{"checkout", "--force", "--detach", "FETCH_HEAD"},
		{"submodule", "update", "--init", "--recursive"},
	}
	if len(runner.calls) != len(want) {
		t.Fatalf("runner calls = %v", runner.calls)
	}
	for i, call := range runner.calls {
		if call.Dir != dest {
			t.Errorf("call[%d].Dir = %q, want %q", i, call.Dir, dest)
		}
		if !sliceEqual(call.Args, want[i]) {
			t.Errorf("call[%d].Args = %v, want %v", i, call.Args, want[i])
		}
	}
}

func TestAcquire_RetainsDestinationOnFailure(t *testing.T) {
	dest := t.TempDir()
	dest = filepath.Join(dest, "clone")
	// Simulate a partial clone that left the destination directory behind.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dest, "partial")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeGitRunner{err: errors.New("clone failed")}
	acquirer := NewRepositoryAcquirerWithRunner(runner)

	_, err := acquirer.Acquire(context.Background(), RepositoryRequest{Destination: dest, Ref: "main", URL: "u"}, io.Discard)
	if err == nil {
		t.Fatal("expected error")
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("destination was removed after failure: %v", statErr)
	}
}

func TestAcquire_NoCleanupOnFailure(t *testing.T) {
	dest := t.TempDir()
	dest = filepath.Join(dest, "clone")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dest, "partial")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeGitRunner{err: errors.New("clone failed")}
	acquirer := NewRepositoryAcquirerWithRunner(runner)

	_, err := acquirer.Acquire(context.Background(), RepositoryRequest{Destination: dest, Ref: "main", URL: "u"}, io.Discard)
	if err == nil {
		t.Fatal("expected error")
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("marker removed during failure: %v", statErr)
	}
}

func TestAcquire_OutputContainsRefNotURL(t *testing.T) {
	dest := t.TempDir()
	dest = filepath.Join(dest, "clone")
	runner := &fakeGitRunner{}
	acquirer := NewRepositoryAcquirerWithRunner(runner)
	var out bytes.Buffer

	_, err := acquirer.Acquire(context.Background(), RepositoryRequest{Destination: dest, Ref: "v1", URL: "https://secret:token@example.com/repo.git"}, &out)
	if err != nil {
		t.Fatalf("Acquire error = %v", err)
	}
	if !strings.Contains(out.String(), "v1") {
		t.Fatalf("output missing ref: %s", out.String())
	}
	if strings.Contains(out.String(), "secret") {
		t.Fatalf("output must not contain credential-bearing URL: %s", out.String())
	}
}

func TestBuildRepositoryRequest_UsesVersionTag(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "v0.3.0"

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DOTFILES_DIR", "")
	t.Setenv("DOTFILES_REPO", "")
	t.Setenv("DOTFILES_BRANCH", "")

	req, err := BuildRepositoryRequest()
	if err != nil {
		t.Fatalf("BuildRepositoryRequest error = %v", err)
	}
	if req.Ref != "v0.3.0" {
		t.Fatalf("Ref = %q, want v0.3.0", req.Ref)
	}
	if req.Destination != filepath.Join(home, ".cache", "dotfiles") {
		t.Fatalf("Destination = %q, want %q", req.Destination, filepath.Join(home, ".cache", "dotfiles"))
	}
	if req.URL != "https://github.com/MrUse77/dotfiles.git" {
		t.Fatalf("URL = %q, want default", req.URL)
	}
}

func TestBuildRepositoryRequest_DevUsesMain(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "dev"

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DOTFILES_DIR", "")
	t.Setenv("DOTFILES_BRANCH", "")

	req, err := BuildRepositoryRequest()
	if err != nil {
		t.Fatalf("BuildRepositoryRequest error = %v", err)
	}
	if req.Ref != "main" {
		t.Fatalf("Ref = %q, want main", req.Ref)
	}
}

func TestBuildRepositoryRequest_HonorsOverrides(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "v0.1.0"

	home := t.TempDir()
	dotfilesDir := filepath.Join(home, "custom")
	t.Setenv("HOME", home)
	t.Setenv("DOTFILES_DIR", dotfilesDir)
	t.Setenv("DOTFILES_REPO", "https://override.example/repo.git")
	t.Setenv("DOTFILES_BRANCH", "feature")

	req, err := BuildRepositoryRequest()
	if err != nil {
		t.Fatalf("BuildRepositoryRequest error = %v", err)
	}
	if req.Destination != dotfilesDir {
		t.Fatalf("Destination = %q, want %q", req.Destination, dotfilesDir)
	}
	if req.Ref != "feature" {
		t.Fatalf("Ref = %q, want feature", req.Ref)
	}
	if req.URL != "https://override.example/repo.git" {
		t.Fatalf("URL = %q, want override", req.URL)
	}
}

func TestEnsureRepositoryClone_WrapperStillWorks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping local Git integration test")
	}
	source := localGitRepo(t, "main")
	localGitCommit(t, source, "init")

	home := t.TempDir()
	dest := filepath.Join(home, "custom", "clone")
	t.Setenv("HOME", home)
	t.Setenv("DOTFILES_DIR", dest)
	t.Setenv("DOTFILES_REPO", source)
	t.Setenv("DOTFILES_BRANCH", "main")

	root, err := ensureRepositoryClone(io.Discard)
	if err != nil {
		t.Fatalf("ensureRepositoryClone error = %v", err)
	}
	if root != dest {
		t.Fatalf("root = %q, want %q", root, dest)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf("clone missing .git: %v", err)
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRepositoryAcquirer_LocalGitIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping local Git integration test")
	}
	source := localGitRepo(t, "main")
	localGitCommit(t, source, "init")

	home := t.TempDir()
	dest := filepath.Join(home, ".cache", "dotfiles")
	t.Setenv("HOME", home)
	t.Setenv("DOTFILES_DIR", "")
	t.Setenv("DOTFILES_REPO", source)
	t.Setenv("DOTFILES_BRANCH", "main")

	var out bytes.Buffer
	root, err := ensureRepositoryClone(&out)
	if err != nil {
		t.Fatalf("ensureRepositoryClone error = %v", err)
	}
	if root != dest {
		t.Fatalf("root = %q, want %q", root, dest)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf("clone missing .git: %v", err)
	}
}

func localGitRepo(t *testing.T, branch string) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q", "-b", branch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func localGitCommit(t *testing.T, dir string, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".zshrc"), []byte("zsh"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "."},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", msg},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}
