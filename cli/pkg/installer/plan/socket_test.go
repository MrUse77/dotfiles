package plan

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestStateReader_DirectoryWithNestedSocketIsDeterministic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix sockets are not supported on Windows")
	}

	makeTree := func(t *testing.T) string {
		t.Helper()
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "herdr"), 0o755); err != nil {
			t.Fatalf("mkdir socket directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "regular"), []byte("managed"), 0o644); err != nil {
			t.Fatalf("write regular file: %v", err)
		}
		listener, err := net.Listen("unix", filepath.Join(root, "herdr", "herdr-client.sock"))
		if err != nil {
			t.Fatalf("listen on Unix socket: %v", err)
		}
		t.Cleanup(func() { _ = listener.Close() })
		return root
	}

	reader := DefaultStateReader()
	first := makeTree(t)
	second := makeTree(t)

	firstState, err := reader.Read(first)
	if err != nil {
		t.Fatalf("Read(first) error = %v", err)
	}
	secondState, err := reader.Read(second)
	if err != nil {
		t.Fatalf("Read(second) error = %v", err)
	}
	if firstState.Digest != secondState.Digest {
		t.Fatalf("directory digest changed because of runtime socket: %q vs %q", firstState.Digest, secondState.Digest)
	}
}

func TestPlanner_RejectsSpecialFileInRepositorySourceTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIFOs are not supported on Windows")
	}

	repo, home := t.TempDir(), t.TempDir()
	source := filepath.Join(repo, "config")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(source, "pipe"), 0o644); err != nil {
		t.Fatalf("mkfifo source entry: %v", err)
	}

	_, err := New(
		WithDiscoverer(&fakeDiscoverer{targets: []Target{{Source: source, Destination: filepath.Join(home, "config"), Kind: CopyTree}}}),
		WithCatalog(&fakeCatalog{}),
	).Build(repo, home, Options{})
	if err == nil {
		t.Fatal("expected planner to reject special file in repository source tree")
	}
}
