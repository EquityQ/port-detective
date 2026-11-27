package core

import (
	"context"
	"time"
)

// Inspector provides port inspection, concurrent scanning, project discovery,
// cleanup, and workflows with an explicitly injected ProjectDetectorFunc,
// avoiding reliance on package-level global hooks.
type Inspector struct {
	ProjectDetector ProjectDetectorFunc
}

// NewInspector returns an Inspector configured with the supplied project detector.
// Passing nil disables project fingerprinting for calls made through this Inspector.
func NewInspector(detector ProjectDetectorFunc) *Inspector {
	return &Inspector{ProjectDetector: detector}
}

// InspectPort inspects one TCP port using this Inspector's project detector.
func (i *Inspector) InspectPort(ctx context.Context, port int) (*PortDiagnostic, error) {
	return i.InspectPortProtocol(ctx, port, ProtocolTCP)
}

// InspectPortProtocol inspects one TCP or UDP port using this Inspector's project detector.
func (i *Inspector) InspectPortProtocol(ctx context.Context, port int, protocol string) (*PortDiagnostic, error) {
	return inspectPortWithDetector(ctx, port, protocol, i.detector())
}

// ScanPortsConcurrent scans TCP ports concurrently using this Inspector's project detector.
func (i *Inspector) ScanPortsConcurrent(ctx context.Context, ports []int) ([]PortDiagnostic, error) {
	summary, err := i.ScanPortsProtocolSummary(ctx, ports, ProtocolTCP)
	return summary.Diagnostics, err
}

// ScanPortsConcurrentSummary scans TCP ports concurrently and reports completeness.
func (i *Inspector) ScanPortsConcurrentSummary(ctx context.Context, ports []int) (ScanSummary, error) {
	return i.ScanPortsProtocolSummary(ctx, ports, ProtocolTCP)
}

// ScanPortsProtocolSummary scans TCP or UDP ports concurrently using this Inspector's detector.
func (i *Inspector) ScanPortsProtocolSummary(ctx context.Context, ports []int, protocol string) (ScanSummary, error) {
	return scanPortsWithDetector(ctx, ports, protocol, i.detector())
}

// DiscoverProjectPorts discovers local TCP listeners belonging to path using this Inspector's detector.
func (i *Inspector) DiscoverProjectPorts(ctx context.Context, path string, ports []int) ([]PortDiagnostic, error) {
	return discoverProjectPortsWithDetector(ctx, path, ports, i.detector())
}

// CleanCurrentProjectPorts discovers and cleans listeners belonging to cwd using this Inspector's detector.
func (i *Inspector) CleanCurrentProjectPorts(ctx context.Context, cwd string, dryRun bool) ([]PortDiagnostic, error) {
	return cleanCurrentProjectPortsWithDetector(ctx, cwd, dryRun, i.detector())
}

// CleanProjectTargets acts on the supplied preview snapshot using this Inspector's detector.
func (i *Inspector) CleanProjectTargets(ctx context.Context, path string, targets []PortDiagnostic, dryRun bool) ([]PortDiagnostic, error) {
	return cleanProjectTargetsWithDetector(ctx, path, targets, dryRun, i.detector())
}

// KillPortProcess terminates or previews the process on a TCP port using this Inspector's detector.
func (i *Inspector) KillPortProcess(ctx context.Context, port int, dryRun bool) (*PortDiagnostic, string, error) {
	return i.KillPortProcessProtocol(ctx, port, ProtocolTCP, dryRun)
}

// KillPortProcessProtocol terminates or previews the process on a TCP or UDP port using this Inspector's detector.
func (i *Inspector) KillPortProcessProtocol(ctx context.Context, port int, protocol string, dryRun bool) (*PortDiagnostic, string, error) {
	return killPortProcessWithDetector(ctx, port, protocol, dryRun, i.detector())
}

// KillPortTarget revalidates and terminates or previews target using this Inspector's detector.
func (i *Inspector) KillPortTarget(ctx context.Context, target PortDiagnostic, dryRun bool) (*PortDiagnostic, string, error) {
	return killPortTargetWithDetector(ctx, target, dryRun, i.detector())
}

// WaitForPort polls a TCP port until state matches or timeout expires.
func (i *Inspector) WaitForPort(ctx context.Context, port int, state string, timeout, interval time.Duration) (*PortDiagnostic, error) {
	return i.WaitForPortProtocol(ctx, port, ProtocolTCP, state, timeout, interval)
}

// WaitForPortProtocol polls a TCP or UDP port until state matches or timeout expires.
func (i *Inspector) WaitForPortProtocol(ctx context.Context, port int, protocol, state string, timeout, interval time.Duration) (*PortDiagnostic, error) {
	return waitForPortWithDetector(ctx, port, protocol, state, timeout, interval, i.detector())
}

// FindFreePort finds a currently bindable loopback TCP port in [from, to].
func (i *Inspector) FindFreePort(ctx context.Context, from, to int) (*FreePortResult, error) {
	return FindFreePort(ctx, from, to)
}

// RunDoctor diagnoses runtime environment and temporary listener visibility.
func (i *Inspector) RunDoctor(ctx context.Context, binaryPath, cwd, buildVersion string) (*DoctorReport, error) {
	return runDoctorWithDetector(ctx, binaryPath, cwd, buildVersion, i.detector())
}

func (i *Inspector) detector() ProjectDetectorFunc {
	if i == nil {
		return currentProjectDetector()
	}
	return i.ProjectDetector
}
