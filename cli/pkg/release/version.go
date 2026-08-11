package release

import (
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// VersionIdentity is an exact configuration release identity. Tag is the exact
// "config-vMAJOR.MINOR.PATCH" selector; Digest is populated only after the
// artifact bytes have been admitted and verified.
type VersionIdentity struct {
	Tag    string
	Digest string
}

// configReleaseVersionRe matches the exact shape config-vMAJOR.MINOR.PATCH
// with strictly numeric segments.
var configReleaseVersionRe = regexp.MustCompile(`^config-v[0-9]+\.[0-9]+\.[0-9]+$`)

// ParseConfigVersion accepts only an exact stable config-vMAJOR.MINOR.PATCH
// configuration release tag. It rejects latest, channels, pins, prereleases,
// build metadata, legacy v* CLI tags, bare "config", and malformed shapes.
// The returned identity carries the tag but no digest until admission.
func ParseConfigVersion(raw string) (VersionIdentity, error) {
	if !configReleaseVersionRe.MatchString(raw) {
		return VersionIdentity{}, &InvalidConfigVersionError{Value: raw}
	}
	// Strict semver re-validates the numeric core so shapes like
	// config-v01.2.3 (leading zeros) are rejected even though they match the
	// shape regex.
	if _, err := semver.StrictNewVersion(strings.TrimPrefix(raw, "config-v")); err != nil {
		return VersionIdentity{}, &InvalidConfigVersionError{Value: raw}
	}
	return VersionIdentity{Tag: raw}, nil
}

// VersionComparison is the semantic relationship between installed and latest.
type VersionComparison int

const (
	// InstalledOlder means the installed version is less than the latest release.
	InstalledOlder VersionComparison = iota
	// InstalledEqual means the installed version matches the latest release.
	InstalledEqual
	// InstalledNewer means the installed version is greater than the latest release.
	InstalledNewer
)

// CompareVersions parses both values as strict SemVer versions, strips at most one
// leading lowercase "v" prefix from each, and returns the semantic relationship
// between them.
func CompareVersions(installedRaw, latestRaw string) (VersionComparison, error) {
	installed, err := parseVersion(installedRaw, "installed version")
	if err != nil {
		return 0, err
	}
	latest, err := parseVersion(latestRaw, "release tag")
	if err != nil {
		return 0, err
	}

	switch {
	case installed.LessThan(latest):
		return InstalledOlder, nil
	case installed.Equal(latest):
		return InstalledEqual, nil
	default:
		return InstalledNewer, nil
	}
}

func parseVersion(raw, subject string) (*semver.Version, error) {
	normalized := strings.TrimPrefix(raw, "v")
	v, err := semver.StrictNewVersion(normalized)
	if err != nil {
		return nil, &InvalidVersionError{Subject: subject, Value: raw, Cause: err}
	}
	return v, nil
}
