package cli

import (
	"bytes"
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/EquityQ/port-detective/internal/core"
)

// captureRun executes the root command and captures stdout/stderr for rendering assertions.
func captureRun(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	root := NewRootCommand()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)

	err := root.Execute()
	return out.String(), errOut.String(), err
}

func TestDisplayWidthHandlesWideUnicode(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"port-detective", 14},
		{"🙂", 2},            // The emoji occupies two terminal columns.
		{"🙂🙂", 4},           // Two wide runes occupy four terminal columns.
		{"wide🙂", 6},        // ASCII and wide runes can be mixed.
		{"PID", 3},          // ASCII text occupies one column per character.
		{"<no-access>", 11}, // ASCII placeholder.
	}

	for _, c := range cases {
		if got := displayWidth(c.in); got != c.want {
			t.Errorf("displayWidth(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCompletionCommandGeneratesSupportedShells(t *testing.T) {
	markers := map[string]string{
		"bash":       "# bash completion V2",
		"zsh":        "#compdef port-detective",
		"fish":       "# fish completion for port-detective",
		"powershell": "# powershell completion for port-detective",
	}
	for shell, marker := range markers {
		t.Run(shell, func(t *testing.T) {
			out, _, err := captureRun(t, "completion", shell)
			if err != nil {
				t.Fatalf("completion %s failed: %v", shell, err)
			}
			if !strings.Contains(out, marker) {
				t.Fatalf("completion %s output does not contain marker %q", shell, marker)
			}
		})
	}
}

func TestCompletionCommandRejectsUnknownShell(t *testing.T) {
	if _, _, err := captureRun(t, "completion", "cmd"); err == nil {
		t.Fatal("unknown shell should be rejected")
	}
}

func TestPadRightUsesDisplayWidth(t *testing.T) {
	got := padRight("🙂", 10)
	if want := "🙂" + strings.Repeat(" ", 8); got != want {
		t.Errorf("padRight(%q, 10) = %q, want %q", "🙂", got, want)
	}

	got = padRight("🙂🙂", 3)
	if got != "🙂🙂" {
		t.Errorf("padRight should preserve over-wide values, got %q", got)
	}
}

func TestRenderTableAlignsMixedWidthColumns(t *testing.T) {
	var buf bytes.Buffer
	columns := []tableColumn{
		{header: "PORT", align: alignRight},
		{header: "PROCESS", align: alignLeft},
		{header: "RSS", align: alignRight},
	}
	rows := [][]string{
		{"135", "<no-access>", "-"},
		{"8080", "server", "120.5 MB"},
		{"19387", "DeepSeek Harness.exe", "206.2 MB"},
	}
	renderTable(&buf, palette{}, columns, rows)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(rows)+1 {
		t.Fatalf("table rows = %d, want %d including the header", len(lines), len(rows)+1)
	}

	dataWidth := displayWidth(lines[1])
	for i := 1; i < len(lines); i++ {
		if got := displayWidth(lines[i]); got != dataWidth {
			t.Errorf("row %d display width = %d, want %d\n  row: %q",
				i, got, dataWidth, lines[i])
		}
	}
}

func TestRenderTableColorDoesNotBreakAlignment(t *testing.T) {
	var plain, colored bytes.Buffer
	columns := []tableColumn{{header: "A", align: alignLeft}, {header: "B", align: alignLeft}}
	rows := [][]string{{"x", "y"}}

	renderTable(&plain, palette{}, columns, rows)

	p := palette{reset: "\x1b[0m", head: "\x1b[38;5;245m"}
	renderTable(&colored, p, columns, rows)

	stripped := stripANSI(colored.String())
	if stripped != plain.String() {
		t.Errorf("coloring changed the text:\n  plain: %q\n  colored: %q", plain.String(), stripped)
	}
}

func TestParsePort(t *testing.T) {
	valid := map[string]int{"1": 1, "80": 80, " 8080 ": 8080, "65535": 65535}
	for in, want := range valid {
		got, err := parsePort(in)
		if err != nil {
			t.Errorf("parsePort(%q) returned an error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parsePort(%q) = %d, want %d", in, got, want)
		}
	}

	for _, in := range []string{"0", "-1", "65536", "abc", "", "80a"} {
		if _, err := parsePort(in); err == nil {
			t.Errorf("parsePort(%q) should return an error, got nil", in)
		}
	}
}

func TestParsePortsList(t *testing.T) {
	ports, err := parsePortsList("3000, 8080, 5432")
	if err != nil {
		t.Fatalf("parsePortsList returned an unexpected error: %v", err)
	}
	if len(ports) != 3 || ports[0] != 3000 || ports[1] != 5432 || ports[2] != 8080 {
		t.Errorf("parsePortsList returned unexpected values: %+v", ports)
	}

	if _, err := parsePortsList("3000,abc"); err == nil {
		t.Error("invalid port list should return an error")
	}
}

func TestTruncateRespectsRuneBoundary(t *testing.T) {
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Errorf("truncate(abcdef, 4) = %q, want %q", got, "abc…")
	}
	if got := truncate("abc", 4); got != "abc" {
		t.Errorf("truncate should preserve short values, got %q", got)
	}
	if got := truncate("testing", 3); got != "te…" {
		t.Errorf("truncate(testing, 3) = %q, want %q", got, "te…")
	}
}

func TestCheckJSONIsPristine(t *testing.T) {
	out, _, err := captureRun(t, "check", "1", "--json")
	if err != nil {
		t.Fatalf("check --json failed: %v", err)
	}

	var diag core.PortDiagnostic
	if err := json.Unmarshal([]byte(out), &diag); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw output: %q", err, out)
	}

	for _, forbidden := range []string{"PORT", "─────", "\x1b["} {
		if strings.Contains(out, forbidden) {
			t.Errorf("JSON output contains presentation text %q:\n%s", forbidden, out)
		}
	}
}

func TestScanJSONIsArray(t *testing.T) {
	out, _, err := captureRun(t, "scan", "--json", "--ports", "80,443")
	if err != nil {
		t.Fatalf("scan --json failed: %v", err)
	}

	var diags []core.PortDiagnostic
	if err := json.Unmarshal([]byte(out), &diags); err != nil {
		t.Fatalf("output is not a valid JSON array: %v\nraw output: %q", err, out)
	}
}

func TestKillDryRunCLI(t *testing.T) {
	// Test an idle port.
	out, _, err := captureRun(t, "kill", "65432", "--dry-run")
	if err == nil {
		t.Errorf("an idle port should return an error, got: %s", out)
	}
}

func TestKillJSONRequiresExplicitConfirmation(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create disposable listener: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	_, _, err = captureRun(t, "kill", strconv.Itoa(port), "--json")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("kill --json without --yes should refuse destructive execution, got %v", err)
	}
	if _, err := net.DialTimeout("tcp", ln.Addr().String(), 250*time.Millisecond); err != nil {
		t.Fatalf("safety refusal must leave the listener alive: %v", err)
	}
}

func TestCleanJSONRequiresExplicitConfirmation(t *testing.T) {
	_, _, err := captureRun(t, "clean", "--json")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("clean --json without --yes should refuse destructive execution, got %v", err)
	}
}

func TestMCPCommandKeepsStdoutClean(t *testing.T) {
	out, _, err := captureRun(t, "mcp")
	if err != nil {
		t.Fatalf("mcp command failed: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("mcp command must not write to stdout, got: %q", out)
	}
}

func TestNoColorEnforcedByEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer
	if p := newPalette(&buf); p.title != "" || p.dim != "" || p.accent != "" {
		t.Errorf("NO_COLOR should disable colors, got palette=%+v", p)
	}
}

func TestDegradedMarkerIsASCII(t *testing.T) {
	for _, r := range degradedMarker {
		if r > 0x7f {
			t.Fatalf("degraded marker must be ASCII, found %q", r)
		}
	}
	if displayWidth(degradedMarker) != len(degradedMarker) {
		t.Errorf("ASCII marker width should equal its byte length")
	}
}

func TestScanTableHeadersEnglish(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create disposable listener: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	out, _, err := captureRun(t, "scan", "--ports", strconv.Itoa(port))
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	for _, header := range []string{"PORT", "PID", "PROCESS", "STATUS"} {
		if !strings.Contains(out, header) {
			t.Errorf("English scan output is missing %q: %s", header, out)
		}
	}
}

func TestCleanCommandDryRun(t *testing.T) {
	out, _, err := captureRun(t, "clean", "--dry-run")
	if err != nil {
		t.Fatalf("clean --dry-run failed: %v", err)
	}
	if !strings.Contains(out, "Clean") {
		t.Errorf("output does not contain the Clean heading: %s", out)
	}
}

func TestCleanCommandJSON(t *testing.T) {
	out, _, err := captureRun(t, "clean", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("clean --dry-run --json failed: %v", err)
	}

	var diags []core.PortDiagnostic
	if err := json.Unmarshal([]byte(out), &diags); err != nil {
		t.Fatalf("clean --json output is not a valid JSON array: %v\nraw output: %s", err, out)
	}
}

func TestRenderPortTableHeaders(t *testing.T) {
	var buf bytes.Buffer
	diag := []core.PortDiagnostic{
		{
			Port:        3000,
			IsOpen:      true,
			Protocol:    "tcp",
			PID:         1234,
			ProcessName: "node",
			MemoryMB:    80.5,
			CPUPercent:  1.2,
			Status:      "occupied",
			Project: &core.ProjectInfo{
				Name:      "web-app",
				Framework: "Next.js",
				Path:      "/app",
			},
		},
	}

	renderPortTable(&buf, palette{}, diag)
	out := buf.String()

	for _, header := range []string{"PORT", "PID", "PROCESS", "FRAMEWORK", "PROJECT", "RSS", "CPU", "STATUS"} {
		if !strings.Contains(out, header) {
			t.Errorf("English table is missing %q: %s", header, out)
		}
	}
	if !strings.Contains(out, "Next.js") || !strings.Contains(out, "web-app") {
		t.Errorf("English table is missing project data: %s", out)
	}
}
