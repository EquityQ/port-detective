package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultRepository      = "EquityQ/port-detective"
	DefaultAPIBaseURL      = "https://api.github.com"
	DefaultDownloadBaseURL = "https://github.com"
	DefaultTimeout         = 30 * time.Second
	MaxChecksumsBytes      = 1 * 1024 * 1024
	MaxArchiveBytes        = 100 * 1024 * 1024
	MaxBinaryBytes         = 50 * 1024 * 1024
)

// ErrManagedInstall indicates the active CLI binary lives inside an external package-manager cache or tree.
var ErrManagedInstall = errors.New("installation is managed by an external package manager")

// CheckResult describes release availability, install-manager ownership, and update outcome.
type CheckResult struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	Updated         bool   `json:"updated"`
	Status          string `json:"status"`
	AssetName       string `json:"asset_name,omitempty"`
	ReleaseURL      string `json:"release_url,omitempty"`
	ExecutablePath  string `json:"executable_path,omitempty"`
	ManagedBy       string `json:"managed_by,omitempty"`
	UpgradeHint     string `json:"upgrade_hint,omitempty"`
	Message         string `json:"message,omitempty"`
}

// Options configures update checking and installation.
type Options struct {
	CurrentVersion string
	TargetVersion  string
	ExecutablePath string
	Force          bool
	GOOS           string
	GOARCH         string
}

// Updater checks GitHub Releases and applies SHA-256-verified self-updates.
type Updater struct {
	Repository      string
	APIBaseURL      string
	DownloadBaseURL string
	HTTPClient      *http.Client
	AuthToken       string
	VerifyBinary    func(ctx context.Context, binaryPath, expectedVersion string) error
}

// New constructs an Updater using DefaultRepository and the given HTTP timeout.
func New(timeout time.Duration) *Updater {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Updater{
		Repository:      DefaultRepository,
		APIBaseURL:      DefaultAPIBaseURL,
		DownloadBaseURL: DefaultDownloadBaseURL,
		HTTPClient:      &http.Client{Timeout: timeout},
		AuthToken:       strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		VerifyBinary:    defaultVerifyBinary,
	}
}

// AssetName returns the expected release archive filename and enclosed executable name.
func AssetName(version, goos, goarch string) (string, string, error) {
	if strings.TrimSpace(version) == "" {
		return "", "", fmt.Errorf("release version must not be empty")
	}
	switch goos + "/" + goarch {
	case "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64":
		return fmt.Sprintf("port-detective_%s_%s_%s.tar.gz", version, goos, goarch), "port-detective", nil
	case "windows/amd64", "windows/arm64":
		return fmt.Sprintf("port-detective_%s_%s_%s.zip", version, goos, goarch), "port-detective.exe", nil
	default:
		return "", "", fmt.Errorf("unsupported self-update platform: %s/%s", goos, goarch)
	}
}

// DetectInstallManager reports whether execPath appears to be managed by npm or the Python SDK cache.
func DetectInstallManager(execPath string) (string, string) {
	if strings.TrimSpace(execPath) == "" {
		return "", ""
	}
	resolved := execPath
	if target, err := filepath.EvalSymlinks(execPath); err == nil && target != "" {
		resolved = target
	}
	normalized := strings.ToLower(filepath.ToSlash(resolved))
	if strings.Contains(normalized, "/node_modules/") && strings.Contains(normalized, "port-detective-cli-") {
		return "npm", "npm install -g port-detective@latest"
	}
	if strings.Contains(normalized, "/port-detective/bin/") {
		if info, err := os.Stat(resolved + ".sha256"); err == nil && !info.IsDir() {
			return "python-sdk", "python -m pip install --upgrade port-detective"
		}
	}
	return "", ""
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// Check queries GitHub Releases to determine whether an update is available.
func (u *Updater) Check(ctx context.Context, opts Options) (*CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opts.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	var (
		targetVer string
		explicit  bool
	)
	if strings.TrimSpace(opts.TargetVersion) != "" {
		normalized, err := NormalizeTargetVersion(opts.TargetVersion)
		if err != nil {
			return nil, err
		}
		targetVer = normalized
		explicit = true
	}

	// Validate platform compatibility before performing any network call when possible.
	checkVersion := targetVer
	if checkVersion == "" {
		checkVersion = "0.0.0"
	}
	if _, _, err := AssetName(checkVersion, goos, goarch); err != nil {
		return nil, err
	}

	rel, err := u.fetchRelease(ctx, targetVer)
	if err != nil {
		return nil, err
	}

	parsedTarget, err := ParseVersion(rel.TagName)
	if err != nil {
		return nil, fmt.Errorf("release tag %q is not a valid semantic version: %w", rel.TagName, err)
	}
	latest := parsedTarget.String()
	assetName, _, err := AssetName(latest, goos, goarch)
	if err != nil {
		return nil, err
	}

	current := strings.TrimSpace(opts.CurrentVersion)
	var updateAvailable bool
	if explicit {
		if parsedCurrent, err := ParseVersion(current); err == nil {
			updateAvailable = parsedCurrent.String() != latest
		} else {
			updateAvailable = true
		}
	} else {
		if parsedCurrent, err := ParseVersion(current); err == nil {
			updateAvailable = CompareVersions(parsedCurrent, parsedTarget) < 0
		} else {
			updateAvailable = true
		}
	}

	resolvedPath := resolveExecutableTarget(opts.ExecutablePath)
	managedBy, upgradeHint := DetectInstallManager(resolvedPath)

	status := "up_to_date"
	message := fmt.Sprintf("port-detective %s is already up to date", current)
	if updateAvailable {
		status = "update_available"
		message = fmt.Sprintf("update available: %s -> %s", current, latest)
	}
	releaseURL := rel.HTMLURL
	if releaseURL == "" {
		releaseURL = fmt.Sprintf("%s/%s/releases/tag/v%s", u.downloadBase(), u.repo(), latest)
	}

	return &CheckResult{
		CurrentVersion:  current,
		LatestVersion:   latest,
		UpdateAvailable: updateAvailable,
		Updated:         false,
		Status:          status,
		AssetName:       assetName,
		ReleaseURL:      releaseURL,
		ExecutablePath:  resolvedPath,
		ManagedBy:       managedBy,
		UpgradeHint:     upgradeHint,
		Message:         message,
	}, nil
}

// Apply checks for the target release, downloads the archive and checksums, verifies
// SHA-256 and executable integrity, and replaces the current binary atomically.
func (u *Updater) Apply(ctx context.Context, opts Options) (*CheckResult, error) {
	res, err := u.Check(ctx, opts)
	if err != nil {
		return nil, err
	}
	if res.ManagedBy != "" && !opts.Force {
		res.Status = "managed_install"
		res.Message = fmt.Sprintf("CLI is managed by %s; run %q (or pass --force to overwrite)", res.ManagedBy, res.UpgradeHint)
		return res, ErrManagedInstall
	}
	if !res.UpdateAvailable && !opts.Force {
		return res, nil
	}

	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opts.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	assetName, executableName, err := AssetName(res.LatestVersion, goos, goarch)
	if err != nil {
		return nil, err
	}

	targetPath := resolveExecutableTarget(opts.ExecutablePath)
	if targetPath == "" {
		return nil, fmt.Errorf("cannot resolve executable path for self-update")
	}

	baseURL := fmt.Sprintf("%s/%s/releases/download/v%s", u.downloadBase(), u.repo(), res.LatestVersion)
	checksumsData, err := u.downloadBytes(ctx, baseURL+"/checksums.txt", MaxChecksumsBytes)
	if err != nil {
		return nil, fmt.Errorf("download checksums.txt: %w", err)
	}
	expectedDigest, err := parseChecksum(checksumsData, assetName)
	if err != nil {
		return nil, err
	}

	archiveData, err := u.downloadBytes(ctx, baseURL+"/"+assetName, MaxArchiveBytes)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", assetName, err)
	}
	actualSum := sha256.Sum256(archiveData)
	actualDigest := hex.EncodeToString(actualSum[:])
	if subtle.ConstantTimeCompare([]byte(actualDigest), []byte(expectedDigest)) != 1 {
		return nil, fmt.Errorf("SHA-256 checksum mismatch for %s: expected %s, got %s", assetName, expectedDigest, actualDigest)
	}

	binaryData, err := extractBinary(archiveData, assetName, executableName)
	if err != nil {
		return nil, err
	}

	if err := u.installExtractedBinary(ctx, binaryData, targetPath, res.LatestVersion, goos == "windows"); err != nil {
		return nil, err
	}

	res.Updated = true
	res.Status = "updated"
	res.ExecutablePath = targetPath
	res.Message = fmt.Sprintf("Updated port-detective from %s to %s", res.CurrentVersion, res.LatestVersion)
	return res, nil
}

func (u *Updater) installExtractedBinary(ctx context.Context, binaryData []byte, targetPath, expectedVersion string, isWindows bool) error {
	targetDir := filepath.Dir(targetPath)
	pattern := ".port-detective-update-*"
	if isWindows {
		pattern = ".port-detective-update-*.exe"
	}

	tmpFile, err := os.CreateTemp(targetDir, pattern)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("cannot write update in %s: %w (check directory permissions or run with elevated privileges)", targetDir, err)
		}
		return fmt.Errorf("create staging binary in %s: %w", targetDir, err)
	}
	tempPath := tmpFile.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := tmpFile.Write(binaryData); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write staging binary: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync staging binary: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close staging binary: %w", err)
	}

	mode := os.FileMode(0o755)
	if info, statErr := os.Stat(targetPath); statErr == nil {
		mode = info.Mode().Perm()
		if !isWindows {
			mode |= 0o100
		}
	}
	if err := os.Chmod(tempPath, mode); err != nil {
		return fmt.Errorf("set permissions on staging binary: %w", err)
	}

	verify := u.VerifyBinary
	if verify == nil {
		verify = defaultVerifyBinary
	}
	if err := verify(ctx, tempPath, expectedVersion); err != nil {
		return fmt.Errorf("staged binary failed verification: %w", err)
	}

	if err := replaceExecutable(tempPath, targetPath, isWindows); err != nil {
		return err
	}
	cleanupTemp = false
	return nil
}

func replaceExecutable(tempPath, targetPath string, isWindows bool) error {
	if !isWindows {
		if err := os.Rename(tempPath, targetPath); err != nil {
			if os.IsPermission(err) {
				return fmt.Errorf("cannot replace %s: %w (check directory permissions or use sudo)", targetPath, err)
			}
			return fmt.Errorf("replace %s: %w", targetPath, err)
		}
		return nil
	}

	oldPath := targetPath + ".old"
	_ = os.Remove(oldPath)
	if err := os.Rename(targetPath, oldPath); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("cannot stage previous binary %s: %w (check directory permissions)", targetPath, err)
		}
		return fmt.Errorf("move current binary to %s: %w", oldPath, err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Rename(oldPath, targetPath)
		if os.IsPermission(err) {
			return fmt.Errorf("cannot install updated binary %s: %w (check directory permissions)", targetPath, err)
		}
		return fmt.Errorf("activate updated binary %s: %w", targetPath, err)
	}
	_ = os.Remove(oldPath)
	return nil
}

func defaultVerifyBinary(ctx context.Context, binaryPath, expectedVersion string) error {
	verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(verifyCtx, binaryPath, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s --version failed: %w (%s)", filepath.Base(binaryPath), err, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), expectedVersion) {
		return fmt.Errorf("%s --version output %q does not match %q", filepath.Base(binaryPath), strings.TrimSpace(string(out)), expectedVersion)
	}
	return nil
}

func (u *Updater) fetchRelease(ctx context.Context, targetVersion string) (githubRelease, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/releases/latest", u.apiBase(), u.repo())
	if targetVersion != "" {
		endpoint = fmt.Sprintf("%s/repos/%s/releases/tags/v%s", u.apiBase(), u.repo(), targetVersion)
	}

	body, err := u.doGet(ctx, endpoint, "application/vnd.github+json", MaxChecksumsBytes)
	if err != nil {
		return githubRelease{}, err
	}
	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return githubRelease{}, fmt.Errorf("parse release metadata: %w", err)
	}
	if strings.TrimSpace(rel.TagName) == "" {
		if targetVersion != "" {
			rel.TagName = "v" + targetVersion
		} else {
			return githubRelease{}, fmt.Errorf("GitHub release metadata omitted tag_name")
		}
	}
	if rel.Draft {
		return githubRelease{}, fmt.Errorf("release %s is marked as draft", rel.TagName)
	}
	return rel, nil
}

func (u *Updater) downloadBytes(ctx context.Context, url string, maxBytes int64) ([]byte, error) {
	return u.doGet(ctx, url, "application/octet-stream", maxBytes)
}

func (u *Updater) doGet(ctx context.Context, url, accept string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "port-detective-updater")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if strings.TrimSpace(u.AuthToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(u.AuthToken))
	}

	resp, err := u.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errSnippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		msg := strings.TrimSpace(string(errSnippet))
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("release resource not found (%s): HTTP 404", url)
		}
		if (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) && strings.Contains(strings.ToLower(msg), "rate limit") {
			return nil, fmt.Errorf("GitHub API rate limit exceeded (%s); set GITHUB_TOKEN or retry later", resp.Status)
		}
		if msg != "" {
			return nil, fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, url, msg)
		}
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response from %s: %w", url, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("response from %s exceeds %d byte limit", url, maxBytes)
	}
	return data, nil
}

func parseChecksum(data []byte, assetName string) (string, error) {
	lines := strings.Split(string(data), "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		digest := strings.ToLower(fields[0])
		if len(digest) != 64 {
			continue
		}
		if _, err := hex.DecodeString(digest); err != nil {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		name = strings.ReplaceAll(name, "\\", "/")
		if path.Base(name) == assetName {
			return digest, nil
		}
	}
	return "", fmt.Errorf("checksums.txt does not contain an entry for %s", assetName)
}

func extractBinary(archiveData []byte, archiveName, executableName string) ([]byte, error) {
	if strings.HasSuffix(archiveName, ".zip") {
		return extractFromZip(archiveData, executableName)
	}
	return extractFromTarGz(archiveData, executableName)
}

func validateArchiveEntryPath(rawName string) ([]string, error) {
	slash := strings.ReplaceAll(rawName, "\\", "/")
	if strings.HasPrefix(slash, "/") {
		return nil, fmt.Errorf("unsafe absolute path in release archive: %q", rawName)
	}
	parts := strings.Split(strings.TrimSuffix(slash, "/"), "/")
	for _, seg := range parts {
		if seg == ".." {
			return nil, fmt.Errorf("unsafe path traversal in release archive: %q", rawName)
		}
	}
	return parts, nil
}

func extractFromZip(archiveData []byte, executableName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archiveData), int64(len(archiveData)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip release archive: %w", err)
	}
	var found []byte
	for _, f := range zr.File {
		parts, err := validateArchiveEntryPath(f.Name)
		if err != nil {
			return nil, err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			if len(parts) > 0 && parts[len(parts)-1] == executableName {
				return nil, fmt.Errorf("unsafe symlink for executable in release archive: %q", f.Name)
			}
			continue
		}
		if len(parts) <= 2 && parts[len(parts)-1] == executableName {
			if found != nil {
				return nil, fmt.Errorf("release archive contains multiple %s entries", executableName)
			}
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open %s in zip archive: %w", f.Name, err)
			}
			data, readErr := io.ReadAll(io.LimitReader(rc, MaxBinaryBytes+1))
			_ = rc.Close()
			if readErr != nil {
				return nil, fmt.Errorf("read %s from zip archive: %w", f.Name, readErr)
			}
			if len(data) == 0 || len(data) > MaxBinaryBytes {
				return nil, fmt.Errorf("extracted binary %s has invalid size %d bytes", executableName, len(data))
			}
			found = data
		}
	}
	if found == nil {
		return nil, fmt.Errorf("release archive does not contain %s", executableName)
	}
	return found, nil
}

func extractFromTarGz(archiveData []byte, executableName string) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		return nil, fmt.Errorf("invalid gzip release archive: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var found []byte
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid tar release archive: %w", err)
		}
		parts, err := validateArchiveEntryPath(hdr.Name)
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			if len(parts) > 0 && parts[len(parts)-1] == executableName {
				return nil, fmt.Errorf("unsafe link for executable in release archive: %q", hdr.Name)
			}
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if len(parts) <= 2 && parts[len(parts)-1] == executableName {
			if found != nil {
				return nil, fmt.Errorf("release archive contains multiple %s entries", executableName)
			}
			data, readErr := io.ReadAll(io.LimitReader(tr, MaxBinaryBytes+1))
			if readErr != nil {
				return nil, fmt.Errorf("read %s from tar archive: %w", hdr.Name, readErr)
			}
			if len(data) == 0 || len(data) > MaxBinaryBytes {
				return nil, fmt.Errorf("extracted binary %s has invalid size %d bytes", executableName, len(data))
			}
			found = data
		}
	}
	if found == nil {
		return nil, fmt.Errorf("release archive does not contain %s", executableName)
	}
	return found, nil
}

func resolveExecutableTarget(execPath string) string {
	trimmed := strings.TrimSpace(execPath)
	if trimmed == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(trimmed); err == nil && resolved != "" {
		return resolved
	}
	return trimmed
}

func (u *Updater) repo() string {
	if strings.TrimSpace(u.Repository) != "" {
		return strings.TrimSpace(u.Repository)
	}
	return DefaultRepository
}

func (u *Updater) apiBase() string {
	base := strings.TrimRight(strings.TrimSpace(u.APIBaseURL), "/")
	if base != "" {
		return base
	}
	return DefaultAPIBaseURL
}

func (u *Updater) downloadBase() string {
	base := strings.TrimRight(strings.TrimSpace(u.DownloadBaseURL), "/")
	if base != "" {
		return base
	}
	return DefaultDownloadBaseURL
}

func (u *Updater) client() *http.Client {
	if u.HTTPClient != nil {
		return u.HTTPClient
	}
	return &http.Client{Timeout: DefaultTimeout}
}
