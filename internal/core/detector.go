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

	// StatusUndetermined reports a probe whose result cannot be trusted because
	// the local listener table could not be read. A closed loopback probe alone
	// never proves a port is free: a process bound to another interface stays
	// invisible to it, so callers must not treat this as a safe-to-use port.
	StatusUndetermined = "undetermined"

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
	Port        int          `json:"port"`
	IsOpen      bool         `json:"is_open"`
	Protocol    string       `json:"protocol"`          // "tcp"
	PID         int32        `json:"pid"`               // Owning process ID, or 0 when unavailable.
	ProcessName string       `json:"process_name"`      // Process name, such as "node", "python", or "go".
	Cmdline     string       `json:"cmdline"`           // Process startup command line.
	MemoryMB    float64      `json:"memory_mb"`         // Resident memory in megabytes.
	CPUPercent  float64      `json:"cpu_percent"`       // Process CPU usage percentage.
	Status      string       `json:"status"`            // "occupied", "free", "permission_required", or "undetermined".
	Project     *ProjectInfo `json:"project,omitempty"` // Project fingerprint metadata.

	// ListenerTableMissing marks a result whose listener table could not be read.
	// The loopback probe may still have seen an open port, but a closed probe is
	// inconclusive, so this diagnostic must not be presented as proof of a free
	// port. Consumers should surface it instead of promising "ready for use".
	ListenerTableMissing bool `json:"listener_table_missing,omitempty"`

	// PostActionStatus records the verified outcome of a destructive action that
	// already ran against this snapshot: "released", "release_unverified", or
	// "not_attempted". The rest of the struct stays a pre-action observation.
	PostActionStatus string `json:"post_action_status,omitempty"`

	processCreated int64 // Internal process creation timestamp used for identity checks.
	ambiguousOwner bool  // Multiple or unknown owners share the local port.
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
	conns, err := listTCPConnections(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return indexTCPListeners(conns), nil
}

// listTCPConnections reads the local TCP connection table. It is a package
// variable so tests can inject a controlled failure without changing behavior.
var listTCPConnections = func(ctx context.Context) ([]gnet.ConnectionStat, error) {
	return gnet.ConnectionsWithContext(ctx, "tcp")
}

// SetConnectionsListerForTesting replaces the connection-table reader and
// returns a restore function. It exists so the degraded path can be tested
// deterministically instead of hoping the host hides its listener table.
func SetConnectionsListerForTesting(fn func(context.Context) ([]gnet.ConnectionStat, error)) func() {
	previous := listTCPConnections
	if fn != nil {
		listTCPConnections = fn
	}
	return func() { listTCPConnections = previous }
}

// indexTCPListeners keeps every binding, including listeners whose PID is hidden.
// A negative owner marks an ambiguous port; terminating an arbitrary owner is unsafe.
func indexTCPListeners(conns []gnet.ConnectionStat) map[connKey]int32 {
	index := make(map[connKey]int32, len(conns))
	for _, c := range conns {
		if c.Laddr.Port == 0 || c.Type != sockStream || !isTCPListener(c.Status) {
			continue
		}
		key := connKey{port: c.Laddr.Port, sockTyp: c.Type}
		pid := c.Pid
		if pid < 0 {
			pid = 0
		}
		if current, exists := index[key]; !exists {
			index[key] = pid
		} else if current != pid {
			index[key] = -1
		}
	}
	return index
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

	if name, err := proc.NameWithContext(ctx); err == nil {
		diag.ProcessName = name
	}
	if cmdline, err := proc.CmdlineWithContext(ctx); err == nil {
		diag.Cmdline = cmdline
	}
	if mem, err := proc.MemoryInfoWithContext(ctx); err == nil && mem != nil {
		// Convert bytes to megabytes and retain two decimal places.
		diag.MemoryMB = float64(mem.RSS) / (1024 * 1024)
	}
	if cpu, err := proc.CPUPercentWithContext(ctx); err == nil {
		diag.CPUPercent = cpu
	}
	if diag.PID > 1 {
		if info := detectProjectSafe(ctx, diag.PID); info != nil {
			copy := *info
			diag.Project = &copy
		}
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
	pid, found := index[connKey{port: uint32(diag.Port), sockTyp: sockStream}]
	if !found || pid <= 0 {
		diag.ambiguousOwner = pid < 0
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
	if index == nil {
		diag := probePort(ctx, port)
		if diag.IsOpen {
			diag.Status = StatusPermissionRequired
		}
		return diag
	}
	_, listening := index[connKey{port: uint32(port), sockTyp: sockStream}]
	diag := &PortDiagnostic{Port: port, Protocol: ProtocolTCP, IsOpen: listening, Status: StatusFree}
	enrichProcessDetail(ctx, diag, index)
	return diag
}

// inspectPortWithoutListenerTable inspects a port when the local listener table
// could not be read. Such a result is never reported as a free port: a closed
// loopback probe cannot see a process bound only to another interface, so the
// only honest answer is "undetermined". When the probe does connect, the port is
// definitely occupied but its owner is unknown.
func inspectPortWithoutListenerTable(ctx context.Context, port int) *PortDiagnostic {
	diag := probePort(ctx, port)
	diag.ListenerTableMissing = true
	if diag.IsOpen {
		diag.Status = StatusPermissionRequired
	} else {
		diag.Status = StatusUndetermined
	}
	return diag
}

// ============================ Public Core API ============================

// InspectPort checks whether a port is open and associates it with process diagnostics.
//
// When the local listener table cannot be read, InspectPort degrades instead of
// guessing: the result is marked with ListenerTableMissing and a closed loopback
// probe is reported as StatusUndetermined rather than StatusFree, because a
// process bound only to another interface stays invisible to that probe.
func InspectPort(ctx context.Context, port int) (*PortDiagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validatePort(port); err != nil {
		return nil, err
	}

	index, err := buildProcIndex(ctx)
	var diag *PortDiagnostic
	if err != nil {
		// Context cancellation is a real cancellation, not a degraded host.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		diag = inspectPortWithoutListenerTable(ctx, port)
	} else {
		diag = inspectPortInternal(ctx, port, index)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return diag, nil
}

// ScanSummary reports a concurrent scan plus whether the result is complete.
// Diagnostics keeps the historical JSON array shape when serialized alone, so
// existing consumers of ScanPortsConcurrent are unaffected.
type ScanSummary struct {
	Diagnostics []PortDiagnostic `json:"diagnostics"`
	Degraded    bool             `json:"degraded"`
	Warning     string           `json:"warning,omitempty"`
}

// ScanPortsConcurrent probes multiple ports concurrently and returns results in ascending order.
func ScanPortsConcurrent(ctx context.Context, ports []int) ([]PortDiagnostic, error) {
	summary, err := ScanPortsConcurrentSummary(ctx, ports)
	return summary.Diagnostics, err
}

// ScanPortsConcurrentSummary scans ports and additionally reports whether the
// result is complete. A scan whose listener table could not be read cannot rule
// out a listener bound only to another interface; callers must not present such
// a result as proof that the scanned ports are free.
func ScanPortsConcurrentSummary(ctx context.Context, ports []int) (ScanSummary, error) {
	occupied, degraded, err := scanPortsConcurrentInternal(ctx, ports, nil, false)
	summary := ScanSummary{Diagnostics: occupied, Degraded: degraded}
	if degraded {
		summary.Warning = listenerTableWarning
	}
	return summary, err
}

// listenerTableWarning explains a degraded scan in user-facing terms.
const listenerTableWarning = "the local TCP listener table could not be read, so ports with no loopback listener are undetermined rather than free; " +
	"listeners bound only to other interfaces may be missing from this result"

// scanPortsConcurrent optionally reuses a listener snapshot for project discovery.
//
// The worker pool uses a producer, a jobs channel, workers, and a results channel.
//
//	┌────────────────┐        ┌── worker 1 ──┐        ┌──────────────────┐
//	│ ports channel  ├───────>│  worker 2    ├───────>│ results channel  │
//	│ (buffered jobs)│        └── worker N ──┘        │ (open results)   │
//	└────────────────┘               ▲                └─────────┬────────┘
//	                                 │                          │
//	              sync.WaitGroup closes the channel after workers exit; results are sorted.
func scanPortsConcurrent(ctx context.Context, ports []int, index map[connKey]int32, indexReady bool) ([]PortDiagnostic, error) {
	occupied, _, err := scanPortsConcurrentInternal(ctx, ports, index, indexReady)
	return occupied, err
}

// scanPortsConcurrentInternal optionally reuses a listener snapshot for project
// discovery and reports whether the listener table was unavailable.
func scanPortsConcurrentInternal(ctx context.Context, ports []int, index map[connKey]int32, indexReady bool) ([]PortDiagnostic, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if len(ports) == 0 {
		return []PortDiagnostic{}, false, nil
	}
	unique, err := normalizePorts(ports)
	if err != nil {
		return []PortDiagnostic{}, false, err
	}
	ports = unique
	degraded := false
	if !indexReady {
		built, buildErr := buildProcIndex(ctx)
		if buildErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, false, ctxErr
			}
			// Keep probing: an open port is still a real finding even without the
			// listener table, but a closed one cannot be called free.
			degraded = true
		} else {
			index = built
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	inspect := inspectPortInternal
	if degraded {
		inspect = func(ctx context.Context, port int, _ map[connKey]int32) *PortDiagnostic {
			return inspectPortWithoutListenerTable(ctx, port)
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

	// 生产者通过 channel 分发端口；取消后停止投递并关闭任务通道。
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

	// 有界 goroutine 工作池共享只读监听快照，避免每个端口重复读取系统连接表。
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for port := range portsChan {
				if ctx.Err() != nil {
					return
				}
				diag := inspect(ctx, port, index)
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

	// WaitGroup 确保所有生产者退出后才关闭结果 channel，主协程汇总并排序。
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect open-port results from the workers.
	occupied := make([]PortDiagnostic, 0)
	for res := range resultsChan {
		occupied = append(occupied, res)
	}

	// Return deterministic results sorted by port number.
	sort.Slice(occupied, func(i, j int) bool {
		return occupied[i].Port < occupied[j].Port
	})

	if err := ctx.Err(); err != nil {
		return occupied, degraded, fmt.Errorf("scan canceled: %w", err)
	}
	return occupied, degraded, nil
}
