//go:build !unix

package main

import (
	"errors"
	"os"
	"os/exec"
)

// execCommand runs argv as a child process where exec(2) is not available and
// exits with its exit code. It only returns on error.
func execCommand(argv []string, vars map[string]string) error {
	for k, v := range vars {
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
