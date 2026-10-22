// Package core provides process termination and project cleanup helpers.
package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
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
	diag, err := InspectPort(ctx, port)
	if err != nil {
		return nil, "", err
	}
	return KillPortTarget(ctx, *diag, dryRun)
}

// KillPortTarget revalidates the exact in-memory inspection snapshot, so a
// confirmation prompt cannot authorize terminating a replacement process.
func KillPortTarget(ctx context.Context, target PortDiagnostic, dryRun bool) (*PortDiagnostic, string, error) {
	return cleanProjectTarget(ctx, target, "", dryRun)
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
	if latest.ambiguousOwner || latest.Status == StatusPermissionRequired {
		return latest, nil, fmt.Errorf("unable to confirm a unique owner for port %d; termination canceled", port)
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
		if !projectDiagnosticMatches(*latest, projectPath) {
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
		index, err := buildProcIndex(ctx)
		if err != nil {
			return fmt.Errorf("cannot verify release across all local TCP bindings: %w", err)
		}
		if _, listening := index[connKey{port: uint32(port), sockTyp: sockStream}]; !listening {
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

// CleanCurrentProjectPorts discovers all local project listeners before cleanup.
func CleanCurrentProjectPorts(ctx context.Context, cwd string, dryRun bool) ([]PortDiagnostic, error) {
	targets, err := DiscoverProjectPorts(ctx, cwd, nil)
	if err != nil {
		return nil, err
	}
	return CleanProjectTargets(ctx, cwd, targets, dryRun)
}

// CleanProjectTargets acts only on the supplied preview snapshot. Processes that
// acquire new listeners after the preview are never added to this operation.
func CleanProjectTargets(ctx context.Context, path string, targets []PortDiagnostic, dryRun bool) ([]PortDiagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	projectPath, err := ResolveProjectPath(path)
	if err != nil {
		return nil, err
	}
	selected := make([]PortDiagnostic, 0, len(targets))
	seen := make(map[int]PortDiagnostic, len(targets))
	for _, target := range targets {
		if previous, exists := seen[target.Port]; exists {
			if previous.PID != target.PID || previous.processCreated != target.processCreated {
				return nil, fmt.Errorf("conflicting preview identities for port %d; cleanup canceled", target.Port)
			}
			continue
		}
		seen[target.Port] = target
		selected = append(selected, target)
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Port < selected[j].Port })
	// 同一进程可能监听多个端口：先复核整组预览身份，再只终止一次，防止把已释放的兄弟端口误报失败。
	type identity struct {
		pid     int32
		created int64
	}
	groups := make(map[identity][]int)
	order := make([]identity, 0)
	for i, target := range selected {
		key := identity{target.PID, target.processCreated}
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], i)
	}
	var failures []error
	for _, key := range order {
		indices := groups[key]
		var groupErr error
		for _, i := range indices {
			target := selected[i]
			if !projectDiagnosticMatches(target, projectPath) {
				groupErr = fmt.Errorf("preview project path for port %d does not match %q; cleanup canceled", target.Port, projectPath)
				break
			}
			if _, _, err := cleanProjectTarget(ctx, target, projectPath, true); err != nil {
				groupErr = err
				break
			}
		}
		if groupErr == nil && !dryRun {
			_, _, groupErr = cleanProjectTarget(ctx, selected[indices[0]], projectPath, false)
			if groupErr == nil {
				for _, i := range indices[1:] {
					if err := waitForPortRelease(ctx, selected[i].Port, terminationGracePeriod); err != nil {
						groupErr = fmt.Errorf("port %d release could not be confirmed: %w", selected[i].Port, err)
						break
					}
				}
			}
		}
		if groupErr != nil {
			for _, i := range indices {
				selected[i].Status = fmt.Sprintf("kill_failed: %v", groupErr)
			}
			failures = append(failures, groupErr)
		}
		if ctx.Err() != nil {
			return selected, errors.Join(append(failures, ctx.Err())...)
		}
	}
	return selected, errors.Join(failures...)
}

// cleanProjectTarget validates the original scan result again before a project
// cleanup action. The public KillPortProcess API cannot enforce project scope,
// so cleanup uses this private path with an additional project-path check.
func cleanProjectTarget(ctx context.Context, target PortDiagnostic, cwd string, dryRun bool) (*PortDiagnostic, string, error) {
	if err := ctx.Err(); err != nil {
		return &target, "", err
	}
	if err := validatePort(target.Port); err != nil {
		return &target, "", err
	}
	if !target.IsOpen {
		return &target, "", fmt.Errorf("port %d is currently free; no process to terminate", target.Port)
	}
	if target.ambiguousOwner {
		return &target, "", fmt.Errorf("port %d has multiple or unknown owners; termination canceled", target.Port)
	}
	if target.PID == 1 || target.PID < 0 {
		return &target, "", fmt.Errorf("refusing to terminate protected system process (PID %d)", target.PID)
	}
	if target.PID == 0 || target.Status == StatusPermissionRequired || target.processCreated <= 0 {
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
