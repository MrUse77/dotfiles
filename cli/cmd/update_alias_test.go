package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/release"
	"github.com/spf13/cobra"
)

// runCommandWithDeps executes a command with injected self-update fakes and
// returns its captured output. It is the shared harness for alias-equivalence
// tests: both commands must behave byte-identically against the same fakes.
func runCommandWithDeps(cmd *cobra.Command, version string, deps selfUpdateDependencies, args ...string) (string, error) {
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	installCommandWithDeps(cmd, version, deps)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestUpdateAlias_ByteIdenticalOutcome(t *testing.T) {
	home := t.TempDir()
	deps := selfUpdateDependencies{
		releaseClient: &fakeReleaseClient{latestTag: "v1.1.0", binary: []byte("binary bytes")},
		replacer:      release.NewAtomicReplacer(release.OSFileOps{}, release.SHA256Verifier{}),
		homeResolver:  func() (string, error) { return home, nil },
		arch:          func() string { return "amd64" },
	}

	selfOut, selfErr := runCommandWithDeps(newSelfUpdateCommand(), "v1.0.0", deps)
	aliasOut, aliasErr := runCommandWithDeps(newUpdateCommand(), "v1.0.0", deps)

	if selfErr != nil || aliasErr != nil {
		t.Fatalf("errors: self=%v alias=%v", selfErr, aliasErr)
	}
	if selfOut != aliasOut {
		t.Fatalf("byte-identical output expected:\nself:  %q\nalias: %q", selfOut, aliasOut)
	}

	got, err := os.ReadFile(filepath.Join(home, ".local", "bin", "moonarch-cli"))
	if err != nil {
		t.Fatalf("binary not replaced: %v", err)
	}
	if string(got) != "binary bytes" {
		t.Fatalf("binary content = %q, want the verified candidate", string(got))
	}
}

func TestUpdateAlias_AlreadyCurrentOutcomeIsIdentical(t *testing.T) {
	deps := selfUpdateDependencies{
		releaseClient: &fakeReleaseClient{latestTag: "v1.1.0", binary: []byte("binary")},
		replacer:      &fakeBinaryReplacer{},
		homeResolver:  fixedHome,
		arch:          func() string { return "amd64" },
	}

	selfOut, selfErr := runCommandWithDeps(newSelfUpdateCommand(), "v1.1.0", deps)
	aliasOut, aliasErr := runCommandWithDeps(newUpdateCommand(), "v1.1.0", deps)
	if selfErr != nil || aliasErr != nil {
		t.Fatalf("errors: self=%v alias=%v", selfErr, aliasErr)
	}
	if selfOut != aliasOut {
		t.Fatalf("already-current outputs differ:\nself:  %q\nalias: %q", selfOut, aliasOut)
	}
	if selfOut == "" {
		t.Fatalf("expected an already-current report")
	}
}

func TestUpdateAlias_DevBuildIsByteIdentical(t *testing.T) {
	var called bool
	deps := selfUpdateDependencies{releaseClient: &fakeReleaseClient{latestTag: "v1.1.0", binary: []byte("x")}}

	run := func(cmd *cobra.Command) (string, error) {
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.RunE = func(c *cobra.Command, args []string) error {
			return runSelfUpdateWithFactory(c, "dev", func(*cobra.Command) selfUpdateDependencies {
				called = true
				return deps
			})
		}
		cmd.SetArgs(nil)
		err := cmd.Execute()
		return out.String(), err
	}

	selfOut, selfErr := run(newSelfUpdateCommand())
	aliasOut, aliasErr := run(newUpdateCommand())
	if selfErr != nil || aliasErr != nil {
		t.Fatalf("dev guard errors: %v %v", selfErr, aliasErr)
	}
	if called {
		t.Fatalf("factory invoked in dev build")
	}
	if selfOut != aliasOut || selfOut == "" {
		t.Fatalf("dev outputs differ or empty: %q vs %q", selfOut, aliasOut)
	}
}

func TestUpdateAlias_BothRejectConfigSelectorWithSameError(t *testing.T) {
	deps := selfUpdateDependencies{
		releaseClient: &fakeReleaseClient{latestTag: "v1.1.0", binary: []byte("x")},
		replacer:      &fakeBinaryReplacer{},
		homeResolver:  fixedHome,
		arch:          func() string { return "amd64" },
	}

	selfOut, selfErr := runCommandWithDeps(newSelfUpdateCommand(), "v1.0.0", deps, "config-v1.2.3")
	aliasOut, aliasErr := runCommandWithDeps(newUpdateCommand(), "v1.0.0", deps, "config-v1.2.3")

	if selfErr == nil || aliasErr == nil {
		t.Fatalf("expected rejection: selfErr=%v aliasErr=%v", selfErr, aliasErr)
	}
	if selfErr.Error() != aliasErr.Error() {
		t.Fatalf("rejection errors differ:\nself:  %v\nalias: %v", selfErr, aliasErr)
	}
	if selfOut != aliasOut {
		t.Fatalf("rejection outputs differ:\nself:  %q\nalias: %q", selfOut, aliasOut)
	}
}
