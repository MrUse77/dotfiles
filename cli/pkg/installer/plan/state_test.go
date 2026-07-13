package plan

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestStateReader_Absent(t *testing.T) {
	reader := DefaultStateReader()
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	state, err := reader.Read(missing)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if state.Type != StateAbsent {
		t.Errorf("Type = %q, want %q", state.Type, StateAbsent)
	}
	if state.Digest != "" {
		t.Errorf("Digest for absent target should be empty, got %q", state.Digest)
	}
}

func TestStateReader_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "regular")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	reader := DefaultStateReader()
	state, err := reader.Read(path)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if state.Type != StateFile {
		t.Errorf("Type = %q, want %q", state.Type, StateFile)
	}
	if state.Mode != 0644 {
		t.Errorf("Mode = %o, want 0644", state.Mode)
	}
	if state.Digest == "" {
		t.Error("Digest is empty")
	}
}

func TestStateReader_DirectoryDigestDeterministic(t *testing.T) {
	reader := DefaultStateReader()

	dir1 := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir1, "a"))
	mustWriteFile(t, filepath.Join(dir1, "a", "f1"), []byte("1"))
	mustMkdirAll(t, filepath.Join(dir1, "b"))
	mustWriteFile(t, filepath.Join(dir1, "b", "f2"), []byte("2"))

	dir2 := t.TempDir()
	// Create the same tree in a different order.
	mustMkdirAll(t, filepath.Join(dir2, "b"))
	mustWriteFile(t, filepath.Join(dir2, "b", "f2"), []byte("2"))
	mustMkdirAll(t, filepath.Join(dir2, "a"))
	mustWriteFile(t, filepath.Join(dir2, "a", "f1"), []byte("1"))

	state1, err := reader.Read(dir1)
	if err != nil {
		t.Fatalf("Read(dir1) error = %v", err)
	}
	state2, err := reader.Read(dir2)
	if err != nil {
		t.Fatalf("Read(dir2) error = %v", err)
	}

	if state1.Digest != state2.Digest {
		t.Errorf("directory digest depends on traversal order: %q vs %q", state1.Digest, state2.Digest)
	}
}

func TestStateReader_SymlinkUsesLstat(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	mustWriteFile(t, target, []byte("target content"))

	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	reader := DefaultStateReader()
	state, err := reader.Read(link)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if state.Type != StateSymlink {
		t.Errorf("Type = %q, want %q", state.Type, Symlink)
	}
	if state.LinkValue != target {
		t.Errorf("LinkValue = %q, want %q", state.LinkValue, target)
	}
	// Digest should represent the link value, not the target content.
	if state.Digest == "" {
		t.Error("Digest for symlink is empty")
	}
}

func TestStateReader_UnsupportedSpecialFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("named pipes not supported on Windows")
	}

	dir := t.TempDir()
	pipe := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(pipe, 0644); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	_, err := DefaultStateReader().Read(pipe)
	if err == nil {
		t.Fatal("expected error for unsupported special file")
	}
}

func TestStateReader_UnreadableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	mustWriteFile(t, path, []byte("x"))
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(path, 0644)

	_, err := DefaultStateReader().Read(path)
	if err == nil {
		t.Fatal("expected error reading unreadable file")
	}
}

func TestStateReader_MissingSource(t *testing.T) {
	_, err := DefaultStateReader().Read(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("missing path should be reported as absent, not error: %v", err)
	}
}

func TestStateReader_DirectoryWithNestedUnsupportedSpecialFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("named pipes not supported on Windows")
	}

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "regular"), []byte("ok"))
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0644); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	if _, err := DefaultStateReader().Read(dir); err == nil {
		t.Fatal("expected error for nested unsupported special file")
	}
}
