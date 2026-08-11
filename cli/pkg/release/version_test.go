package release

import (
	"errors"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name        string
		installed   string
		releaseTag  string
		want        VersionComparison
		wantErr     bool
		wantInvalid bool
		wantSubject string
	}{
		{
			name:       "both have leading v, installed older",
			installed:  "v1.0.0",
			releaseTag: "v1.1.0",
			want:       InstalledOlder,
		},
		{
			name:       "both have leading v, installed newer",
			installed:  "v2.0.0",
			releaseTag: "v1.9.0",
			want:       InstalledNewer,
		},
		{
			name:       "no leading v on installed, equal",
			installed:  "2.0.0",
			releaseTag: "v2.0.0",
			want:       InstalledEqual,
		},
		{
			name:       "no leading v on release tag, equal",
			installed:  "v3.0.0",
			releaseTag: "3.0.0",
			want:       InstalledEqual,
		},
		{
			name:       "semantic ordering, 1.10.0 after 1.9.0",
			installed:  "v1.9.0",
			releaseTag: "v1.10.0",
			want:       InstalledOlder,
		},
		{
			name:        "partial installed version invalid",
			installed:   "1.2",
			releaseTag:  "v1.2.3",
			wantErr:     true,
			wantInvalid: true,
			wantSubject: "installed version",
		},
		{
			name:        "partial release tag invalid",
			installed:   "v1.2.3",
			releaseTag:  "1.2",
			wantErr:     true,
			wantInvalid: true,
			wantSubject: "release tag",
		},
		{
			name:        "invalid installed version string",
			installed:   "latest",
			releaseTag:  "v1.2.3",
			wantErr:     true,
			wantInvalid: true,
			wantSubject: "installed version",
		},
		{
			name:        "invalid release tag string",
			installed:   "v1.2.3",
			releaseTag:  "release-2024",
			wantErr:     true,
			wantInvalid: true,
			wantSubject: "release tag",
		},
		{
			name:        "empty installed version invalid",
			installed:   "",
			releaseTag:  "v1.2.3",
			wantErr:     true,
			wantInvalid: true,
			wantSubject: "installed version",
		},
		{
			name:        "empty release tag invalid",
			installed:   "v1.2.3",
			releaseTag:  "",
			wantErr:     true,
			wantInvalid: true,
			wantSubject: "release tag",
		},
		{
			name:        "bare v prefix invalid installed",
			installed:   "v",
			releaseTag:  "v1.2.3",
			wantErr:     true,
			wantInvalid: true,
			wantSubject: "installed version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareVersions(tt.installed, tt.releaseTag)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("CompareVersions() error = nil, want error")
				}
				if tt.wantInvalid {
					var target *InvalidVersionError
					if !errors.As(err, &target) {
						t.Fatalf("CompareVersions() error type = %T, want *InvalidVersionError", err)
					}
					if target.Subject != tt.wantSubject {
						t.Fatalf("InvalidVersionError.Subject = %q, want %q", target.Subject, tt.wantSubject)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("CompareVersions() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("CompareVersions() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseConfigVersion proves ParseConfigVersion accepts only exact stable
// config-vMAJOR.MINOR.PATCH tags and rejects latest, channels, prereleases,
// legacy v* CLI tags, bare "config", and malformed version shapes.
func TestParseConfigVersion(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantTag string
		wantErr bool
	}{
		{name: "exact stable version", raw: "config-v1.2.3", wantTag: "config-v1.2.3"},
		{name: "strict semver ordering shape", raw: "config-v1.10.0", wantTag: "config-v1.10.0"},
		{name: "zero minor version", raw: "config-v2.0.0", wantTag: "config-v2.0.0"},
		{name: "latest rejected", raw: "latest", wantErr: true},
		{name: "cli v tag rejected", raw: "v1.2.3", wantErr: true},
		{name: "bare config rejected", raw: "config", wantErr: true},
		{name: "partial version rejected", raw: "config-v1.2", wantErr: true},
		{name: "prerelease rejected", raw: "config-v1.2.3-beta.1", wantErr: true},
		{name: "build metadata rejected", raw: "config-v1.2.3+build", wantErr: true},
		{name: "leading zero rejected", raw: "config-v01.2.3", wantErr: true},
		{name: "four segments rejected", raw: "config-v1.2.3.4", wantErr: true},
		{name: "missing segments rejected", raw: "config-v1", wantErr: true},
		{name: "bare config-v rejected", raw: "config-v", wantErr: true},
		{name: "empty rejected", raw: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConfigVersion(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseConfigVersion(%q) error = nil, want error", tt.raw)
				}
				var ive *InvalidConfigVersionError
				if !errors.As(err, &ive) {
					t.Fatalf("ParseConfigVersion(%q) error type = %T, want *InvalidConfigVersionError", tt.raw, err)
				}
				if ive.Value != tt.raw {
					t.Fatalf("InvalidConfigVersionError.Value = %q, want %q", ive.Value, tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseConfigVersion(%q) error = %v", tt.raw, err)
			}
			if got.Tag != tt.wantTag {
				t.Fatalf("ParseConfigVersion(%q) Tag = %q, want %q", tt.raw, got.Tag, tt.wantTag)
			}
			if got.Digest != "" {
				t.Fatalf("ParseConfigVersion(%q) Digest = %q, want empty before admission", tt.raw, got.Digest)
			}
		})
	}
}
