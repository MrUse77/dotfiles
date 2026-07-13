// Package external executes structured external installer actions.
package external

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

// CommandFunc is the process boundary used by Runner. It is injectable for tests.
type CommandFunc func(context.Context, plan.CommandSpec, io.Reader, io.Writer, io.Writer) error

// CommandRunner executes reviewed external actions.
type CommandRunner interface {
	Run(context.Context, plan.ExternalAction) error
}

// Runner executes commands without invoking a shell.
type Runner struct {
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	command CommandFunc
}

// NewRunner constructs a runner. A nil command uses os/exec and the process stdio.
func NewRunner(command CommandFunc) *Runner {
	return &Runner{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, command: command}
}

// WithStdio configures the streams forwarded to subsequently executed commands.
func (r *Runner) WithStdio(stdin io.Reader, stdout, stderr io.Writer) *Runner {
	r.stdin, r.stdout, r.stderr = stdin, stdout, stderr
	return r
}

func (r *Runner) Run(ctx context.Context, action plan.ExternalAction) error {
	command := r.command
	if command == nil {
		command = func(ctx context.Context, spec plan.CommandSpec, stdin io.Reader, stdout, stderr io.Writer) error {
			cmd := exec.CommandContext(ctx, spec.Name, spec.Args...)
			cmd.Dir = spec.Dir
			cmd.Env = mergedEnvironment(spec.Env)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
			return cmd.Run()
		}
	}
	if err := command(ctx, action.Command, r.stdin, r.stdout, r.stderr); err != nil {
		return &report.ExternalActionError{Action: action, Cause: err}
	}
	return nil
}

func mergedEnvironment(overrides map[string]string) []string {
	values := make(map[string]string, len(overrides))
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

// RunAll executes actions in their supplied reviewed order and stops at the first failure.
func (r *Runner) RunAll(ctx context.Context, actions []plan.ExternalAction) error {
	for _, action := range actions {
		if err := r.Run(ctx, action); err != nil {
			return err
		}
	}
	return nil
}
