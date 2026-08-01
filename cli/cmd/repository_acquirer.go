package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// RepositoryRequest is a frozen description of the repository to acquire.
// It captures the canonical destination, the ref selected by the installer
// version and development overrides, and the internal clone URL. The URL is
// not rendered to the user because it may contain credentials or overrides.
type RepositoryRequest struct {
	Destination string
	Ref         string
	URL         string
}

// RepositoryAcquisition is the successful result of repository acquisition.
type RepositoryAcquisition struct {
	Root        string
	Destination string
	Ref         string
}

// RepositoryAcquirer clones or updates the dotfiles repository into the
// canonical destination at the requested ref.
type RepositoryAcquirer interface {
	Acquire(ctx context.Context, request RepositoryRequest, output io.Writer) (RepositoryAcquisition, error)
}

// gitCommandRunner abstracts the Git commands used by the production acquirer
// so tests can observe behavior without invoking a real Git binary.
type gitCommandRunner interface {
	Run(ctx context.Context, dir string, stdout, stderr io.Writer, args ...string) error
}

type osGitCommandRunner struct{}

func (osGitCommandRunner) Run(ctx context.Context, dir string, stdout, stderr io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// NewRepositoryAcquirer returns a production acquirer that invokes Git directly.
func NewRepositoryAcquirer() RepositoryAcquirer {
	return &productionRepositoryAcquirer{runner: osGitCommandRunner{}}
}

// NewRepositoryAcquirerWithRunner returns a production acquirer that uses the
// supplied runner for Git commands. It is primarily used by tests.
func NewRepositoryAcquirerWithRunner(runner gitCommandRunner) RepositoryAcquirer {
	return &productionRepositoryAcquirer{runner: runner}
}

// productionRepositoryAcquirer implements the missing-clone repository phase.
// It retains the existing clone contract: absent destinations are cloned,
// usable destinations are updated in place, and non-repository destinations are
// an error.
type productionRepositoryAcquirer struct {
	runner gitCommandRunner
}

// Acquire clones or updates the repository described by request. It does not
// delete or overwrite a non-repository destination. Acquisition failures leave
// any partially-created directory in place for diagnosis.
func (a *productionRepositoryAcquirer) Acquire(ctx context.Context, request RepositoryRequest, output io.Writer) (RepositoryAcquisition, error) {
	if err := PreflightRepositoryDestination(request.Destination); err != nil {
		return RepositoryAcquisition{}, err
	}

	dest := request.Destination
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return RepositoryAcquisition{}, fmt.Errorf("prepare repository destination parent: %w", err)
	}

	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		fmt.Fprintf(output, "Actualizando dotfiles en %s (%s)...\n", dest, request.Ref)
		for _, args := range [][]string{
			{"fetch", "origin", request.Ref},
			{"checkout", "--force", "--detach", "FETCH_HEAD"},
			{"submodule", "update", "--init", "--recursive"},
		} {
			if err := a.runner.Run(ctx, dest, output, output, args...); err != nil {
				return RepositoryAcquisition{}, fmt.Errorf("actualizar dotfiles en %s: %w", dest, err)
			}
		}
		return RepositoryAcquisition{Root: dest, Destination: dest, Ref: request.Ref}, nil
	}

	fmt.Fprintf(output, "Clonando dotfiles (%s) en %s...\n", request.Ref, dest)
	if err := a.runner.Run(ctx, "", output, output, "clone", "--recurse-submodules", "--branch", request.Ref, request.URL, dest); err != nil {
		return RepositoryAcquisition{}, fmt.Errorf("clonar dotfiles en %s: %w", dest, err)
	}
	return RepositoryAcquisition{Root: dest, Destination: dest, Ref: request.Ref}, nil
}

// PreflightRepositoryDestination rejects a destination that already exists but
// is not a usable Git repository. It is read-only and therefore safe to run
// before the user accepts the installation plan.
func PreflightRepositoryDestination(dest string) error {
	info, err := os.Stat(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read repository destination %q: %w", dest, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("repository destination %q exists and is not a directory", dest)
	}
	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s ya existe pero no es un clon de dotfiles", dest)
		}
		return fmt.Errorf("read repository marker in %q: %w", dest, err)
	}
	return nil
}

// BuildRepositoryRequest freezes the destination, ref, and URL for the missing-
// clone route. It honors DOTFILES_DIR, DOTFILES_REPO, DOTFILES_BRANCH, and the
// binary Version exactly once so the reviewed plan cannot drift before execution.
func BuildRepositoryRequest() (RepositoryRequest, error) {
	candidates := repositoryCandidates()
	if len(candidates) == 0 {
		return RepositoryRequest{}, errors.New("cannot resolve repository destination")
	}
	dest := candidates[0]

	repoURL := os.Getenv("DOTFILES_REPO")
	if repoURL == "" {
		repoURL = "https://github.com/MrUse77/dotfiles.git"
	}
	ref := os.Getenv("DOTFILES_BRANCH")
	if ref == "" {
		if Version != "" && Version != "dev" {
			ref = Version
		} else {
			ref = "main"
		}
	}

	return RepositoryRequest{Destination: dest, Ref: ref, URL: repoURL}, nil
}

// ensureRepositoryClone remains a narrow compatibility wrapper around the
// production acquirer for callers that already depend on it. New code should use
// RepositoryAcquirer.Acquire with a frozen RepositoryRequest.
func ensureRepositoryClone(out io.Writer) (string, error) {
	req, err := BuildRepositoryRequest()
	if err != nil {
		return "", err
	}
	acq, err := NewRepositoryAcquirer().Acquire(context.Background(), req, out)
	if err != nil {
		return "", err
	}
	return acq.Root, nil
}
