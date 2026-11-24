// Package fingerprint detects project metadata and frameworks for local processes.
// It supports Node.js, Python, Go, Rust, Java/Kotlin, Ruby, and PHP by inspecting
// process working directories and command lines.
package fingerprint

import (
	"context"
	"path/filepath"
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
)

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

			// Rust project detection (Cargo.toml).
			if info := checkRustProject(ctx, filepath.Join(currDir, "Cargo.toml"), currDir); info != nil {
				return info
			}
			if contextErr(ctx) != nil {
				return nil
			}

			// Java / Kotlin project detection (pom.xml, build.gradle, build.gradle.kts).
			if info := checkJavaProject(
				ctx,
				filepath.Join(currDir, "pom.xml"),
				filepath.Join(currDir, "build.gradle"),
				filepath.Join(currDir, "build.gradle.kts"),
				currDir,
			); info != nil {
				return info
			}
			if contextErr(ctx) != nil {
				return nil
			}

			// Ruby project detection (Gemfile).
			if info := checkRubyProject(ctx, filepath.Join(currDir, "Gemfile"), currDir); info != nil {
				return info
			}
			if contextErr(ctx) != nil {
				return nil
			}

			// PHP project detection (composer.json).
			if info := checkPHPProject(ctx, filepath.Join(currDir, "composer.json"), currDir); info != nil {
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
