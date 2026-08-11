package release

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// CatalogEntry describes one managed object inside a config release artifact.
// Kind distinguishes the filesystem object type so admission can reject
// unsupported kinds; Executable is the classification gate used by the
// fail-closed extractor.
type CatalogEntry struct {
	Path       string `json:"path"`
	Digest     string `json:"digest"`
	Mode       int64  `json:"mode"`
	Kind       string `json:"kind"`
	Executable bool   `json:"executable"`
}

// DependencyDecl declares one external dependency that must be present for the
// artifact to apply. Checks are read-only: nothing is installed, upgraded,
// removed, or rolled back.
type DependencyDecl struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint,omitempty"`
}

// Manifest is the schema-versioned description of a config release artifact.
// It carries the CLI compatibility range, the declared executable binaries,
// the declared external dependencies, and the complete managed-target catalog
// with per-entry digests.
type Manifest struct {
	SchemaVersion   string           `json:"schema_version"`
	CLICompatRange  string           `json:"cli_compat_range"`
	Binaries        []string         `json:"binaries"`
	DependencyDecls []DependencyDecl `json:"dependency_decls,omitempty"`
	Catalog         []CatalogEntry   `json:"catalog"`
}

// ParseManifest decodes and validates a manifest document. It rejects empty or
// malformed JSON and documents that are missing required fields. An explicit
// empty catalog ([]) is valid; a missing catalog field is not.
func ParseManifest(data []byte) (Manifest, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Manifest{}, fmt.Errorf("manifest is empty")
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if m.SchemaVersion == "" {
		return Manifest{}, fmt.Errorf("manifest missing required field schema_version")
	}
	if m.Catalog == nil {
		return Manifest{}, fmt.Errorf("manifest missing required field catalog")
	}
	for i, entry := range m.Catalog {
		if entry.Path == "" {
			return Manifest{}, fmt.Errorf("catalog entry %d missing required field path", i)
		}
		if entry.Digest == "" {
			return Manifest{}, fmt.Errorf("catalog entry %d (%s) missing required field digest", i, entry.Path)
		}
	}
	return m, nil
}
