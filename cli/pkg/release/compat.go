package release

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
)

// SupportedManifestSchema is the only manifest schema_version this CLI admits.
const SupportedManifestSchema = "1"

// CompatibilityError identifies an unsupported manifest schema or an
// unsatisfied CLI compatibility range. CheckCompatibility returns it and never
// mutates anything.
type CompatibilityError struct {
	Reason string
}

func (e *CompatibilityError) Error() string { return e.Reason }

// CheckCompatibility verifies that the manifest schema is supported and that
// the running CLI version satisfies the declared cli_compat_range. It is a
// pure read-only check: no manifest, cache, or filesystem mutation. An empty
// cli_compat_range declares no CLI constraint and passes; a malformed range,
// an unparsable CLI version, or an unsatisfied range fails closed.
func CheckCompatibility(m Manifest, cliVersion string) error {
	if m.SchemaVersion != SupportedManifestSchema {
		return &CompatibilityError{Reason: fmt.Sprintf(
			"unsupported manifest schema %q; supported: %s", m.SchemaVersion, SupportedManifestSchema)}
	}
	if m.CLICompatRange == "" {
		return nil
	}
	constraint, err := semver.NewConstraint(m.CLICompatRange)
	if err != nil {
		return &CompatibilityError{Reason: fmt.Sprintf(
			"invalid cli_compat_range %q: %v", m.CLICompatRange, err)}
	}
	cliSemver, err := parseVersion(cliVersion, "cli version")
	if err != nil {
		return &CompatibilityError{Reason: fmt.Sprintf(
			"invalid cli version %q: %v", cliVersion, err)}
	}
	if !constraint.Check(cliSemver) {
		return &CompatibilityError{Reason: fmt.Sprintf(
			"cli version %s does not satisfy cli_compat_range %q", cliVersion, m.CLICompatRange)}
	}
	return nil
}
