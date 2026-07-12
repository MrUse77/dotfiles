package installer

import (
	"os"
	"os/exec"
)

// runCommand is a helper to run commands and redirect I/O to the current terminal.
// This is critical for interactive prompts in pacman, paru, etc.
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
