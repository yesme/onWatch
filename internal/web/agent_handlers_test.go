package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/onllm-dev/onwatch/v2/internal/config"
	"github.com/onllm-dev/onwatch/v2/internal/hub"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return NewHandler(s, nil, nil, nil, &config.Config{})
}

func TestAgentSync_EndToEnd(t *testing.T) {
	h := newTestHandler(t)

	// Create a token
	raw, err := hub.GenerateRawToken()
	if err != nil {
		t.Fatal(err)
	}
	tokenHash := hub.HashToken(raw)
	prefix := hub.TokenDisplayPrefix(raw)
	_, err = h.store.CreateAgentToken(tokenHash, prefix, "test-agent", "tester", "sync", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Build sync request
	syncReq := hub.SyncRequest{
		AgentID: "test-uuid-123",
		AgentInfo: hub.AgentInfo{
			Hostname:          "test-machine",
			OS:                "darwin",
			Arch:              "arm64",
			Version:           "1.0.0",
			Mode:              "full",
			DetectedProviders: []string{"anthropic", "codex"},
		},
		Credentials: []hub.CredentialPayload{
			{
				Provider:       "anthropic",
				CredentialType: "oauth",
				Data:           map[string]string{"access_token": "test-access-token"},
			},
		},
		CostLogs: []hub.CostLogPayload{
			{
				Provider:     "anthropic",
				Date:         "2026-05-03",
				InputTokens:  100000,
				OutputTokens: 50000,
				CostUSD:      12.50,
				ModelsUsed:   []string{"claude-opus-4-7"},
			},
		},
	}

	body, _ := json.Marshal(syncReq)

	// Test: sync with valid bearer token
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sync", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+raw)
	req.Header.Set("Content-Type", "application/json")

	// Apply middleware manually
	var handler http.Handler = http.HandlerFunc(h.AgentSync)
	handler = hub.AgentAuthMiddleware(h.store, handler)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp hub.SyncResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.CredentialsAccepted != 1 {
		t.Errorf("expected 1 credential accepted, got %d", resp.CredentialsAccepted)
	}
	if resp.CostLogsAccepted != 1 {
		t.Errorf("expected 1 cost log accepted, got %d", resp.CostLogsAccepted)
	}

	// Verify agent was registered
	agent, err := h.store.GetAgent("test-uuid-123")
	if err != nil {
		t.Fatal(err)
	}
	if agent == nil {
		t.Fatal("expected agent to be registered")
	}
	if agent.Hostname != "test-machine" {
		t.Errorf("expected hostname 'test-machine', got %q", agent.Hostname)
	}
	if agent.Status != "active" {
		t.Errorf("expected status 'active', got %q", agent.Status)
	}
}

func TestAgentSync_Unauthorized(t *testing.T) {
	h := newTestHandler(t)

	body, _ := json.Marshal(hub.SyncRequest{AgentID: "test"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sync", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer owt_invalid_token_000000000000000000000000000000000000000000000000")

	var handler http.Handler = http.HandlerFunc(h.AgentSync)
	handler = hub.AgentAuthMiddleware(h.store, handler)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAgentSync_NoToken(t *testing.T) {
	h := newTestHandler(t)

	body, _ := json.Marshal(hub.SyncRequest{AgentID: "test"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sync", bytes.NewReader(body))
	// No Authorization header

	var handler http.Handler = http.HandlerFunc(h.AgentSync)
	handler = hub.AgentAuthMiddleware(h.store, handler)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminTokens_CRUD(t *testing.T) {
	h := newTestHandler(t)

	// Create token via admin API
	createBody, _ := json.Marshal(map[string]string{"name": "admin-test", "owner": "admin"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tokens", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	h.AdminTokens(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var createResp map[string]string
	json.Unmarshal(w.Body.Bytes(), &createResp)
	if createResp["token"] == "" {
		t.Fatal("expected token in response")
	}
	if createResp["name"] != "admin-test" {
		t.Errorf("expected name 'admin-test', got %q", createResp["name"])
	}

	// List tokens
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/tokens", nil)
	w = httptest.NewRecorder()
	h.AdminTokens(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}

	var tokens []hub.AgentToken
	json.Unmarshal(w.Body.Bytes(), &tokens)
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}

	// Revoke token
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/tokens?id=1", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w = httptest.NewRecorder()
	h.AdminTokens(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminAgents_ListAndDelete(t *testing.T) {
	h := newTestHandler(t)

	// Register an agent first
	tokenID, _ := h.store.CreateAgentToken("h", "p", "t", "", "sync", nil)
	h.store.RegisterOrUpdateAgent("agent-1", tokenID, hub.AgentInfo{Hostname: "m1"})

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents", nil)
	w := httptest.NewRecorder()
	h.AdminAgents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}

	var agents []hub.AgentRecord
	json.Unmarshal(w.Body.Bytes(), &agents)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	// Delete
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/agents?id=1", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w = httptest.NewRecorder()
	h.AdminAgents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
