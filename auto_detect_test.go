package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/config"
)

// TestResolveAutoDetectedTokens_PopulatesAnthropicFromCredentialsFile
// guards issue #67: the macOS menubar companion process did `config.Load()`
// without calling the same auto-detection routines that main.go runs.
// Users with OAuth-only Anthropic auth (no ANTHROPIC_TOKEN env var) ended
// up with cfg.AnthropicToken == "" inside the menubar process, so
// HasProvider("anthropic") returned false and the tray title silently
// dropped any selected Anthropic quota.
func TestResolveAutoDetectedTokens_PopulatesAnthropicFromCredentialsFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("credentials file layout differs on windows")
	}

	api.SetTestMode(true)
	t.Cleanup(func() { api.SetTestMode(false) })

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")
	t.Setenv("GEMINI_ENABLED", "false")

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	credentials := `{"claudeAiOauth":{"accessToken":"oauth-token-from-keychain"}}`
	if err := os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), []byte(credentials), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	cfg := &config.Config{
		Port:   9211,
		DBPath: filepath.Join(t.TempDir(), "test.db"),
	}

	if cfg.HasProvider("anthropic") {
		t.Fatal("precondition failed: empty cfg already reports anthropic")
	}

	resolveAutoDetectedTokens(cfg, slog.New(slog.DiscardHandler))

	if cfg.AnthropicToken != "oauth-token-from-keychain" {
		t.Fatalf("AnthropicToken = %q, want oauth-token-from-keychain", cfg.AnthropicToken)
	}
	if !cfg.AnthropicAutoToken {
		t.Fatal("AnthropicAutoToken should be true after auto-detection")
	}
	if !cfg.HasProvider("anthropic") {
		t.Fatal("HasProvider(anthropic) should be true after auto-detection - this is what the menubar process needs to keep Anthropic in the tray title")
	}
}

func TestResolveAutoDetectedTokens_RespectsExistingEnvToken(t *testing.T) {
	api.SetTestMode(true)
	t.Cleanup(func() { api.SetTestMode(false) })

	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", "")
	t.Setenv("GEMINI_ENABLED", "false")

	cfg := &config.Config{
		AnthropicToken: "env-supplied-token",
		Port:           9211,
		DBPath:         filepath.Join(t.TempDir(), "test.db"),
	}

	resolveAutoDetectedTokens(cfg, slog.New(slog.DiscardHandler))

	if cfg.AnthropicToken != "env-supplied-token" {
		t.Fatalf("AnthropicToken = %q, want env-supplied-token (must not be overwritten)", cfg.AnthropicToken)
	}
	if cfg.AnthropicAutoToken {
		t.Fatal("AnthropicAutoToken should remain false when token came from env")
	}
}

func TestResolveAutoDetectedTokens_NilCfgIsSafe(t *testing.T) {
	resolveAutoDetectedTokens(nil, nil)
}
