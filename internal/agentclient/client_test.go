package agentclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/onllm-dev/onwatch/v2/internal/hub"
)

func TestSync_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/agent/sync" {
			t.Errorf("expected /api/v1/agent/sync, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %s", r.Header.Get("Authorization"))
		}

		var req hub.SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.AgentID != "agent-123" {
			t.Errorf("expected agent_id 'agent-123', got %q", req.AgentID)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hub.SyncResponse{
			CredentialsAccepted: 1,
			CostLogsAccepted:    0,
			NextSyncIn:          120,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	resp, err := client.Sync(context.Background(), &hub.SyncRequest{
		AgentID: "agent-123",
		Credentials: []hub.CredentialPayload{
			{Provider: "anthropic", CredentialType: "oauth", Data: map[string]string{"access_token": "abc"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.CredentialsAccepted != 1 {
		t.Errorf("expected 1 credential accepted, got %d", resp.CredentialsAccepted)
	}
}

func TestSync_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(server.URL, "bad-token")
	_, err := client.Sync(context.Background(), &hub.SyncRequest{AgentID: "test"})
	if err == nil {
		t.Error("expected error for unauthorized response")
	}
}

func TestHeartbeat_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/heartbeat" {
			t.Errorf("expected /api/v1/agent/heartbeat, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.Heartbeat(context.Background(), "agent-123")
	if err != nil {
		t.Fatal(err)
	}
}
