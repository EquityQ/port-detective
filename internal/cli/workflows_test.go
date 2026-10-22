package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/EquityQ/port-detective/internal/core"
)

func testListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func TestWaitOpenAndFree(t *testing.T) {
	listener := testListener(t)
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	for _, state := range []string{"open", "free"} {
		if state == "free" {
			_ = listener.Close()
		}
		out, _, err := captureRun(t, "wait", port, "--state", state, "--timeout", "5s", "--json")
		if err != nil {
			t.Fatalf("wait %s: %v", state, err)
		}
		var diag core.PortDiagnostic
		if err := json.Unmarshal([]byte(out), &diag); err != nil {
			t.Fatal(err)
		}
		if diag.IsOpen != (state == "open") {
			t.Fatalf("unexpected diagnostic: %+v", diag)
		}
	}
}

func TestWaitTimeoutAndInvalidDurations(t *testing.T) {
	listener := testListener(t)
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	_, _, err := captureRun(t, "wait", port, "--state", "free", "--timeout", "30ms", "--interval", "5ms")
	var typed *commandError
	if !errors.As(err, &typed) || typed.code != "timeout" {
		t.Fatalf("expected typed timeout, got %v", err)
	}
	for _, args := range [][]string{
		{"wait", port, "--timeout", "0s"},
		{"wait", port, "--state", "ready"},
		{"check", port, "--watch", "--interval", "0s"},
	} {
		if _, _, err := captureRun(t, args...); err == nil {
			t.Fatalf("expected validation error for %v", args)
		}
	}
}

type cancelWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
}

func (w *cancelWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if bytes.Contains(p, []byte("\n")) {
		w.cancel()
	}
	return n, err
}

func TestWatchJSONIsNDJSONAndCancels(t *testing.T) {
	listener := testListener(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := &cancelWriter{cancel: cancel}
	root := NewRootCommand()
	root.SetContext(ctx)
	root.SetOut(out)
	root.SetArgs([]string{"check", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port), "--watch", "--interval", "1ms", "--json"})
	if err := root.Execute(); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one compact NDJSON line, got %q", out.String())
	}
	var diag core.PortDiagnostic
	if err := json.Unmarshal([]byte(lines[0]), &diag); err != nil || !diag.IsOpen {
		t.Fatalf("invalid watch sample: %v, %q", err, lines[0])
	}
}

func TestFindFreeRejectsOccupiedAndReturnsSnapshot(t *testing.T) {
	listener := testListener(t)
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	_, _, err := captureRun(t, "find-free", "--from", port, "--to", port)
	var typed *commandError
	if !errors.As(err, &typed) || typed.code != "no_free_port" {
		t.Fatalf("expected no_free_port, got %v", err)
	}
	_ = listener.Close()
	out, _, err := captureRun(t, "find-free", "--from", port, "--to", port, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var result freePortResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.Reserved || !result.IPv4 || strconv.Itoa(result.Port) != port {
		t.Fatalf("invalid snapshot: %+v", result)
	}
	probe, err := net.Listen("tcp4", "127.0.0.1:"+port)
	if err != nil {
		t.Fatalf("find-free left port reserved: %v", err)
	}
	_ = probe.Close()
}

func TestFindFreeIPv6UnavailableAndOccupied(t *testing.T) {
	for _, test := range []struct {
		name               string
		bindError          error
		wantFree, wantIPv6 bool
	}{
		{"unsupported", syscall.EAFNOSUPPORT, true, false},
		{"no-loopback", syscall.EADDRNOTAVAIL, true, false},
		{"occupied", syscall.EADDRINUSE, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			listener := testListener(t)
			free, ipv6, err := checkPortBindings(context.Background(), 3000, func(ctx context.Context, network, address string) (net.Listener, error) {
				if network == "tcp4" {
					return listener, nil
				}
				return nil, &net.OpError{Op: "listen", Net: network, Err: test.bindError}
			})
			if err != nil || free != test.wantFree || ipv6 != test.wantIPv6 {
				t.Fatalf("got %v/%v/%v", free, ipv6, err)
			}
			if _, err := listener.Accept(); !errors.Is(err, net.ErrClosed) {
				t.Fatalf("IPv4 listener was not closed: %v", err)
			}
		})
	}
}

func TestDoctorJSONIncludesRuntimeAndSelfCheck(t *testing.T) {
	out, _, err := captureRun(t, "doctor", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report doctorReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.OS == "" || report.Arch == "" || report.Version == "" || report.BinaryPath == "" || report.Hints == nil {
		t.Fatalf("incomplete report: %+v", report)
	}
	if report.Status != "ok" && report.Status != "degraded" {
		t.Fatalf("unexpected doctor status: %s", report.Status)
	}
}

type callbackReader struct {
	call func()
	data io.Reader
}

func (r *callbackReader) Read(p []byte) (int, error) {
	if r.call != nil {
		call := r.call
		r.call = nil
		call()
	}
	return r.data.Read(p)
}

func TestCleanShowsTargetsBeforeConfirmation(t *testing.T) {
	listener := testListener(t)
	port := listener.Addr().(*net.TCPAddr).Port
	projectPath, err := core.ResolveProjectPath(".")
	if err != nil {
		t.Fatal(err)
	}
	targets, err := core.DiscoverProjectPorts(context.Background(), projectPath, []int{port})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) == 0 {
		t.Skip("self process project metadata unavailable in this environment")
	}
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"clean", "--project", projectPath, "--ports", strconv.Itoa(port)})
	root.SetIn(&callbackReader{data: strings.NewReader("no\n"), call: func() {
		if !strings.Contains(out.String(), strconv.Itoa(port)) || !strings.Contains(out.String(), "PROCESS") || !strings.Contains(out.String(), projectPath) {
			t.Errorf("confirmation requested before target preview: %s", out.String())
		}
	}})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Cancelled") {
		t.Fatalf("expected cancellation: %s", out.String())
	}
	connection, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatalf("declined cleanup modified listener: %v", err)
	}
	_ = connection.Close()
}

func TestKillRevalidatesAfterConfirmation(t *testing.T) {
	listener := testListener(t)
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetArgs([]string{"kill", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)})
	root.SetIn(&callbackReader{data: strings.NewReader("yes\n"), call: func() { _ = listener.Close() }})
	if err := root.Execute(); err == nil {
		t.Fatal("a listener that exited during confirmation must not be terminated")
	}
}

func TestCleanRevalidatesPreviewAfterConfirmation(t *testing.T) {
	listener := testListener(t)
	port := listener.Addr().(*net.TCPAddr).Port
	projectPath, err := core.ResolveProjectPath(".")
	if err != nil {
		t.Fatal(err)
	}
	targets, err := core.DiscoverProjectPorts(context.Background(), projectPath, []int{port})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) == 0 {
		t.Skip("self process project metadata unavailable in this environment")
	}
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetArgs([]string{"clean", "--project", projectPath, "--ports", strconv.Itoa(port)})
	root.SetIn(&callbackReader{data: strings.NewReader("yes\n"), call: func() { _ = listener.Close() }})
	if err := root.Execute(); err == nil {
		t.Fatal("cleanup must reject a target that exited after its preview")
	}
}

func TestConfirmationRequiresExactYes(t *testing.T) {
	for _, test := range []struct {
		input string
		want  bool
	}{{"y\n", true}, {"YES\n", true}, {"yesterday\n", false}, {"no\n", false}, {"\n", false}} {
		if got, err := confirm(context.Background(), strings.NewReader(test.input)); err != nil || got != test.want {
			t.Errorf("confirm(%q) = %v, %v; want %v", test.input, got, err, test.want)
		}
	}
}

type blockingConfirmationReader struct {
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (r *blockingConfirmationReader) Read(p []byte) (int, error) {
	close(r.started)
	<-r.release
	close(r.finished)
	return 0, io.EOF
}

func confirmationInput(t *testing.T) *blockingConfirmationReader {
	t.Helper()
	r := &blockingConfirmationReader{started: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	t.Cleanup(func() {
		close(r.release)
		select {
		case <-r.finished:
		case <-time.After(time.Second):
			t.Error("confirmation reader did not exit after release")
		}
	})
	return r
}

func TestConfirmationCancellationReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input := confirmationInput(t)
	done := make(chan error, 1)
	go func() {
		confirmed, err := confirm(ctx, input)
		if confirmed {
			done <- errors.New("canceled confirmation was accepted")
			return
		}
		done <- err
	}()
	select {
	case <-input.started:
	case <-time.After(time.Second):
		t.Fatal("confirmation did not start reading")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("confirmation ignored cancellation while input blocked")
	}
}

func TestProcessConfirmationCancellationExits130(t *testing.T) {
	for _, action := range []string{"kill", "clean"} {
		t.Run(action, func(t *testing.T) {
			listener := testListener(t)
			port := listener.Addr().(*net.TCPAddr).Port
			args := []string{"kill", strconv.Itoa(port)}
			if action == "clean" {
				projectPath, err := core.ResolveProjectPath(".")
				if err != nil {
					t.Fatal(err)
				}
				targets, err := core.DiscoverProjectPorts(context.Background(), projectPath, []int{port})
				if err != nil {
					t.Fatal(err)
				}
				if len(targets) == 0 {
					t.Skip("self process project metadata unavailable")
				}
				args = []string{"clean", "--project", projectPath, "--ports", strconv.Itoa(port)}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			root := NewRootCommand()
			root.SetContext(ctx)
			var out, errOut bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errOut)
			root.SetArgs(args)
			input := confirmationInput(t)
			root.SetIn(input)
			done := make(chan int, 1)
			go func() { done <- executeCommand(root) }()
			select {
			case <-input.started:
			case status := <-done:
				t.Fatalf("command exited before confirmation: %d %s", status, errOut.String())
			case <-time.After(3 * time.Second):
				t.Fatal("command did not request confirmation")
			}
			cancel()
			select {
			case status := <-done:
				if status != 130 {
					t.Fatalf("expected canceled exit status 130, got %d: %s", status, errOut.String())
				}
				if strings.Contains(out.String(), "Cancelled.") {
					t.Fatalf("context cancel was treated as user refusal: %s", out.String())
				}
			case <-time.After(time.Second):
				t.Fatal("command ignored canceled confirmation")
			}
			connection, err := net.Dial("tcp4", listener.Addr().String())
			if err != nil {
				t.Fatalf("canceled confirmation modified listener: %v", err)
			}
			_ = connection.Close()
		})
	}
}

func TestStructuredErrorsStayOnStderr(t *testing.T) {
	root := NewRootCommand()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"wait", "3000", "--timeout", "0s", "--json"})
	if status := executeCommand(root); status != 2 {
		t.Fatalf("expected usage status 2, got %d", status)
	}
	if out.Len() != 0 {
		t.Fatalf("error polluted stdout: %s", out.String())
	}
	var result struct {
		Error struct {
			Code       string `json:"code"`
			ExitStatus int    `json:"exit_status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(errOut.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON error: %v: %s", err, errOut.String())
	}
	if result.Error.Code != "invalid_arguments" || result.Error.ExitStatus != 2 {
		t.Fatalf("unexpected error: %+v", result)
	}
}

func TestPortRangesAreSortedAndDeduplicated(t *testing.T) {
	ports, err := parsePortsList("8080,3000-3002,3001")
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 4 || ports[0] != 3000 || ports[3] != 8080 {
		t.Fatalf("unexpected ports: %v", ports)
	}
}
