package fingerprint

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/EquityQ/port-detective/internal/core"
)

var (
	pyprojectReName    = regexp.MustCompile(`(?m)^\s*name\s*=\s*["']([^"']+)["']`)
	pyprojectReVersion = regexp.MustCompile(`(?m)^\s*version\s*=\s*["']([^"']+)["']`)
	mavenArtifactRe    = regexp.MustCompile(`<artifactId>\s*([^<\s]+)\s*</artifactId>`)
	mavenVersionRe     = regexp.MustCompile(`<version>\s*([^<\s]+)\s*</version>`)
)

type packageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
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
	case strings.Contains(lowerCmd, "rails server") || strings.Contains(lowerCmd, "bin/rails"):
		return "Rails"
	case strings.Contains(lowerCmd, "php artisan"):
		return "Laravel"
	case strings.Contains(lowerCmd, "spring-boot"):
		return "Spring Boot"
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

// checkRustProject parses Cargo.toml for crate metadata and web frameworks.
func checkRustProject(ctx context.Context, cargoPath, projectDir string) *core.ProjectInfo {
	data, err := readManifestFile(ctx, cargoPath, maxManifestFileSize)
	if err != nil || contextErr(ctx) != nil {
		return nil
	}
	content := string(data)
	name := ""
	version := ""
	if m := pyprojectReName.FindStringSubmatch(content); len(m) > 1 {
		name = m[1]
	}
	if m := pyprojectReVersion.FindStringSubmatch(content); len(m) > 1 {
		version = m[1]
	}
	if name == "" {
		name = filepath.Base(projectDir)
	}
	lower := strings.ToLower(content)
	framework := "Rust"
	switch {
	case strings.Contains(lower, "axum"):
		framework = "Axum"
	case strings.Contains(lower, "actix-web"):
		framework = "Actix Web"
	case strings.Contains(lower, "rocket"):
		framework = "Rocket"
	case strings.Contains(lower, "warp"):
		framework = "Warp"
	case strings.Contains(lower, "tonic"):
		framework = "Tonic"
	}
	return &core.ProjectInfo{
		Name:      name,
		Framework: framework,
		Path:      projectDir,
		Version:   version,
	}
}

// checkJavaProject analyzes Maven pom.xml or Gradle build scripts.
func checkJavaProject(ctx context.Context, pomPath, gradlePath, gradleKtsPath, projectDir string) *core.ProjectInfo {
	if data, err := readManifestFile(ctx, pomPath, maxManifestFileSize); err == nil && contextErr(ctx) == nil {
		content := string(data)
		name := filepath.Base(projectDir)
		version := ""
		if m := mavenArtifactRe.FindStringSubmatch(content); len(m) > 1 {
			name = m[1]
		}
		if m := mavenVersionRe.FindStringSubmatch(content); len(m) > 1 && !strings.Contains(m[1], "${") {
			version = m[1]
		}
		framework := matchJVMFramework(strings.ToLower(content), "Java")
		return &core.ProjectInfo{
			Name:      name,
			Framework: framework,
			Path:      projectDir,
			Version:   version,
		}
	}
	if contextErr(ctx) != nil {
		return nil
	}
	for _, candidate := range []struct {
		path     string
		fallback string
	}{
		{path: gradleKtsPath, fallback: "Kotlin"},
		{path: gradlePath, fallback: "Java"},
	} {
		data, err := readManifestFile(ctx, candidate.path, maxManifestFileSize)
		if contextErr(ctx) != nil {
			return nil
		}
		if err != nil {
			continue
		}
		framework := matchJVMFramework(strings.ToLower(string(data)), candidate.fallback)
		return &core.ProjectInfo{
			Name:      filepath.Base(projectDir),
			Framework: framework,
			Path:      projectDir,
		}
	}
	return nil
}

func matchJVMFramework(lowerContent, fallback string) string {
	switch {
	case strings.Contains(lowerContent, "spring-boot") || strings.Contains(lowerContent, "org.springframework.boot"):
		return "Spring Boot"
	case strings.Contains(lowerContent, "io.quarkus"):
		return "Quarkus"
	case strings.Contains(lowerContent, "io.micronaut"):
		return "Micronaut"
	case strings.Contains(lowerContent, "io.ktor"):
		return "Ktor"
	case strings.Contains(lowerContent, "io.vertx"):
		return "Vert.x"
	default:
		return fallback
	}
}

// checkRubyProject inspects a Gemfile for Ruby web frameworks.
func checkRubyProject(ctx context.Context, gemfilePath, projectDir string) *core.ProjectInfo {
	data, err := readManifestFile(ctx, gemfilePath, maxManifestFileSize)
	if err != nil || contextErr(ctx) != nil {
		return nil
	}
	lower := strings.ToLower(string(data))
	framework := "Ruby"
	switch {
	case strings.Contains(lower, "rails"):
		framework = "Rails"
	case strings.Contains(lower, "sinatra"):
		framework = "Sinatra"
	case strings.Contains(lower, "hanami"):
		framework = "Hanami"
	case strings.Contains(lower, "puma"):
		framework = "Puma"
	}
	return &core.ProjectInfo{
		Name:      filepath.Base(projectDir),
		Framework: framework,
		Path:      projectDir,
	}
}

type composerJSON struct {
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	Require    map[string]string `json:"require"`
	RequireDev map[string]string `json:"require-dev"`
}

// checkPHPProject parses composer.json for PHP frameworks.
func checkPHPProject(ctx context.Context, composerPath, projectDir string) *core.ProjectInfo {
	data, err := readManifestFile(ctx, composerPath, maxManifestFileSize)
	if err != nil || contextErr(ctx) != nil {
		return nil
	}
	var pkg composerJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}
	hasReq := func(prefix string) bool {
		for dep := range pkg.Require {
			if strings.HasPrefix(strings.ToLower(dep), prefix) {
				return true
			}
		}
		for dep := range pkg.RequireDev {
			if strings.HasPrefix(strings.ToLower(dep), prefix) {
				return true
			}
		}
		return false
	}
	framework := "PHP"
	switch {
	case hasReq("laravel/"):
		framework = "Laravel"
	case hasReq("symfony/"):
		framework = "Symfony"
	case hasReq("slim/"):
		framework = "Slim"
	case hasReq("wordpress") || hasReq("roots/bedrock"):
		framework = "WordPress"
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
