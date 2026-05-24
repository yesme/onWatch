package hub

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

const tokenPrefix = "owt_"

// GenerateRawToken creates a new random agent token string.
func GenerateRawToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("hub.GenerateRawToken: %w", err)
	}
	return tokenPrefix + hex.EncodeToString(b), nil
}

// HashToken returns the SHA-256 hex digest of a raw token.
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// TokenDisplayPrefix returns the first 12 characters of a raw token for UI display (e.g. "owt_a1b2c3d4").
func TokenDisplayPrefix(raw string) string {
	if len(raw) > 12 {
		return raw[:12] + "..."
	}
	return raw
}

// TokenStore is the interface the hub needs for token persistence.
type TokenStore interface {
	CreateAgentToken(tokenHash, prefix, name, owner, scopes string, expiresAt *time.Time) (int64, error)
	GetAgentTokenByHash(tokenHash string) (*AgentToken, error)
	UpdateAgentTokenLastUsed(id int64) error
	ListAgentTokens() ([]AgentToken, error)
	RevokeAgentToken(id int64) error
	RevokeAgentTokenByName(name string) error
}

// ValidateToken checks a raw token against the store and returns the token record if valid.
func ValidateToken(store TokenStore, raw string) (*AgentToken, error) {
	hash := HashToken(raw)
	t, err := store.GetAgentTokenByHash(hash)
	if err != nil {
		return nil, fmt.Errorf("hub.ValidateToken: %w", err)
	}
	if t == nil {
		return nil, fmt.Errorf("hub.ValidateToken: invalid token")
	}
	if t.RevokedAt != nil {
		return nil, fmt.Errorf("hub.ValidateToken: token revoked")
	}
	if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
		return nil, fmt.Errorf("hub.ValidateToken: token expired")
	}
	return t, nil
}
