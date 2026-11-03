// Package fingerprint detects project metadata and frameworks for local processes.
// It supports Node.js, Python, and Go by inspecting process working directories and command lines.
package fingerprint

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/EquityQ/port-detective/internal/core"
)

func init() {
	// Register the analyzer with core through a decoupled hook.
	core.RegisterProjectDetector(DetectProject)
}

const (
	// maxTraverseLevels is the maximum number of parent directories to inspect.
	maxTraverseLevels = 3
	// defaultDetectionTimeout bounds fingerprint detection for one process.
	defaultDetectionTimeout = 50 * time.Millisecond
	// maxManifestFileSize is the largest manifest file the analyzer is willing to
	// read. A file at or above this limit is treated as "not a usable manifest"
	// and is rejected before it is read, so an oversized or hostile manifest can
	// never stall detection or exhaust memory.
	maxManifestFileSize int64 = 1 << 20 // 1 MiB
	// manifestReadChunkSize is the read granularity for manifest files. Reads are
	// chunked so a slow filesystem (network share, antivirus scan, cold disk) can
	// be abandoned at a chunk boundary as soon as the detection context is done.
	manifestReadChunkSize = 32 << 10 // 32 KiB
	// maxManifestReadStalls is the number of consecutive zero-byte, error-free
	// reads tolerated before a reader is treated as stalled. It prevents a broken
	// reader from spinning forever instead of reporting no fingerprint.
	maxManifestReadStalls = 100
)

var (
	pyprojectReName    = regexp.MustCompile(`(?m)^\s*name\s*=\s*["']([^"']+)["']`)
	pyprojectReVersion = regexp.MustCompile(`(?m)^\s*version\s*=\s*["']([^"']+)["']`)

	// errManifestTooLarge reports a manifest at or above maxManifestFileSize.
	errManifestTooLarge = errors.New("fingerprint: manifest file exceeds the size limit")
	// errManifestNotRegular reports a manifest path that is not a plain file.
	errManifestNotRegular = errors.New("fingerprint: manifest path is not a regular file")
	// errManifestReadStalled reports a reader that repeatedly returns no data.
	errManifestReadStalled = errors.New("fingerprint: manifest read stalled")
)

// manifestFileReader loads at most limit bytes from a manifest file, aborting as
// soon as ctx is done. It is a package-level seam so tests can substitute a
// blocking reader and prove the detection walk honors context cancellation.
type manifestFileReader func(ctx context.Context, path string, limit int64) ([]byte, error)

// readManifestFile is the reader used for every manifest file in the walk.
var readManifestFile manifestFileReader = readManifestFileLimited

type packageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// DetectProject identifies project metadata and the framework for a process.
// Errors and permission limitations return nil; detection, including all
// directory traversal and manifest reads, is bounded by the same timeout.
func DetectProject(ctx context.Context, pid int32) (res *core.ProjectInfo) {
	if pid <= 1 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Recover from unexpected errors and enforce the timeout.
	defer func() {
		if r := recover(); r != nil {
			res = nil
		}
	}()

	tCtx, cancel := context.WithTimeout(ctx, defaultDetectionTimeout)
	defer cancel()

	if contextErr(tCtx) != nil {
		return nil
	}

	proc, err := process.NewProcessWithContext(tCtx, pid)
	if err != nil {
		return nil
	}

	cwd, _ := proc.CwdWithContext(tCtx)
	cmdline, _ := proc.CmdlineWithContext(tCtx)

	// The filesystem walk shares the same deadline as the gopsutil calls: every
	// read checks tCtx, so a slow or huge manifest cannot blow the 50ms budget.
	return detectFromDirAndCmdlineContext(tCtx, cwd, cmdline)
}

// DetectFromDirAndCmdline detects project metadata from a working directory and command line.
// It is the context-free wrapper around the context-aware walk, so it keeps its
// historical behavior: it runs to completion unless a manifest breaks a limit.
func DetectFromDirAndCmdline(cwd, cmdline string) *core.ProjectInfo {
	return detectFromDirAndCmdlineContext(context.Background(), cwd, cmdline)
}

// detectFromDirAndCmdlineContext is the context-aware detection walk. It returns
// nil as soon as ctx is done instead of reporting a possibly stale fingerprint.
func detectFromDirAndCmdlineContext(ctx context.Context, cwd, cmdline string) *core.ProjectInfo {
	lowerCmd := strings.ToLower(cmdline)

	// Search the working directory and up to two parent directories for project manifests.
	if cwd != "" {
		cleanCwd := filepath.Clean(cwd)
		currDir := cleanCwd

		for level := 0; level < maxTraverseLevels; level++ {
			// Never start another traversal level once the budget is spent; the
			// level cap stays fixed, cancellation only ever shortens the walk.
			if contextErr(ctx) != nil {
				return nil
			}

			// Node.js project detection (package.json).
			pkgPath := filepath.Join(currDir, "package.json")
			if info := checkNodeProject(ctx, pkgPath, currDir); info != nil {
				return info
			}
			if contextErr(ctx) != nil {
				return nil
			}

			// Go project detection (go.mod).
			goModPath := filepath.Join(currDir, "go.mod")
			if info := checkGoProject(ctx, goModPath, currDir); info != nil {
				return info
			}
			if contextErr(ctx) != nil {
				return nil
			}

			// Python project detection (pyproject.toml or requirements.txt).
			pyprojectPath := filepath.Join(currDir, "pyproject.toml")
			reqPath := filepath.Join(currDir, "requirements.txt")
			if info := checkPythonProject(ctx, pyprojectPath, reqPath, currDir, lowerCmd); info != nil {
				return info
			}
			if contextErr(ctx) != nil {
				return nil
			}

			parent := filepath.Dir(currDir)
			if parent == currDir {
				break
			}
			currDir = parent
		}
	}

	if contextErr(ctx) != nil {
		return nil
	}

	// Fall back to command-line signatures when no project manifest is found.
	if framework := identifyFrameworkFromCmdline(lowerCmd); framework != "" {
		dir := ""
		name := ""
		if cwd != "" {
			dir = filepath.Clean(cwd)
			name = filepath.Base(dir)
		}
		return &core.ProjectInfo{
			Name:      name,
			Framework: framework,
			Path:      dir,
		}
	}

	return nil
}

// checkNodeProject parses package.json for the project name, version, and framework.
func checkNodeProject(ctx context.Context, pkgPath, projectDir string) *core.ProjectInfo {
	// A canceled detection must never report a fingerprint, even when the read
	// happened to finish just before the cancellation was observed.
	data, err := readManifestFile(ctx, pkgPath, maxManifestFileSize)
	if err != nil || contextErr(ctx) != nil {
		return nil
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	hasDep := func(name string) bool {
		if pkg.Dependencies != nil {
			if _, ok := pkg.Dependencies[name]; ok {
				return true
			}
		}
		if pkg.DevDependencies != nil {
			if _, ok := pkg.DevDependencies[name]; ok {
				return true
			}
		}
		return false
	}

	framework := "Node.js"
	switch {
	case hasDep("next"):
		framework = "Next.js"
	case hasDep("nuxt") || hasDep("@nuxt/kit"):
		framework = "Nuxt"
	case hasDep("vite"):
		framework = "Vite"
	case hasDep("@nestjs/core") || hasDep("@nestjs/common"):
		framework = "NestJS"
	case hasDep("express"):
		framework = "Express"
	case hasDep("react"):
		framework = "React"
	case hasDep("vue"):
		framework = "Vue"
	}

	name := pkg.Name
	if name == "" {
		name = filepath.Base(projectDir)
	}

	return &core.ProjectInfo{
		Name:      name,
		Framework: framework,
		Path:      projectDir,
		Version:   pkg.Version,
	}
}

// checkGoProject parses go.mod for the module name and Go web framework.
func checkGoProject(ctx context.Context, goModPath, projectDir string) *core.ProjectInfo {
	data, err := readManifestFile(ctx, goModPath, maxManifestFileSize)
	if err != nil || contextErr(ctx) != nil {
		return nil
	}

	content := string(data)
	moduleName := ""
	framework := "Go"

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			moduleName = strings.TrimSpace(strings.TrimPrefix(line, "module"))
			break
		}
	}

	switch {
	case strings.Contains(content, "github.com/gin-gonic/gin"):
		framework = "Gin"
	case strings.Contains(content, "github.com/gofiber/fiber"):
		framework = "Fiber"
	case strings.Contains(content, "github.com/labstack/echo"):
		framework = "Echo"
	case strings.Contains(content, "github.com/beego/beego"):
		framework = "Beego"
	}

	name := moduleName
	if name == "" {
		name = filepath.Base(projectDir)
	}

	return &core.ProjectInfo{
		Name:      name,
		Framework: framework,
		Path:      projectDir,
	}
}

// checkPythonProject analyzes pyproject.toml, requirements.txt, and command-line arguments.
func checkPythonProject(ctx context.Context, pyprojectPath, reqPath, projectDir, lowerCmd string) *core.ProjectInfo {
	var (
		name      string
		version   string
		framework string
		found     bool
	)

	// Check the command line for common framework signatures first.
	framework = identifyFrameworkFromCmdline(lowerCmd)

	// Inspect pyproject.toml. A canceled context aborts the walk before the
	// second manifest is even considered.
	data, err := readManifestFile(ctx, pyprojectPath, maxManifestFileSize)
	if contextErr(ctx) != nil {
		return nil
	}
	if err == nil {
		found = true
		content := string(data)
		if m := pyprojectReName.FindStringSubmatch(content); len(m) > 1 {
			name = m[1]
		}
		if m := pyprojectReVersion.FindStringSubmatch(content); len(m) > 1 {
			version = m[1]
		}

		if framework == "" {
			lowerContent := strings.ToLower(content)
			framework = matchPythonFrameworkKeywords(lowerContent)
		}
	}

	// Inspect requirements.txt.
	data, err = readManifestFile(ctx, reqPath, maxManifestFileSize)
	if contextErr(ctx) != nil {
		return nil
	}
	if err == nil {
		found = true
		if framework == "" {
			lowerContent := strings.ToLower(string(data))
			framework = matchPythonFrameworkKeywords(lowerContent)
		}
	}

	if !found && framework == "" {
		return nil
	}

	if name == "" {
		name = filepath.Base(projectDir)
	}
	if framework == "" {
		framework = "Python"
	}

	return &core.ProjectInfo{
		Name:      name,
		Framework: framework,
		Path:      projectDir,
		Version:   version,
	}
}

// readManifestFileLimited reads a manifest file without trusting its size: the
// path is stat'ed first so an oversized file is rejected unread, then the body
// is read in chunks with a cancellation check before every chunk.
func readManifestFileLimited(ctx context.Context, path string, limit int64) ([]byte, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}

	// Open fail-closed: permission errors, missing files, and directories are all
	// "not a usable manifest" for the caller.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errManifestNotRegular
	}
	// Reject an oversized manifest before any of its bytes are read.
	if info.Size() >= limit {
		return nil, errManifestTooLarge
	}

	data, err := readManifestLimited(ctx, f, limit)
	if err != nil {
		return nil, err
	}
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	return data, nil
}

// readManifestLimited buffers at most limit bytes from r, checking ctx before
// every chunk so a slow or growing file cannot outrun the detection budget.
func readManifestLimited(ctx context.Context, r io.Reader, limit int64) ([]byte, error) {
	var (
		buf    bytes.Buffer
		chunk  = make([]byte, manifestReadChunkSize)
		total  int64
		stalls int
	)

	for {
		if err := contextErr(ctx); err != nil {
			return nil, err
		}

		n, readErr := r.Read(chunk)
		if n > 0 {
			stalls = 0
			total += int64(n)
			// The file grew past the cap between the stat and the read.
			if total >= limit {
				return nil, errManifestTooLarge
			}
			buf.Write(chunk[:n])
		} else if readErr == nil {
			stalls++
			if stalls >= maxManifestReadStalls {
				return nil, errManifestReadStalled
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				if err := contextErr(ctx); err != nil {
					return nil, err
				}
				return buf.Bytes(), nil
			}
			return nil, readErr
		}
	}
}

// contextErr returns ctx's cancellation error, treating a nil context as
// "no deadline" so callers never panic on an unset context.
func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// identifyFrameworkFromCmdline detects frameworks from process command lines.
func identifyFrameworkFromCmdline(lowerCmd string) string {
	if lowerCmd == "" {
		return ""
	}
	switch {
	case strings.Contains(lowerCmd, "uvicorn") || strings.Contains(lowerCmd, "gunicorn"):
		return "FastAPI"
	case strings.Contains(lowerCmd, "manage.py"):
		return "Django"
	case strings.Contains(lowerCmd, "streamlit"):
		return "Streamlit"
	case strings.Contains(lowerCmd, "jupyter"):
		return "Jupyter"
	case strings.Contains(lowerCmd, "flask"):
		return "Flask"
	default:
		return ""
	}
}

// matchPythonFrameworkKeywords matches framework keywords in Python dependencies.
func matchPythonFrameworkKeywords(lowerContent string) string {
	switch {
	case strings.Contains(lowerContent, "fastapi"):
		return "FastAPI"
	case strings.Contains(lowerContent, "django"):
		return "Django"
	case strings.Contains(lowerContent, "flask"):
		return "Flask"
	case strings.Contains(lowerContent, "streamlit"):
		return "Streamlit"
	case strings.Contains(lowerContent, "jupyter") || strings.Contains(lowerContent, "notebook"):
		return "Jupyter"
	default:
		return ""
	}
}
