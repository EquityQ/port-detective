// Package core provides local port inspection and process diagnostics.
// It depends only on the standard library and gopsutil, keeping the business
// logic reusable by the CLI and MCP layers without depending on either.
package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	gnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// ============================ Constants and Types ============================

const (
	// ProtocolTCP identifies the TCP protocol.
	ProtocolTCP = "tcp"

	// Diagnostic status values.
	StatusOccupied           = "occupied"
	StatusFree               = "free"
	StatusPermissionRequired = "permission_required"

	// DefaultDialTimeout is the timeout for a single TCP probe.
	DefaultDialTimeout = 300 * time.Millisecond

	// DefaultWorkers is the default worker count for concurrent scans.
	DefaultWorkers = 64
)

// DefaultDevPorts lists ten commonly used development ports.
var DefaultDevPorts = []int{3000, 3306, 5173, 5432, 6379, 8000, 8080, 8888, 9000, 27017}

// ProjectInfo contains project fingerprint metadata for a port-owning process.
type ProjectInfo struct {
	Name      string `json:"name,omitempty"`
	Framework string `json:"framework,omitempty"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
}

// PortDiagnostic contains a diagnostic report for a port and its owning process.
type PortDiagnostic struct {
	Port        int          `json:"port"`
	IsOpen      bool         `json:"is_open"`
	Protocol    string       `json:"protocol"`          // "tcp"
	PID         int32        `json:"pid"`               // Owning process ID, or 0 when unavailable.
	ProcessName string       `json:"process_name"`      // Process name, such as "node", "python", or "go".
	Cmdline     string       `json:"cmdline"`           // Process startup command line.
	MemoryMB    float64      `json:"memory_mb"`         // Resident memory in megabytes.
	CPUPercent  float64      `json:"cpu_percent"`       // Process CPU usage percentage.
	Status      string       `json:"status"`            // "occupied", "free", or "permission_required".
	Project     *ProjectInfo `json:"project,omitempty"` // Project fingerprint metadata.
}

// ProjectDetectorFunc defines the process fingerprint detector signature.
type ProjectDetectorFunc func(ctx context.Context, pid int32) *ProjectInfo

var (
	projectDetectorMu sync.RWMutex
	projectDetector   ProjectDetectorFunc
)

// RegisterProjectDetector registers an external project fingerprint detector.
func RegisterProjectDetector(fn ProjectDetectorFunc) {
	projectDetectorMu.Lock()
	defer projectDetectorMu.Unlock()
	projectDetector = fn
}

// detectProjectSafe invokes the registered project fingerprint detector safely.
func detectProjectSafe(ctx context.Context, pid int32) *ProjectInfo {
	projectDetectorMu.RLock()
	fn := projectDetector
	projectDetectorMu.RUnlock()
	if fn == nil || pid <= 1 {
		return nil
	}
	return fn(ctx, pid)
}

// PortInfo is retained as a backward-compatible type alias.
type PortInfo = PortDiagnostic

const sockStream = syscall.SOCK_STREAM

// connKey combines a local port and socket type.
type connKey struct {
	port    uint32
	sockTyp uint32
}

// ============================ Probe Primitives ============================

// checkTCPOpen uses a TCP handshake to determine whether a process is listening.
// It probes both loopback addresses to cover IPv4 and IPv6 listeners.
func checkTCPOpen(port int, timeout time.Duration) bool {
	targets := []string{
		fmt.Sprintf("127.0.0.1:%d", port),
		fmt.Sprintf("[::1]:%d", port),
	}
	for _, target := range targets {
		conn, err := net.DialTimeout("tcp", target, timeout)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

// buildProcIndex scans the system connection table once and builds a port-to-PID index.
// Workers share this read-only index to avoid repeated system calls across platforms.
func buildProcIndex(ctx context.Context) (map[connKey]int32, error) {
	conns, err := gnet.ConnectionsWithContext(ctx, "tcp")
	if err != nil {
		return nil, err
	}
	index := make(map[connKey]int32, len(conns))
	for _, c := range conns {
		if c.Pid <= 0 || c.Laddr.Port == 0 {
			continue
		}
		index[connKey{port: c.Laddr.Port, sockTyp: c.Type}] = c.Pid
	}
	return index, nil
}

// fillProcessDetail collects the process name, command line, resident memory, and CPU usage.
func fillProcessDetail(ctx context.Context, diag *PortDiagnostic) {
	proc, err := process.NewProcessWithContext(ctx, diag.PID)
	if err != nil {
		diag.Status = StatusPermissionRequired
		return
	}

	if name, err := proc.Name(); err == nil {
		diag.ProcessName = name
	}
	if cmdline, err := proc.Cmdline(); err == nil {
		diag.Cmdline = cmdline
	}
	if mem, err := proc.MemoryInfo(); err == nil && mem != nil {
		// Convert bytes to megabytes and retain two decimal places.
		diag.MemoryMB = float64(mem.RSS) / (1024 * 1024)
	}
	if cpu, err := proc.CPUPercentWithContext(ctx); err == nil {
		diag.CPUPercent = cpu
	}
	if diag.PID > 1 {
		diag.Project = detectProjectSafe(ctx, diag.PID)
	}
}

// inspectPortInternal inspects one port and can reuse a prebuilt process index.
func inspectPortInternal(ctx context.Context, port int, index map[connKey]int32) *PortDiagnostic {
	diag := &PortDiagnostic{
		Port:     port,
		Protocol: ProtocolTCP,
		Status:   StatusFree,
	}

	if !checkTCPOpen(port, DefaultDialTimeout) {
		diag.IsOpen = false
		diag.Status = StatusFree
		return diag
	}

	diag.IsOpen = true
	if index == nil {
		built, err := buildProcIndex(ctx)
		if err != nil {
			diag.Status = StatusPermissionRequired
			return diag
		}
		index = built
	}

	pid, found := index[connKey{port: uint32(port), sockTyp: sockStream}]
	if !found || pid <= 0 {
		diag.Status = StatusPermissionRequired
		return diag
	}

	diag.PID = pid
	fillProcessDetail(ctx, diag)
	if diag.ProcessName != "" {
		diag.Status = StatusOccupied
	} else {
		diag.Status = StatusPermissionRequired
	}
	return diag
}

// ============================ Public Core API ============================

// InspectPort checks whether a port is open and associates it with process diagnostics.
func InspectPort(ctx context.Context, port int) (*PortDiagnostic, error) {
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid port %d: valid range is 1-65535", port)
	}
	return inspectPortInternal(ctx, port, nil), nil
}

// ScanPortsConcurrent probes multiple ports concurrently and returns results in ascending order.
//
// The worker pool uses a producer, a jobs channel, workers, and a results channel.
//
//	┌────────────────┐        ┌── worker 1 ──┐        ┌──────────────────┐
//	│ ports channel  ├───────>│  worker 2    ├───────>│ results channel  │
//	│ (buffered jobs)│        └── worker N ──┘        │ (open results)   │
//	└────────────────┘               ▲                └─────────┬────────┘
//	                                 │                          │
//	              sync.WaitGroup closes the channel after workers exit; results are sorted.
func ScanPortsConcurrent(ctx context.Context, ports []int) ([]PortDiagnostic, error) {
	if len(ports) == 0 {
		return []PortDiagnostic{}, nil
	}

	// Cap the worker pool to the number of ports being scanned.
	workers := len(ports)
	if workers > DefaultWorkers {
		workers = DefaultWorkers
	}
	if workers <= 0 {
		workers = 1
	}

	// Build the process index once and share it read-only across all workers.
	index, _ := buildProcIndex(ctx)

	var wg sync.WaitGroup
	portsChan := make(chan int, workers)
	resultsChan := make(chan PortDiagnostic, workers)

	// The producer submits ports and closes the jobs channel when done.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(portsChan)
		for _, port := range ports {
			select {
			case <-ctx.Done():
				return // Exit promptly when the context is canceled.
			case portsChan <- port:
			}
		}
	}()

	// Start a bounded pool of workers to probe ports concurrently.
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for port := range portsChan {
				if port < 1 || port > 65535 {
					continue
				}
				diag := inspectPortInternal(ctx, port, index)
				// Collect open ports only.
				if diag.IsOpen {
					select {
					case <-ctx.Done():
						return
					case resultsChan <- *diag:
					}
				}
			}
		}()
	}

	// Close the results channel after every worker has finished.
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect open-port results from the workers.
	var occupied []PortDiagnostic
	for res := range resultsChan {
		occupied = append(occupied, res)
	}

	// Return deterministic results sorted by port number.
	sort.Slice(occupied, func(i, j int) bool {
		return occupied[i].Port < occupied[j].Port
	})

	if err := ctx.Err(); err != nil {
		return occupied, fmt.Errorf("扫描被取消：%w", err)
	}
	return occupied, nil
}

// KillPortProcess terminates the process occupying a port, or previews the action
// when dryRun is true. System processes with PID <= 1 are never terminated.
func KillPortProcess(ctx context.Context, port int, dryRun bool) (*PortDiagnostic, string, error) {
	diag, err := InspectPort(ctx, port)
	if err != nil {
		return nil, "", err
	}
	if !diag.IsOpen {
		return diag, "", fmt.Errorf("端口 %d 当前未被占用，无需释放", port)
	}
	// Never terminate protected system processes.
	if diag.PID <= 1 {
		return diag, "", fmt.Errorf("禁止终止系统保护进程 (PID %d)", diag.PID)
	}
	if diag.PID <= 0 || diag.Status == StatusPermissionRequired {
		return diag, "", errors.New("端口已被占用，但当前权限不足以读取或终止目标进程（请以管理员/root 身份运行）")
	}

	// Preview the action without terminating the process.
	if dryRun {
		msg := fmt.Sprintf("预演成功：将终止占用端口 %d 的进程 %q (PID: %d)，当前物理内存占用 %.1f MB",
			port, diag.ProcessName, diag.PID, diag.MemoryMB)
		return diag, msg, nil
	}

	proc, err := process.NewProcessWithContext(ctx, diag.PID)
	if err != nil {
		return diag, "", fmt.Errorf("无法打开目标进程 %d：%w", diag.PID, err)
	}

	// Attempt graceful termination first, then force termination if needed.
	err = proc.Terminate()
	if err != nil {
		err = proc.Kill()
	}
	if err != nil {
		return diag, "", fmt.Errorf("终止进程 %d 失败（可能需要管理员/root 权限）：%w", diag.PID, err)
	}

	msg := fmt.Sprintf("已成功终止占用端口 %d 的进程 %q (PID: %d)", port, diag.ProcessName, diag.PID)
	return diag, msg, nil
}

// CleanCurrentProjectPorts scans common development ports and selects processes
// whose detected project path matches cwd.
func CleanCurrentProjectPorts(ctx context.Context, cwd string, dryRun bool) ([]PortDiagnostic, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("无法获取当前工作目录：%w", err)
		}
	}

	cleanCwd := filepath.Clean(cwd)
	// Scan commonly used development ports concurrently.
	occupied, err := ScanPortsConcurrent(ctx, DefaultDevPorts)
	if err != nil {
		return nil, fmt.Errorf("扫描开发端口失败：%w", err)
	}

	var targets []PortDiagnostic
	for _, diag := range occupied {
		if diag.Project != nil && diag.Project.Path != "" {
			targetPath := filepath.Clean(diag.Project.Path)
			matched := false
			if strings.EqualFold(targetPath, cleanCwd) {
				matched = true
			} else if fi1, err1 := os.Stat(targetPath); err1 == nil {
				if fi2, err2 := os.Stat(cleanCwd); err2 == nil && os.SameFile(fi1, fi2) {
					matched = true
				}
			}

			if matched {
				targets = append(targets, diag)
			}
		}
	}

	if len(targets) == 0 {
		return []PortDiagnostic{}, nil
	}

	var cleaned []PortDiagnostic
	for _, target := range targets {
		diag, _, err := KillPortProcess(ctx, target.Port, dryRun)
		if err != nil {
			target.Status = fmt.Sprintf("kill_failed: %v", err)
			cleaned = append(cleaned, target)
			continue
		}
		if diag != nil {
			cleaned = append(cleaned, *diag)
		} else {
			cleaned = append(cleaned, target)
		}
	}

	return cleaned, nil
}
