//go:build windows

package services

import (
	"bytes"
	"context"
	"os/exec"
	"syscall"
)

// execCLI runs the engram CLI with the given args and env on Windows.
// HideWindow=true prevents a console window from flashing during tests.
func (s *CloudControlService) execCLI(ctx context.Context, args []string, env []string) (string, string, error) {
	bin, err := exec.LookPath(s.cli)
	if err != nil {
		return "", "", err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}

// execProbe runs a probe command (no token env injection, no hide-window requirement,
// just capturing output). Reuses execCLI logic.
func (s *CloudControlService) execProbe(ctx context.Context, args []string) (string, string, error) {
	bin, err := exec.LookPath(s.cli)
	if err != nil {
		return "", "", err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
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
	if notFound {
		return true
	}
	// exec.LookPath returns *exec.Error wrapping os.ErrNotExist on Windows too.
	return false
}

// isKilled returns true when the process was killed (i.e., context deadline exceeded).
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
