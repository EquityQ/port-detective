package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"port-detective/internal/core"
)

// captureRun 执行根命令并捕获 stdout/stderr，用于断言渲染结果。
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

func TestDisplayWidthHandlesCJK(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"port-detective", 14},
		{"端口", 4},           // 2 个汉字 = 4 列
		{"进程名", 6},          // 3 个汉字 = 6 列
		{"可执行文件", 10},       // 5 个汉字 = 10 列
		{"PID", 3},          // 纯 ASCII
		{"中文abc", 7},        // 4 + 3
		{"<no-access>", 11}, // 纯 ASCII 占位符
	}

	for _, c := range cases {
		if got := displayWidth(c.in); got != c.want {
			t.Errorf("displayWidth(%q) = %d，期望 %d", c.in, got, c.want)
		}
	}
}

func TestPadRightUsesDisplayWidth(t *testing.T) {
	got := padRight("端口", 10)
	if want := "端口" + strings.Repeat(" ", 6); got != want {
		t.Errorf("padRight(%q, 10) = %q，期望 %q", "端口", got, want)
	}

	got = padRight("可执行文件", 4)
	if got != "可执行文件" {
		t.Errorf("padRight 超宽时应原样返回，实际 %q", got)
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
		{"8080", "微信", "120.5 MB"},
		{"19387", "DeepSeek Harness.exe", "206.2 MB"},
	}
	renderTable(&buf, palette{}, columns, rows)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(rows)+1 {
		t.Fatalf("表格行数 = %d，期望 %d（含表头）", len(lines), len(rows)+1)
	}

	dataWidth := displayWidth(lines[1])
	for i := 1; i < len(lines); i++ {
		if got := displayWidth(lines[i]); got != dataWidth {
			t.Errorf("第 %d 行显示宽度 = %d，期望 %d（与其他数据行不一致）\n  该行内容: %q",
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
		t.Errorf("着色后文本内容发生变化：\n  无色: %q\n  着色: %q", plain.String(), stripped)
	}
}

func TestParsePort(t *testing.T) {
	valid := map[string]int{"1": 1, "80": 80, " 8080 ": 8080, "65535": 65535}
	for in, want := range valid {
		got, err := parsePort(in)
		if err != nil {
			t.Errorf("parsePort(%q) 返回错误：%v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parsePort(%q) = %d，期望 %d", in, got, want)
		}
	}

	for _, in := range []string{"0", "-1", "65536", "abc", "", "80a"} {
		if _, err := parsePort(in); err == nil {
			t.Errorf("parsePort(%q) 期望返回错误，实际返回 nil", in)
		}
	}
}

func TestParsePortsList(t *testing.T) {
	ports, err := parsePortsList("3000, 8080, 5432")
	if err != nil {
		t.Fatalf("parsePortsList 返回意外错误: %v", err)
	}
	if len(ports) != 3 || ports[0] != 3000 || ports[1] != 8080 || ports[2] != 5432 {
		t.Errorf("parsePortsList 解析结果不符: %+v", ports)
	}

	if _, err := parsePortsList("3000,abc"); err == nil {
		t.Error("含非法端口时期望报错，实际返回 nil")
	}
}

func TestTruncateRespectsRuneBoundary(t *testing.T) {
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Errorf("truncate(abcdef, 4) = %q，期望 %q", got, "abc…")
	}
	if got := truncate("abc", 4); got != "abc" {
		t.Errorf("未超长时不应截断，实际 %q", got)
	}
	if got := truncate("这是一个测试", 3); got != "这是…" {
		t.Errorf("truncate 中文 = %q，期望 %q", got, "这是…")
	}
}

func TestCheckJSONIsPristine(t *testing.T) {
	out, _, err := captureRun(t, "check", "1", "--json")
	if err != nil {
		t.Fatalf("check --json 执行失败：%v", err)
	}

	var diag core.PortDiagnostic
	if err := json.Unmarshal([]byte(out), &diag); err != nil {
		t.Fatalf("输出不是合法 JSON：%v\n原始输出：%q", err, out)
	}

	for _, forbidden := range []string{"端口体检", "─────", "\x1b["} {
		if strings.Contains(out, forbidden) {
			t.Errorf("JSON 输出中混入了装饰内容 %q：\n%s", forbidden, out)
		}
	}
}

func TestScanJSONIsArray(t *testing.T) {
	out, _, err := captureRun(t, "scan", "--json", "--ports", "80,443")
	if err != nil {
		t.Fatalf("scan --json 执行失败：%v", err)
	}

	var diags []core.PortDiagnostic
	if err := json.Unmarshal([]byte(out), &diags); err != nil {
		t.Fatalf("输出不是合法 JSON 数组：%v\n原始输出：%q", err, out)
	}
}

func TestKillDryRunCLI(t *testing.T) {
	// 针对空闲端口测试
	out, _, err := captureRun(t, "kill", "65432", "--dry-run")
	if err == nil {
		t.Errorf("针对空闲端口期望报错，实际返回: %s", out)
	}
}

func TestMCPCommandKeepsStdoutClean(t *testing.T) {
	out, _, err := captureRun(t, "mcp")
	if err != nil {
		t.Fatalf("mcp 子命令执行失败：%v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("mcp 子命令不应向 stdout 写入内容，实际输出：%q", out)
	}
}

func TestNoColorEnforcedByEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer
	if p := newPalette(&buf); p.title != "" || p.dim != "" || p.accent != "" {
		t.Errorf("NO_COLOR 已设置时不应启用颜色，实际 palette=%+v", p)
	}
}

func TestDegradedMarkerIsASCII(t *testing.T) {
	for _, r := range degradedMarker {
		if r > 0x7f {
			t.Fatalf("降级占位符必须为纯 ASCII，发现非 ASCII 字符 %q", r)
		}
	}
	if displayWidth(degradedMarker) != len(degradedMarker) {
		t.Errorf("纯 ASCII 占位符的显示宽度应等于字节长度")
	}
}
