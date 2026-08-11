package release

import (
	"errors"
	"strings"
	"testing"
)

// evidenceObservation is a compact builder for test observations.
func evidenceObservation(path, expected, observed, class string) EvidenceObservation {
	return EvidenceObservation{
		Path:             path,
		ExpectedIdentity: expected,
		ObservedIdentity: observed,
		DriftClass:       class,
	}
}

// evidenceFixture is the canonical input the golden token is bound to. The
// golden value is hex(SHA256) of the canonical JSON:
// {"tag":"config-v1.0.0","artifact_digest":"<64×a>","observations":[...]}
func evidenceFixture() EvidenceTokenInput {
	return EvidenceTokenInput{
		Tag:            "config-v1.0.0",
		ArtifactDigest: strings.Repeat("a", 64),
		Observations: []EvidenceObservation{
			evidenceObservation("home/.config/hypr/hyprland.conf", "legacy/unknown", "dev:123:456:deadbeef", "replacement"),
			evidenceObservation("home/.config/waybar/config.jsonc", "legacy/unknown", "dev:789:012:cafebabe", "removal"),
		},
	}
}

const evidenceGoldenToken = "a7dfffe57aa9ea669c1f3af4abfb7a117179cec3b7b879b226168b58f9574f00"

func TestComputeEvidenceToken_GoldenDeterminism(t *testing.T) {
	token := ComputeEvidenceToken(evidenceFixture())
	if token != evidenceGoldenToken {
		t.Fatalf("golden token mismatch:\n got %s\nwant %s", token, evidenceGoldenToken)
	}
	// Recomputing must be byte-identical (determinism across calls).
	if again := ComputeEvidenceToken(evidenceFixture()); again != token {
		t.Fatalf("token not deterministic: %s vs %s", again, token)
	}
}

func TestComputeEvidenceToken_IsHexSHA256(t *testing.T) {
	token := ComputeEvidenceToken(evidenceFixture())
	if len(token) != 64 {
		t.Fatalf("token length = %d, want 64 hex chars of SHA-256", len(token))
	}
	for _, r := range token {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("token contains non-lowercase-hex char %q", r)
		}
	}
}

func TestEvidenceToken_NilEqualsEmptyObservations(t *testing.T) {
	// canonicalization normalizes nil to an empty array so the token does not
	// depend on how the input was constructed.
	withEmpty := evidenceFixture()
	withEmpty.Observations = []EvidenceObservation{}
	withNil := evidenceFixture()
	withNil.Observations = nil
	if ComputeEvidenceToken(withEmpty) != ComputeEvidenceToken(withNil) {
		t.Fatal("nil and empty observation sets must canonicalize to the same token")
	}
}

func TestEvidenceToken_MismatchRejects(t *testing.T) {
	in := evidenceFixture()
	good := ComputeEvidenceToken(in)
	if err := VerifyEvidenceToken(in, good); err != nil {
		t.Fatalf("VerifyEvidenceToken with the correct token: %v", err)
	}

	cases := []struct {
		name      string
		mutate    func(*EvidenceTokenInput)
		presented string
	}{
		{
			name:      "wrong token",
			presented: strings.Repeat("0", 64),
		},
		{
			name:      "uppercase presentation of the same hex",
			presented: strings.ToUpper(good),
		},
		{
			name:      "truncated token",
			presented: good[:63],
		},
		{
			name: "tag change rejects",
			mutate: func(in *EvidenceTokenInput) {
				in.Tag = "config-v1.0.1"
			},
			presented: good,
		},
		{
			name: "artifact digest change rejects",
			mutate: func(in *EvidenceTokenInput) {
				in.ArtifactDigest = strings.Repeat("b", 64)
			},
			presented: good,
		},
		{
			name: "observation set change rejects",
			mutate: func(in *EvidenceTokenInput) {
				in.Observations = append(in.Observations,
					evidenceObservation("home/.zshrc", "legacy/unknown", "dev:000:111:feedface", "replacement"))
			},
			presented: good,
		},
		{
			name: "observation reordering rejects",
			mutate: func(in *EvidenceTokenInput) {
				rev := make([]EvidenceObservation, len(in.Observations))
				for i, o := range in.Observations {
					rev[len(in.Observations)-1-i] = o
				}
				in.Observations = rev
			},
			presented: good,
		},
		{
			name: "observed identity change rejects",
			mutate: func(in *EvidenceTokenInput) {
				in.Observations[0].ObservedIdentity = "dev:999:999:ffffffff"
			},
			presented: good,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := evidenceFixture()
			if tc.mutate != nil {
				tc.mutate(&changed)
			}
			err := VerifyEvidenceToken(changed, tc.presented)
			if err == nil {
				t.Fatal("expected authorization to be rejected")
			}
			var ee *EvidenceMismatchError
			if !errors.As(err, &ee) {
				t.Fatalf("expected *EvidenceMismatchError, got %T: %v", err, err)
			}
			if ee.Expected == "" || ee.Presented == "" {
				t.Fatalf("EvidenceMismatchError must carry both tokens, got %+v", ee)
			}
		})
	}
}

func TestEvidenceToken_FreshScanMatches(t *testing.T) {
	// A fresh full-plan scan that reproduces the exact observation set with
	// the same release identity MUST authorize; only the exact bound set may.
	in := evidenceFixture()
	presented := ComputeEvidenceToken(in)

	// An independent reconstruction with the same field order and values.
	fresh := EvidenceTokenInput{
		Tag:            "config-v1.0.0",
		ArtifactDigest: strings.Repeat("a", 64),
		Observations: []EvidenceObservation{
			evidenceObservation("home/.config/hypr/hyprland.conf", "legacy/unknown", "dev:123:456:deadbeef", "replacement"),
			evidenceObservation("home/.config/waybar/config.jsonc", "legacy/unknown", "dev:789:012:cafebabe", "removal"),
		},
	}
	if err := VerifyEvidenceToken(fresh, presented); err != nil {
		t.Fatalf("fresh scan reproducing the exact set must authorize: %v", err)
	}
}
