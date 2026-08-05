package release

import (
	"strings"

	"github.com/Masterminds/semver/v3"
)

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
