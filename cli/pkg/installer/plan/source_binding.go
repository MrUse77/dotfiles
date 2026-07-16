package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"syscall"

	"golang.org/x/sys/unix"
)

// FileIdentity identifies a filesystem object observed during planning.
type FileIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

// SourceBinding records the exact source object reviewed during planning.
type SourceBinding struct {
	Kind         string              `json:"kind"`
	Identity     FileIdentity        `json:"identity"`
	PathIdentity FileIdentity        `json:"path_identity,omitempty"`
	Digest       string              `json:"digest"`
	LinkValue    string              `json:"link_value,omitempty"`
	LinkDigest   string              `json:"link_digest,omitempty"`
	Mode         os.FileMode         `json:"mode"`
	TreeManifest []TreeManifestEntry `json:"tree_manifest,omitempty"`
}

// TreeManifestEntry records an observed directory entry. It is intentionally
// additive so directory-binding details can evolve without weakening files.
type TreeManifestEntry struct {
	RelativePath string       `json:"relative_path"`
	Kind         string       `json:"kind"`
	Mode         os.FileMode  `json:"mode"`
	LinkValue    string       `json:"link_value,omitempty"`
	Digest       string       `json:"digest,omitempty"`
	Identity     FileIdentity `json:"identity"`
}

const sourceBindingCaptureAttempts = 3

var errSourceBindingChanged = errors.New("source changed during binding capture")

// SourceBindingDriftError reports a source that could not be captured coherently.
type SourceBindingDriftError struct {
	Source   string
	Attempts int
}

func (e *SourceBindingDriftError) Error() string {
	return fmt.Sprintf("source %q changed during binding capture after %d attempts", e.Source, e.Attempts)
}

// sourceBindingCaptureHook is a deterministic test seam invoked after the
// declared source identity and link value have been captured.
var sourceBindingCaptureHook func()

// buildSourceBinding inspects source using descriptor-bound operations where
// required by the threat model and returns the resolved path plus a binding.
// Symlink sources for the Symlink mutation kind are bound to their link value;
// file and directory sources are bound to opened object identity and content.
func buildSourceBinding(source string, kind MutationKind) (string, SourceBinding, error) {
	for attempt := 1; attempt <= sourceBindingCaptureAttempts; attempt++ {
		resolved, binding, err := buildSourceBindingAttempt(source, kind)
		if !errors.Is(err, errSourceBindingChanged) {
			return resolved, binding, err
		}
	}
	return "", SourceBinding{}, &SourceBindingDriftError{Source: source, Attempts: sourceBindingCaptureAttempts}
}

func buildSourceBindingAttempt(source string, kind MutationKind) (string, SourceBinding, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return "", SourceBinding{}, fmt.Errorf("source %q unreadable: %w", source, err)
	}
	pathIdentity := fileIdentity(info)
	declaredLink := ""
	declaredLinkDigest := ""
	declaredSymlink := info.Mode()&os.ModeSymlink != 0
	if declaredSymlink {
		declaredLink, err = os.Readlink(source)
		if err != nil {
			return "", SourceBinding{}, err
		}
		declaredLinkDigest = linkDigest(declaredLink)
	}
	if sourceBindingCaptureHook != nil {
		sourceBindingCaptureHook()
	}
	if declaredSymlink {
		if kind == Symlink {
			if err := verifyDeclaredSource(source, pathIdentity, declaredLink); err != nil {
				return "", SourceBinding{}, err
			}
			return source, SourceBinding{
				Kind:         "symlink",
				Identity:     pathIdentity,
				PathIdentity: pathIdentity,
				Digest:       declaredLinkDigest,
				LinkValue:    declaredLink,
				LinkDigest:   declaredLinkDigest,
			}, nil
		}
	}

	resolved, info, err := resolveSource(source)
	if err != nil {
		return "", SourceBinding{}, err
	}

	if info.IsDir() {
		fd, err := unix.Open(resolved, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
		if err != nil {
			return "", SourceBinding{}, fmt.Errorf("open source directory %q: %w", resolved, err)
		}
		dir := os.NewFile(uintptr(fd), resolved)
		defer dir.Close()

		opened, err := dir.Stat()
		if err != nil {
			return "", SourceBinding{}, fmt.Errorf("stat source directory %q: %w", resolved, err)
		}
		manifest, digest, err := buildTreeManifest(dir, "")
		if err != nil {
			return "", SourceBinding{}, fmt.Errorf("manifest source directory %q: %w", resolved, err)
		}
		if err := verifyDeclaredSource(source, pathIdentity, declaredLink); err != nil {
			return "", SourceBinding{}, err
		}
		return resolved, SourceBinding{
			Kind:         "directory",
			Identity:     fileIdentity(opened),
			PathIdentity: pathIdentity,
			Digest:       digest,
			LinkValue:    declaredLink,
			LinkDigest:   declaredLinkDigest,
			Mode:         supportedMode(opened.Mode()),
			TreeManifest: manifest,
		}, nil
	}

	fd, err := unix.Open(resolved, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", SourceBinding{}, fmt.Errorf("open source %q without following links: %w", resolved, err)
	}
	file := os.NewFile(uintptr(fd), resolved)
	defer file.Close()

	opened, err := file.Stat()
	if err != nil {
		return "", SourceBinding{}, err
	}
	if !opened.Mode().IsRegular() {
		return "", SourceBinding{}, fmt.Errorf("source %q is not a regular file", resolved)
	}
	digest, err := hashFileDescriptor(file)
	if err != nil {
		return "", SourceBinding{}, err
	}
	if err := verifyDeclaredSource(source, pathIdentity, declaredLink); err != nil {
		return "", SourceBinding{}, err
	}
	return resolved, SourceBinding{
		Kind:         "file",
		Identity:     fileIdentity(opened),
		PathIdentity: pathIdentity,
		Digest:       digest,
		LinkValue:    declaredLink,
		LinkDigest:   declaredLinkDigest,
		Mode:         supportedMode(opened.Mode()),
	}, nil
}

// hashFileDescriptor returns the SHA-256 hex digest of the content read from f.
func verifyDeclaredSource(source string, identity FileIdentity, link string) error {
	info, err := os.Lstat(source)
	if err != nil || fileIdentity(info) != identity {
		return errSourceBindingChanged
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	currentLink, err := os.Readlink(source)
	if err != nil || currentLink != link {
		return errSourceBindingChanged
	}
	return nil
}

func hashFileDescriptor(f *os.File) (string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// buildTreeManifest walks dir descriptor-relative without following symlinks and
// returns a deterministic manifest plus its JSON digest.
func buildTreeManifest(dir *os.File, prefix string) ([]TreeManifestEntry, string, error) {
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, "", fmt.Errorf("read directory %q: %w", dir.Name(), err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	manifest := make([]TreeManifestEntry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}

		rel := name
		if prefix != "" {
			rel = path.Join(prefix, name)
		}

		var stat unix.Stat_t
		if err := unix.Fstatat(int(dir.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return nil, "", fmt.Errorf("stat %q: %w", rel, err)
		}
		mode := FileModeFromUnix(stat.Mode)

		entry := TreeManifestEntry{
			RelativePath: rel,
			Kind:         entryKind(mode),
			Mode:         supportedMode(mode),
			Identity:     identityFromUnixStat(&stat),
		}

		switch entry.Kind {
		case "file":
			fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if err != nil {
				return nil, "", fmt.Errorf("open %q: %w", rel, err)
			}
			f := os.NewFile(uintptr(fd), name)
			digest, err := hashFileDescriptor(f)
			_ = f.Close()
			if err != nil {
				return nil, "", fmt.Errorf("digest %q: %w", rel, err)
			}
			entry.Digest = digest

		case "directory":
			fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
			if err != nil {
				return nil, "", fmt.Errorf("open directory %q: %w", rel, err)
			}
			sub := os.NewFile(uintptr(fd), name)
			subManifest, _, err := buildTreeManifest(sub, rel)
			_ = sub.Close()
			if err != nil {
				return nil, "", err
			}
			manifest = append(manifest, subManifest...)

		case "symlink":
			buf := make([]byte, 4096)
			n, err := unix.Readlinkat(int(dir.Fd()), name, buf)
			if err != nil {
				return nil, "", fmt.Errorf("readlink %q: %w", rel, err)
			}
			link := string(buf[:n])
			entry.LinkValue = link
			entry.Digest = linkDigest(link)

		default:
			return nil, "", fmt.Errorf("unsupported entry %q: %v", rel, mode)
		}

		manifest = append(manifest, entry)
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(data)
	return manifest, hex.EncodeToString(sum[:]), nil
}

func fileIdentity(info os.FileInfo) FileIdentity {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return FileIdentity{}
	}
	return FileIdentity{Device: uint64(stat.Dev), Inode: uint64(stat.Ino)}
}

func identityFromUnixStat(stat *unix.Stat_t) FileIdentity {
	return FileIdentity{Device: uint64(stat.Dev), Inode: uint64(stat.Ino)}
}

func entryKind(mode os.FileMode) string {
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

// FileModeFromUnix converts raw Unix st_mode bits to an os.FileMode.
func FileModeFromUnix(mode uint32) os.FileMode {
	m := os.FileMode(mode & 0777)
	switch mode & unix.S_IFMT {
	case unix.S_IFDIR:
		m |= os.ModeDir
	case unix.S_IFLNK:
		m |= os.ModeSymlink
	case unix.S_IFIFO:
		m |= os.ModeNamedPipe
	case unix.S_IFSOCK:
		m |= os.ModeSocket
	case unix.S_IFCHR:
		m |= os.ModeDevice | os.ModeCharDevice
	case unix.S_IFBLK:
		m |= os.ModeDevice
	case unix.S_IFREG:
		// Regular files have no additional os.FileMode type bit.
	default:
		m |= os.ModeIrregular
	}
	if mode&unix.S_ISUID != 0 {
		m |= os.ModeSetuid
	}
	if mode&unix.S_ISGID != 0 {
		m |= os.ModeSetgid
	}
	if mode&unix.S_ISVTX != 0 {
		m |= os.ModeSticky
	}
	return m
}
