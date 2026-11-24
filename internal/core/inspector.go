package core

import "context"

// Inspector provides port inspection and concurrent scanning with an explicitly
// injected ProjectDetectorFunc, avoiding reliance on package-level global hooks.
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

func (i *Inspector) detector() ProjectDetectorFunc {
	if i == nil {
		return currentProjectDetector()
	}
	return i.ProjectDetector
}
