package hub

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateRawToken(t *testing.T) {
	tok, err := GenerateRawToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "owt_") {
		t.Errorf("expected owt_ prefix, got %q", tok)
	}
	// owt_ (4) + 48 hex chars = 52
	if len(tok) != 52 {
		t.Errorf("expected 52 chars, got %d", len(tok))
	}

	// Uniqueness
	tok2, _ := GenerateRawToken()
	if tok == tok2 {
		t.Error("two generated tokens should not be equal")
	}
}

func TestHashToken(t *testing.T) {
	hash := HashToken("owt_test123")
	if len(hash) != 64 {
		t.Errorf("expected 64-char hex SHA-256, got %d chars", len(hash))
	}

	// Deterministic
	hash2 := HashToken("owt_test123")
	if hash != hash2 {
		t.Error("same input should produce same hash")
	}

	// Different inputs produce different hashes
	hash3 := HashToken("owt_test456")
	if hash == hash3 {
		t.Error("different inputs should produce different hashes")
	}
}

func TestTokenDisplayPrefix(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"owt_abcdef1234567890", "owt_abcdef12..."},
		{"short", "short"},
		{"exactly12ch", "exactly12ch"},
		{"owt_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2", "owt_a1b2c3d4..."},
	}
	for _, tc := range cases {
		got := TokenDisplayPrefix(tc.in)
		if got != tc.want {
			t.Errorf("TokenDisplayPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// mockTokenStore implements TokenStore for testing.
type mockTokenStore struct {
	tokens map[string]*AgentToken
}

func newMockStore() *mockTokenStore {
	return &mockTokenStore{tokens: make(map[string]*AgentToken)}
}

func (m *mockTokenStore) CreateAgentToken(tokenHash, prefix, name, owner, scopes string, expiresAt *time.Time) (int64, error) {
	t := &AgentToken{
		ID:        int64(len(m.tokens) + 1),
		TokenHash: tokenHash,
		Prefix:    prefix,
		Name:      name,
		Owner:     owner,
		Scopes:    scopes,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}
	m.tokens[tokenHash] = t
	return t.ID, nil
}

func (m *mockTokenStore) GetAgentTokenByHash(tokenHash string) (*AgentToken, error) {
	t, ok := m.tokens[tokenHash]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (m *mockTokenStore) UpdateAgentTokenLastUsed(id int64) error { return nil }
func (m *mockTokenStore) ListAgentTokens() ([]AgentToken, error) { return nil, nil }
func (m *mockTokenStore) RevokeAgentToken(id int64) error        { return nil }
func (m *mockTokenStore) RevokeAgentTokenByName(name string) error { return nil }

func TestValidateToken_Valid(t *testing.T) {
	store := newMockStore()
	raw, _ := GenerateRawToken()
	hash := HashToken(raw)
	store.CreateAgentToken(hash, "p", "test", "", "sync", nil)

	tok, err := ValidateToken(store, raw)
	if err != nil {
		t.Fatal(err)
	}
	if tok == nil {
		t.Fatal("expected token")
	}
	if tok.Name != "test" {
		t.Errorf("expected name 'test', got %q", tok.Name)
	}
}

func TestValidateToken_NotFound(t *testing.T) {
	store := newMockStore()
	_, err := ValidateToken(store, "owt_nonexistent000000000000000000000000000000000000")
	if err == nil {
		t.Error("expected error for nonexistent token")
	}
}

func TestValidateToken_Revoked(t *testing.T) {
	store := newMockStore()
	raw, _ := GenerateRawToken()
	hash := HashToken(raw)
	store.CreateAgentToken(hash, "p", "revoked", "", "sync", nil)
	now := time.Now()
	store.tokens[hash].RevokedAt = &now

	_, err := ValidateToken(store, raw)
	if err == nil {
		t.Error("expected error for revoked token")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	store := newMockStore()
	raw, _ := GenerateRawToken()
	hash := HashToken(raw)
	past := time.Now().Add(-time.Hour)
	store.CreateAgentToken(hash, "p", "expired", "", "sync", &past)

	_, err := ValidateToken(store, raw)
	if err == nil {
		t.Error("expected error for expired token")
	}
}
