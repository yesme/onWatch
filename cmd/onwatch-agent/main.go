package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/agentclient"
	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/hub"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "onwatch-agent: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	args := os.Args[1:]

	// Handle simple subcommands
	if hasArg(args, "--version", "-v") {
		fmt.Printf("onwatch-agent v%s\n", version)
		return nil
	}
	if hasArg(args, "--help", "-h") {
		printHelp()
		return nil
	}
	if hasArg(args, "status") {
		return showStatus()
	}
	if hasArg(args, "stop") {
		return stopAgent()
	}

	cfg, err := agentclient.LoadConfig(args)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Info("onwatch-agent starting",
		"version", version,
		"hub", cfg.HubURL,
		"mode", cfg.Mode,
		"name", cfg.AgentName,
		"interval", cfg.SyncInterval,
	)

	agentID, err := agentclient.LoadOrCreateAgentID()
	if err != nil {
		return fmt.Errorf("failed to load agent ID: %w", err)
	}
	logger.Info("agent ID loaded", "id", agentID[:8]+"...")

	client := agentclient.NewClient(cfg.HubURL, cfg.AgentToken)

	home, _ := os.UserHomeDir()
	queueDir := filepath.Join(home, ".onwatch", "sync-queue")
	queue, err := agentclient.NewQueue(queueDir)
	if err != nil {
		return fmt.Errorf("failed to create sync queue: %w", err)
	}

	statePath := filepath.Join(home, ".onwatch", "cost-scan-state.json")
	costScanner := agentclient.NewCostScanner(statePath)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Drain any queued data first
	drainQueue(ctx, client, queue, logger)

	// Main sync loop
	ticker := time.NewTicker(cfg.SyncInterval)
	defer ticker.Stop()

	syncOnce(ctx, cfg, client, queue, costScanner, agentID, logger)

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down")
			return nil
		case <-ticker.C:
			syncOnce(ctx, cfg, client, queue, costScanner, agentID, logger)
		}
	}
}

func syncOnce(ctx context.Context, cfg *agentclient.Config, client *agentclient.Client, queue *agentclient.Queue, costScanner *agentclient.CostScanner, agentID string, logger *slog.Logger) {
	req := &hub.SyncRequest{
		AgentID: agentID,
		AgentInfo: hub.AgentInfo{
			Hostname: cfg.AgentName,
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
			Version:  version,
			Mode:     string(cfg.Mode),
		},
	}

	// Credential detection
	if cfg.Mode == agentclient.ModeFull || cfg.Mode == agentclient.ModeTokenSync {
		creds := detectCredentials(logger)
		if len(creds) > 0 {
			req.Credentials = creds
			req.AgentInfo.DetectedProviders = extractProviders(creds)
		}
	}

	// Cost log scanning
	if cfg.Mode == agentclient.ModeFull || cfg.Mode == agentclient.ModeCostSync {
		costLogs, err := costScanner.Scan()
		if err != nil {
			logger.Warn("cost scan failed", "error", err)
		} else if len(costLogs) > 0 {
			req.CostLogs = costLogs
		}
	}

	if len(req.Credentials) == 0 && len(req.CostLogs) == 0 {
		// Nothing to sync, just heartbeat
		if err := client.Heartbeat(ctx, agentID); err != nil {
			logger.Warn("heartbeat failed", "error", err)
		}
		return
	}

	resp, err := client.Sync(ctx, req)
	if err != nil {
		logger.Warn("sync failed, queuing", "error", err)
		if qErr := queue.Enqueue(req); qErr != nil {
			logger.Error("queue failed", "error", qErr)
		}
		return
	}

	logger.Info("sync complete",
		"credentials", resp.CredentialsAccepted,
		"cost_logs", resp.CostLogsAccepted,
	)
}

func drainQueue(ctx context.Context, client *agentclient.Client, queue *agentclient.Queue, logger *slog.Logger) {
	if queue.IsEmpty() {
		return
	}

	logger.Info("draining sync queue")
	requests, remove, err := queue.Drain(100)
	if err != nil {
		logger.Warn("queue drain failed", "error", err)
		return
	}

	allOk := true
	for _, req := range requests {
		reqCopy := req
		if _, err := client.Sync(ctx, &reqCopy); err != nil {
			logger.Warn("queued sync failed", "error", err)
			allOk = false
			break
		}
	}

	if allOk {
		remove()
		logger.Info("queue drained", "count", len(requests))
	}
}

func detectCredentials(logger *slog.Logger) []hub.CredentialPayload {
	var creds []hub.CredentialPayload

	// Anthropic (Claude Code)
	if ac := api.DetectAnthropicCredentials(logger); ac != nil && ac.AccessToken != "" {
		creds = append(creds, hub.CredentialPayload{
			Provider:       "anthropic",
			CredentialType: "oauth",
			Data: map[string]string{
				"access_token":  ac.AccessToken,
				"refresh_token": ac.RefreshToken,
				"expires_at":    ac.ExpiresAt.Format(time.RFC3339),
			},
		})
	}

	// Codex
	if cc := api.DetectCodexCredentials(logger); cc != nil && cc.AccessToken != "" {
		data := map[string]string{
			"access_token":  cc.AccessToken,
			"refresh_token": cc.RefreshToken,
		}
		if cc.APIKey != "" {
			data["api_key"] = cc.APIKey
		}
		if cc.AccountID != "" {
			data["account_id"] = cc.AccountID
		}
		if cc.UserID != "" {
			data["user_id"] = cc.UserID
		}
		creds = append(creds, hub.CredentialPayload{
			Provider:       "codex",
			CredentialType: "oauth",
			Data:           data,
		})
	}

	// Gemini
	if gc := api.DetectGeminiCredentials(logger); gc != nil && gc.AccessToken != "" {
		creds = append(creds, hub.CredentialPayload{
			Provider:       "gemini",
			CredentialType: "oauth",
			Data: map[string]string{
				"access_token":  gc.AccessToken,
				"refresh_token": gc.RefreshToken,
				"expires_at":    gc.ExpiresAt.Format(time.RFC3339),
			},
		})
	}

	// Cursor
	if ct := api.DetectCursorToken(logger); ct != "" {
		creds = append(creds, hub.CredentialPayload{
			Provider:       "cursor",
			CredentialType: "session",
			Data: map[string]string{
				"token": ct,
			},
		})
	}

	// Filter out unchanged credentials (compare hash)
	return filterChanged(creds)
}

var lastCredHashes = make(map[string]string)

func filterChanged(creds []hub.CredentialPayload) []hub.CredentialPayload {
	var changed []hub.CredentialPayload
	for _, c := range creds {
		key := c.Provider + ":" + c.AccountName
		dataJSON, _ := json.Marshal(c.Data)
		h := sha256.Sum256(dataJSON)
		hash := hex.EncodeToString(h[:])

		if lastCredHashes[key] != hash {
			lastCredHashes[key] = hash
			changed = append(changed, c)
		}
	}
	return changed
}

func extractProviders(creds []hub.CredentialPayload) []string {
	seen := make(map[string]bool)
	var providers []string
	for _, c := range creds {
		if !seen[c.Provider] {
			seen[c.Provider] = true
			providers = append(providers, c.Provider)
		}
	}
	return providers
}

func showStatus() error {
	fmt.Println("onwatch-agent status: TODO - check PID file")
	return nil
}

func stopAgent() error {
	fmt.Println("onwatch-agent stop: TODO - signal PID")
	return nil
}

func printHelp() {
	fmt.Printf("onwatch-agent v%s - lightweight sync client for onWatch hub\n\n", version)
	fmt.Println("Usage:")
	fmt.Println("  onwatch-agent --hub <url> --token <token> [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --hub <url>      Hub server URL (required)")
	fmt.Println("  --token <token>  Agent token from 'onwatch token create' (required)")
	fmt.Println("  --name <name>    Agent display name (default: hostname)")
	fmt.Println("  --mode <mode>    full, token-sync, or cost-sync (default: full)")
	fmt.Println("  --interval <sec> Sync interval in seconds (default: 120)")
	fmt.Println("  --insecure       Allow non-HTTPS hub URL")
	fmt.Println("  --daemon         Run as background daemon")
	fmt.Println("  --version, -v    Print version")
	fmt.Println("  --help, -h       Show this help")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  status           Show agent status")
	fmt.Println("  stop             Stop running agent")
	fmt.Println()
	fmt.Println("Config file: ~/.onwatch/agent.env")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  onwatch-agent --hub https://hub.example.com --token owt_abc123")
	fmt.Println("  onwatch-agent --hub http://localhost:9211 --token owt_abc123 --insecure")
	fmt.Println("  onwatch-agent --mode token-sync --hub https://hub.example.com --token owt_abc123")
}

func hasArg(args []string, flags ...string) bool {
	for _, arg := range args {
		for _, f := range flags {
			if strings.TrimLeft(arg, "-") == strings.TrimLeft(f, "-") {
				return true
			}
		}
	}
	return false
}
