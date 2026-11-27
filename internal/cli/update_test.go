package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/EquityQ/port-detective/internal/updater"
	"github.com/EquityQ/port-detective/internal/version"
)

func buildTestArchive(t *testing.T, assetName, binName string, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if strings.HasSuffix(assetName, ".zip") {
		zw := zip.NewWriter(&buf)
		w, err := zw.Create(binName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	if err := tw.WriteHeader(&tar.Header{
		Name:     binName,
		Mode:     0o755,
		Size:     int64(len(payload)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func setupUpdateServerAndHooks(t *testing.T, latestTag string, targetPath string, payload []byte) {
	t.Helper()
	stripped := strings.TrimPrefix(latestTag, "v")
	assetName, binName, err := updater.AssetName(stripped, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	archiveBytes := buildTestArchive(t, assetName, binName, payload)
	sum := sha256.Sum256(archiveBytes)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			_, _ = w.Write([]byte(fmt.Sprintf(`{"tag_name":%q,"html_url":"https://example.test/%s"}`, latestTag, latestTag)))
		case strings.HasSuffix(r.URL.Path, "/releases/tags/"+latestTag):
			_, _ = w.Write([]byte(fmt.Sprintf(`{"tag_name":%q,"html_url":"https://example.test/%s"}`, latestTag, latestTag)))
		case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
			_, _ = w.Write([]byte(checksums))
		case strings.HasSuffix(r.URL.Path, "/"+assetName):
			_, _ = w.Write(archiveBytes)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	restore := setUpdaterHooksForTesting(
		func(timeout time.Duration) *updater.Updater {
			u := updater.New(timeout)
			u.APIBaseURL = srv.URL
			u.DownloadBaseURL = srv.URL
			u.VerifyBinary = func(_ context.Context, binaryPath, expectedVersion string) error {
				got, err := os.ReadFile(binaryPath)
				if err != nil {
					return err
				}
				if !bytes.Equal(got, payload) {
					return fmt.Errorf("unexpected payload %q", string(got))
				}
				return nil
			}
			return u
		},
		func() (string, error) {
			return targetPath, nil
		},
	)
	t.Cleanup(restore)
}

func TestUpdateCommandCheckAndApply(t *testing.T) {
	prevVer := version.Value
	version.Value = "1.5.0"
	t.Cleanup(func() { version.Value = prevVer })

	dir := t.TempDir()
	binPath := filepath.Join(dir, "port-detective")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	if err := os.WriteFile(binPath, []byte("old-cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	newPayload := []byte("new-cli-1.6.0")
	setupUpdateServerAndHooks(t, "v1.6.0", binPath, newPayload)

	// 1. --check --json reports update available without replacing binary.
	out, _, err := captureRun(t, "update", "--check", "--json")
	if err != nil {
		t.Fatalf("update --check --json failed: %v", err)
	}
	var chk updater.CheckResult
	if err := json.Unmarshal([]byte(out), &chk); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if !chk.UpdateAvailable || chk.Updated || chk.LatestVersion != "1.6.0" {
		t.Fatalf("unexpected check JSON: %+v", chk)
	}
	if cur, _ := os.ReadFile(binPath); string(cur) != "old-cli" {
		t.Fatalf("check modified binary")
	}

	// 2. --json without --yes must fail with confirmation_required (exit status 2).
	_, _, err = captureRun(t, "update", "--json")
	var cmdErr *commandError
	if !errors.As(err, &cmdErr) || cmdErr.status != 2 || cmdErr.code != "confirmation_required" {
		t.Fatalf("expected confirmation_required exit 2, got %v", err)
	}

	// 3. Interactive decline leaves binary untouched and returns nil.
	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetIn(strings.NewReader("n\n"))
	root.SetArgs([]string{"update"})
	if err := root.Execute(); err != nil {
		t.Fatalf("canceled update returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "Update canceled") {
		t.Fatalf("expected cancel message, got %q", buf.String())
	}
	if cur, _ := os.ReadFile(binPath); string(cur) != "old-cli" {
		t.Fatalf("declined update modified binary")
	}

	// 4. --yes --json applies verified update and replaces binary.
	out, _, err = captureRun(t, "update", "--yes", "--json")
	if err != nil {
		t.Fatalf("update --yes --json failed: %v", err)
	}
	var applied updater.CheckResult
	if err := json.Unmarshal([]byte(out), &applied); err != nil {
		t.Fatalf("invalid apply JSON: %v\n%s", err, out)
	}
	if !applied.Updated || applied.Status != "updated" || applied.LatestVersion != "1.6.0" {
		t.Fatalf("unexpected apply JSON: %+v", applied)
	}
	if cur, _ := os.ReadFile(binPath); !bytes.Equal(cur, newPayload) {
		t.Fatalf("binary not updated, got %q", string(cur))
	}
}

func TestUpdateCommandInteractiveAndFlags(t *testing.T) {
	prevVer := version.Value
	version.Value = "1.5.0"
	t.Cleanup(func() { version.Value = prevVer })

	dir := t.TempDir()
	binPath := filepath.Join(dir, "port-detective")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	if err := os.WriteFile(binPath, []byte("old-cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	newPayload := []byte("cli-1.6.0")
	setupUpdateServerAndHooks(t, "v1.6.0", binPath, newPayload)

	// Invalid arguments must fail with usageError (exit status 2).
	for _, args := range [][]string{
		{"update", "--timeout", "0s"},
		{"update", "--to-version", "../bad"},
		{"update", "--to-version", "1.6.0-dev"},
	} {
		_, _, err := captureRun(t, args...)
		var cmdErr *commandError
		if !errors.As(err, &cmdErr) || cmdErr.status != 2 {
			t.Fatalf("expected invalid_arguments exit 2 for %v, got %v", args, err)
		}
	}

	// Interactive yes applies update and renders human summary.
	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetIn(strings.NewReader("y\n"))
	root.SetArgs([]string{"update", "--to-version", "v1.6.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("interactive update failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Port Detective Updated") {
		t.Fatalf("expected terminal summary, got %q", buf.String())
	}

	// Up-to-date path in human output exits 0 cleanly without updating.
	version.Value = "1.6.0"
	out, _, err := captureRun(t, "update")
	if err != nil {
		t.Fatalf("up-to-date check failed: %v", err)
	}
	if !strings.Contains(out, "Up to date") {
		t.Fatalf("expected Up to date output, got %q", out)
	}
}

func TestUpdateCommandManagedInstallGuard(t *testing.T) {
	prevVer := version.Value
	version.Value = "1.5.0"
	t.Cleanup(func() { version.Value = prevVer })

	dir := t.TempDir()
	npmBin := filepath.Join(dir, "node_modules", "port-detective-cli-linux-x64", "bin", "port-detective")
	if runtime.GOOS == "windows" {
		npmBin += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(npmBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(npmBin, []byte("old-npm-cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	newPayload := []byte("forced-1.6.0")
	setupUpdateServerAndHooks(t, "v1.6.0", npmBin, newPayload)

	// --check human output shows upgrade hint.
	out, _, err := captureRun(t, "update", "--check")
	if err != nil {
		t.Fatalf("update --check failed: %v", err)
	}
	if !strings.Contains(out, "Managed by npm") {
		t.Fatalf("expected npm management hint, got %q", out)
	}

	// Non-forced update refuses managed installation.
	_, _, err = captureRun(t, "update", "--yes")
	var cmdErr *commandError
	if !errors.As(err, &cmdErr) || cmdErr.code != "managed_install" {
		t.Fatalf("expected managed_install error, got %v", err)
	}

	// --force overrides guard.
	if _, _, err := captureRun(t, "update", "--yes", "--force"); err != nil {
		t.Fatalf("update --yes --force failed: %v", err)
	}
	if cur, _ := os.ReadFile(npmBin); !bytes.Equal(cur, newPayload) {
		t.Fatalf("forced update did not replace binary")
	}
}
