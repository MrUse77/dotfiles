package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Identity is the durable persisted configuration identity, serialized into
// state.json. It is deliberately a separate type from VersionIdentity (the
// resolution-side identity in version.go): Identity carries the JSON tags the
// design's state contract requires and is only produced after admission.
type Identity struct {
	Tag    string `json:"tag"`
	Digest string `json:"digest"`
}

// EvidenceObservation is one managed-target destination captured during a
// whole-plan preflight scan. The exact set of observations is bound into the
// drift-authorization token so authorization cannot transfer across releases
// or review sessions.
type EvidenceObservation struct {
	Path             string `json:"path"`     // managed-target destination
	ExpectedIdentity string `json:"expected"` // planned pre-state identity (dev:ino + digest prefix)
	ObservedIdentity string `json:"observed"` // actual pre-state at preflight
	DriftClass       string `json:"class"`    // "replacement" | "removal" | "creation-pre"
}

// EvidenceTokenInput is the canonical-JSON input for the evidence token. The
// struct field order is the canonical serialization order.
type EvidenceTokenInput struct {
	Tag            string                `json:"tag"`
	ArtifactDigest string                `json:"artifact_digest"`
	Observations   []EvidenceObservation `json:"observations"`
}

// EvidenceMismatchError signals that a presented drift-authorization token does
// not match the freshly computed token for the release + observation set.
type EvidenceMismatchError struct {
	Expected  string
	Presented string
}

func (e *EvidenceMismatchError) Error() string {
	return fmt.Sprintf("evidence token mismatch: expected %s, presented %s", e.Expected, e.Presented)
}

// ComputeEvidenceToken returns hex(SHA256(canonicalJSON(in))). Canonicalization
// means deterministic field order (the struct declaration order) and a nil
// observation set normalized to an empty array, so the token never depends on
// how the input slice was constructed. json.Marshal cannot fail for the
// defined field types (strings and structs of strings).
func ComputeEvidenceToken(in EvidenceTokenInput) string {
	normalized := in
	if normalized.Observations == nil {
		normalized.Observations = []EvidenceObservation{}
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// VerifyEvidenceToken authorizes only an exact byte-for-byte match between the
// freshly computed token for in and the presented hex token. Any release,
// digest, or observation-set change produces a different token and is
// rejected. The comparison is case-sensitive: tokens are machine-printed
// lowercase hex and machine-verified without normalization.
func VerifyEvidenceToken(in EvidenceTokenInput, presented string) error {
	expected := ComputeEvidenceToken(in)
	if expected != presented {
		return &EvidenceMismatchError{Expected: expected, Presented: presented}
	}
	return nil
}
