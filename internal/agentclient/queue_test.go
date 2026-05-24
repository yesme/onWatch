package agentclient

import (
	"os"
	"testing"

	"github.com/onllm-dev/onwatch/v2/internal/hub"
)

func TestQueue_EnqueueAndDrain(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(dir)
	if err != nil {
		t.Fatal(err)
	}

	req := &hub.SyncRequest{
		AgentID: "test-agent",
		Credentials: []hub.CredentialPayload{
			{Provider: "anthropic", CredentialType: "oauth", Data: map[string]string{"token": "abc"}},
		},
	}

	if err := q.Enqueue(req); err != nil {
		t.Fatal(err)
	}

	if q.IsEmpty() {
		t.Error("expected queue to not be empty")
	}

	items, remove, err := q.Drain(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].AgentID != "test-agent" {
		t.Errorf("expected agent_id 'test-agent', got %q", items[0].AgentID)
	}

	remove()

	if !q.IsEmpty() {
		t.Error("expected queue to be empty after drain+remove")
	}
}

func TestQueue_DrainLimit(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(dir)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		q.Enqueue(&hub.SyncRequest{AgentID: "agent"})
	}

	items, _, err := q.Drain(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items (limit), got %d", len(items))
	}
}

func TestQueue_EmptyDrain(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(dir)
	if err != nil {
		t.Fatal(err)
	}

	items, _, err := q.Drain(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestQueue_EvictsOldFiles(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Create an old file that should be evicted
	oldFile := dir + "/2020-01-01.jsonl"
	os.WriteFile(oldFile, []byte(`{"agent_id":"old"}`+"\n"), 0600)

	// Enqueue something (triggers eviction)
	q.Enqueue(&hub.SyncRequest{AgentID: "new"})

	// Old file should be gone
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("expected old file to be evicted")
	}
}
