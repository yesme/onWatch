package agentclient

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCostScanner_ClaudeCodeLogs(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	// Create fake Claude Code session log
	projectDir := filepath.Join(dir, ".claude", "projects", "-Users-test-myproject")
	os.MkdirAll(projectDir, 0755)

	sessionLog := filepath.Join(projectDir, "abc123.jsonl")
	lines := []string{
		`{"type":"user","timestamp":"2026-05-03T10:00:00Z","message":{"content":"hello"}}`,
		`{"type":"assistant","timestamp":"2026-05-03T10:00:01Z","message":{"id":"msg1","model":"claude-opus-4-7-20250514","usage":{"input_tokens":1000,"output_tokens":500,"cache_read_input_tokens":200,"cache_creation_input_tokens":50}}}`,
		`{"type":"assistant","timestamp":"2026-05-03T10:00:02Z","message":{"id":"msg2","model":"claude-sonnet-4-6-20250514","usage":{"input_tokens":800,"output_tokens":300,"cache_read_input_tokens":100,"cache_creation_input_tokens":0}}}`,
		`{"type":"assistant","timestamp":"2026-05-03T10:00:03Z","message":{"id":"msg1","model":"claude-opus-4-7-20250514","usage":{"input_tokens":1000,"output_tokens":500,"cache_read_input_tokens":200,"cache_creation_input_tokens":50}}}`,
	}

	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	os.WriteFile(sessionLog, []byte(content), 0644)

	// Override home for testing
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", origHome)

	cs := NewCostScanner(statePath)
	results, err := cs.Scan()
	if err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least 1 cost log bucket")
	}

	found := false
	for _, r := range results {
		if r.Provider == "anthropic" && r.Date == "2026-05-03" {
			found = true
			// msg1 appears twice but should be deduped
			// msg2 is unique
			// So: opus(1000+500) + sonnet(800+300) = 1800 input, 800 output
			if r.InputTokens != 1800 {
				t.Errorf("expected 1800 input tokens, got %d", r.InputTokens)
			}
			if r.OutputTokens != 800 {
				t.Errorf("expected 800 output tokens, got %d", r.OutputTokens)
			}
			if len(r.ModelsUsed) != 2 {
				t.Errorf("expected 2 models, got %d", len(r.ModelsUsed))
			}
		}
	}

	if !found {
		t.Error("expected anthropic cost log for 2026-05-03")
	}

	// Second scan should find nothing new (mtime hasn't changed)
	results2, _ := cs.Scan()
	if len(results2) != 0 {
		t.Errorf("expected 0 new results on re-scan, got %d", len(results2))
	}
}

func TestNormalizeModel(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"claude-opus-4-7-20250514", "claude-opus-4-7"},
		{"claude-sonnet-4-6-20250514", "claude-sonnet-4-6"},
		{"claude-haiku-4-5-20251001", "claude-haiku-4-5"},
		{"gpt-4o", "gpt-4o"},
		{"", "unknown"},
	}
	for _, tc := range cases {
		got := normalizeModel(tc.in)
		if got != tc.want {
			t.Errorf("normalizeModel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEstimateCost(t *testing.T) {
	cost := EstimateCost("claude-opus-4-7", 1_000_000, 100_000, 0, 0)
	if cost < 20.0 || cost > 25.0 {
		t.Errorf("expected cost ~22.5 for opus, got %.2f", cost)
	}
}
