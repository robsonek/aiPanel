// Package systemd provides safe exec wrappers and system adapter implementations.
package systemd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Runner abstracts shell command execution to support tests and dry-run flows.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// ExecRunner executes commands using os/exec.
type ExecRunner struct {
	DryRun bool
}

// Run executes a command and returns combined output.
func (r ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	if r.DryRun {
		return fmt.Sprintf("dry-run: %s %s", name, strings.Join(args, " ")), nil
	}
	// Command name and args are provided by installer-owned call sites.
	//nolint:gosec // G204
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("exec %s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// DaemonReload triggers systemd to reload unit files.
func DaemonReload(ctx context.Context, runner Runner) error {
	_, err := runner.Run(ctx, "systemctl", "daemon-reload")
	return err
}

// EnableNow enables and starts a unit.
func EnableNow(ctx context.Context, runner Runner, unit string) error {
	_, err := runner.Run(ctx, "systemctl", "enable", "--now", unit)
	return err
}

// Restart restarts a unit.
func Restart(ctx context.Context, runner Runner, unit string) error {
	_, err := runner.Run(ctx, "systemctl", "restart", unit)
	return err
}

// IsActive checks whether a unit is active.
func IsActive(ctx context.Context, runner Runner, unit string) (bool, error) {
	out, err := runner.Run(ctx, "systemctl", "is-active", unit)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "active", nil
}
