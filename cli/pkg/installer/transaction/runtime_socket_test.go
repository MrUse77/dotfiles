package transaction

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

func TestTransaction_CopyTreePreservesExistingRuntimeSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix sockets are not supported on Windows")
	}

	repo, home := t.TempDir(), t.TempDir()
	source := filepath.Join(repo, "config")
	mustMkdir(t, filepath.Join(source, "herdr"))
	mustWriteFile(t, filepath.Join(source, "managed.conf"), []byte("new"))
	destination := filepath.Join(home, ".config")
	mustMkdir(t, filepath.Join(destination, "herdr"))
	listener := listenUnixSocket(t, filepath.Join(destination, "herdr", "herdr-client.sock"))

	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyTree}})
	if _, err := New(p).Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertUnixSocketLive(t, filepath.Join(destination, "herdr", "herdr-client.sock"))
	if got := readFileString(t, filepath.Join(destination, "managed.conf")); got != "new" {
		t.Errorf("managed file = %q, want new", got)
	}
	_ = listener
}

func TestTransaction_PartialRuntimeSocketReverseRetainsRecoveryStage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix sockets are not supported on Windows")
	}
	repo, home := t.TempDir(), t.TempDir()
	source, destination := repo, home
	mustWriteFile(t, filepath.Join(source, "managed.conf"), []byte("new"))
	listener := listenUnixSocket(t, filepath.Join(destination, "runtime.sock"))
	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyTree}})
	fs := &hookFS{Filesystem: OSFilesystem(), renameHook: func(old, new string) error {
		if new == destination && strings.Contains(old, ".dots-staging-") && !strings.Contains(old, ".dots-trash") {
			return os.ErrPermission
		}
		if strings.Contains(old, ".dots-staging-") && strings.HasSuffix(old, "runtime.sock") && strings.Contains(new, ".dots-trash") {
			return os.ErrPermission
		}
		return nil
	}}
	tx := New(p, WithFilesystem(fs))
	if _, err := tx.Execute(); err == nil {
		t.Fatal("expected partial runtime socket recovery failure")
	}
	entry := entryFor(t, tx.Inventory(), destination)
	if tx.Inventory().Lifecycle != InventoryRecoveryIncomplete || entry.StagePath == "" {
		t.Fatalf("lifecycle=%q stage=%q, want incomplete recovery with retained stage", tx.Inventory().Lifecycle, entry.StagePath)
	}
	info, statErr := os.Lstat(filepath.Join(entry.StagePath, "runtime.sock"))
	if statErr != nil || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("retained runtime socket = %v, want socket", statErr)
	}
	_ = listener
}

func TestTransaction_RollbackAfterLaterFailurePreservesExistingRuntimeSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix sockets are not supported on Windows")
	}

	repo, home := t.TempDir(), t.TempDir()
	sourceDir := filepath.Join(repo, "config")
	mustMkdir(t, filepath.Join(sourceDir, "herdr"))
	mustWriteFile(t, filepath.Join(sourceDir, "managed.conf"), []byte("new"))
	sourceFile := filepath.Join(repo, "later-file")
	mustWriteFile(t, sourceFile, []byte("later"))

	destinationDir := filepath.Join(home, ".config")
	mustMkdir(t, destinationDir)
	listener := listenUnixSocket(t, filepath.Join(destinationDir, "runtime.sock"))
	destinationFile := filepath.Join(home, "later-file")
	mustWriteFile(t, destinationFile, []byte("old"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: sourceDir, Destination: destinationDir, Kind: plan.CopyTree},
		{Source: sourceFile, Destination: destinationFile, Kind: plan.CopyFile},
	})
	if err := os.Remove(sourceFile); err != nil {
		t.Fatalf("remove later source: %v", err)
	}

	if _, err := New(p).Execute(); err == nil {
		t.Fatal("expected later target failure")
	}
	assertUnixSocketLive(t, filepath.Join(destinationDir, "runtime.sock"))
	if _, err := os.Stat(filepath.Join(destinationDir, "managed.conf")); !os.IsNotExist(err) {
		t.Errorf("managed file after rollback should be absent, stat error = %v", err)
	}
	_ = listener
}

func listenUnixSocket(t *testing.T, path string) net.Listener {
	t.Helper()
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen on Unix socket: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func assertUnixSocketLive(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat Unix socket: %v", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("mode = %v, want Unix socket", info.Mode())
	}
	connection, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial preserved Unix socket: %v", err)
	}
	_ = connection.Close()
}
