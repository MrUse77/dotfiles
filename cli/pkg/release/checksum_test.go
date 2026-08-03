package release

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
)

func checksumLine(name string, data []byte) string {
	sum := sha256.Sum256(data)
	sumSlice := sum[:]
	return hex.EncodeToString(sumSlice) + "  " + name
}

func TestSHA256Verifier(t *testing.T) {
	binary := []byte("binary contents")
	goodLine := checksumLine("moonarch-cli-linux-amd64", binary)

	tests := []struct {
		name        string
		binary      []byte
		list        string
		wantErr     bool
		wantErrType any
	}{
		{
			name:   "valid one-entry match",
			binary: binary,
			list:   goodLine,
		},
		{
			name:        "missing asset entry",
			binary:      binary,
			list:        checksumLine("other-asset", binary),
			wantErr:     true,
			wantErrType: &ChecksumEntryMissingError{},
		},
		{
			name:        "duplicate asset entry",
			binary:      binary,
			list:        goodLine + "\n" + goodLine,
			wantErr:     true,
			wantErrType: &ChecksumEntryAmbiguousError{},
		},
		{
			name:        "malformed line wrong hash length",
			binary:      binary,
			list:        "abcd  moonarch-cli-linux-amd64",
			wantErr:     true,
			wantErrType: &ChecksumFormatError{},
		},
		{
			name:        "malformed line non-hex",
			binary:      binary,
			list:        strings.Repeat("g", 64) + "  moonarch-cli-linux-amd64",
			wantErr:     true,
			wantErrType: &ChecksumFormatError{},
		},
		{
			name:        "malformed line wrong separator",
			binary:      binary,
			list:        checksumLine("moonarch-cli-linux-amd64", binary)[:64] + " moonarch-cli-linux-amd64",
			wantErr:     true,
			wantErrType: &ChecksumFormatError{},
		},
		{
			name:        "checksum mismatch",
			binary:      []byte("different binary"),
			list:        goodLine,
			wantErr:     true,
			wantErrType: &ChecksumMismatchError{},
		},
		{
			name:        "empty checksum list",
			binary:      binary,
			list:        "",
			wantErr:     true,
			wantErrType: &ChecksumEntryMissingError{},
		},
		{
			name:   "valid line with ignored blank lines",
			binary: binary,
			list:   "\n" + goodLine + "\n",
		},
	}

	verifier := SHA256Verifier{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifier.Verify("moonarch-cli-linux-amd64", bytes.NewReader(tt.binary), strings.NewReader(tt.list))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Verify() error = nil, want error")
				}
				var target any
				switch tt.wantErrType.(type) {
				case *ChecksumFormatError:
					target = new(*ChecksumFormatError)
				case *ChecksumEntryMissingError:
					target = new(*ChecksumEntryMissingError)
				case *ChecksumEntryAmbiguousError:
					target = new(*ChecksumEntryAmbiguousError)
				case *ChecksumMismatchError:
					target = new(*ChecksumMismatchError)
				default:
					t.Fatalf("unknown error type in test: %T", tt.wantErrType)
				}
				if !errors.As(err, target) {
					t.Fatalf("Verify() error type = %T, want %T", err, tt.wantErrType)
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify() error = %v, want nil", err)
			}
		})
	}
}

func TestSHA256Verifier_BinaryReadError(t *testing.T) {
	binary := []byte("binary contents")
	list := checksumLine("moonarch-cli-linux-amd64", binary)
	verifier := SHA256Verifier{}

	err := verifier.Verify("moonarch-cli-linux-amd64", &errorReader{err: errors.New("read failed")}, strings.NewReader(list))
	if err == nil {
		t.Fatalf("expected error")
	}
}

type errorReader struct {
	err error
}

func (r *errorReader) Read(_ []byte) (int, error) { return 0, r.err }

func TestChecksumVerifier_Interface(t *testing.T) {
	var _ ChecksumVerifier = SHA256Verifier{}
}

func TestSHA256Verifier_PassesWithFailingReaderAfterHash(t *testing.T) {
	// The reader is exhausted after reading the first byte, causing the verifier
	// to fail during hashing. This confirms the binary is not read twice.
	binary := []byte("x")
	list := checksumLine("asset", binary)
	verifier := SHA256Verifier{}

	err := verifier.Verify("asset", io.LimitReader(bytes.NewReader(binary), 0), strings.NewReader(list))
	if err == nil {
		t.Fatalf("expected error from short reader")
	}
	var me *ChecksumMismatchError
	if !errors.As(err, &me) {
		t.Fatalf("expected ChecksumMismatchError, got %T", err)
	}
}
