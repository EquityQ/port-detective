// Package fingerprint detects project metadata and frameworks for local processes.
// It supports Node.js, Python, and Go by inspecting process working directories and command lines.
package fingerprint

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"port-detective/internal/core"
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
)

var (
	pyprojectReName    = regexp.MustCompile(`(?m)^\s*name\s*=\s*["']([^"']+)["']`)
	pyprojectReVersion = regexp.MustCompile(`(?m)^\s*version\s*=\s*["']([^"']+)["']`)
)

type packageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// DetectProject identifies project metadata and the framework for a process.
// Errors and permission limitations return nil; detection is bounded by a timeout.
func DetectProject(ctx context.Context, pid int32) (res *core.ProjectInfo) {
	if pid <= 1 {
		return nil
	}

	// Recover from unexpected errors and enforce the timeout.
	defer func() {
		if r := recover(); r != nil {
			res = nil
		}
	}()

	tCtx, cancel := context.WithTimeout(ctx, defaultDetectionTimeout)
	defer cancel()

	proc, err := process.NewProcessWithContext(tCtx, pid)
	if err != nil {
		return nil
	}

	cwd, _ := proc.CwdWithContext(tCtx)
	cmdline, _ := proc.CmdlineWithContext(tCtx)

	return DetectFromDirAndCmdline(cwd, cmdline)
}

// DetectFromDirAndCmdline detects project metadata from a working directory and command line.
func DetectFromDirAndCmdline(cwd, cmdline string) *core.ProjectInfo {
	lowerCmd := strings.ToLower(cmdline)

	// Search the working directory and up to two parent directories for project manifests.
	if cwd != "" {
		cleanCwd := filepath.Clean(cwd)
		currDir := cleanCwd

		for level := 0; level < maxTraverseLevels; level++ {
			// Node.js project detection (package.json).
			pkgPath := filepath.Join(currDir, "package.json")
			if info := checkNodeProject(pkgPath, currDir); info != nil {
				return info
			}

			// Go project detection (go.mod).
			goModPath := filepath.Join(currDir, "go.mod")
			if info := checkGoProject(goModPath, currDir); info != nil {
				return info
			}

			// Python project detection (pyproject.toml or requirements.txt).
			pyprojectPath := filepath.Join(currDir, "pyproject.toml")
			reqPath := filepath.Join(currDir, "requirements.txt")
			if info := checkPythonProject(pyprojectPath, reqPath, currDir, lowerCmd); info != nil {
				return info
			}

			parent := filepath.Dir(currDir)
			if parent == currDir {
				break
			}
			currDir = parent
		}
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
func checkNodeProject(pkgPath, projectDir string) *core.ProjectInfo {
	data, err := os.ReadFile(pkgPath)
	if err != nil {
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
func checkGoProject(goModPath, projectDir string) *core.ProjectInfo {
	data, err := os.ReadFile(goModPath)
	if err != nil {
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
func checkPythonProject(pyprojectPath, reqPath, projectDir, lowerCmd string) *core.ProjectInfo {
	var (
		name      string
		version   string
		framework string
		found     bool
	)

	// Check the command line for common framework signatures first.
	framework = identifyFrameworkFromCmdline(lowerCmd)

	// Inspect pyproject.toml.
	if data, err := os.ReadFile(pyprojectPath); err == nil {
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
	if data, err := os.ReadFile(reqPath); err == nil {
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
