package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	ruleLine = "─"
	keyWidth = 10
)

type palette struct {
	reset  string
	title  string
	head   string
	dim    string
	accent string
	warn   string
	key    string
}

func newPalette(out io.Writer) palette {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return palette{}
	}
	if f, ok := out.(*os.File); ok {
		if info, err := f.Stat(); err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return palette{}
		}
	} else {
		return palette{}
	}
	return palette{
		reset:  "\x1b[0m",
		title:  "\x1b[38;5;39m",  // Cyan-blue.
		head:   "\x1b[38;5;245m", // Neutral gray.
		dim:    "\x1b[38;5;244m", // Secondary gray.
		accent: "\x1b[38;5;81m",  // Light cyan.
		warn:   "\x1b[38;5;179m", // Amber.
		key:    "\x1b[38;5;250m", // Light gray.
	}
}

func (p palette) paint(code, s string) string {
	if code == "" {
		return s
	}
	return code + s + p.reset
}

func rule(width int) string {
	if width <= 0 {
		width = 60
	}
	return strings.Repeat(ruleLine, width)
}

func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%.0fms", float64(d.Microseconds())/1000)
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

func sectionTitle(w io.Writer, p palette, title string, width int) {
	fmt.Fprintf(w, "%s\n", p.paint(p.title, title))
	fmt.Fprintf(w, "%s\n", p.paint(p.dim, rule(width)))
}

func field(w io.Writer, p palette, key string, keyWidth int, value string) {
	fmt.Fprintf(w, "  %s %s %s\n",
		p.paint(p.key, padRight(key, keyWidth)),
		p.paint(p.dim, ":"),
		value)
}

func hint(w io.Writer, p palette, text string) {
	fmt.Fprintf(w, "  %s %s\n", p.paint(p.warn, "!"), p.paint(p.dim, text))
}

func stdout(cmd *cobra.Command) io.Writer {
	if out := cmd.OutOrStdout(); out != nil {
		return out
	}
	return os.Stdout
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func displayWidth(s string) int {
	width := 0
	for _, r := range s {
		switch {
		case r == 0:
		case r < 0x20 || (r >= 0x7f && r < 0xa0):
		case isWideRune(r):
			width += 2
		default:
			width++
		}
	}
	return width
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F:
		return true
	case r >= 0x2E80 && r <= 0x303E:
		return true
	case r >= 0x3041 && r <= 0x33FF:
		return true
	case r >= 0x3400 && r <= 0x4DBF:
		return true
	case r >= 0x4E00 && r <= 0x9FFF:
		return true
	case r >= 0xA000 && r <= 0xA4CF:
		return true
	case r >= 0xAC00 && r <= 0xD7A3:
		return true
	case r >= 0xF900 && r <= 0xFAFF:
		return true
	case r >= 0xFE10 && r <= 0xFE19:
		return true
	case r >= 0xFE30 && r <= 0xFE6F:
		return true
	case r >= 0xFF00 && r <= 0xFF60:
		return true
	case r >= 0xFFE0 && r <= 0xFFE6:
		return true
	case r >= 0x1F300 && r <= 0x1F64F:
		return true
	case r >= 0x1F900 && r <= 0x1F9FF:
		return true
	case r >= 0x20000 && r <= 0x3FFFD:
		return true
	}
	return false
}

func padRight(s string, width int) string {
	if gap := width - displayWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

func padLeft(s string, width int) string {
	if gap := width - displayWidth(s); gap > 0 {
		return strings.Repeat(" ", gap) + s
	}
	return s
}

type alignment int

const (
	alignLeft alignment = iota
	alignRight
)

type tableColumn struct {
	header string
	align  alignment
}

func renderTable(w io.Writer, p palette, columns []tableColumn, rows [][]string) {
	if len(columns) == 0 {
		return
	}

	colWidths := make([]int, len(columns))
	for i, col := range columns {
		colWidths[i] = displayWidth(col.header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) {
				clean := stripANSI(cell)
				if w := displayWidth(clean); w > colWidths[i] {
					colWidths[i] = w
				}
			}
		}
	}

	// Print the table header.
	var headParts []string
	for i, col := range columns {
		formatted := padCell(col.header, colWidths[i], col.align)
		headParts = append(headParts, p.paint(p.head, formatted))
	}
	fmt.Fprintf(w, "  %s\n", strings.Join(headParts, "   "))

	// Print data rows.
	for _, row := range rows {
		var rowParts []string
		for i, col := range columns {
			var cell string
			if i < len(row) {
				cell = row[i]
			}
			clean := stripANSI(cell)
			visibleLen := displayWidth(clean)
			gap := colWidths[i] - visibleLen
			if gap < 0 {
				gap = 0
			}

			var formatted string
			if col.align == alignRight {
				formatted = strings.Repeat(" ", gap) + cell
			} else {
				formatted = cell + strings.Repeat(" ", gap)
			}
			rowParts = append(rowParts, formatted)
		}
		line := strings.TrimRight("  "+strings.Join(rowParts, "   "), " ")
		fmt.Fprintf(w, "%s\n", line)
	}
}

func padCell(s string, width int, align alignment) string {
	if align == alignRight {
		return padLeft(s, width)
	}
	return padRight(s, width)
}

func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b[") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	inEsc := false
	for i := 0; i < len(s); i++ {
		if !inEsc && s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			inEsc = true
			i++
			continue
		}
		if inEsc {
			if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
				inEsc = false
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
