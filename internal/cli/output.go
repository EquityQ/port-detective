package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/EquityQ/port-detective/internal/core"
)

const (
	maxProcessNameWidth = 26
	degradedMarker      = "<no-access>"
)

type portRow struct {
	port      string
	pid       string
	process   string
	framework string
	project   string
	rss       string
	cpu       string
	status    string
}

func buildRow(info core.PortDiagnostic) portRow {
	process := info.ProcessName
	if process == "" {
		if info.Status == core.StatusPermissionRequired {
			process = degradedMarker
		} else {
			process = "-"
		}
	}

	pid := "-"
	if info.PID > 0 {
		pid = strconv.Itoa(int(info.PID))
	}

	framework := "-"
	project := "-"
	if info.Project != nil {
		if info.Project.Framework != "" {
			framework = info.Project.Framework
		}
		if info.Project.Name != "" {
			project = info.Project.Name
		} else if info.Project.Path != "" {
			project = filepath.Base(info.Project.Path)
		}
	}

	rss := "-"
	if info.MemoryMB > 0 {
		rss = fmt.Sprintf("%.1f MB", info.MemoryMB)
	}

	cpu := "-"
	if info.CPUPercent > 0 {
		cpu = fmt.Sprintf("%.1f%%", info.CPUPercent)
	}

	return portRow{
		port:      strconv.Itoa(info.Port),
		pid:       pid,
		process:   truncate(process, maxProcessNameWidth),
		framework: truncate(framework, 14),
		project:   truncate(project, 18),
		rss:       rss,
		cpu:       cpu,
		status:    info.Status,
	}
}

func renderPortTable(w io.Writer, p palette, infos []core.PortDiagnostic) {
	headers := []string{"PORT", "PID", "PROCESS", "FRAMEWORK", "PROJECT", "RSS", "CPU", "STATUS"}

	columns := []tableColumn{
		{header: headers[0], align: alignRight},
		{header: headers[1], align: alignRight},
		{header: headers[2], align: alignLeft},
		{header: headers[3], align: alignLeft},
		{header: headers[4], align: alignLeft},
		{header: headers[5], align: alignRight},
		{header: headers[6], align: alignRight},
		{header: headers[7], align: alignLeft},
	}

	rows := make([][]string, 0, len(infos))
	for _, info := range infos {
		row := buildRow(info)
		process := row.process
		if strings.HasPrefix(process, "<") {
			process = p.paint(p.warn, process)
		}

		rows = append(rows, []string{
			p.paint(p.accent, row.port),
			row.pid,
			process,
			row.framework,
			row.project,
			row.rss,
			row.cpu,
			p.paint(p.dim, row.status),
		})
	}
	renderTable(w, p, columns, rows)
}

func summaryLine(w io.Writer, p palette, parts ...string) {
	fmt.Fprintf(w, "  %s\n", p.paint(p.dim, strings.Join(parts, "  ·  ")))
}

func degradedNote(w io.Writer, p palette, infos []core.PortDiagnostic) {
	degraded := 0
	for _, info := range infos {
		if info.Status == core.StatusPermissionRequired {
			degraded++
		}
	}
	if degraded == 0 {
		return
	}
	hint(w, p, fmt.Sprintf("%d port(s) have unavailable or ambiguous process metadata.", degraded))
	fmt.Fprintf(w, "    %s\n", p.paint(p.dim, "Check the host/network scope and run port-detective doctor before considering elevated privileges."))
}
