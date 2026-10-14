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

// TestDisplayWidthHandlesCJK 是本包最关键的测试：
// 表格对齐完全依赖显示宽度计算，若把中文按 1 列计算，所有后续列都会错位。
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
		{"<no-access>", 11}, // 占位符必须是纯 ASCII，宽度等于字符数
	}

	for _, c := range cases {
		if got := displayWidth(c.in); got != c.want {
			t.Errorf("displayWidth(%q) = %d，期望 %d", c.in, got, c.want)
		}
	}
}

// TestPadRightUsesDisplayWidth 验证补齐按显示宽度而非字节数进行。
func TestPadRightUsesDisplayWidth(t *testing.T) {
	// 「端口」占 4 列，补到 10 列应追加 6 个空格。
	got := padRight("端口", 10)
	if want := "端口" + strings.Repeat(" ", 6); got != want {
		t.Errorf("padRight(%q, 10) = %q，期望 %q", "端口", got, want)
	}

	// 超宽时不截断，原样返回。
	got = padRight("可执行文件", 4)
	if got != "可执行文件" {
		t.Errorf("padRight 超宽时应原样返回，实际 %q", got)
	}
}

// TestRenderTableAlignsMixedWidthColumns 验证中英文混排时每一列都对齐。
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

	// 核心断言：所有**数据行**的显示宽度必须完全一致。
	// 这是「列对齐」的等价判据——若某行含双列宽中文却没做宽度补偿，
	// 该行宽度就会与其他行不一致，从而在此暴露。
	//
	// 说明：表头宽度允许与数据行不同（数值列右对齐的补齐发生在左侧），
	// 因此这里以首个数据行为基准，而不是以表头为基准。
	dataWidth := displayWidth(lines[1])
	for i := 1; i < len(lines); i++ {
		if got := displayWidth(lines[i]); got != dataWidth {
			t.Errorf("第 %d 行显示宽度 = %d，期望 %d（与其他数据行不一致）\n  该行内容: %q",
				i, got, dataWidth, lines[i])
		}
	}

	// 表头不得窄于数据行，否则说明列宽测量漏算了某个单元格。
	if headWidth := displayWidth(lines[0]); headWidth < dataWidth {
		t.Errorf("表头显示宽度 %d 小于数据行宽度 %d，列宽测量有误", headWidth, dataWidth)
	}

	// 逐列校验：含中文「微信」的那一行必须也占满整行。
	// 若把中文按 1 列计算，该行会比其他行短 2 列。
	if got := displayWidth(lines[2]); got != dataWidth {
		t.Errorf("含中文的行未按显示宽度对齐：宽度 %d，期望 %d\n  该行内容: %q",
			got, dataWidth, lines[2])
	}
}

// TestRenderTableColorDoesNotBreakAlignment 验证 ANSI 颜色不参与宽度计算。
func TestRenderTableColorDoesNotBreakAlignment(t *testing.T) {
	var plain, colored bytes.Buffer
	columns := []tableColumn{{header: "A", align: alignLeft}, {header: "B", align: alignLeft}}
	rows := [][]string{{"x", "y"}}

	renderTable(&plain, palette{}, columns, rows)

	// 构造一个「表头着色」的调色板，字符串内容不变但会包裹转义序列。
	p := palette{reset: "\x1b[0m", head: "\x1b[38;5;245m"}
	renderTable(&colored, p, columns, rows)

	// 去掉 ANSI 转义序列后，着色版本必须与无色版本完全一致。
	stripped := stripANSI(colored.String())
	if stripped != plain.String() {
		t.Errorf("着色后文本内容发生变化：\n  无色: %q\n  着色: %q", plain.String(), stripped)
	}
}

// stripANSI 移除 ANSI 转义序列，便于比较纯文本。
func stripANSI(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && !(s[i] >= '@' && s[i] <= '~') {
				i++
			}
			i++ // 跳过终止字母
			continue
		}
		sb.WriteByte(s[i])
		i++
	}
	return sb.String()
}

// TestParsePort 覆盖端口号解析与范围校验。
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

// TestParsePortRange 覆盖区间解析。
func TestParsePortRange(t *testing.T) {
	start, end, err := parsePortRange("8000-8100")
	if err != nil || start != 8000 || end != 8100 {
		t.Errorf("parsePortRange(8000-8100) = (%d, %d, %v)，期望 (8000, 8100, nil)", start, end, err)
	}

	// 单端口形式应退化为闭区间
	start, end, err = parsePortRange("8080")
	if err != nil || start != 8080 || end != 8080 {
		t.Errorf("parsePortRange(8080) = (%d, %d, %v)，期望 (8080, 8080, nil)", start, end, err)
	}

	for _, in := range []string{"9000-8000", "abc", "80-abc", "0-10"} {
		if _, _, err := parsePortRange(in); err == nil {
			t.Errorf("parsePortRange(%q) 期望返回错误，实际返回 nil", in)
		}
	}
}

// TestHumanBytes 覆盖可读内存单位的换算。
func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%d) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestTruncateRespectsRuneBoundary 验证截断按字符而非字节，不会切断多字节字符。
func TestTruncateRespectsRuneBoundary(t *testing.T) {
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Errorf("truncate(abcdef, 4) = %q，期望 %q", got, "abc…")
	}
	if got := truncate("abc", 4); got != "abc" {
		t.Errorf("未超长时不应截断，实际 %q", got)
	}
	// 5 个汉字，限制 3 个字符
	if got := truncate("这是一个测试", 3); got != "这是…" {
		t.Errorf("truncate 中文 = %q，期望 %q", got, "这是…")
	}
}

// TestCheckJSONIsPristine 验证 --json 输出是可直接解析的 JSON，不含任何装饰。
func TestCheckJSONIsPristine(t *testing.T) {
	out, _, err := captureRun(t, "check", "1", "--json")
	if err != nil {
		t.Fatalf("check --json 执行失败：%v", err)
	}

	var info core.PortInfo
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatalf("输出不是合法 JSON：%v\n原始输出：%q", err, out)
	}

	// 面板装饰字符绝不能出现在 JSON 模式里。
	for _, forbidden := range []string{"端口体检", "─────", "\x1b["} {
		if strings.Contains(out, forbidden) {
			t.Errorf("JSON 输出中混入了装饰内容 %q：\n%s", forbidden, out)
		}
	}
}

// TestScanJSONIsArray 验证 scan --json 输出为数组。
func TestScanJSONIsArray(t *testing.T) {
	out, _, err := captureRun(t, "scan", "--json", "--range", "1-200")
	if err != nil {
		t.Fatalf("scan --json 执行失败：%v", err)
	}

	var infos []core.PortInfo
	if err := json.Unmarshal([]byte(out), &infos); err != nil {
		t.Fatalf("输出不是合法 JSON 数组：%v\n原始输出：%q", err, out)
	}
}

// TestScanRejectsInvalidRange 验证非法区间能被拦截并给出可读错误。
func TestScanRejectsInvalidRange(t *testing.T) {
	_, _, err := captureRun(t, "scan", "--range", "9000-8000")
	if err == nil {
		t.Fatal("非法区间期望返回错误，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "起始端口不能大于结束端口") {
		t.Errorf("错误信息不够明确：%v", err)
	}
}

// TestMCPCommandKeepsStdoutClean 验证 mcp 子命令不污染标准输出。
// Step 4 会在此接入 Stdio 协议服务端，届时 stdout 必须专供协议通信。
func TestMCPCommandKeepsStdoutClean(t *testing.T) {
	out, _, err := captureRun(t, "mcp")
	if err != nil {
		t.Fatalf("mcp 子命令执行失败：%v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("mcp 子命令不应向 stdout 写入内容，实际输出：%q", out)
	}
}

// TestNoColorEnforcedByEnv 验证 NO_COLOR 约定生效。
func TestNoColorEnforcedByEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer
	if p := newPalette(&buf); p.title != "" || p.dim != "" || p.accent != "" {
		t.Errorf("NO_COLOR 已设置时不应启用颜色，实际 palette=%+v", p)
	}
}

// TestDegradedMarkerIsASCII 验证降级占位符是纯 ASCII。
// 使用中日韩占位符会因双列宽而破坏表格对齐，此处作为回归保护。
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
