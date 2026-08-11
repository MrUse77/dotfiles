package release

import (
	"testing"
)

func TestParseManifest(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
		want    Manifest
	}{
		{
			name: "valid manifest with full catalog",
			data: `{
				"schema_version": "1",
				"cli_compat_range": ">= v0.3.0",
				"binaries": ["home/.local/bin/moonarch"],
				"catalog": [
					{"path": "home/.local/bin/moonarch", "digest": "abc123", "mode": 493, "kind": "file", "executable": true},
					{"path": "home/.config/hypr", "digest": "def456", "mode": 493, "kind": "dir", "executable": false}
				]
			}`,
			want: Manifest{
				SchemaVersion:  "1",
				CLICompatRange: ">= v0.3.0",
				Binaries:       []string{"home/.local/bin/moonarch"},
				Catalog: []CatalogEntry{
					{Path: "home/.local/bin/moonarch", Digest: "abc123", Mode: 493, Kind: "file", Executable: true},
					{Path: "home/.config/hypr", Digest: "def456", Mode: 493, Kind: "dir", Executable: false},
				},
			},
		},
		{
			name: "valid manifest without binaries or compat range",
			data: `{"schema_version": "1", "catalog": [{"path": "a", "digest": "d1"}]}`,
			want: Manifest{
				SchemaVersion: "1",
				Catalog:       []CatalogEntry{{Path: "a", Digest: "d1"}},
			},
		},
		{
			name: "explicit empty catalog is valid",
			data: `{"schema_version": "1", "catalog": []}`,
			want: Manifest{SchemaVersion: "1", Catalog: []CatalogEntry{}},
		},
		{name: "empty document", data: "", wantErr: true},
		{name: "whitespace only document", data: "   \n\t ", wantErr: true},
		{name: "malformed json", data: `{"schema_version":`, wantErr: true},
		{name: "missing schema_version", data: `{"catalog": []}`, wantErr: true},
		{name: "missing catalog field", data: `{"schema_version": "1"}`, wantErr: true},
		{name: "entry missing path", data: `{"schema_version": "1", "catalog": [{"digest": "abc"}]}`, wantErr: true},
		{name: "entry missing digest", data: `{"schema_version": "1", "catalog": [{"path": "a"}]}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseManifest([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseManifest() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseManifest() error = %v", err)
			}
			if got.SchemaVersion != tt.want.SchemaVersion {
				t.Fatalf("SchemaVersion = %q, want %q", got.SchemaVersion, tt.want.SchemaVersion)
			}
			if got.CLICompatRange != tt.want.CLICompatRange {
				t.Fatalf("CLICompatRange = %q, want %q", got.CLICompatRange, tt.want.CLICompatRange)
			}
			if len(got.Binaries) != len(tt.want.Binaries) {
				t.Fatalf("Binaries = %v, want %v", got.Binaries, tt.want.Binaries)
			}
			for i := range tt.want.Binaries {
				if got.Binaries[i] != tt.want.Binaries[i] {
					t.Fatalf("Binaries[%d] = %q, want %q", i, got.Binaries[i], tt.want.Binaries[i])
				}
			}
			if len(got.Catalog) != len(tt.want.Catalog) {
				t.Fatalf("Catalog len = %d, want %d", len(got.Catalog), len(tt.want.Catalog))
			}
			for i := range tt.want.Catalog {
				if got.Catalog[i] != tt.want.Catalog[i] {
					t.Fatalf("Catalog[%d] = %+v, want %+v", i, got.Catalog[i], tt.want.Catalog[i])
				}
			}
		})
	}
}
