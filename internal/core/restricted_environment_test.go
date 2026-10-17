package core

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
)

// TestRestrictedEnvironmentDiagnosticContract is opt-in because host metadata
// restrictions vary across WSL, containers, and sandbox implementations.
func TestRestrictedEnvironmentDiagnosticContract(t *testing.T) {
	if !isTruthy(os.Getenv("PORT_DETECTIVE_RESTRICTED_ENV_TESTS")) {
		t.Skip("set PORT_DETECTIVE_RESTRICTED_ENV_TESTS=1 to run restricted-environment checks")
	}
	environment := restrictedEnvironmentName()
	if environment == "" {
		t.Skip("no supported WSL, container, or sandbox marker detected")
	}

	ln, port := listenLoopback(t)
	t.Cleanup(func() { _ = ln.Close() })

	diag, err := InspectPort(context.Background(), port)
	if err != nil {
		t.Fatalf("InspectPort failed in %s: %v", environment, err)
	}
	if diag == nil {
		t.Fatalf("InspectPort returned a nil diagnostic in %s", environment)
	}
	if diag.Port != port || diag.Protocol != ProtocolTCP || !diag.IsOpen {
		t.Fatalf("invalid listener diagnostic in %s: %+v", environment, diag)
	}
	if diag.Status != StatusOccupied && diag.Status != StatusPermissionRequired {
		t.Fatalf("restricted environment returned an invalid status %q", diag.Status)
	}
	if diag.Status == StatusOccupied && diag.PID <= 1 {
		t.Fatalf("occupied diagnostic exposed a protected PID: %+v", diag)
	}
	t.Logf("restricted environment=%s status=%s pid=%d process=%q", environment, diag.Status, diag.PID, diag.ProcessName)
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func restrictedEnvironmentName() string {
	if os.Getenv("WSL_INTEROP") != "" || os.Getenv("WSL_DISTRO_NAME") != "" {
		return "wsl"
	}
	if isTruthy(os.Getenv("PORT_DETECTIVE_SANDBOX")) || os.Getenv("FLATPAK_ID") != "" ||
		os.Getenv("SNAP") != "" || os.Getenv("APP_SANDBOX_CONTAINER_ID") != "" {
		return "sandbox"
	}
	if runtime.GOOS != "linux" {
		return ""
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "container"
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return ""
	}
	contents := strings.ToLower(string(data))
	for _, marker := range []string{"docker", "containerd", "kubepods", "podman", "lxc"} {
		if strings.Contains(contents, marker) {
			return "container"
		}
	}
	return ""
}
