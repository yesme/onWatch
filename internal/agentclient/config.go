package agentclient

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// AgentMode controls which sync channels the agent activates.
type AgentMode string

const (
	ModeFull      AgentMode = "full"
	ModeTokenSync AgentMode = "token-sync"
	ModeCostSync  AgentMode = "cost-sync"
)

// Config holds agent runtime configuration.
type Config struct {
	HubURL       string
	AgentToken   string
	AgentName    string
	Mode         AgentMode
	SyncInterval time.Duration
	Insecure     bool // allow non-HTTPS hub for local dev
	Daemon       bool
}

// LoadConfig loads agent config from CLI flags, then ~/.onwatch/agent.env, then env vars.
func LoadConfig(args []string) (*Config, error) {
	cfg := &Config{
		Mode:         ModeFull,
		SyncInterval: 120 * time.Second,
	}

	// Parse CLI flags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--hub":
			if i+1 < len(args) {
				i++
				cfg.HubURL = args[i]
			}
		case "--token":
			if i+1 < len(args) {
				i++
				cfg.AgentToken = args[i]
			}
		case "--name":
			if i+1 < len(args) {
				i++
				cfg.AgentName = args[i]
			}
		case "--mode":
			if i+1 < len(args) {
				i++
				cfg.Mode = AgentMode(args[i])
			}
		case "--interval":
			if i+1 < len(args) {
				i++
				n, err := strconv.Atoi(args[i])
				if err == nil && n > 0 {
					cfg.SyncInterval = time.Duration(n) * time.Second
				}
			}
		case "--insecure":
			cfg.Insecure = true
		case "--daemon":
			cfg.Daemon = true
		}
	}

	// Fall back to env file
	loadAgentEnvFile(cfg)

	// Fall back to environment variables
	if cfg.HubURL == "" {
		cfg.HubURL = os.Getenv("ONWATCH_HUB_URL")
	}
	if cfg.AgentToken == "" {
		cfg.AgentToken = os.Getenv("ONWATCH_AGENT_TOKEN")
	}
	if cfg.AgentName == "" {
		cfg.AgentName = os.Getenv("ONWATCH_AGENT_NAME")
	}
	if cfg.Mode == ModeFull {
		if m := os.Getenv("ONWATCH_AGENT_MODE"); m != "" {
			cfg.Mode = AgentMode(m)
		}
	}

	// Defaults
	if cfg.AgentName == "" {
		hostname, _ := os.Hostname()
		cfg.AgentName = hostname
	}

	// Validate
	if cfg.HubURL == "" {
		return nil, fmt.Errorf("--hub URL is required (or set ONWATCH_HUB_URL)")
	}
	if cfg.AgentToken == "" {
		return nil, fmt.Errorf("--token is required (or set ONWATCH_AGENT_TOKEN)")
	}
	if !cfg.Insecure && !strings.HasPrefix(cfg.HubURL, "https://") && !strings.HasPrefix(cfg.HubURL, "http://localhost") && !strings.HasPrefix(cfg.HubURL, "http://127.0.0.1") {
		return nil, fmt.Errorf("hub URL must use HTTPS (use --insecure for local development)")
	}

	switch cfg.Mode {
	case ModeFull, ModeTokenSync, ModeCostSync:
	default:
		return nil, fmt.Errorf("invalid mode %q: must be full, token-sync, or cost-sync", cfg.Mode)
	}

	return cfg, nil
}

func loadAgentEnvFile(cfg *Config) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	envPath := filepath.Join(home, ".onwatch", "agent.env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)

		switch key {
		case "ONWATCH_HUB_URL":
			if cfg.HubURL == "" {
				cfg.HubURL = val
			}
		case "ONWATCH_AGENT_TOKEN":
			if cfg.AgentToken == "" {
				cfg.AgentToken = val
			}
		case "ONWATCH_AGENT_NAME":
			if cfg.AgentName == "" {
				cfg.AgentName = val
			}
		case "ONWATCH_AGENT_MODE":
			if cfg.Mode == ModeFull {
				cfg.Mode = AgentMode(val)
			}
		case "ONWATCH_SYNC_INTERVAL":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.SyncInterval = time.Duration(n) * time.Second
			}
		}
	}
}
