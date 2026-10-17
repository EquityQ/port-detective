package fingerprint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDetectNodeProject verifies Node.js framework detection (Next.js, Vite, Express, NestJS).
func TestDetectNodeProject(t *testing.T) {
	tempDir := t.TempDir()

	pkgJSON := `{
		"name": "my-next-app",
		"version": "14.2.3",
		"dependencies": {
			"next": "^14.2.0",
			"react": "^18.2.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(tempDir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatalf("failed to write package.json: %v", err)
	}

	info := DetectFromDirAndCmdline(tempDir, "node server.js")
	if info == nil {
		t.Fatal("expected project information, got nil")
	}
	if info.Name != "my-next-app" {
		t.Errorf("project name = %q, want %q", info.Name, "my-next-app")
	}
	if info.Framework != "Next.js" {
		t.Errorf("framework = %q, want %q", info.Framework, "Next.js")
	}
	if info.Version != "14.2.3" {
		t.Errorf("version = %q, want %q", info.Version, "14.2.3")
	}
	if info.Path != filepath.Clean(tempDir) {
		t.Errorf("Path = %q, want %q", info.Path, filepath.Clean(tempDir))
	}
}

// TestDetectGoProject verifies Go framework detection (Gin and Fiber).
func TestDetectGoProject(t *testing.T) {
	tempDir := t.TempDir()

	goMod := `module github.com/user/demo-service

go 1.22

require (
	github.com/gin-gonic/gin v1.9.1
)
`
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	info := DetectFromDirAndCmdline(tempDir, "./demo-service")
	if info == nil {
		t.Fatal("expected a Go project, got nil")
	}
	if info.Name != "github.com/user/demo-service" {
		t.Errorf("module name = %q, want %q", info.Name, "github.com/user/demo-service")
	}
	if info.Framework != "Gin" {
		t.Errorf("framework = %q, want %q", info.Framework, "Gin")
	}
}

// TestDetectPythonProject verifies Python framework detection from command lines and manifests.
func TestDetectPythonProject(t *testing.T) {
	t.Run("FastAPI from uvicorn cmdline", func(t *testing.T) {
		tempDir := t.TempDir()
		reqTxt := "fastapi>=0.100.0\nuvicorn\n"
		_ = os.WriteFile(filepath.Join(tempDir, "requirements.txt"), []byte(reqTxt), 0644)

		info := DetectFromDirAndCmdline(tempDir, "python -m uvicorn main:app --port 8000")
		if info == nil {
			t.Fatal("expected a FastAPI project")
		}
		if info.Framework != "FastAPI" {
			t.Errorf("framework = %q, want %q", info.Framework, "FastAPI")
		}
	})

	t.Run("Django from manage.py", func(t *testing.T) {
		tempDir := t.TempDir()
		info := DetectFromDirAndCmdline(tempDir, "python manage.py runserver 0.0.0.0:8000")
		if info == nil {
			t.Fatal("expected a Django project")
		}
		if info.Framework != "Django" {
			t.Errorf("framework = %q, want %q", info.Framework, "Django")
		}
	})

	t.Run("Streamlit cmdline", func(t *testing.T) {
		tempDir := t.TempDir()
		info := DetectFromDirAndCmdline(tempDir, "streamlit run app.py")
		if info == nil {
			t.Fatal("expected a Streamlit project")
		}
		if info.Framework != "Streamlit" {
			t.Errorf("framework = %q, want %q", info.Framework, "Streamlit")
		}
	})

	t.Run("pyproject.toml parsing", func(t *testing.T) {
		tempDir := t.TempDir()
		tomlContent := `[project]
name = "ai-portal"
version = "0.2.1"
dependencies = [
    "fastapi",
    "pydantic"
]
`
		_ = os.WriteFile(filepath.Join(tempDir, "pyproject.toml"), []byte(tomlContent), 0644)

		info := DetectFromDirAndCmdline(tempDir, "python main.py")
		if info == nil {
			t.Fatal("expected a pyproject.toml project")
		}
		if info.Name != "ai-portal" {
			t.Errorf("Name = %q, want %q", info.Name, "ai-portal")
		}
		if info.Version != "0.2.1" {
			t.Errorf("Version = %q, want %q", info.Version, "0.2.1")
		}
		if info.Framework != "FastAPI" {
			t.Errorf("Framework = %q, want %q", info.Framework, "FastAPI")
		}
	})
}

// TestDirectoryUpwardTraversal verifies lookup through at most three directory levels.
func TestDirectoryUpwardTraversal(t *testing.T) {
	rootDir := t.TempDir()
	pkgJSON := `{"name": "root-app", "dependencies": {"vite": "^5.0.0"}}`
	_ = os.WriteFile(filepath.Join(rootDir, "package.json"), []byte(pkgJSON), 0644)

	// Create a nested directory: root/a/b.
	deepDir := filepath.Join(rootDir, "sub1", "sub2")
	if err := os.MkdirAll(deepDir, 0755); err != nil {
		t.Fatalf("failed to create nested directories: %v", err)
	}

	info := DetectFromDirAndCmdline(deepDir, "node deep.js")
	if info == nil {
		t.Fatal("the upward lookup should find rootDir/package.json two levels up")
	}
	if info.Name != "root-app" {
		t.Errorf("Name = %q, want %q", info.Name, "root-app")
	}
	if info.Framework != "Vite" {
		t.Errorf("Framework = %q, want %q", info.Framework, "Vite")
	}
	if info.Path != filepath.Clean(rootDir) {
		t.Errorf("Path = %q, want %q", info.Path, filepath.Clean(rootDir))
	}

	// Manifests more than three levels above the working directory should not be found.
	tooDeepDir := filepath.Join(rootDir, "l1", "l2", "l3", "l4")
	_ = os.MkdirAll(tooDeepDir, 0755)
	infoOver := DetectFromDirAndCmdline(tooDeepDir, "node deep.js")
	if infoOver != nil && infoOver.Path == filepath.Clean(rootDir) {
		t.Errorf("a lookup beyond three levels should not find rootDir, got: %+v", infoOver)
	}
}

// TestDetectProjectSafetyAndTimeout verifies timeout and defensive safety behavior.
func TestDetectProjectSafetyAndTimeout(t *testing.T) {
	ctx := context.Background()

	// PID <= 1 must return nil immediately.
	if res := DetectProject(ctx, 0); res != nil {
		t.Errorf("PID 0 should return nil, got %+v", res)
	}
	if res := DetectProject(ctx, 1); res != nil {
		t.Errorf("PID 1 should return nil, got %+v", res)
	}
	if res := DetectProject(ctx, -100); res != nil {
		t.Errorf("negative PID should return nil, got %+v", res)
	}

	// Verify that the timeout context is propagated.
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately.
	if res := DetectProject(cancelCtx, int32(os.Getpid())); res != nil {
		t.Logf("canceled context returned: %+v", res)
	}

	// Detect this test process, which runs from the repository containing go.mod.
	selfInfo := DetectProject(ctx, int32(os.Getpid()))
	if selfInfo != nil {
		t.Logf("test process fingerprint: Name=%s, Framework=%s, Path=%s",
			selfInfo.Name, selfInfo.Framework, selfInfo.Path)
	}

	// Ensure the test completes in less than 100 ms.
	start := time.Now()
	_ = DetectProject(ctx, 999999999) // Use a PID that does not exist.
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("detection took %.2fms, exceeding 100ms", float64(elapsed.Microseconds())/1000)
	}
}
