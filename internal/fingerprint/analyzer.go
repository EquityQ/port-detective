// Package fingerprint 实现了针对本地进程的项目指纹与框架智能探测。
// 支持 Node.js 生态、Python 生态与 Go 生态，通过进程 CWD 向上回溯与启动命令进行特征识别。
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
	// 自动将指纹分析引擎注册到 core 模块，实现解耦与零环依赖
	core.RegisterProjectDetector(DetectProject)
}

const (
	// maxTraverseLevels 向上回溯查找特征文件的最大层级
	maxTraverseLevels = 3
	// defaultDetectionTimeout 单进程指纹探测最大耗时保护
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

// DetectProject 探测指定 PID 归属的项目指纹与开发框架。
// 容错保护：严格保证在 50ms 内返回，任何错误或权限不足均静默返回 nil。
func DetectProject(ctx context.Context, pid int32) (res *core.ProjectInfo) {
	if pid <= 1 {
		return nil
	}

	// 异常恢复与超时保护
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

// DetectFromDirAndCmdline 根据工作目录与命令行参数探测项目指纹（便于独立单元测试）。
func DetectFromDirAndCmdline(cwd, cmdline string) *core.ProjectInfo {
	lowerCmd := strings.ToLower(cmdline)

	// 1. 若获取到有效工作目录，向上逐级回溯特征文件（最多查 3 层）
	if cwd != "" {
		cleanCwd := filepath.Clean(cwd)
		currDir := cleanCwd

		for level := 0; level < maxTraverseLevels; level++ {
			// A. Node.js 生态检测 (package.json)
			pkgPath := filepath.Join(currDir, "package.json")
			if info := checkNodeProject(pkgPath, currDir); info != nil {
				return info
			}

			// B. Go 生态检测 (go.mod)
			goModPath := filepath.Join(currDir, "go.mod")
			if info := checkGoProject(goModPath, currDir); info != nil {
				return info
			}

			// C. Python 生态检测 (pyproject.toml 或 requirements.txt)
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

	// 2. 目录未命中或无法读取 CWD 时，尝试仅从命令行特征识别框架
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

// checkNodeProject 解析 package.json 提取项目名称、版本与框架
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

// checkGoProject 解析 go.mod 提取 module 名称与 Go Web 框架
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

// checkPythonProject 综合分析 pyproject.toml、requirements.txt 以及命令行参数
func checkPythonProject(pyprojectPath, reqPath, projectDir, lowerCmd string) *core.ProjectInfo {
	var (
		name      string
		version   string
		framework string
		found     bool
	)

	// 先从命令行中提取典型框架
	framework = identifyFrameworkFromCmdline(lowerCmd)

	// 探测 pyproject.toml
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

	// 探测 requirements.txt
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

// identifyFrameworkFromCmdline 从启动命令行识别 Python 等框架
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

// matchPythonFrameworkKeywords 匹配 Python 依赖中的框架关键词
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
