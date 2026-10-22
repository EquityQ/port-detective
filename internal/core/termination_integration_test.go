package core

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestCoreListenerChild only runs its listener fixture in an explicitly spawned
// child. Production termination tests never target the test runner or other apps.
func TestCoreListenerChild(t *testing.T) {
	if os.Getenv("PORT_DETECTIVE_TEST_CHILD") != "1" {
		return
	}
	ports := []int{0, 0}
	if raw := os.Getenv("PORT_DETECTIVE_TEST_BIND_PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatal(err)
		}
		ports = []int{port}
	}
	for i, port := range ports {
		listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		ports[i] = listener.Addr().(*net.TCPAddr).Port
		go func() {
			for {
				connection, err := listener.Accept()
				if err != nil {
					return
				}
				_ = connection.Close()
			}
		}()
	}
	if err := json.NewEncoder(os.Stdout).Encode(ports); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
}

type listenerChild struct {
	command *exec.Cmd
	ports   []int
	exited  <-chan error
}

func startListenerChild(t *testing.T, bindPort int) *listenerChild {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "-test.run=^TestCoreListenerChild$")
	command.Env = append(os.Environ(), "PORT_DETECTIVE_TEST_CHILD=1", "PORT_DETECTIVE_TEST_BIND_PORT=")
	if bindPort != 0 {
		command.Env[len(command.Env)-1] = "PORT_DETECTIVE_TEST_BIND_PORT=" + strconv.Itoa(bindPort)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait(); close(exited) }()
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = command.Process.Kill() // Only the exact child created above is cleaned up.
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			t.Error("listener child did not exit")
		}
	})
	ready := make(chan []int, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			var ports []int
			if json.Unmarshal(scanner.Bytes(), &ports) == nil {
				ready <- ports
			}
		}
	}()
	select {
	case ports := <-ready:
		return &listenerChild{command: command, ports: ports, exited: exited}
	case err := <-exited:
		t.Fatalf("listener child exited before ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("listener child readiness timed out")
	}
	return nil
}

func TestCleanProjectTargetsTerminatesOwnedMultiPortChildOnce(t *testing.T) {
	child := startListenerChild(t, 0)
	project := projectTestDir(t)
	RegisterProjectDetector(func(_ context.Context, pid int32) *ProjectInfo {
		if pid == int32(child.command.Process.Pid) {
			return &ProjectInfo{Name: "owned-child", Path: project}
		}
		return nil
	})
	defer RegisterProjectDetector(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	preview, err := DiscoverProjectPorts(ctx, project, child.ports)
	if err != nil || len(preview) != 2 {
		t.Fatalf("multi-port child preview = %+v, %v", preview, err)
	}
	cleaned, err := CleanProjectTargets(ctx, project, preview, false)
	if err != nil || len(cleaned) != 2 {
		t.Fatalf("one process should release both preview ports: %+v, %v", cleaned, err)
	}
	for _, port := range child.ports {
		diag, err := InspectPort(ctx, port)
		if err != nil || diag.IsOpen {
			t.Fatalf("child port %d remained occupied: %+v, %v", port, diag, err)
		}
	}
	select {
	case <-child.exited:
	case <-ctx.Done():
		t.Fatal("terminated child was not reaped")
	}
}

func TestKillPortTargetRejectsOwnedReplacementChild(t *testing.T) {
	original := startListenerChild(t, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	port := original.ports[0]
	preview, err := InspectPort(ctx, port)
	if err != nil || preview.PID != int32(original.command.Process.Pid) {
		t.Fatalf("original child preview = %+v, %v", preview, err)
	}
	if err := original.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-original.exited:
	case <-ctx.Done():
		t.Fatal("original child did not exit")
	}
	if err := waitForPortRelease(ctx, port, time.Second); err != nil {
		t.Fatal(err)
	}
	replacement := startListenerChild(t, port)
	if _, _, err := KillPortTarget(ctx, *preview, false); err == nil || !strings.Contains(err.Error(), "ownership changed") {
		t.Fatalf("stale preview must reject replacement: %v", err)
	}
	diag, err := InspectPort(ctx, port)
	if err != nil || !diag.IsOpen || diag.PID != int32(replacement.command.Process.Pid) {
		t.Fatalf("replacement child should remain alive: %+v, %v", diag, err)
	}
}
