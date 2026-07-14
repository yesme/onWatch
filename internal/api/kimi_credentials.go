package api

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Kimi Code OAuth client ID (public; same as kimi-cli / kimi-code).
const KimiCodeClientID = "17e5f671-d194-4dfb-9706-5516cb48c098"

// DefaultOAuth/API hosts. Overridable via env for testing.
const (
	DefaultKimiOAuthHost = "https://auth.kimi.com"
	DefaultKimiCodeBase  = "https://api.kimi.com/coding/v1"
)

// KimiCredentials is the on-disk OAuth payload from kimi-code / kimi-cli.
type KimiCredentials struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	TokenType    string  `json:"token_type"`
	Scope        string  `json:"scope"`
	ExpiresAt    float64 `json:"expires_at"` // unix seconds (float in some writers)
	ExpiresIn    float64 `json:"expires_in"`
	// Path is the file credentials were loaded from (not serialized).
	Path string `json:"-"`
}

// Expired reports whether the access token is past expires_at (with a 60s skew).
func (c *KimiCredentials) Expired() bool {
	if c == nil || c.AccessToken == "" {
		return true
	}
	if c.ExpiresAt <= 0 {
		return false
	}
	return time.Now().Unix() >= int64(c.ExpiresAt)-60
}

// KimiCredentialsPath returns the preferred credentials file path.
// Order: KIMI_CODE_CREDENTIALS, ~/.kimi-code/credentials/kimi-code.json,
// ~/.kimi/credentials/kimi-code.json (legacy pre-migration).
func KimiCredentialsPath() string {
	if p := os.Getenv("KIMI_CODE_CREDENTIALS"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	primary := filepath.Join(home, ".kimi-code", "credentials", "kimi-code.json")
	if _, err := os.Stat(primary); err == nil {
		return primary
	}
	legacy := filepath.Join(home, ".kimi", "credentials", "kimi-code.json")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	// Default write/read location for kimi-code even if missing yet.
	return primary
}

// DetectKimiCredentials loads local Kimi Code OAuth credentials if present.
func DetectKimiCredentials(logger *slog.Logger) *KimiCredentials {
	if logger == nil {
		logger = slog.Default()
	}
	path := KimiCredentialsPath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var creds KimiCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		logger.Debug("kimi: failed to parse credentials", "path", path, "error", err)
		return nil
	}
	if creds.AccessToken == "" && creds.RefreshToken == "" {
		return nil
	}
	creds.Path = path
	return &creds
}

// SaveKimiCredentials writes credentials back to disk (after refresh).
func SaveKimiCredentials(creds *KimiCredentials) error {
	if creds == nil {
		return nil
	}
	path := creds.Path
	if path == "" {
		path = KimiCredentialsPath()
	}
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// Preserve fields we manage; write compact JSON.
	out := map[string]interface{}{
		"access_token":  creds.AccessToken,
		"refresh_token": creds.RefreshToken,
		"token_type":    creds.TokenType,
		"scope":         creds.Scope,
		"expires_at":    creds.ExpiresAt,
		"expires_in":    creds.ExpiresIn,
	}
	if out["token_type"] == "" {
		out["token_type"] = "Bearer"
	}
	if out["scope"] == "" {
		out["scope"] = "kimi-code"
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// tokenCache avoids hammering the filesystem on every poll when tokens are fresh.
var (
	kimiCredMu    sync.Mutex
	kimiCredCache *KimiCredentials
	kimiCredAt    time.Time
)

// LoadKimiCredentialsCached returns credentials, re-reading disk at most every 30s
// unless force is true.
func LoadKimiCredentialsCached(logger *slog.Logger, force bool) *KimiCredentials {
	kimiCredMu.Lock()
	defer kimiCredMu.Unlock()
	if !force && kimiCredCache != nil && time.Since(kimiCredAt) < 30*time.Second {
		// shallow copy
		cp := *kimiCredCache
		return &cp
	}
	creds := DetectKimiCredentials(logger)
	kimiCredCache = creds
	kimiCredAt = time.Now()
	if creds == nil {
		return nil
	}
	cp := *creds
	return &cp
}
