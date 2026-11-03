package core

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// A preview token binds a destructive action to the exact process snapshot the
// caller observed. Two independent probes are not enough for safety: if a port's
// owner changes between the preview and the action, terminating "the process on
// this port" would silently target a process the caller never saw.
//
// Tokens therefore pin the port, PID, and process creation time, are signed with
// a per-server-process key, and expire quickly. A token minted by another process
// (or an earlier server run) cannot be replayed. Restored snapshots are still fed
// through the ordinary revalidation path, so any live drift fails the action
// instead of being redirected to the new occupant.

// Sentinel errors returned by RestorePreviewToken so callers can distinguish a
// malformed or foreign token from an expired one.
var (
	// ErrPreviewTokenInvalid reports a malformed, forged, truncated, or
	// foreign-process token.
	ErrPreviewTokenInvalid = errors.New("preview token is invalid")

	// ErrPreviewTokenExpired reports a well-formed token past its expiry.
	ErrPreviewTokenExpired = errors.New("preview token has expired")
)

// Preview scopes keep a single-port preview from authorizing a project-wide
// cleanup and vice versa.
const (
	PreviewScopeKill  = "kill"
	PreviewScopeClean = "clean"
)

const (
	// DefaultPreviewTTL bounds how long a preview stays actionable.
	DefaultPreviewTTL = 5 * time.Minute

	// maxPreviewTTL rejects unreasonably long requested lifetimes.
	maxPreviewTTL = 24 * time.Hour

	// maxPreviewTargets bounds the work a single token can authorize.
	maxPreviewTargets = 1024

	// maxPreviewTokenLength rejects absurd inputs before any decoding.
	maxPreviewTokenLength = 64 * 1024

	previewTokenVersion = 1
	previewNonceBytes   = 12
	previewKeyBytes     = 32
)

// previewTarget is the minimal identity a token must pin.
type previewTarget struct {
	Port           int    `json:"port"`
	PID            int32  `json:"pid"`
	ProcessCreated int64  `json:"created"`
	Protocol       string `json:"protocol,omitempty"`
	Status         string `json:"status,omitempty"`
	ProjectName    string `json:"project_name,omitempty"`
	ProjectPath    string `json:"project_path,omitempty"`
	ProjectVersion string `json:"project_version,omitempty"`
	Framework      string `json:"framework,omitempty"`
}

// previewPayload is the signed body of a token.
type previewPayload struct {
	Version   int             `json:"v"`
	Scope     string          `json:"scope"`
	IssuedAt  int64           `json:"iat"`
	ExpiresAt int64           `json:"exp"`
	Nonce     string          `json:"nonce"`
	Targets   []previewTarget `json:"targets"`
}

var (
	previewKeyOnce sync.Once
	previewKey     []byte
	previewKeyErr  error
)

// previewSigningKey lazily creates the per-process signing key.
func previewSigningKey() ([]byte, error) {
	previewKeyOnce.Do(func() {
		key := make([]byte, previewKeyBytes)
		if _, err := rand.Read(key); err != nil {
			previewKeyErr = fmt.Errorf("generate preview signing key: %w", err)
			return
		}
		previewKey = key
	})
	return previewKey, previewKeyErr
}

// setPreviewSigningKeyForTesting installs a deterministic key and returns a
// restore function. Tests use it to simulate a token issued by another process.
func setPreviewSigningKeyForTesting(key []byte) func() {
	previewKeyOnce.Do(func() {}) // Freeze lazy initialization so the swap is visible.
	previewKey = key
	previewKeyErr = nil
	return func() {
		previewKey = nil
		previewKeyErr = nil
		previewKeyOnce = sync.Once{}
	}
}

// previewTTL normalizes a requested lifetime.
func previewTTL(ttl time.Duration) (time.Duration, error) {
	if ttl <= 0 {
		return DefaultPreviewTTL, nil
	}
	if ttl > maxPreviewTTL {
		return 0, fmt.Errorf("preview lifetime %s exceeds the %s maximum", ttl, maxPreviewTTL)
	}
	return ttl, nil
}

// IssuePreviewToken pins the supplied snapshot into a signed, short-lived token.
// Only actionable identities are accepted: a port with a unique, confirmed owner.
func IssuePreviewToken(scope string, targets []PortDiagnostic, ttl time.Duration) (string, int64, error) {
	if err := validatePreviewScope(scope); err != nil {
		return "", 0, err
	}
	lifetime, err := previewTTL(ttl)
	if err != nil {
		return "", 0, err
	}
	if len(targets) == 0 {
		return "", 0, fmt.Errorf("a preview token requires at least one target")
	}
	if len(targets) > maxPreviewTargets {
		return "", 0, fmt.Errorf("a preview token covers at most %d targets, got %d", maxPreviewTargets, len(targets))
	}

	payload := previewPayload{Version: previewTokenVersion, Scope: scope}
	seen := make(map[int]struct{}, len(targets))
	for _, target := range targets {
		if err := validatePort(target.Port); err != nil {
			return "", 0, err
		}
		if _, duplicate := seen[target.Port]; duplicate {
			return "", 0, fmt.Errorf("duplicate port %d in preview snapshot", target.Port)
		}
		seen[target.Port] = struct{}{}
		if err := validateActionablePreviewTarget(target); err != nil {
			return "", 0, err
		}
		item := previewTarget{
			Port:           target.Port,
			PID:            target.PID,
			ProcessCreated: target.processCreated,
			Protocol:       target.Protocol,
			Status:         target.Status,
		}
		if target.Project != nil {
			item.ProjectName = target.Project.Name
			item.ProjectPath = target.Project.Path
			item.ProjectVersion = target.Project.Version
			item.Framework = target.Project.Framework
		}
		payload.Targets = append(payload.Targets, item)
	}
	sort.Slice(payload.Targets, func(i, j int) bool { return payload.Targets[i].Port < payload.Targets[j].Port })

	now := time.Now()
	payload.IssuedAt = now.Unix()
	payload.ExpiresAt = now.Add(lifetime).Unix()

	nonce := make([]byte, previewNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return "", 0, fmt.Errorf("generate preview nonce: %w", err)
	}
	payload.Nonce = base64.RawURLEncoding.EncodeToString(nonce)

	token, err := signPreviewPayload(&payload)
	if err != nil {
		return "", 0, err
	}
	return token, payload.ExpiresAt, nil
}

// validateActionablePreviewTarget rejects snapshots that can never be acted on
// safely, so an unusable preview fails at issue time rather than at kill time.
func validateActionablePreviewTarget(target PortDiagnostic) error {
	switch {
	case !target.IsOpen:
		return fmt.Errorf("port %d is free and cannot be previewed for termination", target.Port)
	case target.ambiguousOwner:
		return fmt.Errorf("port %d has multiple or unknown owners; refusing to issue a preview token", target.Port)
	case target.PID == 1 || target.PID < 0:
		return fmt.Errorf("refusing to preview protected system process (PID %d)", target.PID)
	case target.PID == 0 || target.Status == StatusPermissionRequired:
		return fmt.Errorf("unable to confirm the owner of port %d; refusing to issue a preview token", target.Port)
	case target.processCreated <= 0:
		return fmt.Errorf("unable to confirm the process creation time for port %d; refusing to issue a preview token", target.Port)
	}
	return nil
}

func validatePreviewScope(scope string) error {
	switch scope {
	case PreviewScopeKill, PreviewScopeClean:
		return nil
	default:
		return fmt.Errorf("unsupported preview scope %q", scope)
	}
}

// signPreviewPayload encodes and signs a payload.
func signPreviewPayload(payload *previewPayload) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode preview payload: %w", err)
	}
	key, err := previewSigningKey()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(body)
	signature := mac.Sum(nil)

	encoded := base64.RawURLEncoding
	return encoded.EncodeToString(body) + "." + encoded.EncodeToString(signature), nil
}

// RestorePreviewToken verifies a token and returns the pinned snapshots. When
// expectedPorts is non-nil the token must cover exactly that selection: a token
// is never silently widened or narrowed to fit the request.
func RestorePreviewToken(scope, token string, expectedPorts []int) ([]PortDiagnostic, int64, error) {
	if err := validatePreviewScope(scope); err != nil {
		return nil, 0, err
	}
	if len(token) == 0 {
		return nil, 0, fmt.Errorf("%w: a preview token is required", ErrPreviewTokenInvalid)
	}
	if len(token) > maxPreviewTokenLength {
		return nil, 0, fmt.Errorf("%w: token is too large", ErrPreviewTokenInvalid)
	}

	payload, err := openPreviewToken(token)
	if err != nil {
		return nil, 0, err
	}
	if payload.Scope != scope {
		return nil, 0, fmt.Errorf("%w: token was issued for %q, not %q", ErrPreviewTokenInvalid, payload.Scope, scope)
	}
	if err := matchPreviewSelection(expectedPorts, payload.Targets); err != nil {
		return nil, 0, err
	}

	targets := make([]PortDiagnostic, 0, len(payload.Targets))
	for _, item := range payload.Targets {
		diag := PortDiagnostic{
			Port:           item.Port,
			IsOpen:         true,
			Protocol:       item.Protocol,
			PID:            item.PID,
			Status:         item.Status,
			processCreated: item.ProcessCreated,
		}
		if diag.Protocol == "" {
			diag.Protocol = ProtocolTCP
		}
		if diag.Status == "" {
			diag.Status = StatusOccupied
		}
		if item.ProjectPath != "" {
			diag.Project = &ProjectInfo{
				Name:      item.ProjectName,
				Framework: item.Framework,
				Path:      item.ProjectPath,
				Version:   item.ProjectVersion,
			}
		}
		if err := validatePort(diag.Port); err != nil {
			return nil, 0, fmt.Errorf("%w: %v", ErrPreviewTokenInvalid, err)
		}
		targets = append(targets, diag)
	}
	return targets, payload.ExpiresAt, nil
}

// openPreviewToken splits, authenticates, and expiry-checks a token.
func openPreviewToken(token string) (*previewPayload, error) {
	separator := -1
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			if separator >= 0 {
				return nil, fmt.Errorf("%w: malformed token", ErrPreviewTokenInvalid)
			}
			separator = i
		}
	}
	if separator <= 0 || separator == len(token)-1 {
		return nil, fmt.Errorf("%w: malformed token", ErrPreviewTokenInvalid)
	}

	encoded := base64.RawURLEncoding
	body, err := encoded.DecodeString(token[:separator])
	if err != nil {
		return nil, fmt.Errorf("%w: undecodable payload", ErrPreviewTokenInvalid)
	}
	signature, err := encoded.DecodeString(token[separator+1:])
	if err != nil {
		return nil, fmt.Errorf("%w: undecodable signature", ErrPreviewTokenInvalid)
	}

	key, err := previewSigningKey()
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, fmt.Errorf("%w: signature mismatch (token came from another process or was modified)", ErrPreviewTokenInvalid)
	}

	var payload previewPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: undecodable body", ErrPreviewTokenInvalid)
	}
	if payload.Version != previewTokenVersion {
		return nil, fmt.Errorf("%w: unsupported token version %d", ErrPreviewTokenInvalid, payload.Version)
	}
	if len(payload.Targets) == 0 || len(payload.Targets) > maxPreviewTargets {
		return nil, fmt.Errorf("%w: token target count is out of range", ErrPreviewTokenInvalid)
	}
	if payload.ExpiresAt <= 0 {
		return nil, fmt.Errorf("%w: token has no expiry", ErrPreviewTokenInvalid)
	}
	if time.Now().Unix() >= payload.ExpiresAt {
		return nil, fmt.Errorf("%w at %s; preview the target again", ErrPreviewTokenExpired, time.Unix(payload.ExpiresAt, 0).UTC().Format(time.RFC3339))
	}
	return &payload, nil
}

// matchPreviewSelection enforces exact selection equality.
func matchPreviewSelection(expectedPorts []int, targets []previewTarget) error {
	if expectedPorts == nil {
		return nil
	}
	if len(expectedPorts) == 0 {
		return fmt.Errorf("%w: an explicit empty selection cannot be authorized", ErrPreviewTokenInvalid)
	}
	expected, err := normalizePorts(expectedPorts)
	if err != nil {
		return err
	}
	if len(expected) != len(targets) {
		return fmt.Errorf("%w: token covers %d target(s) but the request selected %d", ErrPreviewTokenInvalid, len(targets), len(expected))
	}
	for i, port := range expected {
		if targets[i].Port != port {
			return fmt.Errorf("%w: token does not cover the selected port %d", ErrPreviewTokenInvalid, port)
		}
	}
	return nil
}

// IssueKillPreview is a convenience wrapper for a single-port termination preview.
func IssueKillPreview(target PortDiagnostic, ttl time.Duration) (string, int64, error) {
	return IssuePreviewToken(PreviewScopeKill, []PortDiagnostic{target}, ttl)
}

// IssueCleanPreview is a convenience wrapper for a project-cleanup preview.
func IssueCleanPreview(targets []PortDiagnostic, ttl time.Duration) (string, int64, error) {
	return IssuePreviewToken(PreviewScopeClean, targets, ttl)
}
