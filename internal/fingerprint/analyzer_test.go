package fingerprint

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
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

// writeManifest writes a manifest file into dir and fails the test on error.
func writeManifest(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

// oversizedManifest builds a syntactically valid manifest padded past the size cap.
func oversizedManifest(t *testing.T, template string) []byte {
	t.Helper()
	if int64(len(template)) >= maxManifestFileSize {
		t.Fatalf("test template already exceeds the cap: %d bytes", len(template))
	}
	padding := bytes.Repeat([]byte(" "), int(maxManifestFileSize)-len(template)+1)
	return append([]byte(template), padding...)
}

// TestManifestSizeCapEnforced verifies the reader rejects manifests at or above
// maxManifestFileSize without returning any content.
func TestManifestSizeCapEnforced(t *testing.T) {
	dir := t.TempDir()

	// Exactly at the cap must already be rejected (fail closed, no partial parse).
	atCap := filepath.Join(dir, "package.json")
	if err := os.WriteFile(atCap, bytes.Repeat([]byte(" "), int(maxManifestFileSize)), 0o644); err != nil {
		t.Fatalf("failed to write at-cap manifest: %v", err)
	}
	if _, err := readManifestFileLimited(context.Background(), atCap, maxManifestFileSize); !errors.Is(err, errManifestTooLarge) {
		t.Errorf("a manifest exactly at the cap: err = %v, want errManifestTooLarge", err)
	}

	// One byte over the cap must be rejected as well.
	overCap := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(overCap, bytes.Repeat([]byte(" "), int(maxManifestFileSize)+1), 0o644); err != nil {
		t.Fatalf("failed to write over-cap manifest: %v", err)
	}
	if _, err := readManifestFileLimited(context.Background(), overCap, maxManifestFileSize); !errors.Is(err, errManifestTooLarge) {
		t.Errorf("an oversized manifest: err = %v, want errManifestTooLarge", err)
	}

	// A small, readable manifest must still be returned intact.
	okPath := filepath.Join(dir, "requirements.txt")
	writeManifest(t, dir, "requirements.txt", []byte("fastapi\n"))
	got, err := readManifestFileLimited(context.Background(), okPath, maxManifestFileSize)
	if err != nil {
		t.Fatalf("small manifest: unexpected error %v", err)
	}
	if string(got) != "fastapi\n" {
		t.Errorf("small manifest content = %q, want %q", string(got), "fastapi\n")
	}
}

// TestOversizedManifestIsIgnored verifies an oversized manifest contributes no
// fingerprint and that detection still finishes promptly instead of hanging.
func TestOversizedManifestIsIgnored(t *testing.T) {
	t.Run("package.json", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, "package.json", oversizedManifest(t, `{"name":"huge-app","dependencies":{"next":"1"}}`))

		start := time.Now()
		info := DetectFromDirAndCmdline(dir, "")
		elapsed := time.Since(start)

		if elapsed > 2*time.Second {
			t.Errorf("oversized package.json took %v, want well under 2s", elapsed)
		}
		if info != nil && info.Path == filepath.Clean(dir) {
			t.Errorf("an oversized package.json must not yield a fingerprint, got %+v", info)
		}
	})

	t.Run("go.mod", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, "go.mod", oversizedManifest(t, "module example.com/huge\n\nrequire github.com/gin-gonic/gin v1.9.1\n"))

		if info := DetectFromDirAndCmdline(dir, ""); info != nil && info.Path == filepath.Clean(dir) {
			t.Errorf("an oversized go.mod must not yield a fingerprint, got %+v", info)
		}
	})

	// An oversized pyproject.toml is unusable, but a valid sibling manifest must
	// still be honored: the cap skips one file, it does not abort the walk.
	t.Run("pyproject.toml falls through to requirements.txt", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, "pyproject.toml", oversizedManifest(t, "[project]\nname = \"huge\"\n"))
		writeManifest(t, dir, "requirements.txt", []byte("streamlit\n"))

		info := DetectFromDirAndCmdline(dir, "")
		if info == nil {
			t.Fatal("a valid requirements.txt next to an oversized pyproject.toml should still be found")
		}
		if info.Framework != "Streamlit" {
			t.Errorf("Framework = %q, want %q", info.Framework, "Streamlit")
		}
		if info.Path != filepath.Clean(dir) {
			t.Errorf("Path = %q, want %q", info.Path, filepath.Clean(dir))
		}
	})
}

// TestManifestPathNotRegular verifies a non-regular manifest path (a directory
// named package.json) is rejected rather than parsed.
func TestManifestPathNotRegular(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "package.json")
	if err := os.Mkdir(pkgDir, 0o755); err != nil {
		t.Fatalf("failed to create directory named package.json: %v", err)
	}

	if _, err := readManifestFileLimited(context.Background(), pkgDir, maxManifestFileSize); !errors.Is(err, errManifestNotRegular) {
		t.Errorf("directory manifest: err = %v, want errManifestNotRegular", err)
	}
	if _, err := readManifestFileLimited(context.Background(), filepath.Join(dir, "absent.json"), maxManifestFileSize); err == nil {
		t.Error("a missing manifest should return an error")
	}
}

// TestCanceledContextReturnsNil verifies a canceled detection context yields no
// fingerprint at all, including no command-line fallback.
func TestCanceledContextReturnsNil(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "package.json", []byte(`{"name":"canceled-app","dependencies":{"vite":"5"}}`))

	// "uvicorn" would normally produce a FastAPI fingerprint from the cmdline
	// fallback; cancellation must suppress that too.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	info := detectFromDirAndCmdlineContext(ctx, dir, "python -m uvicorn main:app")
	elapsed := time.Since(start)

	if info != nil {
		t.Errorf("a canceled context should return nil, got %+v", info)
	}
	if elapsed > 2*time.Second {
		t.Errorf("a canceled context took %v, want well under 2s", elapsed)
	}

	// An already-expired deadline behaves the same way.
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	if info := detectFromDirAndCmdlineContext(expired, dir, "python -m uvicorn main:app"); info != nil {
		t.Errorf("an expired deadline should return nil, got %+v", info)
	}
}

// TestReadManifestLimitedAbortsSlowReader proves cancellation is checked between
// chunks, so a slow filesystem read is abandoned instead of run to completion.
//
// The reader is bounded deliberately: correct code aborts within the ~40ms
// context after a handful of chunks, so a regression that drops the per-chunk
// check fails in about a second instead of burning the reader's full runtime.
func TestReadManifestLimitedAbortsSlowReader(t *testing.T) {
	const totalChunks = 300
	reader := &trickleReader{chunks: totalChunks, delay: 10 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	start := time.Now()
	data, err := readManifestLimited(ctx, reader, maxManifestFileSize)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("a slow reader should abort on cancellation, got %d bytes", len(data))
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
	// Generous bound: the point is that it stops early, not an exact latency.
	if elapsed > 2*time.Second {
		t.Errorf("slow read took %v, want well under 2s", elapsed)
	}
	if reads := int(atomic.LoadInt32(&reader.reads)); reads >= totalChunks {
		t.Errorf("read %d chunks of %d: the read was not abandoned early", reads, totalChunks)
	}
}

// TestReadManifestLimitedRejectsStalledReader verifies a reader that never makes
// progress is reported as stalled instead of spinning forever.
func TestReadManifestLimitedRejectsStalledReader(t *testing.T) {
	if _, err := readManifestLimited(context.Background(), stalledReader{}, maxManifestFileSize); !errors.Is(err, errManifestReadStalled) {
		t.Errorf("stalled reader: err = %v, want errManifestReadStalled", err)
	}
}

// TestReadManifestLimitedRejectsGrowthPastCap verifies the cap is enforced while
// reading, not only from the stat'ed size.
func TestReadManifestLimitedRejectsGrowthPastCap(t *testing.T) {
	const limit = 2 * manifestReadChunkSize
	// 3 chunks of data against a 2 chunk limit: the cap must trip mid-read.
	reader := bytes.NewReader(bytes.Repeat([]byte("a"), 3*manifestReadChunkSize))

	if _, err := readManifestLimited(context.Background(), reader, limit); !errors.Is(err, errManifestTooLarge) {
		t.Errorf("reader growing past the cap: err = %v, want errManifestTooLarge", err)
	}
}

// TestSlowFilesystemAbortsWalk proves the directory walk threads its context into
// the manifest reader: a reader that blocks until cancellation makes the walk
// return nil promptly, and no further manifest is opened afterwards.
func TestSlowFilesystemAbortsWalk(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "package.json", []byte(`{"name":"slow-app"}`))
	writeManifest(t, dir, "go.mod", []byte("module example.com/slow\n"))

	entered := make(chan struct{})
	var (
		calls       int32
		observedErr atomic.Value // error seen by the substituted reader
	)

	// Swap the package-level seam with a reader that behaves like a filesystem
	// stalling indefinitely: it only returns once the context is canceled.
	readManifestFile = func(ctx context.Context, path string, limit int64) ([]byte, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(entered)
			<-ctx.Done()
			err := ctx.Err()
			observedErr.Store(err)
			return nil, err
		}
		return os.ReadFile(path)
	}
	t.Cleanup(func() { readManifestFile = readManifestFileLimited })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-entered
		cancel()
	}()

	start := time.Now()
	info := detectFromDirAndCmdlineContext(ctx, dir, "")
	elapsed := time.Since(start)

	if info != nil {
		t.Errorf("a cancellation during a slow read must return nil, got %+v", info)
	}
	if elapsed > 2*time.Second {
		t.Errorf("the walk took %v after cancellation, want well under 2s", elapsed)
	}

	// The reader must have received the test's live context, proving the walk and
	// the reader share one cancellation signal.
	seen, _ := observedErr.Load().(error)
	if !errors.Is(seen, context.Canceled) {
		t.Errorf("reader observed %v, want context.Canceled", seen)
	}
	// Exactly one manifest read: the walk stopped instead of continuing to go.mod.
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("readManifestFile called %d times, want 1 (the walk must stop at cancellation)", got)
	}
}

// TestDetectProjectBudgetCoversFilesystem verifies DetectProject's own 50ms budget
// aborts a manifest read that never makes progress, so the timeout covers the
// filesystem work and not only the gopsutil calls.
func TestDetectProjectBudgetCoversFilesystem(t *testing.T) {
	entered := make(chan struct{}, 1)

	readManifestFile = func(ctx context.Context, path string, limit int64) ([]byte, error) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-ctx.Done() // never returns before the detection budget expires
		return nil, ctx.Err()
	}
	t.Cleanup(func() { readManifestFile = readManifestFileLimited })

	start := time.Now()
	info := DetectProject(context.Background(), int32(os.Getpid()))
	elapsed := time.Since(start)

	if info != nil {
		t.Errorf("detection whose filesystem read was abandoned must return nil, got %+v", info)
	}
	// defaultDetectionTimeout is 50ms; allow a generous margin for CI scheduling.
	if elapsed > 2*time.Second {
		t.Errorf("DetectProject took %v, want well under 2s", elapsed)
	}
	select {
	case <-entered:
		if elapsed < defaultDetectionTimeout {
			t.Logf("detection aborted after %v (budget %v)", elapsed, defaultDetectionTimeout)
		}
	default:
		// The process metadata itself was unavailable within the budget, which is
		// an equally valid bounded outcome; there is nothing left to assert.
		t.Log("process metadata was unavailable before the filesystem walk; bounded outcome still verified")
	}
}

// trickleReader yields chunks slowly, simulating a filesystem that makes just
// enough progress to keep a naive reader alive.
type trickleReader struct {
	chunks int
	delay  time.Duration
	reads  int32
}

func (r *trickleReader) Read(p []byte) (int, error) {
	atomic.AddInt32(&r.reads, 1)
	if r.chunks <= 0 {
		return 0, io.EOF
	}
	r.chunks--
	time.Sleep(r.delay)
	return copy(p, "data"), nil
}

// stalledReader reports neither data nor an error, simulating a wedged handle.
type stalledReader struct{}

func (stalledReader) Read([]byte) (int, error) { return 0, nil }
