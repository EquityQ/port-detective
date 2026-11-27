package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

var (
	// ErrNoFreePort reports that no loopback TCP port in the requested range could be bound.
	ErrNoFreePort = errors.New("no bindable loopback TCP port found")
	// ErrWaitTimeout reports that waiting for a port state exceeded the requested deadline.
	ErrWaitTimeout = errors.New("timed out waiting for port state")
)

// FreePortResult describes a bindability snapshot returned by FindFreePort.
type FreePortResult struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	IPv4     bool   `json:"ipv4"`
	IPv6     bool   `json:"ipv6"`
	Reserved bool   `json:"reserved"`
	Message  string `json:"message"`
}

// DoctorReport describes binary, project-root, and local listener visibility diagnostics.
type DoctorReport struct {
	Version            string          `json:"version"`
	OS                 string          `json:"os"`
	Arch               string          `json:"arch"`
	BinaryPath         string          `json:"binary_path"`
	Cwd                string          `json:"cwd"`
	ProjectPath        string          `json:"project_path,omitempty"`
	Status             string          `json:"status"`
	SelfCheck          *PortDiagnostic `json:"self_check,omitempty"`
	MetadataAccessible bool            `json:"metadata_accessible"`
	Hints              []string        `json:"hints"`
}

// ResolvePresetPorts resolves an explicit scan preset ("web", "python", "database",
// or "project") to a sorted, deduplicated port slice and human-readable scope label.
func ResolvePresetPorts(preset, projectPath string) ([]int, string, error) {
	switch preset {
	case "web":
		return []int{3000, 3001, 4173, 5173, 8000, 8080}, "preset web", nil
	case "python":
		return []int{5000, 5001, 8000, 8080, 8888}, "preset python", nil
	case "database":
		return []int{3306, 5432, 6379, 27017}, "preset database", nil
	case "project":
		root, err := ResolveProjectPath(projectPath)
		if err != nil {
			return nil, "", err
		}
		ports, err := ReadProjectPresetPorts(root)
		return ports, filepath.Join(root, ".port-detective.json"), err
	default:
		return nil, "", fmt.Errorf("preset must be web, python, database, or project")
	}
}

// ReadProjectPresetPorts reads and validates .port-detective.json at root.
func ReadProjectPresetPorts(root string) ([]int, error) {
	path := filepath.Join(root, ".port-detective.json")
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read project preset %s: %w; expected {\"ports\":\"3000-3005,8081\"}", path, err)
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || info.Size() > 65536 {
		return nil, fmt.Errorf("project preset %s cannot be read or exceeds 64 KiB", path)
	}
	var config struct {
		Ports string `json:"ports"`
	}
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("invalid project preset %s: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("project preset %s must contain one JSON object", path)
	}
	ports, err := ParsePortsList(config.Ports)
	if err != nil {
		return nil, fmt.Errorf("invalid ports in %s: %w", path, err)
	}
	if len(ports) == 0 {
		return nil, fmt.Errorf("no valid ports were provided in %s", path)
	}
	return ports, nil
}

// FindFreePort searches [from, to] for the first TCP port bindable on IPv4 and
// supported IPv6 loopback interfaces. The test listener is closed before returning,
// so the result is a snapshot rather than a reservation.
func FindFreePort(ctx context.Context, from, to int) (*FreePortResult, error) {
	if from < 1 || to > 65535 || from > to {
		return nil, fmt.Errorf("expected 1 <= from <= to <= 65535")
	}
	for port := from; port <= to; port++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		available, ipv6, err := bindablePort(ctx, port)
		if err != nil {
			return nil, err
		}
		if !available {
			continue
		}
		return &FreePortResult{
			Port:     port,
			Protocol: ProtocolTCP,
			IPv4:     true,
			IPv6:     ipv6,
			Reserved: false,
			Message:  "Loopback bind checks passed. Port is not reserved; another process may claim it.",
		}, nil
	}
	return nil, fmt.Errorf("%w in %d-%d", ErrNoFreePort, from, to)
}

func bindablePort(ctx context.Context, port int) (bool, bool, error) {
	var config net.ListenConfig
	return CheckPortBindings(ctx, port, config.Listen)
}

// CheckPortBindings probes IPv4 and IPv6 loopback bindability with the supplied listen function.
func CheckPortBindings(ctx context.Context, port int, listen func(context.Context, string, string) (net.Listener, error)) (bool, bool, error) {
	listener4, err := listen(ctx, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		if ctx.Err() != nil {
			return false, false, ctx.Err()
		}
		if socketError(err, syscall.EADDRINUSE, 10048) || socketError(err, syscall.EACCES, 10013) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("IPv4 loopback bind check failed: %w", err)
	}
	defer listener4.Close()
	listener6, err := listen(ctx, "tcp6", net.JoinHostPort("::1", strconv.Itoa(port)))
	if err != nil {
		if ctx.Err() != nil {
			return false, false, ctx.Err()
		}
		if socketError(err, syscall.EAFNOSUPPORT, 10047) || socketError(err, syscall.EPROTONOSUPPORT, 10043) || socketError(err, syscall.EADDRNOTAVAIL, 10049) {
			return true, false, nil
		}
		if socketError(err, syscall.EADDRINUSE, 10048) || socketError(err, syscall.EACCES, 10013) {
			return false, true, nil
		}
		return false, false, fmt.Errorf("IPv6 loopback bind check failed: %w", err)
	}
	defer listener6.Close()
	return true, true, nil
}

// Winsock uses distinct error numbers from the syscall package's POSIX values.
func socketError(err error, posix syscall.Errno, winsock syscall.Errno) bool {
	return errors.Is(err, posix) || (runtime.GOOS == "windows" && errors.Is(err, winsock))
}

// WaitForPort polls a TCP port until its state is "open" or "free" or timeout expires.
func WaitForPort(ctx context.Context, port int, state string, timeout, interval time.Duration) (*PortDiagnostic, error) {
	return WaitForPortProtocol(ctx, port, ProtocolTCP, state, timeout, interval)
}

// WaitForPortProtocol polls a TCP or UDP port until its state is "open" or "free"
// or timeout expires. A degraded result never satisfies "free".
func WaitForPortProtocol(ctx context.Context, port int, protocol, state string, timeout, interval time.Duration) (*PortDiagnostic, error) {
	return waitForPortWithDetector(ctx, port, protocol, state, timeout, interval, currentProjectDetector())
}

func waitForPortWithDetector(ctx context.Context, port int, protocol, state string, timeout, interval time.Duration, detector ProjectDetectorFunc) (*PortDiagnostic, error) {
	if err := validatePort(port); err != nil {
		return nil, err
	}
	proto, err := NormalizeProtocol(protocol)
	if err != nil {
		return nil, err
	}
	if state != "open" && state != "free" {
		return nil, fmt.Errorf("state must be open or free")
	}
	if timeout <= 0 || interval <= 0 {
		return nil, fmt.Errorf("timeout and interval must be greater than zero")
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		diag, err := inspectPortWithDetector(waitCtx, port, proto, detector)
		if err != nil {
			return nil, mapWaitError(ctx, waitCtx, port, state, timeout, err)
		}
		matched := diag.IsOpen
		if state == "free" {
			// Never treat an unreadable listener table as proof that a port is free.
			matched = !diag.IsOpen && !diag.ListenerTableMissing && diag.Status != StatusUndetermined
		}
		if matched {
			return diag, nil
		}
		if err := pauseWait(waitCtx, interval); err != nil {
			return nil, mapWaitError(ctx, waitCtx, port, state, timeout, err)
		}
	}
}

func pauseWait(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func mapWaitError(parent, waitCtx context.Context, port int, state string, timeout time.Duration, err error) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: timed out after %s waiting for port %d to become %s", ErrWaitTimeout, timeout, port, state)
	}
	return err
}

// RunDoctor collects runtime details and runs a temporary local TCP listener
// self-check to surface host permission or namespace limitations.
func RunDoctor(ctx context.Context, binaryPath, cwd, buildVersion string) (*DoctorReport, error) {
	return runDoctorWithDetector(ctx, binaryPath, cwd, buildVersion, currentProjectDetector())
}

func runDoctorWithDetector(ctx context.Context, binaryPath, cwd, buildVersion string, detector ProjectDetectorFunc) (*DoctorReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	report := &DoctorReport{
		Version:    buildVersion,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		BinaryPath: binaryPath,
		Cwd:        cwd,
		Status:     "ok",
		Hints:      []string{},
	}
	var err error
	if report.BinaryPath == "" {
		report.BinaryPath, err = os.Executable()
		if err != nil {
			report.Hints = append(report.Hints, "Cannot resolve the running executable: "+err.Error())
		}
	}
	if report.Cwd == "" {
		report.Cwd, err = os.Getwd()
		if err != nil {
			report.Hints = append(report.Hints, "Cannot read the working directory: "+err.Error())
		}
	}
	report.ProjectPath, err = ResolveProjectPath(report.Cwd)
	if err != nil {
		report.Hints = append(report.Hints, "Select a valid project using clean --project <directory>: "+err.Error())
	}
	listener, listenErr := net.Listen("tcp4", "127.0.0.1:0")
	if listenErr != nil {
		report.Hints = append(report.Hints, "Unable to create a temporary loopback listener; check local firewall or sandbox settings: "+listenErr.Error())
	} else {
		defer listener.Close()
		port := listener.Addr().(*net.TCPAddr).Port
		report.SelfCheck, err = inspectPortWithDetector(ctx, port, ProtocolTCP, detector)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			report.Hints = append(report.Hints, "Temporary listener inspection failed: "+err.Error())
		} else if report.SelfCheck == nil || !report.SelfCheck.IsOpen {
			report.Hints = append(report.Hints, "A local listener was not detected. Run the CLI in the same host and network namespace as the application; check WSL, containers, or sandbox restrictions.")
		} else if report.SelfCheck.ListenerTableMissing {
			report.Hints = append(report.Hints, "The local TCP listener table could not be read. Ports bound only to other interfaces may be missed; results are incomplete.")
		} else {
			report.MetadataAccessible = report.SelfCheck.PID == int32(os.Getpid()) && report.SelfCheck.Status == StatusOccupied
			if !report.MetadataAccessible {
				report.Hints = append(report.Hints, "TCP visibility works, but process ownership is restricted. Inspect the target with check <port> --json; consider Administrator/root only when you understand the target and need its metadata.")
			}
		}
	}
	if report.SelfCheck != nil {
		report.SelfCheck.Cmdline = ""
		report.SelfCheck.Project = nil
	}
	if len(report.Hints) > 0 {
		report.Status = "degraded"
	}
	return report, nil
}
