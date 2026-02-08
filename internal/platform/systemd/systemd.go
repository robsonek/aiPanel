// Package systemd provides safe exec wrappers and system adapter implementations.
package systemd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Runner abstracts shell command execution to support tests and dry-run flows.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// LiveRunner streams command output line-by-line while command is running.
type LiveRunner interface {
	RunLive(
		ctx context.Context,
		name string,
		args []string,
		onLine func(line string, isStderr bool),
	) (string, error)
}

// ExecRunner executes commands using os/exec.
type ExecRunner struct {
	DryRun bool
}

// Run executes a command and returns combined output.
func (r ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	return r.RunLive(ctx, name, args, nil)
}

// RunLive executes a command and streams output while preserving combined output.
func (r ExecRunner) RunLive(
	ctx context.Context,
	name string,
	args []string,
	onLine func(line string, isStderr bool),
) (string, error) {
	if r.DryRun {
		out := fmt.Sprintf("dry-run: %s %s", name, strings.Join(args, " "))
		if onLine != nil {
			onLine(out, false)
		}
		return out, nil
	}
	// Command name and args are provided by installer-owned call sites.
	//nolint:gosec // G204
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("exec %s %s: %w", name, strings.Join(args, " "), err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("exec %s %s: %w", name, strings.Join(args, " "), err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("exec %s %s: %w", name, strings.Join(args, " "), err)
	}

	var (
		mu      sync.Mutex
		joined  bytes.Buffer
		readErr error
	)
	appendLine := func(line string) {
		mu.Lock()
		if joined.Len() > 0 {
			joined.WriteByte('\n')
		}
		joined.WriteString(line)
		mu.Unlock()
	}
	setReadErr := func(err error) {
		if err == nil || isIgnorablePipeReadErr(err) {
			return
		}
		mu.Lock()
		if readErr == nil {
			readErr = err
		}
		mu.Unlock()
	}
	stream := func(reader io.Reader, isStderr bool) {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			appendLine(line)
			if onLine != nil {
				onLine(line, isStderr)
			}
		}
		setReadErr(scanner.Err())
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		stream(stdout, false)
	}()
	go func() {
		defer wg.Done()
		stream(stderr, true)
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	mu.Lock()
	out := joined.String()
	scanErr := readErr
	mu.Unlock()
	if waitErr == nil && scanErr != nil {
		waitErr = scanErr
	}
	if waitErr != nil {
		return out, fmt.Errorf("exec %s %s: %w (%s)", name, strings.Join(args, " "), waitErr, strings.TrimSpace(out))
	}
	return out, nil
}

func isIgnorablePipeReadErr(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrClosed) {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "file already closed") || strings.Contains(msg, "use of closed file")
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
