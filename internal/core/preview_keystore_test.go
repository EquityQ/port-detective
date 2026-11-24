package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCLIPreviewTokenPersistsAcrossProcesses(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "preview-key.json")
	t.Setenv(PreviewKeyFileEnv, keyFile)

	target := actionablePreview(4123, 8888, 1700000001000)
	token, expiresAt, err := IssueCLIPreviewToken(PreviewScopeKill, []PortDiagnostic{target}, time.Minute)
	if err != nil {
		t.Fatalf("IssueCLIPreviewToken failed: %v", err)
	}
	if token == "" || expiresAt <= time.Now().Unix() {
		t.Fatalf("invalid token or expiry: %q / %d", token, expiresAt)
	}

	// Simulate a brand-new OS process with a fresh in-memory key.
	restore := setPreviewSigningKeyForTesting(nil)
	defer restore()

	restored, gotExp, err := RestoreCLIPreviewToken(PreviewScopeKill, token, []int{4123})
	if err != nil {
		t.Fatalf("RestoreCLIPreviewToken failed across processes: %v", err)
	}
	if gotExp != expiresAt || len(restored) != 1 || restored[0].PID != 8888 {
		t.Fatalf("unexpected restored snapshot: %+v (exp=%d)", restored, gotExp)
	}

	// Cross-scope and wrong-port tokens must still be rejected.
	if _, _, err := RestoreCLIPreviewToken(PreviewScopeClean, token, []int{4123}); !errors.Is(err, ErrPreviewTokenInvalid) {
		t.Fatalf("cross-scope restore error = %v, want ErrPreviewTokenInvalid", err)
	}
	if _, _, err := RestoreCLIPreviewToken(PreviewScopeKill, token, []int{4124}); !errors.Is(err, ErrPreviewTokenInvalid) {
		t.Fatalf("wrong-port restore error = %v, want ErrPreviewTokenInvalid", err)
	}
}

func TestCLIPreviewKeyRotatesWhenExpiredAndRejectsMissing(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "preview-key.json")
	t.Setenv(PreviewKeyFileEnv, keyFile)

	restore := setPreviewSigningKeyForTesting(nil)
	defer restore()

	// Without a key file, verification must fail closed and never create a key.
	if _, _, err := RestoreCLIPreviewToken(PreviewScopeKill, "abc.def", []int{3000}); !errors.Is(err, ErrPreviewTokenInvalid) {
		t.Fatalf("missing key error = %v, want ErrPreviewTokenInvalid", err)
	}
	if _, err := os.Stat(keyFile); !os.IsNotExist(err) {
		t.Fatalf("verification must not create key file, stat err=%v", err)
	}

	// Write an expired key file and confirm IssueCLIPreviewToken rotates it.
	expired := persistedPreviewKey{
		Version:   previewTokenVersion,
		Key:       "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		ExpiresAt: time.Now().Add(-time.Hour).Unix(),
	}
	raw, _ := json.Marshal(expired)
	if err := os.WriteFile(keyFile, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	target := actionablePreview(3000, 1234, 1700000002000)
	token, _, err := IssueCLIPreviewToken(PreviewScopeKill, []PortDiagnostic{target}, time.Minute)
	if err != nil {
		t.Fatalf("IssueCLIPreviewToken should rotate expired key: %v", err)
	}
	if _, _, err := RestoreCLIPreviewToken(PreviewScopeKill, token, []int{3000}); err != nil {
		t.Fatalf("token issued after rotation failed verification: %v", err)
	}
}

func TestCLIPreviewFallbackAllowsInProcessToken(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "preview-key.json")
	t.Setenv(PreviewKeyFileEnv, keyFile)

	target := actionablePreview(3001, 4321, 1700000003000)
	token, _, err := IssueKillPreview(target, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	restored, _, err := RestoreCLIPreviewToken(PreviewScopeKill, token, []int{3001})
	if err != nil {
		t.Fatalf("in-process token should remain restorable via CLI helper: %v", err)
	}
	if len(restored) != 1 || restored[0].PID != 4321 {
		t.Fatalf("unexpected restored target: %+v", restored)
	}
}
