//go:build linux

package release

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// The lock tests use real child processes: the child acquires an exclusive
// flock, signals readiness through a file, and either blocks until killed
// (contention) or exits immediately (kernel release on process exit). They are
// linux-only because unix.Flock semantics are the behavior under test.

func TestLock_Contention(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("flock tests are linux-only")
	}
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "lock")
	readyFile := filepath.Join(dir, "ready")

	cmd := lockHelperCommand(t, lockPath, "hold", readyFile)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start lock holder: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})
	waitForFile(t, readyFile, 10*time.Second)

	var l Lock
	release, err := l.Acquire(lockPath)
	if err == nil {
		release()
		t.Fatal("expected ErrLockContended while another process holds the flock")
	}
	if !errors.Is(err, ErrLockContended) {
		t.Fatalf("expected ErrLockContended, got %v", err)
	}

	// Terminate the holder; the kernel must release the flock so the parent
	// can acquire it without any userspace cleanup.
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill holder: %v", err)
	}
	// Wait returns an error for a killed process; that is the expected exit.
	_ = cmd.Wait()
	release, err = l.Acquire(lockPath)
	if err != nil {
		t.Fatalf("expected acquire to succeed after holder exit: %v", err)
	}
	release()
}

func TestLock_ReleasedOnProcessExit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("flock tests are linux-only")
	}
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "lock")
	readyFile := filepath.Join(dir, "ready")

	cmd := lockHelperCommand(t, lockPath, "exit", readyFile)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start lock holder: %v", err)
	}
	waitForFile(t, readyFile, 10*time.Second)

	// Wait for the holder process to actually exit: the flock is released by
	// the kernel during process teardown, not by any explicit close.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("holder exit: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("lock holder did not exit")
	}

	var l Lock
	release, err := l.Acquire(lockPath)
	if err != nil {
		t.Fatalf("expected flock to be free after holder exit: %v", err)
	}
	release()
}

func TestLock_AcquireUncontendedAndRelease(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("flock tests are linux-only")
	}
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "sub", "dir", "lock")

	var l Lock
	release, err := l.Acquire(lockPath)
	if err != nil {
		t.Fatalf("acquire uncontended: %v", err)
	}
	// The same process cannot re-acquire while holding it (LOCK_NB fails), and
	// after release the lock is available again.
	_, err = l.Acquire(lockPath)
	if !errors.Is(err, ErrLockContended) {
		t.Fatalf("expected self-contention while held, got %v", err)
	}
	release()
	release2, err := l.Acquire(lockPath)
	if err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
	release2()
}

// lockHelperCommand builds the child command that runs this test binary in
// helper mode: it acquires the flock at lockPath, writes readyFile, then either
// blocks ("hold") or exits ("exit").
func lockHelperCommand(t *testing.T, lockPath, mode, readyFile string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperLockChild$")
	cmd.Env = append(os.Environ(),
		"MOONARCH_LOCK_HELPER=1",
		"MOONARCH_LOCK_PATH="+lockPath,
		"MOONARCH_LOCK_MODE="+mode,
		"MOONARCH_READY_FILE="+readyFile,
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd
}

// TestHelperLockChild is the re-executed helper entry point. It does nothing
// when run directly by the suite; only the env-var switch arms the child
// behavior. Its name deliberately avoids matching "-run TestLock".
func TestHelperLockChild(t *testing.T) {
	if os.Getenv("MOONARCH_LOCK_HELPER") != "1" {
		return
	}
	path := os.Getenv("MOONARCH_LOCK_PATH")
	mode := os.Getenv("MOONARCH_LOCK_MODE")
	ready := os.Getenv("MOONARCH_READY_FILE")

	var l Lock
	_, err := l.Acquire(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "child acquire %s: %v\n", path, err)
		os.Exit(10)
	}
	if err := os.WriteFile(ready, []byte("ready"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "child ready file: %v\n", err)
		os.Exit(11)
	}
	switch mode {
	case "exit":
		// Deliberately do NOT call release(): the kernel must release the
		// flock when this process exits.
		os.Exit(0)
	default:
		select {}
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
