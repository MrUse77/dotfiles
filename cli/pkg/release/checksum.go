package release

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ChecksumVerifier verifies a binary against a checksum list.
type ChecksumVerifier interface {
	Verify(assetName string, binary io.Reader, checksumList io.Reader) error
}

// SHA256Verifier parses GNU sha256sum-format lists and compares SHA-256 digests.
type SHA256Verifier struct{}

var checksumLineRe = regexp.MustCompile(`^([0-9a-f]{64})  (.+)$`)

// Verify computes the SHA-256 of binary and compares it against the single entry
// for assetName in checksumList. The checksum list must be in GNU sha256sum
// format: 64 lowercase hex digits, two ASCII spaces, and a non-empty filename.
func (SHA256Verifier) Verify(assetName string, binary io.Reader, checksumList io.Reader) error {
	entries, err := parseChecksumList(checksumList)
	if err != nil {
		return err
	}
	expected, ok := entries[assetName]
	if !ok {
		return &ChecksumEntryMissingError{AssetName: assetName}
	}

	h := sha256.New()
	if _, err := io.Copy(h, binary); err != nil {
		return &ChecksumMismatchError{AssetName: assetName, Expected: expected, Computed: fmt.Sprintf("read error: %v", err)}
	}
	computed := hex.EncodeToString(h.Sum(nil))
	if computed != expected {
		return &ChecksumMismatchError{AssetName: assetName, Expected: expected, Computed: computed}
	}
	return nil
}

func parseChecksumList(r io.Reader) (map[string]string, error) {
	entries := make(map[string]string)
	scanner := bufio.NewScanner(r)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		matches := checksumLineRe.FindStringSubmatch(line)
		if matches == nil {
			return nil, &ChecksumFormatError{Line: line}
		}
		name := matches[2]
		if _, exists := entries[name]; exists {
			return nil, &ChecksumEntryAmbiguousError{AssetName: name}
		}
		entries[name] = strings.ToLower(matches[1])
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksum list: %w", err)
	}
	return entries, nil
}
