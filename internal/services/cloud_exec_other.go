//go:build !windows

package services

import (
	"bytes"
	"context"
	"os/exec"
)

// execCLI runs the engram CLI with the given args and env on non-Windows platforms.
func (s *CloudControlService) execCLI(ctx context.Context, args []string, env []string) (string, string, error) {
	bin, err := exec.LookPath(s.cli)
	if err != nil {
		return "", "", err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}

// execProbe runs a probe command capturing output.
func (s *CloudControlService) execProbe(ctx context.Context, args []string) (string, string, error) {
	bin, err := exec.LookPath(s.cli)
	if err != nil {
		return "", "", err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}

// isENOENT returns true when the error indicates the binary was not found.
func isENOENT(err error) bool {
	if err == nil {
		return false
	}
	_, notFound := err.(*exec.Error)
	return notFound
}

// isKilled returns true when the process was killed (context deadline exceeded).
func isKilled(err error) bool {
	if err == nil {
		return false
	}
	if ee, ok := err.(*exec.ExitError); ok {
		if ee.ExitCode() == -1 {
			return true
		}
	}
	return false
}
