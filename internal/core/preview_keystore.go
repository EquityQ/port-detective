package core

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// PreviewKeyFileEnv overrides the local user-scoped CLI signing key path.
	PreviewKeyFileEnv = "PORT_DETECTIVE_PREVIEW_KEY_FILE"

	cliPreviewKeyTTL = 24 * time.Hour
	maxKeyFileBytes  = 4096
)

type persistedPreviewKey struct {
	Version   int    `json:"v"`
	Key       string `json:"key"`
	ExpiresAt int64  `json:"expires_at"`
}

// IssueCLIPreviewToken issues a preview token signed with the user-scoped local
// CLI key so a preview minted by one CLI invocation can be redeemed by a
// subsequent CLI invocation within its TTL. MCP servers continue to use
// IssuePreviewToken (in-memory per-process key) for strict process isolation.
func IssueCLIPreviewToken(scope string, targets []PortDiagnostic, ttl time.Duration) (string, int64, error) {
	key, err := loadOrCreateCLIPreviewKey(true)
	if err != nil {
		return "", 0, err
	}
	return issuePreviewTokenWithKey(scope, targets, ttl, key)
}

// RestoreCLIPreviewToken validates a preview token across CLI invocations using
// the user-scoped local CLI key, falling back to an initialized in-process key
// only when the token was minted in the current process.
func RestoreCLIPreviewToken(scope, token string, expectedPorts []int) ([]PortDiagnostic, int64, error) {
	key, keyErr := loadOrCreateCLIPreviewKey(false)
	if keyErr == nil {
		targets, expiresAt, err := restorePreviewTokenWithKey(scope, token, expectedPorts, key)
		if err == nil || errors.Is(err, ErrPreviewTokenExpired) {
			return targets, expiresAt, err
		}
		// If the local key did not match and an in-process key exists, allow
		// tokens minted directly via IssuePreviewToken in the same process.
		if inProc := currentInProcessKey(); len(inProc) >= previewKeyBytes {
			if t2, exp2, err2 := restorePreviewTokenWithKey(scope, token, expectedPorts, inProc); err2 == nil || errors.Is(err2, ErrPreviewTokenExpired) {
				return t2, exp2, err2
			}
		}
		return nil, 0, err
	}
	if inProc := currentInProcessKey(); len(inProc) >= previewKeyBytes {
		return restorePreviewTokenWithKey(scope, token, expectedPorts, inProc)
	}
	return nil, 0, fmt.Errorf("%w: no active CLI preview signing key (%v)", ErrPreviewTokenInvalid, keyErr)
}

func currentInProcessKey() []byte {
	previewKeyMu.RLock()
	defer previewKeyMu.RUnlock()
	return previewKey
}

func cliPreviewKeyPath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv(PreviewKeyFileEnv)); configured != "" {
		return filepath.Clean(configured), nil
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(cacheDir) == "" {
		cacheDir = os.TempDir()
	}
	return filepath.Join(cacheDir, "port-detective", "preview-key.json"), nil
}

func loadOrCreateCLIPreviewKey(createIfMissing bool) ([]byte, error) {
	path, err := cliPreviewKeyPath()
	if err != nil {
		return nil, err
	}
	if key, ok := readValidCLIPreviewKey(path); ok {
		return key, nil
	}
	if !createIfMissing {
		return nil, errors.New("CLI preview key missing, expired, or unreadable")
	}
	return writeFreshCLIPreviewKey(path)
}

func readValidCLIPreviewKey(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxKeyFileBytes {
		return nil, false
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var record persistedPreviewKey
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, false
	}
	if record.Version != previewTokenVersion || record.ExpiresAt <= time.Now().Unix() {
		return nil, false
	}
	key, err := base64.RawURLEncoding.DecodeString(record.Key)
	if err != nil || len(key) != previewKeyBytes {
		return nil, false
	}
	return key, true
}

func writeFreshCLIPreviewKey(path string) ([]byte, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create preview key directory: %w", err)
	}
	key := make([]byte, previewKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate CLI preview key: %w", err)
	}
	record := persistedPreviewKey{
		Version:   previewTokenVersion,
		Key:       base64.RawURLEncoding.EncodeToString(key),
		ExpiresAt: time.Now().Add(cliPreviewKeyTTL).Unix(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode CLI preview key: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".preview-key-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create temp preview key: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if runtime.GOOS != "windows" {
		if err := tmp.Chmod(0o600); err != nil {
			_ = tmp.Close()
			return nil, fmt.Errorf("restrict preview key permissions: %w", err)
		}
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("write CLI preview key: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close CLI preview key: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return nil, fmt.Errorf("install CLI preview key: %w", err)
	}
	cleanup = false
	return key, nil
}
