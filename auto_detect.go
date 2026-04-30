package main

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/config"
)

// resolveAutoDetectedTokens fills any empty provider tokens from
// platform-specific stores (keychain, credential files, OAuth caches) and
// flips the corresponding auto-token flags. Both the main daemon and the
// macOS menubar companion process must run this so that providers like
// Anthropic, which are typically configured via OAuth (no env var), report
// HasProvider() == true in both processes. Without this in the menubar
// runtime, the tray title silently dropped Anthropic/Codex/Cursor/Gemini
// quotas because buildMenubarProviders gates each provider on HasProvider.
func resolveAutoDetectedTokens(cfg *config.Config, logger *slog.Logger) {
	if cfg == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.AnthropicToken == "" {
		if token := api.DetectAnthropicToken(logger); token != "" {
			cfg.AnthropicToken = token
			cfg.AnthropicAutoToken = true
		}
	}
	if cfg.CodexToken == "" {
		if token := api.DetectCodexToken(logger); token != "" {
			cfg.CodexToken = token
			cfg.CodexAutoToken = true
		}
	}
	if !cfg.HasProvider("codex") {
		profilesDir := codexProfilesDirWithDataDir(filepath.Dir(cfg.DBPath))
		if hasSavedCodexProfiles(profilesDir) {
			cfg.CodexHasProfiles = true
		}
	}
	if os.Getenv("GEMINI_ENABLED") != "false" && !cfg.GeminiEnabled {
		if creds := api.DetectGeminiCredentials(nil); creds != nil {
			cfg.GeminiEnabled = true
		}
	}
	if cfg.CursorToken == "" {
		if token := api.DetectCursorToken(logger); token != "" {
			cfg.CursorToken = token
			cfg.CursorAutoToken = true
		}
	}
}
