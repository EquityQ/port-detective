package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseAndCompareVersions(t *testing.T) {
	v1, err := ParseVersion("1.4.0")
	if err != nil {
		t.Fatalf("ParseVersion(1.4.0) failed: %v", err)
	}
	v2, err := ParseVersion("v1.5.0")
	if err != nil {
		t.Fatalf("ParseVersion(v1.5.0) failed: %v", err)
	}
	dev, err := ParseVersion("1.5.0-dev")
	if err != nil {
		t.Fatalf("ParseVersion(1.5.0-dev) failed: %v", err)
	}
	alpha1, _ := ParseVersion("1.5.0-alpha.1")
	alpha2, _ := ParseVersion("1.5.0-alpha.2")
	beta, _ := ParseVersion("1.5.0-beta")

	if CompareVersions(v1, v2) >= 0 {
		t.Fatalf("expected 1.4.0 < 1.5.0")
	}
	if CompareVersions(v2, v1) <= 0 {
		t.Fatalf("expected 1.5.0 > 1.4.0")
	}
	if CompareVersions(dev, v2) >= 0 {
		t.Fatalf("expected 1.5.0-dev < 1.5.0")
	}
	if CompareVersions(alpha1, alpha2) >= 0 || CompareVersions(alpha2, beta) >= 0 {
		t.Fatalf("expected prerelease order alpha.1 < alpha.2 < beta")
	}
	if got := CompareVersions(v2, SemVer{Major: 1, Minor: 5, Patch: 0}); got != 0 {
		t.Fatalf("expected equal versions to return 0, got %d", got)
	}

	for _, bad := range []string{"", "v", "1.2", "1.2.3.4", "dev", "1.2.-1", "1.2.3-", "1.2.3+"} {
		if _, err := ParseVersion(bad); err == nil {
			t.Errorf("ParseVersion(%q) should fail", bad)
		}
	}

	norm, err := NormalizeTargetVersion("v1.5.0")
	if err != nil || norm != "1.5.0" {
		t.Fatalf("NormalizeTargetVersion(v1.5.0) = %q, %v", norm, err)
	}
	for _, badTarget := range []string{"", "1.5", "1.5.0-dev", "1.5.0+meta", "../1.5.0"} {
		if _, err := NormalizeTargetVersion(badTarget); err == nil {
			t.Errorf("NormalizeTargetVersion(%q) should reject invalid target", badTarget)
		}
	}
}

func TestAssetNameMatrix(t *testing.T) {
	cases := []struct {
		goos, goarch string
		archive, bin string
	}{
		{"linux", "amd64", "port-detective_1.5.0_linux_amd64.tar.gz", "port-detective"},
		{"linux", "arm64", "port-detective_1.5.0_linux_arm64.tar.gz", "port-detective"},
		{"darwin", "amd64", "port-detective_1.5.0_darwin_amd64.tar.gz", "port-detective"},
		{"darwin", "arm64", "port-detective_1.5.0_darwin_arm64.tar.gz", "port-detective"},
		{"windows", "amd64", "port-detective_1.5.0_windows_amd64.zip", "port-detective.exe"},
		{"windows", "arm64", "port-detective_1.5.0_windows_arm64.zip", "port-detective.exe"},
	}
	for _, tc := range cases {
		archive, bin, err := AssetName("1.5.0", tc.goos, tc.goarch)
		if err != nil {
			t.Fatalf("AssetName(%s/%s) failed: %v", tc.goos, tc.goarch, err)
		}
		if archive != tc.archive || bin != tc.bin {
			t.Fatalf("AssetName(%s/%s) = (%q, %q), want (%q, %q)", tc.goos, tc.goarch, archive, bin, tc.archive, tc.bin)
		}
	}
	if _, _, err := AssetName("1.5.0", "freebsd", "amd64"); err == nil {
		t.Fatal("unsupported platform should fail")
	}
}

func TestDetectInstallManager(t *testing.T) {
	dir := t.TempDir()

	standalone := filepath.Join(dir, "bin", "port-detective")
	if mgr, _ := DetectInstallManager(standalone); mgr != "" {
		t.Fatalf("expected standalone path not managed, got %q", mgr)
	}

	npmBin := filepath.Join(dir, "node_modules", "port-detective-cli-win32-x64", "bin", "port-detective.exe")
	mgr, hint := DetectInstallManager(npmBin)
	if mgr != "npm" || !strings.Contains(hint, "npm install") {
		t.Fatalf("expected npm manager detection, got (%q, %q)", mgr, hint)
	}

	pyDir := filepath.Join(dir, "cache", "port-detective", "bin", "1.5.0", "linux-amd64")
	if err := os.MkdirAll(pyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pyBin := filepath.Join(pyDir, "port-detective")
	if err := os.WriteFile(pyBin, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pyBin+".sha256", []byte("abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr, hint = DetectInstallManager(pyBin)
	if mgr != "python-sdk" || !strings.Contains(hint, "pip install") {
		t.Fatalf("expected python-sdk detection, got (%q, %q)", mgr, hint)
	}
}

type archiveEntry struct {
	name    string
	content []byte
}

func makeOrderedZip(t *testing.T, entries []archiveEntry, symlinkName string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, entry := range entries {
		w, err := zw.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entry.content); err != nil {
			t.Fatal(err)
		}
	}
	if symlinkName != "" {
		hdr := &zip.FileHeader{Name: symlinkName}
		hdr.SetMode(os.ModeSymlink | 0o777)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("/etc/passwd"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeOrderedTarGz(t *testing.T, entries []archiveEntry, symlinkName string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for _, entry := range entries {
		hdr := &tar.Header{
			Name:     entry.name,
			Mode:     0o755,
			Size:     int64(len(entry.content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(entry.content); err != nil {
			t.Fatal(err)
		}
	}
	if symlinkName != "" {
		hdr := &tar.Header{
			Name:     symlinkName,
			Mode:     0o777,
			Typeflag: tar.TypeSymlink,
			Linkname: "/etc/passwd",
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeTestZip(t *testing.T, entries map[string][]byte, symlinkName string) []byte {
	t.Helper()
	ordered := make([]archiveEntry, 0, len(entries))
	for name, content := range entries {
		ordered = append(ordered, archiveEntry{name: name, content: content})
	}
	return makeOrderedZip(t, ordered, symlinkName)
}

func makeTestTarGz(t *testing.T, entries map[string][]byte, symlinkName string) []byte {
	t.Helper()
	ordered := make([]archiveEntry, 0, len(entries))
	for name, content := range entries {
		ordered = append(ordered, archiveEntry{name: name, content: content})
	}
	return makeOrderedTarGz(t, ordered, symlinkName)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestCheckAndApplyEndToEndZipAndTarGz(t *testing.T) {
	newPayload := []byte("new-port-detective-binary-v1.6.0")
	zipAsset, zipBin, err := AssetName("1.6.0", "windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	tarAsset, tarBin, err := AssetName("1.6.0", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}

	zipBytes := makeTestZip(t, map[string][]byte{"prefix/" + zipBin: newPayload, "README.md": []byte("hi")}, "")
	tarBytes := makeTestTarGz(t, map[string][]byte{tarBin: newPayload, "README.md": []byte("hi")}, "")
	checksums := fmt.Sprintf("%s  %s\n%s  %s\n", sha256Hex(zipBytes), zipAsset, sha256Hex(tarBytes), tarAsset)

	var (
		authMu   sync.Mutex
		seenAuth string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authMu.Lock()
		seenAuth = r.Header.Get("Authorization")
		authMu.Unlock()
		switch r.URL.Path {
		case "/repos/EquityQ/port-detective/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v1.6.0","html_url":"https://example.test/v1.6.0"}`))
		case "/repos/EquityQ/port-detective/releases/tags/v1.5.0":
			_, _ = w.Write([]byte(`{"tag_name":"v1.5.0","html_url":"https://example.test/v1.5.0"}`))
		case "/EquityQ/port-detective/releases/download/v1.6.0/checksums.txt":
			_, _ = w.Write([]byte(checksums))
		case "/EquityQ/port-detective/releases/download/v1.6.0/" + zipAsset:
			_, _ = w.Write(zipBytes)
		case "/EquityQ/port-detective/releases/download/v1.6.0/" + tarAsset:
			_, _ = w.Write(tarBytes)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	u := New(5 * time.Second)
	u.APIBaseURL = srv.URL
	u.DownloadBaseURL = srv.URL
	u.AuthToken = "secret-token"
	u.VerifyBinary = func(_ context.Context, binaryPath, expectedVersion string) error {
		got, err := os.ReadFile(binaryPath)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, newPayload) || expectedVersion != "1.6.0" {
			return fmt.Errorf("unexpected payload or version %q", expectedVersion)
		}
		return nil
	}

	// Check update available + AuthToken forwarding.
	dir := t.TempDir()
	winTarget := filepath.Join(dir, "port-detective.exe")
	if err := os.WriteFile(winTarget, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	chk, err := u.Check(context.Background(), Options{
		CurrentVersion: "1.5.0-dev",
		ExecutablePath: winTarget,
		GOOS:           "windows",
		GOARCH:         "amd64",
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !chk.UpdateAvailable || chk.LatestVersion != "1.6.0" || chk.AssetName != zipAsset {
		t.Fatalf("unexpected Check result: %+v", chk)
	}
	authMu.Lock()
	gotAuth := seenAuth
	authMu.Unlock()
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("expected Authorization header forwarded, got %q", gotAuth)
	}

	// Apply zip update cleanly.
	res, err := u.Apply(context.Background(), Options{
		CurrentVersion: "1.5.0",
		ExecutablePath: winTarget,
		GOOS:           "windows",
		GOARCH:         "amd64",
	})
	if err != nil {
		t.Fatalf("Apply zip failed: %v", err)
	}
	if !res.Updated || res.Status != "updated" {
		t.Fatalf("expected updated result, got %+v", res)
	}
	got, _ := os.ReadFile(winTarget)
	if !bytes.Equal(got, newPayload) {
		t.Fatalf("target binary not replaced: %q", string(got))
	}

	// Apply tar.gz update cleanly.
	linTarget := filepath.Join(dir, "port-detective-linux")
	if err := os.WriteFile(linTarget, []byte("old-linux"), 0o755); err != nil {
		t.Fatal(err)
	}
	resLin, err := u.Apply(context.Background(), Options{
		CurrentVersion: "1.5.0",
		ExecutablePath: linTarget,
		GOOS:           "linux",
		GOARCH:         "amd64",
	})
	if err != nil || !resLin.Updated {
		t.Fatalf("Apply tar.gz failed: %+v, %v", resLin, err)
	}
	gotLin, _ := os.ReadFile(linTarget)
	if !bytes.Equal(gotLin, newPayload) {
		t.Fatalf("linux binary not replaced: %q", string(gotLin))
	}

	// Already up-to-date skips download without Force.
	upToDate, err := u.Apply(context.Background(), Options{
		CurrentVersion: "1.6.0",
		ExecutablePath: winTarget,
		GOOS:           "windows",
		GOARCH:         "amd64",
	})
	if err != nil || upToDate.Updated || upToDate.Status != "up_to_date" {
		t.Fatalf("expected up_to_date without Force, got %+v, %v", upToDate, err)
	}

	// Explicit TargetVersion allows downgrade checks.
	downCheck, err := u.Check(context.Background(), Options{
		CurrentVersion: "1.6.0",
		TargetVersion:  "v1.5.0",
		ExecutablePath: winTarget,
		GOOS:           "windows",
		GOARCH:         "amd64",
	})
	if err != nil || !downCheck.UpdateAvailable || downCheck.LatestVersion != "1.5.0" {
		t.Fatalf("expected explicit downgrade to 1.5.0 available, got %+v, %v", downCheck, err)
	}
}

func TestApplySafetyGuardsAndArchiveValidation(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "port-detective")
	if err := os.WriteFile(target, []byte("original-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	assetName, binName, _ := AssetName("1.6.0", runtime.GOOS, runtime.GOARCH)
	validArchive := makeTestTarGz(t, map[string][]byte{binName: []byte("new-bin")}, "")
	if runtime.GOOS == "windows" {
		validArchive = makeTestZip(t, map[string][]byte{binName: []byte("new-bin")}, "")
	}

	t.Run("checksum mismatch keeps original binary untouched", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/releases/latest"):
				_, _ = w.Write([]byte(`{"tag_name":"v1.6.0"}`))
			case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
				_, _ = w.Write([]byte(strings.Repeat("0", 64) + "  " + assetName + "\n"))
			default:
				_, _ = w.Write(validArchive)
			}
		}))
		defer srv.Close()

		u := New(5 * time.Second)
		u.APIBaseURL = srv.URL
		u.DownloadBaseURL = srv.URL
		if _, err := u.Apply(context.Background(), Options{CurrentVersion: "1.5.0", ExecutablePath: target}); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
			t.Fatalf("expected checksum mismatch error, got %v", err)
		}
		got, _ := os.ReadFile(target)
		if string(got) != "original-binary" {
			t.Fatalf("original binary was modified on checksum failure")
		}
	})

	t.Run("verification failure aborts swap", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/releases/latest"):
				_, _ = w.Write([]byte(`{"tag_name":"v1.6.0"}`))
			case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
				_, _ = w.Write([]byte(sha256Hex(validArchive) + "  " + assetName + "\n"))
			default:
				_, _ = w.Write(validArchive)
			}
		}))
		defer srv.Close()

		u := New(5 * time.Second)
		u.APIBaseURL = srv.URL
		u.DownloadBaseURL = srv.URL
		u.VerifyBinary = func(context.Context, string, string) error { return errors.New("corrupt binary") }
		if _, err := u.Apply(context.Background(), Options{CurrentVersion: "1.5.0", ExecutablePath: target}); err == nil || !strings.Contains(err.Error(), "verification") {
			t.Fatalf("expected verification error, got %v", err)
		}
		got, _ := os.ReadFile(target)
		if string(got) != "original-binary" {
			t.Fatalf("original binary was modified on verification failure")
		}
	})

	t.Run("managed install refuses without Force and proceeds with Force", func(t *testing.T) {
		npmPath := filepath.Join(dir, "node_modules", "port-detective-cli-linux-x64", "bin", binName)
		if err := os.MkdirAll(filepath.Dir(npmPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(npmPath, []byte("original-npm"), 0o755); err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/releases/latest"):
				_, _ = w.Write([]byte(`{"tag_name":"v1.6.0"}`))
			case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
				_, _ = w.Write([]byte(sha256Hex(validArchive) + "  " + assetName + "\n"))
			default:
				_, _ = w.Write(validArchive)
			}
		}))
		defer srv.Close()

		u := New(5 * time.Second)
		u.APIBaseURL = srv.URL
		u.DownloadBaseURL = srv.URL
		u.VerifyBinary = func(context.Context, string, string) error { return nil }

		res, err := u.Apply(context.Background(), Options{CurrentVersion: "1.5.0", ExecutablePath: npmPath})
		if !errors.Is(err, ErrManagedInstall) || res.Status != "managed_install" {
			t.Fatalf("expected ErrManagedInstall, got res=%+v err=%v", res, err)
		}
		res, err = u.Apply(context.Background(), Options{CurrentVersion: "1.5.0", ExecutablePath: npmPath, Force: true})
		if err != nil || !res.Updated {
			t.Fatalf("expected Force update to succeed, got res=%+v err=%v", res, err)
		}
	})

	t.Run("rejects traversal and symlink entries in zip and tar archives", func(t *testing.T) {
		for _, entries := range [][]archiveEntry{
			{{name: "port-detective.exe", content: []byte("bin")}, {name: "../evil", content: []byte("x")}},
			{{name: "../evil", content: []byte("x")}, {name: "port-detective.exe", content: []byte("bin")}},
		} {
			badZip := makeOrderedZip(t, entries, "")
			if _, err := extractBinary(badZip, "a.zip", "port-detective.exe"); err == nil || !strings.Contains(err.Error(), "traversal") {
				t.Fatalf("expected zip path traversal error for %+v, got %v", entries, err)
			}
		}
		dupZip := makeOrderedZip(t, []archiveEntry{
			{name: "port-detective.exe", content: []byte("a")},
			{name: "prefix/port-detective.exe", content: []byte("b")},
		}, "")
		if _, err := extractBinary(dupZip, "a.zip", "port-detective.exe"); err == nil || !strings.Contains(err.Error(), "multiple") {
			t.Fatalf("expected duplicate zip executable error, got %v", err)
		}
		symZip := makeTestZip(t, nil, "port-detective.exe")
		if _, err := extractBinary(symZip, "a.zip", "port-detective.exe"); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected zip symlink error, got %v", err)
		}

		for _, entries := range [][]archiveEntry{
			{{name: "port-detective", content: []byte("bin")}, {name: "../evil", content: []byte("x")}},
			{{name: "../evil", content: []byte("x")}, {name: "port-detective", content: []byte("bin")}},
		} {
			badTar := makeOrderedTarGz(t, entries, "")
			if _, err := extractBinary(badTar, "a.tar.gz", "port-detective"); err == nil || !strings.Contains(err.Error(), "traversal") {
				t.Fatalf("expected tar path traversal error for %+v, got %v", entries, err)
			}
		}
		dupTar := makeOrderedTarGz(t, []archiveEntry{
			{name: "port-detective", content: []byte("a")},
			{name: "prefix/port-detective", content: []byte("b")},
		}, "")
		if _, err := extractBinary(dupTar, "a.tar.gz", "port-detective"); err == nil || !strings.Contains(err.Error(), "multiple") {
			t.Fatalf("expected duplicate tar executable error, got %v", err)
		}
		symTar := makeTestTarGz(t, nil, "port-detective")
		if _, err := extractBinary(symTar, "a.tar.gz", "port-detective"); err == nil || !strings.Contains(err.Error(), "link") {
			t.Fatalf("expected tar symlink error, got %v", err)
		}
	})

	t.Run("replaceExecutable simulates Windows swap and rollback", func(t *testing.T) {
		wDir := t.TempDir()
		oldBin := filepath.Join(wDir, "port-detective.exe")
		newBin := filepath.Join(wDir, "staged.exe")
		if err := os.WriteFile(oldBin, []byte("v1"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(newBin, []byte("v2"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := replaceExecutable(newBin, oldBin, true); err != nil {
			t.Fatalf("Windows replaceExecutable failed: %v", err)
		}
		got, _ := os.ReadFile(oldBin)
		if string(got) != "v2" {
			t.Fatalf("expected replaced bytes v2, got %q", string(got))
		}

		// Rollback path when staged path does not exist.
		if err := replaceExecutable(filepath.Join(wDir, "missing.exe"), oldBin, true); err == nil {
			t.Fatal("expected error when tempPath is missing")
		}
		restored, _ := os.ReadFile(oldBin)
		if string(restored) != "v2" {
			t.Fatalf("expected rollback to preserve v2, got %q", string(restored))
		}
	})

	t.Run("HTTP errors and rate limits surface clear diagnostics", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "tags/v9.9.9") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
		}))
		defer srv.Close()

		u := New(5 * time.Second)
		u.APIBaseURL = srv.URL
		if _, err := u.Check(context.Background(), Options{CurrentVersion: "1.5.0"}); err == nil || !strings.Contains(err.Error(), "rate limit") {
			t.Fatalf("expected rate limit error, got %v", err)
		}
		if _, err := u.Check(context.Background(), Options{CurrentVersion: "1.5.0", TargetVersion: "9.9.9"}); err == nil || !strings.Contains(err.Error(), "404") {
			t.Fatalf("expected 404 error, got %v", err)
		}
	})
}
