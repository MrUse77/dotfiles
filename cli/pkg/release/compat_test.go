package release

import (
	"errors"
	"testing"
)

func TestCheckCompatibility_PassesForSupportedSchemaAndRange(t *testing.T) {
	m := Manifest{SchemaVersion: "1", CLICompatRange: ">= v0.3.0", Catalog: []CatalogEntry{}}
	if err := CheckCompatibility(m, "v0.3.0"); err != nil {
		t.Fatalf("CheckCompatibility() error = %v, want nil", err)
	}
}

func TestCheckCompatibility_PassesAtRangeBoundary(t *testing.T) {
	m := Manifest{SchemaVersion: "1", CLICompatRange: ">= v1.0.0", Catalog: []CatalogEntry{}}
	if err := CheckCompatibility(m, "v1.0.0"); err != nil {
		t.Fatalf("CheckCompatibility() at boundary error = %v, want nil", err)
	}
}

func TestCheckCompatibility_PassesWithoutDeclaredRange(t *testing.T) {
	m := Manifest{SchemaVersion: "1", Catalog: []CatalogEntry{}}
	if err := CheckCompatibility(m, "v0.3.0"); err != nil {
		t.Fatalf("CheckCompatibility() with empty range error = %v, want nil", err)
	}
}

func TestCheckCompatibility_RejectsUnsupportedSchema(t *testing.T) {
	for _, schema := range []string{"2", "banana", ""} {
		m := Manifest{SchemaVersion: schema, Catalog: []CatalogEntry{}}
		err := CheckCompatibility(m, "v0.3.0")
		var ce *CompatibilityError
		if !errors.As(err, &ce) {
			t.Fatalf("schema %q: error type = %T, want *CompatibilityError", schema, err)
		}
	}
}

func TestCheckCompatibility_RejectsUnsatisfiedRange(t *testing.T) {
	m := Manifest{SchemaVersion: "1", CLICompatRange: ">= v1.0.0", Catalog: []CatalogEntry{}}
	err := CheckCompatibility(m, "v0.3.0")
	var ce *CompatibilityError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T, want *CompatibilityError", err)
	}
}

func TestCheckCompatibility_RejectsInvalidRange(t *testing.T) {
	m := Manifest{SchemaVersion: "1", CLICompatRange: ">= banana!", Catalog: []CatalogEntry{}}
	err := CheckCompatibility(m, "v0.3.0")
	var ce *CompatibilityError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T, want *CompatibilityError", err)
	}
}

func TestCheckCompatibility_RejectsInvalidCliVersion(t *testing.T) {
	m := Manifest{SchemaVersion: "1", CLICompatRange: ">= v0.3.0", Catalog: []CatalogEntry{}}
	err := CheckCompatibility(m, "not-a-version")
	var ce *CompatibilityError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T, want *CompatibilityError", err)
	}
}

func TestCheckCompatibility_DoesNotMutateManifest(t *testing.T) {
	m := Manifest{
		SchemaVersion:  "1",
		CLICompatRange: ">= v0.3.0",
		Binaries:       []string{"home/.local/bin/moonarch"},
		Catalog: []CatalogEntry{
			{Path: "home/.zshrc", Digest: "d", Mode: 0o644, Kind: "file"},
		},
	}
	before := m
	if err := CheckCompatibility(m, "v0.3.0"); err != nil {
		t.Fatalf("CheckCompatibility() error = %v", err)
	}
	if m.SchemaVersion != before.SchemaVersion || m.CLICompatRange != before.CLICompatRange {
		t.Fatalf("CheckCompatibility mutated manifest")
	}
	if len(m.Catalog) != len(before.Catalog) || m.Catalog[0] != before.Catalog[0] {
		t.Fatalf("CheckCompatibility mutated catalog")
	}
	if len(m.Binaries) != len(before.Binaries) || m.Binaries[0] != before.Binaries[0] {
		t.Fatalf("CheckCompatibility mutated binaries")
	}
}
