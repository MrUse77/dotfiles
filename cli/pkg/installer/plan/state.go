package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// StateReader captures pre-installation state without following symlinks.
type StateReader interface {
	Read(path string) (PreState, error)
}

// DefaultStateReader reads the local filesystem using Lstat semantics.
func DefaultStateReader() StateReader {
	return &defaultStateReader{}
}

type defaultStateReader struct{}

func (r *defaultStateReader) Read(path string) (PreState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return PreState{Type: StateAbsent}, nil
		}
		return PreState{}, err
	}

	mode := info.Mode()
	if mode&os.ModeSocket != 0 || mode&os.ModeDevice != 0 || mode&os.ModeNamedPipe != 0 || mode&os.ModeIrregular != 0 {
		return PreState{}, fmt.Errorf("unsupported special file at %q: %v", path, mode)
	}

	switch {
	case mode.IsRegular():
		digest, err := fileDigest(path)
		if err != nil {
			return PreState{}, fmt.Errorf("digest file %q: %w", path, err)
		}
		return PreState{Type: StateFile, Mode: supportedMode(mode), Digest: digest}, nil

	case mode.IsDir():
		digest, err := directoryDigest(path)
		if err != nil {
			return PreState{}, fmt.Errorf("digest directory %q: %w", path, err)
		}
		return PreState{Type: StateDirectory, Mode: supportedMode(mode), Digest: digest}, nil

	case mode&os.ModeSymlink != 0:
		link, err := os.Readlink(path)
		if err != nil {
			return PreState{}, fmt.Errorf("readlink %q: %w", path, err)
		}
		digest := linkDigest(link)
		return PreState{Type: StateSymlink, Mode: supportedMode(mode), LinkValue: link, Digest: digest}, nil

	default:
		return PreState{}, fmt.Errorf("unsupported file type at %q: %v", path, mode)
	}
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func linkDigest(link string) string {
	sum := sha256.Sum256([]byte(link))
	return hex.EncodeToString(sum[:])
}

type dirEntry struct {
	RelativePath string `json:"relative_path"`
	Type         string `json:"type"`
	Mode         uint32 `json:"mode"`
	LinkValue    string `json:"link_value"`
	Digest       string `json:"digest"`
}

func directoryDigest(path string) (string, error) {
	entries, err := collectDirectoryEntries(path)
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func collectDirectoryEntries(root string) ([]dirEntry, error) {
	var entries []dirEntry
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()

		entry := dirEntry{
			RelativePath: rel,
			Type:         entryType(mode),
			Mode:         uint32(mode.Perm()),
		}

		if entry.Type == "unsupported" {
			return fmt.Errorf("unsupported special file at %q: %v", path, mode)
		}

		switch entry.Type {
		case "file":
			digest, err := fileDigest(path)
			if err != nil {
				return err
			}
			entry.Digest = digest
		case "symlink":
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry.LinkValue = link
			entry.Digest = linkDigest(link)
		}

		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].RelativePath < entries[j].RelativePath
	})
	return entries, nil
}

// supportedMode returns every chmod-persistable mode bit captured by the installer.
func supportedMode(mode os.FileMode) os.FileMode {
	return mode & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
}

func entryType(mode os.FileMode) string {
	switch {
	case mode.IsRegular():
		return "file"
	case mode.IsDir():
		return "directory"
	case mode&os.ModeSymlink != 0:
		return "symlink"
	default:
		return "unsupported"
	}
}
