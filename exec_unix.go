//go:build unix

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// execCommand replaces the current process with argv, adding vars to the
// environment. It only returns on error.
func execCommand(argv []string, vars map[string]string) error {
	for k, v := range vars {
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, argv, os.Environ())
}
