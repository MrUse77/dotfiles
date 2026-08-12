package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type themePhase struct {
	currentPath     string
	originalLink    string
	originalFile    []byte
	originalMode    os.FileMode
	originalExists  bool
	originalWasFile bool
	replacement     string
	rewrite         bool
	committed       bool
}

func prepareThemePhase(currentPath string, desiredBundles []string, replacement string) (*themePhase, error) {
	desired := make(map[string]struct{}, len(desiredBundles))
	for _, bundle := range desiredBundles {
		if !validThemeBundle(bundle) {
			return nil, fmt.Errorf("desired theme bundle %q is invalid", bundle)
		}
		desired[bundle] = struct{}{}
	}

	current, err := readThemeLink(currentPath)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		if replacement == "" {
			return nil, fmt.Errorf("read current theme selection: %w", err)
		}
		info, statErr := os.Lstat(currentPath)
		if statErr != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("read current theme selection: %w", err)
		}
		content, readErr := os.ReadFile(currentPath)
		if readErr != nil {
			return nil, fmt.Errorf("read invalid current theme selection: %w", readErr)
		}
		exists = true
		current = ""
		phase := &themePhase{
			currentPath:     currentPath,
			originalFile:    content,
			originalMode:    info.Mode().Perm(),
			originalExists:  true,
			originalWasFile: true,
		}
		if !validThemeBundle(replacement) {
			return nil, fmt.Errorf("replacement theme %q is invalid", replacement)
		}
		if _, ok := desired[replacement]; !ok {
			return nil, fmt.Errorf("replacement theme %q is unavailable in the desired release", replacement)
		}
		phase.replacement = replacement
		phase.rewrite = true
		return phase, nil
	}
	phase := &themePhase{
		currentPath:    currentPath,
		originalLink:   current,
		originalExists: exists,
	}

	if replacement != "" {
		if !validThemeBundle(replacement) {
			return nil, fmt.Errorf("replacement theme %q is invalid", replacement)
		}
		if _, ok := desired[replacement]; !ok {
			return nil, fmt.Errorf("replacement theme %q is unavailable in the desired release", replacement)
		}
		phase.replacement = replacement
		phase.rewrite = !exists || current != replacement
		return phase, nil
	}

	if !exists {
		return nil, errors.New("current theme selection is missing; pass --theme-replace with a desired bundle")
	}
	bundle, ok := selectedThemeBundle(current)
	if !ok {
		return nil, fmt.Errorf("current theme selection %q is unsafe; pass --theme-replace with a desired bundle", current)
	}
	if _, ok := desired[bundle]; !ok {
		return nil, fmt.Errorf("current theme %q is unavailable in the desired release; pass --theme-replace with a desired bundle", bundle)
	}
	return phase, nil
}

func (p *themePhase) Commit() error {
	if p == nil || !p.rewrite || p.committed {
		return nil
	}
	if err := rewriteThemeLink(p.currentPath, p.replacement); err != nil {
		return err
	}
	p.committed = true
	return nil
}

func (p *themePhase) Rollback() error {
	if p == nil || !p.committed {
		return nil
	}
	if p.originalExists {
		if p.originalWasFile {
			if err := rewriteThemeFile(p.currentPath, p.originalFile, p.originalMode); err != nil {
				return err
			}
		} else {
			if err := rewriteThemeLink(p.currentPath, p.originalLink); err != nil {
				return err
			}
		}
	} else if err := os.Remove(p.currentPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove replacement theme link: %w", err)
	}
	p.committed = false
	return syncDirectory(filepath.Dir(p.currentPath))
}

func selectedThemeBundle(link string) (string, bool) {
	if link == "" || filepath.IsAbs(link) {
		return "", false
	}
	clean := filepath.Clean(link)
	if !validThemeBundle(clean) {
		return "", false
	}
	return clean, true
}

func validThemeBundle(bundle string) bool {
	return bundle != "" && bundle != "." && bundle != ".." && bundle != "current" &&
		!filepath.IsAbs(bundle) && filepath.Clean(bundle) == bundle && filepath.Base(bundle) == bundle
}

func readThemeLink(path string) (string, error) {
	for size := 256; size <= 64*1024; size *= 2 {
		buffer := make([]byte, size)
		n, err := unix.Readlink(path, buffer)
		if err != nil {
			return "", err
		}
		if n < len(buffer) {
			return string(buffer[:n]), nil
		}
	}
	return "", fmt.Errorf("theme link target exceeds 64 KiB")
}

func rewriteThemeLink(currentPath, target string) error {
	parent := filepath.Dir(currentPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create themes directory: %w", err)
	}
	temp, err := os.CreateTemp(parent, ".current-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary theme link name: %w", err)
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temporary theme file: %w", err)
	}
	if err := os.Remove(tempPath); err != nil {
		return fmt.Errorf("prepare temporary theme link: %w", err)
	}
	defer os.Remove(tempPath)
	if err := os.Symlink(target, tempPath); err != nil {
		return fmt.Errorf("create temporary theme link: %w", err)
	}
	if err := os.Rename(tempPath, currentPath); err != nil {
		return fmt.Errorf("commit theme link: %w", err)
	}
	return syncDirectory(parent)
}

func rewriteThemeFile(currentPath string, content []byte, mode os.FileMode) error {
	parent := filepath.Dir(currentPath)
	temp, err := os.CreateTemp(parent, ".current-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary theme file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary theme file: %w", err)
	}
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return fmt.Errorf("chmod temporary theme file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary theme file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary theme file: %w", err)
	}
	if err := os.Rename(tempPath, currentPath); err != nil {
		return fmt.Errorf("restore theme file: %w", err)
	}
	return syncDirectory(parent)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open theme directory for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync theme directory: %w", err)
	}
	return nil
}
