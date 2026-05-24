package store

import (
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/hub"
)

func TestCreateAndGetAgentToken(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	id, err := s.CreateAgentToken("hash123", "owt_a1b2...", "test-machine", "prakersh", "sync", nil)
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero ID")
	}

	tok, err := s.GetAgentTokenByHash("hash123")
	if err != nil {
		t.Fatal(err)
	}
	if tok == nil {
		t.Fatal("expected token, got nil")
	}
	if tok.Name != "test-machine" {
		t.Errorf("expected name 'test-machine', got %q", tok.Name)
	}
	if tok.Owner != "prakersh" {
		t.Errorf("expected owner 'prakersh', got %q", tok.Owner)
	}
	if tok.Scopes != "sync" {
		t.Errorf("expected scopes 'sync', got %q", tok.Scopes)
	}
	if tok.RevokedAt != nil {
		t.Error("expected nil RevokedAt")
	}
}

func TestGetAgentTokenByHash_NotFound(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	tok, err := s.GetAgentTokenByHash("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if tok != nil {
		t.Fatal("expected nil for nonexistent token")
	}
}

func TestCreateAgentToken_WithExpiry(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	expires := time.Now().Add(24 * time.Hour)
	_, err = s.CreateAgentToken("hash_exp", "owt_exp...", "expiring", "", "sync", &expires)
	if err != nil {
		t.Fatal(err)
	}

	tok, err := s.GetAgentTokenByHash("hash_exp")
	if err != nil {
		t.Fatal(err)
	}
	if tok.ExpiresAt == nil {
		t.Fatal("expected non-nil ExpiresAt")
	}
}

func TestListAgentTokens(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.CreateAgentToken("h1", "p1", "token1", "owner1", "sync", nil)
	s.CreateAgentToken("h2", "p2", "token2", "owner2", "sync", nil)

	tokens, err := s.ListAgentTokens()
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
}

func TestRevokeAgentToken(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	id, _ := s.CreateAgentToken("hrev", "prev", "revokable", "", "sync", nil)

	err = s.RevokeAgentToken(id)
	if err != nil {
		t.Fatal(err)
	}

	tok, _ := s.GetAgentTokenByHash("hrev")
	if tok.RevokedAt == nil {
		t.Error("expected non-nil RevokedAt after revocation")
	}

	// Double-revoke should error
	err = s.RevokeAgentToken(id)
	if err == nil {
		t.Error("expected error on double revoke")
	}
}

func TestRevokeAgentTokenByName(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.CreateAgentToken("hname", "pname", "named-token", "", "sync", nil)

	err = s.RevokeAgentTokenByName("named-token")
	if err != nil {
		t.Fatal(err)
	}

	err = s.RevokeAgentTokenByName("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent name")
	}
}

func TestUpdateAgentTokenLastUsed(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	id, _ := s.CreateAgentToken("hlu", "plu", "lastused", "", "sync", nil)

	tok, _ := s.GetAgentTokenByHash("hlu")
	if tok.LastUsedAt != nil {
		t.Error("expected nil LastUsedAt initially")
	}

	err = s.UpdateAgentTokenLastUsed(id)
	if err != nil {
		t.Fatal(err)
	}

	tok, _ = s.GetAgentTokenByHash("hlu")
	if tok.LastUsedAt == nil {
		t.Error("expected non-nil LastUsedAt after update")
	}
}

func TestRegisterAndUpdateAgent(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	tokenID, _ := s.CreateAgentToken("htok", "ptok", "tok", "", "sync", nil)

	info := hub.AgentInfo{
		Hostname:          "dev-macbook",
		OS:                "darwin",
		Arch:              "arm64",
		Version:           "1.0.0",
		Mode:              "full",
		DetectedProviders: []string{"anthropic", "codex"},
	}

	agentDBID, err := s.RegisterOrUpdateAgent("uuid-123", tokenID, info)
	if err != nil {
		t.Fatal(err)
	}
	if agentDBID == 0 {
		t.Fatal("expected non-zero agent DB ID")
	}

	agent, err := s.GetAgent("uuid-123")
	if err != nil {
		t.Fatal(err)
	}
	if agent == nil {
		t.Fatal("expected agent, got nil")
	}
	if agent.Hostname != "dev-macbook" {
		t.Errorf("expected hostname 'dev-macbook', got %q", agent.Hostname)
	}
	if agent.OS != "darwin" {
		t.Errorf("expected OS 'darwin', got %q", agent.OS)
	}
	if agent.Status != "active" {
		t.Errorf("expected status 'active', got %q", agent.Status)
	}

	// Update (second call with same agent_id)
	info.Version = "1.1.0"
	agentDBID2, err := s.RegisterOrUpdateAgent("uuid-123", tokenID, info)
	if err != nil {
		t.Fatal(err)
	}
	if agentDBID2 != agentDBID {
		t.Errorf("expected same ID on update, got %d vs %d", agentDBID, agentDBID2)
	}

	agent, _ = s.GetAgent("uuid-123")
	if agent.AgentVersion != "1.1.0" {
		t.Errorf("expected version '1.1.0', got %q", agent.AgentVersion)
	}
}

func TestListAgents(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	tokenID, _ := s.CreateAgentToken("ht", "pt", "t", "", "sync", nil)
	s.RegisterOrUpdateAgent("a1", tokenID, hub.AgentInfo{Hostname: "m1"})
	s.RegisterOrUpdateAgent("a2", tokenID, hub.AgentInfo{Hostname: "m2"})

	agents, err := s.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
}

func TestDeleteAgent(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	tokenID, _ := s.CreateAgentToken("hd", "pd", "d", "", "sync", nil)
	agentDBID, _ := s.RegisterOrUpdateAgent("del-uuid", tokenID, hub.AgentInfo{Hostname: "del"})

	err = s.DeleteAgent(agentDBID)
	if err != nil {
		t.Fatal(err)
	}

	agent, _ := s.GetAgent("del-uuid")
	if agent.Status != "deregistered" {
		t.Errorf("expected status 'deregistered', got %q", agent.Status)
	}
}

func TestAgentHeartbeat(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	tokenID, _ := s.CreateAgentToken("hb", "pb", "b", "", "sync", nil)
	s.RegisterOrUpdateAgent("hb-uuid", tokenID, hub.AgentInfo{Hostname: "hb"})

	time.Sleep(1100 * time.Millisecond) // RFC3339 has second resolution

	err = s.UpdateAgentHeartbeat("hb-uuid")
	if err != nil {
		t.Fatal(err)
	}

	after, _ := s.GetAgent("hb-uuid")
	if after == nil {
		t.Fatal("expected agent, got nil")
	}
	if after.Status != "active" {
		t.Errorf("expected status 'active', got %q", after.Status)
	}
}

func TestUpsertAgentCredential(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	tokenID, _ := s.CreateAgentToken("hc", "pc", "c", "", "sync", nil)
	agentDBID, _ := s.RegisterOrUpdateAgent("cred-uuid", tokenID, hub.AgentInfo{Hostname: "c"})

	err = s.UpsertAgentCredential(agentDBID, "anthropic", "", "oauth", `{"access_token":"abc"}`, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Upsert (update existing)
	err = s.UpsertAgentCredential(agentDBID, "anthropic", "", "oauth", `{"access_token":"def"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpsertAgentCostLog(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	tokenID, _ := s.CreateAgentToken("hcl", "pcl", "cl", "", "sync", nil)
	agentDBID, _ := s.RegisterOrUpdateAgent("cost-uuid", tokenID, hub.AgentInfo{Hostname: "cl"})

	log := hub.CostLogPayload{
		Provider:    "anthropic",
		Date:        "2026-05-03",
		InputTokens: 100000,
		OutputTokens: 50000,
		CostUSD:      12.50,
		ModelsUsed:   []string{"claude-opus-4-7"},
	}

	err = s.UpsertAgentCostLog(agentDBID, log)
	if err != nil {
		t.Fatal(err)
	}

	// Upsert (update existing)
	log.CostUSD = 15.00
	err = s.UpsertAgentCostLog(agentDBID, log)
	if err != nil {
		t.Fatal(err)
	}
}
