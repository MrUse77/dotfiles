package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyDirectoryContentsPreservesPathsWithSpacesAndQuotes(t *testing.T) {
	source := filepath.Join(t.TempDir(), `assets 'fonts;$(touch nope)`)
	destination := filepath.Join(t.TempDir(), `share fonts "icons;$(touch nope)`)
	if err := os.MkdirAll(filepath.Join(source, "nested dir"), 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "nested dir", "font 'name.ttf")
	if err := os.WriteFile(file, []byte("font data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyDirectoryContents(source, destination); err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(filepath.Join(destination, "nested dir", "font 'name.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(copied) != "font data" {
		t.Fatalf("copied content = %q, want %q", copied, "font data")
	}
	if _, err := os.Stat(filepath.Join(t.TempDir(), "nope")); !os.IsNotExist(err) {
		t.Fatal("copy unexpectedly interpreted shell metacharacters")
	}
}

func TestCopyDirectoryContentsRefusesDestinationSymlinks(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "font.ttf"), []byte("font"), 0644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	destination := filepath.Join(t.TempDir(), "destination")
	if err := os.Symlink(outside, destination); err != nil {
		t.Fatal(err)
	}
	if err := copyDirectoryContents(source, destination); err == nil {
		t.Fatal("copy followed destination symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "font.ttf")); !os.IsNotExist(err) {
		t.Fatal("copied through destination symlink")
	}
}
