package release

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// fakeFileOps records calls and injects failures for deterministic tests.
type fakeFileOps struct {
	mkdirAllErr error
	createTemp  func(dir, pattern string) (*os.File, error)
	openErr     error
	chmodErr    error
	renameErr   error
	removeErr   error

	calls []string
	temp  string
}

func (f *fakeFileOps) MkdirAll(path string, perm os.FileMode) error {
	f.calls = append(f.calls, fmt.Sprintf("MkdirAll(%s,%#o)", path, perm))
	if f.mkdirAllErr != nil {
		return f.mkdirAllErr
	}
	return os.MkdirAll(path, perm)
}

func (f *fakeFileOps) CreateTemp(dir, pattern string) (*os.File, error) {
	f.calls = append(f.calls, fmt.Sprintf("CreateTemp(%s,%s)", dir, pattern))
	if f.createTemp != nil {
		return f.createTemp(dir, pattern)
	}
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	f.temp = file.Name()
	return file, nil
}

func (f *fakeFileOps) Open(name string) (*os.File, error) {
	f.calls = append(f.calls, fmt.Sprintf("Open(%s)", name))
	if f.openErr != nil {
		return nil, f.openErr
	}
	return os.Open(name)
}

func (f *fakeFileOps) Chmod(name string, mode os.FileMode) error {
	f.calls = append(f.calls, fmt.Sprintf("Chmod(%s,%#o)", name, mode))
	if f.chmodErr != nil {
		return f.chmodErr
	}
	return os.Chmod(name, mode)
}

func (f *fakeFileOps) Rename(oldpath, newpath string) error {
	f.calls = append(f.calls, fmt.Sprintf("Rename(%s,%s)", oldpath, newpath))
	if f.renameErr != nil {
		return f.renameErr
	}
	return os.Rename(oldpath, newpath)
}

func (f *fakeFileOps) Remove(name string) error {
	f.calls = append(f.calls, fmt.Sprintf("Remove(%s)", name))
	if f.removeErr != nil {
		return f.removeErr
	}
	return os.Remove(name)
}

// observingVerifier records whether it was called and can fail.
type observingVerifier struct {
	called bool
	err    error
}

func (v *observingVerifier) Verify(_ string, _ io.Reader, _ io.Reader) error {
	v.called = true
	return v.err
}

func TestAtomicReplacer_Success(t *testing.T) {
	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "moonarch-cli")

	files := &fakeFileOps{}
	verifier := &observingVerifier{}
	replacer := NewAtomicReplacer(files, verifier)

	binary := bytes.NewReader([]byte("new binary"))
	checksum := bytes.NewReader([]byte("sha256sums"))

	err := replacer.Replace(context.Background(), targetPath, "moonarch-cli-linux-amd64", binary, checksum)
	if err != nil {
		t.Fatalf("Replace error = %v", err)
	}
	if !verifier.called {
		t.Fatalf("verifier was not called")
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("Stat target error = %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("target mode = %#o, want executable", info.Mode().Perm())
	}
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile target error = %v", err)
	}
	if string(data) != "new binary" {
		t.Fatalf("target contents = %q", string(data))
	}
}

func TestAtomicReplacer_ChecksumBeforeChmod(t *testing.T) {
	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "moonarch-cli")

	files := &fakeFileOps{}
	verifier := &observingVerifier{}
	replacer := NewAtomicReplacer(files, verifier)

	_ = replacer.Replace(context.Background(), targetPath, "moonarch-cli-linux-amd64", bytes.NewReader([]byte("x")), bytes.NewReader([]byte("y")))

	var verifyIndex, chmodIndex int
	for i, call := range files.calls {
		if call == fmt.Sprintf("Open(%s)", files.temp) {
			verifyIndex = i
		}
		if len(call) > 6 && call[:6] == "Chmod(" {
			chmodIndex = i
		}
	}
	if verifyIndex == 0 {
		t.Fatalf("verify Open not recorded")
	}
	if chmodIndex == 0 {
		t.Fatalf("Chmod not recorded")
	}
	if verifyIndex >= chmodIndex {
		t.Fatalf("verify (%d) did not run before chmod (%d)", verifyIndex, chmodIndex)
	}
}

func TestAtomicReplacer_ChecksumMismatchRemovesTemp(t *testing.T) {
	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "moonarch-cli")
	if err := os.WriteFile(targetPath, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	files := &fakeFileOps{}
	verifier := &observingVerifier{err: &ChecksumMismatchError{AssetName: "a"}}
	replacer := NewAtomicReplacer(files, verifier)

	err := replacer.Replace(context.Background(), targetPath, "moonarch-cli-linux-amd64", bytes.NewReader([]byte("new")), bytes.NewReader([]byte("list")))
	if err == nil {
		t.Fatalf("expected error")
	}

	old, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if string(old) != "old binary" {
		t.Fatalf("old binary was modified")
	}
	if files.temp != "" {
		if _, statErr := os.Stat(files.temp); !os.IsNotExist(statErr) {
			t.Fatalf("temp file %s still exists", files.temp)
		}
	}
}

func TestAtomicReplacer_RenameFailureRemovesTempAndPreservesOld(t *testing.T) {
	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "moonarch-cli")
	if err := os.WriteFile(targetPath, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	files := &fakeFileOps{renameErr: errors.New("rename failed")}
	verifier := &observingVerifier{}
	replacer := NewAtomicReplacer(files, verifier)

	err := replacer.Replace(context.Background(), targetPath, "moonarch-cli-linux-amd64", bytes.NewReader([]byte("new")), bytes.NewReader([]byte("list")))
	if err == nil {
		t.Fatalf("expected error")
	}

	old, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if string(old) != "old binary" {
		t.Fatalf("old binary was modified")
	}
	if files.temp != "" {
		if _, statErr := os.Stat(files.temp); !os.IsNotExist(statErr) {
			t.Fatalf("temp file %s still exists", files.temp)
		}
	}
}

func TestAtomicReplacer_ChmodFailureRemovesTemp(t *testing.T) {
	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "moonarch-cli")

	files := &fakeFileOps{chmodErr: errors.New("chmod failed")}
	verifier := &observingVerifier{}
	replacer := NewAtomicReplacer(files, verifier)

	err := replacer.Replace(context.Background(), targetPath, "moonarch-cli-linux-amd64", bytes.NewReader([]byte("new")), bytes.NewReader([]byte("list")))
	if err == nil {
		t.Fatalf("expected error")
	}
	if files.temp != "" {
		if _, statErr := os.Stat(files.temp); !os.IsNotExist(statErr) {
			t.Fatalf("temp file %s still exists", files.temp)
		}
	}
	if _, statErr := os.Stat(targetPath); !os.IsNotExist(statErr) {
		t.Fatalf("target should not exist")
	}
}

func TestAtomicReplacer_CreatesTargetDirectory(t *testing.T) {
	base := t.TempDir()
	targetDir := filepath.Join(base, "nested", "bin")
	targetPath := filepath.Join(targetDir, "moonarch-cli")

	files := &fakeFileOps{}
	verifier := &observingVerifier{}
	replacer := NewAtomicReplacer(files, verifier)

	err := replacer.Replace(context.Background(), targetPath, "moonarch-cli-linux-amd64", bytes.NewReader([]byte("new")), bytes.NewReader([]byte("list")))
	if err != nil {
		t.Fatalf("Replace error = %v", err)
	}
	if _, statErr := os.Stat(targetDir); os.IsNotExist(statErr) {
		t.Fatalf("target directory was not created")
	}
}

func TestAtomicReplacer_FailingReader(t *testing.T) {
	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "moonarch-cli")

	files := &fakeFileOps{}
	verifier := &observingVerifier{}
	replacer := NewAtomicReplacer(files, verifier)

	err := replacer.Replace(context.Background(), targetPath, "moonarch-cli-linux-amd64", &errorReader{err: errors.New("read failed")}, bytes.NewReader([]byte("list")))
	if err == nil {
		t.Fatalf("expected error")
	}
	if files.temp != "" {
		if _, statErr := os.Stat(files.temp); !os.IsNotExist(statErr) {
			t.Fatalf("temp file %s still exists", files.temp)
		}
	}
}

func TestAtomicReplacer_MkdirAllFailure(t *testing.T) {
	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "moonarch-cli")

	files := &fakeFileOps{mkdirAllErr: errors.New("mkdir failed")}
	verifier := &observingVerifier{}
	replacer := NewAtomicReplacer(files, verifier)

	err := replacer.Replace(context.Background(), targetPath, "moonarch-cli-linux-amd64", bytes.NewReader([]byte("new")), bytes.NewReader([]byte("list")))
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestNewAtomicReplacer(t *testing.T) {
	replacer := NewAtomicReplacer(nil, nil)
	if replacer == nil {
		t.Fatalf("NewAtomicReplacer returned nil")
	}
}
