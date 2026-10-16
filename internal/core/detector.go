// Package core provides local port inspection and process diagnostics.
// It depends only on the standard library and gopsutil, keeping the business
// logic reusable by the CLI and MCP layers without depending on either.
package core

import (
	"context"
	"fmt"
	"net"
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

	tcpListenStatus = "LISTEN"
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
	Port           int          `json:"port"`
	IsOpen         bool         `json:"is_open"`
	Protocol       string       `json:"protocol"`          // "tcp"
	PID            int32        `json:"pid"`               // Owning process ID, or 0 when unavailable.
	ProcessName    string       `json:"process_name"`      // Process name, such as "node", "python", or "go".
	Cmdline        string       `json:"cmdline"`           // Process startup command line.
	MemoryMB       float64      `json:"memory_mb"`         // Resident memory in megabytes.
	CPUPercent     float64      `json:"cpu_percent"`       // Process CPU usage percentage.
	Status         string       `json:"status"`            // "occupied", "free", or "permission_required".
	Project        *ProjectInfo `json:"project,omitempty"` // Project fingerprint metadata.
	processCreated int64        // Internal process creation timestamp used for identity checks.
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
func detectProjectSafe(ctx context.Context, pid int32) (info *ProjectInfo) {
	projectDetectorMu.RLock()
	fn := projectDetector
	projectDetectorMu.RUnlock()
	if fn == nil || pid <= 1 {
		return nil
	}
	defer func() {
		if recover() != nil {
			info = nil
		}
	}()
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
	return checkTCPOpenContext(context.Background(), port, timeout)
}

func checkTCPOpenContext(ctx context.Context, port int, timeout time.Duration) bool {
	targets := []string{
		fmt.Sprintf("127.0.0.1:%d", port),
		fmt.Sprintf("[::1]:%d", port),
	}
	dialer := net.Dialer{Timeout: timeout}
	for _, target := range targets {
		conn, err := dialer.DialContext(ctx, "tcp", target)
		if err == nil {
			_ = conn.Close()
			return true
		}
		if ctx.Err() != nil {
			return false
		}
	}
	return false
}

// buildProcIndex scans the system connection table once and builds a port-to-PID index.
// Callers share the resulting read-only index across process-detail lookups.
func buildProcIndex(ctx context.Context) (map[connKey]int32, error) {
	conns, err := gnet.ConnectionsWithContext(ctx, "tcp")
	if err != nil {
		return nil, err
	}
	index := make(map[connKey]int32, len(conns))
	for _, c := range conns {
		if c.Pid <= 0 || c.Laddr.Port == 0 || !isTCPListener(c.Status) {
			continue
		}
		key := connKey{port: c.Laddr.Port, sockTyp: c.Type}
		if current, exists := index[key]; !exists || c.Pid < current {
			// Multiple listeners can share a port on some systems. Pick a stable
			// owner instead of allowing connection-table order to decide.
			index[key] = c.Pid
		}
	}
	return index, nil
}

func isTCPListener(status string) bool {
	return strings.EqualFold(status, tcpListenStatus)
}

// fillProcessDetail collects the process name, command line, resident memory, and CPU usage.
func fillProcessDetail(ctx context.Context, diag *PortDiagnostic) {
	proc, err := process.NewProcessWithContext(ctx, diag.PID)
	if err != nil {
		diag.Status = StatusPermissionRequired
		return
	}
	if created, err := proc.CreateTimeWithContext(ctx); err == nil {
		diag.processCreated = created
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

// probePort checks the observable TCP state without consulting the process table.
func probePort(ctx context.Context, port int) *PortDiagnostic {
	diag := &PortDiagnostic{
		Port:     port,
		Protocol: ProtocolTCP,
		Status:   StatusFree,
	}

	if !checkTCPOpenContext(ctx, port, DefaultDialTimeout) {
		diag.IsOpen = false
		diag.Status = StatusFree
		return diag
	}

	diag.IsOpen = true
	return diag
}

// enrichProcessDetail associates an open port with a process from a shared index.
func enrichProcessDetail(ctx context.Context, diag *PortDiagnostic, index map[connKey]int32) {
	if !diag.IsOpen {
		return
	}
	if index == nil {
		built, err := buildProcIndex(ctx)
		if err != nil {
			diag.Status = StatusPermissionRequired
			return
		}
		index = built
	}

	pid, found := index[connKey{port: uint32(diag.Port), sockTyp: sockStream}]
	if !found || pid <= 0 {
		diag.Status = StatusPermissionRequired
		return
	}

	diag.PID = pid
	fillProcessDetail(ctx, diag)
	if diag.ProcessName != "" {
		diag.Status = StatusOccupied
	} else {
		diag.Status = StatusPermissionRequired
	}
}

// inspectPortInternal inspects one port and can reuse a prebuilt process index.
func inspectPortInternal(ctx context.Context, port int, index map[connKey]int32) *PortDiagnostic {
	diag := probePort(ctx, port)
	enrichProcessDetail(ctx, diag, index)
	return diag
}

// ============================ Public Core API ============================

// InspectPort checks whether a port is open and associates it with process diagnostics.
func InspectPort(ctx context.Context, port int) (*PortDiagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validatePort(port); err != nil {
		return nil, err
	}

	diag := inspectPortInternal(ctx, port, nil)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return diag, nil
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(ports) == 0 {
		return []PortDiagnostic{}, nil
	}
	for _, port := range ports {
		if err := validatePort(port); err != nil {
			return []PortDiagnostic{}, err
		}
	}

	// Cap the worker pool to the number of ports being scanned.
	workers := len(ports)
	if workers > DefaultWorkers {
		workers = DefaultWorkers
	}
	if workers <= 0 {
		workers = 1
	}

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
				// Probe first so newly-created listeners have time to appear in
				// the system connection table before the shared index is built.
				diag := probePort(ctx, port)
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
	occupied := make([]PortDiagnostic, 0)
	for res := range resultsChan {
		occupied = append(occupied, res)
	}

	// Build the process index once after probing. This ordering avoids a
	// cross-platform race where a fresh listener is reachable by TCP but is not
	// visible in the OS connection table yet.
	if len(occupied) > 0 {
		index, indexErr := buildProcIndex(ctx)
		if indexErr != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			index = make(map[connKey]int32)
		}
		for i := range occupied {
			enrichProcessDetail(ctx, &occupied[i], index)
		}
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
