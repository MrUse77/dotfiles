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
