// Package core provides process termination and project cleanup helpers.
package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

const (
	terminationGracePeriod  = 2 * time.Second
	terminationPollInterval = 25 * time.Millisecond
)

// processIdentity returns a process handle and its creation time. The creation
// time lets callers detect PID reuse between two independent port inspections.
func processIdentity(ctx context.Context, pid int32) (*process.Process, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	proc, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return nil, 0, err
	}
	created, err := proc.CreateTimeWithContext(ctx)
	if err != nil {
		return nil, 0, err
	}
	return proc, created, nil
}

// KillPortProcess terminates the process occupying a port, or previews the action
// when dryRun is true. System processes with PID <= 1 are never terminated.
func KillPortProcess(ctx context.Context, port int, dryRun bool) (*PortDiagnostic, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	diag, err := InspectPort(ctx, port)
	if err != nil {
		return nil, "", err
	}
	if !diag.IsOpen {
		return diag, "", fmt.Errorf("port %d is currently free; no process to terminate", port)
	}
	// Never terminate protected system processes.
	if diag.PID <= 1 {
		return diag, "", fmt.Errorf("refusing to terminate protected system process (PID %d)", diag.PID)
	}
	if diag.PID <= 0 || diag.Status == StatusPermissionRequired {
		return diag, "", errors.New("port is occupied, but current privileges cannot inspect or terminate the process")
	}

	// Preview the action without terminating the process.
	if dryRun {
		msg := fmt.Sprintf("dry run: would terminate process %q (PID: %d) occupying port %d with %.1f MB RSS",
			diag.ProcessName, diag.PID, port, diag.MemoryMB)
		return diag, msg, nil
	}

	_, created, err := processIdentity(ctx, diag.PID)
	if err != nil {
		return diag, "", fmt.Errorf("unable to confirm identity of process %d: %w", diag.PID, err)
	}

	latest, latestProc, err := verifyTerminationTarget(ctx, port, &PortDiagnostic{
		Port:           diag.Port,
		PID:            diag.PID,
		processCreated: created,
	}, "")
	if err != nil {
		return latest, "", err
	}
	if err := terminateProcess(ctx, latestProc); err != nil {
		return latest, "", fmt.Errorf("failed to terminate process %d: %w", diag.PID, err)
	}
	if err := waitForPortRelease(ctx, port, terminationGracePeriod); err != nil {
		return latest, "", fmt.Errorf("process %d received a termination request, but port %d is still occupied: %w", diag.PID, port, err)
	}

	msg := fmt.Sprintf("terminated process %q (PID: %d) occupying port %d", diag.ProcessName, diag.PID, port)
	return diag, msg, nil
}

func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port %d: valid range is 1-65535", port)
	}
	return nil
}

// verifyTerminationTarget rechecks the observable port owner and process identity
// immediately before a destructive action. projectPath is required for project
// cleanup and prevents a process that moved to another project from being killed.
func verifyTerminationTarget(ctx context.Context, port int, expected *PortDiagnostic, projectPath string) (*PortDiagnostic, *process.Process, error) {
	latest, err := InspectPort(ctx, port)
	if err != nil {
		return expected, nil, fmt.Errorf("pre-termination port %d recheck failed: %w", port, err)
	}
	if !latest.IsOpen {
		return latest, nil, fmt.Errorf("port %d was released before termination", port)
	}
	if latest.PID != expected.PID {
		return latest, nil, fmt.Errorf("port %d ownership changed from PID %d to PID %d; termination canceled", port, expected.PID, latest.PID)
	}
	if expected.processCreated <= 0 || latest.processCreated <= 0 {
		return latest, nil, fmt.Errorf("unable to confirm the process creation time for port %d; termination canceled", port)
	}
	if latest.processCreated != expected.processCreated {
		return latest, nil, fmt.Errorf("PID %d for port %d was reused; termination canceled", latest.PID, port)
	}
	if projectPath != "" {
		if latest.Project == nil || latest.Project.Path == "" || !projectPathsMatch(latest.Project.Path, projectPath) {
			return latest, nil, fmt.Errorf("project path for port %d changed or could not be confirmed; termination canceled", port)
		}
	}
	proc, created, err := processIdentity(ctx, latest.PID)
	if err != nil {
		return latest, nil, fmt.Errorf("unable to confirm target process %d before termination: %w", latest.PID, err)
	}
	if created != expected.processCreated {
		return latest, nil, fmt.Errorf("PID %d for port %d was reused; termination canceled", latest.PID, port)
	}
	if err := ctx.Err(); err != nil {
		return latest, nil, err
	}
	return latest, proc, nil
}

// terminateProcess attempts graceful termination first and escalates to a force
// kill only after the bounded grace period expires.
func terminateProcess(ctx context.Context, proc *process.Process) error {
	err := proc.Terminate()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err = proc.Kill(); err == nil {
			err = waitForProcessExit(ctx, proc, terminationGracePeriod)
		}
		return err
	}
	if err = waitForProcessExit(ctx, proc, terminationGracePeriod); err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err = proc.Kill(); err != nil {
		return err
	}
	return waitForProcessExit(ctx, proc, terminationGracePeriod)
}

// waitForProcessExit confirms that the exact process handle is no longer running.
func waitForProcessExit(ctx context.Context, proc *process.Process, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(terminationPollInterval)
	defer ticker.Stop()

	for {
		running, err := proc.IsRunningWithContext(ctx)
		if err != nil {
			if errors.Is(err, process.ErrorProcessNotRunning) {
				return nil
			}
			return err
		}
		if !running {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("process did not exit within %s", timeout)
		case <-ticker.C:
		}
	}
}

// waitForPortRelease verifies the observable user-facing result after termination.
func waitForPortRelease(ctx context.Context, port int, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(terminationPollInterval)
	defer ticker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !checkTCPOpenContext(ctx, port, DefaultDialTimeout) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("port remained open for %s", timeout)
		case <-ticker.C:
		}
	}
}

// CleanCurrentProjectPorts scans common development ports and selects processes
// whose detected project path matches cwd.
func CleanCurrentProjectPorts(ctx context.Context, cwd string, dryRun bool) ([]PortDiagnostic, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("unable to get the current working directory: %w", err)
		}
	}

	cleanCwd := filepath.Clean(cwd)
	// Scan commonly used development ports concurrently.
	occupied, err := ScanPortsConcurrent(ctx, DefaultDevPorts)
	if err != nil {
		return nil, fmt.Errorf("development port scan failed: %w", err)
	}

	var targets []PortDiagnostic
	for _, diag := range occupied {
		if diag.Project != nil && diag.Project.Path != "" {
			targetPath := filepath.Clean(diag.Project.Path)
			matched := projectPathsMatch(targetPath, cleanCwd)

			if matched {
				targets = append(targets, diag)
			}
		}
	}

	if len(targets) == 0 {
		return []PortDiagnostic{}, nil
	}

	cleaned := make([]PortDiagnostic, 0, len(targets))
	var failures []error
	for _, target := range targets {
		diag, _, err := cleanProjectTarget(ctx, target, cleanCwd, dryRun)
		if err != nil {
			target.Status = fmt.Sprintf("kill_failed: %v", err)
			cleaned = append(cleaned, target)
			failures = append(failures, fmt.Errorf("port %d: %w", target.Port, err))
			continue
		}
		if diag != nil {
			cleaned = append(cleaned, *diag)
		} else {
			cleaned = append(cleaned, target)
		}
	}

	return cleaned, errors.Join(failures...)
}

// cleanProjectTarget validates the original scan result again before a project
// cleanup action. The public KillPortProcess API cannot enforce project scope,
// so cleanup uses this private path with an additional project-path check.
func cleanProjectTarget(ctx context.Context, target PortDiagnostic, cwd string, dryRun bool) (*PortDiagnostic, string, error) {
	if err := ctx.Err(); err != nil {
		return &target, "", err
	}
	if !target.IsOpen {
		return &target, "", fmt.Errorf("port %d is currently free; no process to terminate", target.Port)
	}
	if target.PID <= 1 {
		return &target, "", fmt.Errorf("refusing to terminate protected system process (PID %d)", target.PID)
	}
	if target.Status == StatusPermissionRequired || target.processCreated <= 0 {
		return &target, "", fmt.Errorf("unable to confirm the target process for port %d; cleanup canceled", target.Port)
	}

	latest, proc, err := verifyTerminationTarget(ctx, target.Port, &target, cwd)
	if err != nil {
		return latest, "", err
	}
	if dryRun {
		msg := fmt.Sprintf("dry run: would terminate process %q (PID: %d) occupying port %d",
			latest.ProcessName, latest.PID, target.Port)
		return latest, msg, nil
	}
	if err := terminateProcess(ctx, proc); err != nil {
		return latest, "", fmt.Errorf("failed to terminate process %d: %w", latest.PID, err)
	}
	if err := waitForPortRelease(ctx, target.Port, terminationGracePeriod); err != nil {
		return latest, "", fmt.Errorf("process %d received a termination request, but port %d is still occupied: %w", latest.PID, target.Port, err)
	}
	return latest, fmt.Sprintf("terminated process %q (PID: %d) occupying port %d",
		latest.ProcessName, latest.PID, target.Port), nil
}

// projectPathsMatch applies the filesystem's path semantics before cleanup.
// Windows paths are case-insensitive; Unix paths remain case-sensitive.
func projectPathsMatch(targetPath, currentPath string) bool {
	if runtime.GOOS == "windows" && strings.EqualFold(targetPath, currentPath) {
		return true
	}
	if targetPath == currentPath {
		return true
	}
	fi1, err1 := os.Stat(targetPath)
	fi2, err2 := os.Stat(currentPath)
	return err1 == nil && err2 == nil && os.SameFile(fi1, fi2)
}
