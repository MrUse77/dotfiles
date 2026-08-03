package release

import (
	"fmt"
	"net/http"
)

// TransportError wraps a failure at the HTTP transport layer (timeout, DNS,
// TLS, broken connection, etc). It never contains response body data.
type TransportError struct {
	Cause error
}

func (e *TransportError) Error() string { return fmt.Sprintf("transport error: %v", e.Cause) }
func (e *TransportError) Unwrap() error { return e.Cause }

// RateLimitError signals that GitHub rejected the request because the rate
// limit was exhausted. It applies to both 403 and 429 responses.
type RateLimitError struct {
	StatusCode int
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("github rate limit exceeded: HTTP %d", e.StatusCode)
}

// HTTPStatusError signals a non-success HTTP response that is not a rate limit.
type HTTPStatusError struct {
	StatusCode int
	Status     string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("github returned HTTP %d %s", e.StatusCode, e.Status)
}

// MalformedReleaseError signals that the release JSON was missing required data
// or could not be parsed.
type MalformedReleaseError struct {
	Cause error
}

func (e *MalformedReleaseError) Error() string { return fmt.Sprintf("malformed release: %v", e.Cause) }
func (e *MalformedReleaseError) Unwrap() error { return e.Cause }

// InvalidVersionError signals that the installed version or release tag is not
// a valid SemVer value.
type InvalidVersionError struct {
	Subject string // "installed version" or "release tag"
	Value   string
	Cause   error
}

func (e *InvalidVersionError) Error() string {
	return fmt.Sprintf("invalid %s %q: %v", e.Subject, e.Value, e.Cause)
}
func (e *InvalidVersionError) Unwrap() error { return e.Cause }

// UnsupportedArchitectureError signals that the host GOARCH is not amd64 or arm64.
type UnsupportedArchitectureError struct {
	GOARCH string
}

func (e *UnsupportedArchitectureError) Error() string {
	return fmt.Sprintf("unsupported architecture %q; only amd64 and arm64 are supported", e.GOARCH)
}

// AssetNotFoundError signals that a required release asset was missing from the
// release metadata.
type AssetNotFoundError struct {
	AssetName string
}

func (e *AssetNotFoundError) Error() string { return fmt.Sprintf("asset not found: %s", e.AssetName) }

// ChecksumFormatError signals a malformed SHA256SUMS.txt line.
type ChecksumFormatError struct {
	Line string
}

func (e *ChecksumFormatError) Error() string {
	return fmt.Sprintf("checksum line malformed: %q", e.Line)
}

// ChecksumEntryMissingError signals that SHA256SUMS.txt contains no entry for
// the requested asset.
type ChecksumEntryMissingError struct {
	AssetName string
}

func (e *ChecksumEntryMissingError) Error() string {
	return fmt.Sprintf("checksum entry missing for asset %q", e.AssetName)
}

// ChecksumEntryAmbiguousError signals that SHA256SUMS.txt contains more than
// one entry for the requested asset.
type ChecksumEntryAmbiguousError struct {
	AssetName string
}

func (e *ChecksumEntryAmbiguousError) Error() string {
	return fmt.Sprintf("checksum entry ambiguous for asset %q", e.AssetName)
}

// ChecksumMismatchError signals that the computed SHA-256 digest did not match
// the checksum list entry.
type ChecksumMismatchError struct {
	AssetName string
	Expected  string
	Computed  string
}

func (e *ChecksumMismatchError) Error() string {
	return fmt.Sprintf("checksum mismatch for %s: expected %s, computed %s", e.AssetName, e.Expected, e.Computed)
}

// BinaryReplacementError wraps a failure to stage, verify, chmod, or rename
// the executable.
type BinaryReplacementError struct {
	Cause error
}

func (e *BinaryReplacementError) Error() string {
	return fmt.Sprintf("binary replacement failed: %v", e.Cause)
}
func (e *BinaryReplacementError) Unwrap() error { return e.Cause }

// classifyResponse maps a non-success *http.Response to a typed error. It never
// reads the response body.
func classifyResponse(resp *http.Response) error {
	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		return &RateLimitError{StatusCode: resp.StatusCode}
	}
	return &HTTPStatusError{StatusCode: resp.StatusCode, Status: resp.Status}
}
